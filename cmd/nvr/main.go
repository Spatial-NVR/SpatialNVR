// Package main provides the plugin-based NVR system entry point
// This is the new architecture that uses the plugin loader for all services
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/gorilla/websocket"
	"gopkg.in/yaml.v3"

	"github.com/Spatial-NVR/SpatialNVR/internal/api"
	"github.com/Spatial-NVR/SpatialNVR/internal/core"
	"github.com/Spatial-NVR/SpatialNVR/internal/database"
	"github.com/Spatial-NVR/SpatialNVR/internal/detection"
	"github.com/Spatial-NVR/SpatialNVR/internal/logging"
	"github.com/Spatial-NVR/SpatialNVR/internal/video"
	"github.com/Spatial-NVR/SpatialNVR/internal/plugin"
	"github.com/Spatial-NVR/SpatialNVR/sdk"
	nvrcoreapi "github.com/Spatial-NVR/SpatialNVR/plugins/nvr-core-api"
	nvrcoreconfig "github.com/Spatial-NVR/SpatialNVR/plugins/nvr-core-config"
	nvrcoreevents "github.com/Spatial-NVR/SpatialNVR/plugins/nvr-core-events"
	nvrdetection "github.com/Spatial-NVR/SpatialNVR/plugins/nvr-detection"
	nvrrecording "github.com/Spatial-NVR/SpatialNVR/plugins/nvr-recording"
	nvrspatial "github.com/Spatial-NVR/SpatialNVR/plugins/nvr-spatial-tracking"
	nvrstreaming "github.com/Spatial-NVR/SpatialNVR/plugins/nvr-streaming"
	nvrupdates "github.com/Spatial-NVR/SpatialNVR/plugins/nvr-updates"
)

const (
	defaultAddress    = "0.0.0.0"
	defaultDataPath   = "/data"
	defaultPluginsDir = "/plugins"
)

func main() {
	// Initialize structured logging with stream capture
	logLevel := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		logLevel = slog.LevelDebug
	}
	logBuffer := logging.GetLogBuffer()
	handler := logging.NewStreamHandler(logBuffer, os.Stdout, logLevel)
	logger := slog.New(handler)
	slog.SetDefault(logger)

	// Initialize port manager and resolve all ports upfront
	portManager := core.GetPortManager()
	ports, err := portManager.ResolveAllPorts()
	if err != nil {
		slog.Error("Failed to allocate ports", "error", err)
		os.Exit(1)
	}

	slog.Info("Starting NVR System (Plugin Architecture)",
		"version", "0.2.1",
		"mode", "plugin-based",
		"api_port", ports.API,
		"nats_port", ports.NATS,
	)

	// Create application context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Get configuration paths - check multiple locations
	dataPath := getEnv("DATA_PATH", defaultDataPath)
	pluginsDir := getEnv("PLUGINS_DIR", filepath.Join(dataPath, "plugins"))

	// Find config file - check multiple locations
	configPath := findConfigFile(dataPath)
	slog.Info("Using configuration", "config_path", configPath, "data_path", dataPath)

	// Ensure directories exist
	_ = os.MkdirAll(dataPath, 0755)
	_ = os.MkdirAll(pluginsDir, 0755)

	// Open database
	dbConfig := database.DefaultConfig(dataPath)
	db, err := database.Open(dbConfig)
	if err != nil {
		slog.Error("Failed to open database", "error", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	// Run migrations
	migrator := database.NewMigrator(db)
	if err := migrator.Run(ctx); err != nil {
		slog.Error("Failed to run migrations", "error", err)
		os.Exit(1)
	}

	// Initialize embedded NATS event bus with allocated port
	eventBusCfg := core.DefaultEventBusConfig()
	eventBusCfg.Port = ports.NATS
	eventBusCfg.PortManager = portManager
	eventBus, err := core.NewEventBus(eventBusCfg, logger)
	if err != nil {
		slog.Error("Failed to create event bus", "error", err)
		os.Exit(1)
	}
	defer eventBus.Stop()

	// Start embedded detection server before plugins
	// This provides the detection backend that the detection plugin connects to
	embeddedDetection := detection.NewEmbeddedServer(detection.EmbeddedServerConfig{
		Port:   ports.Detection,
		Logger: logger,
	})
	if err := embeddedDetection.Start(ctx); err != nil {
		slog.Error("Failed to start embedded detection server", "error", err)
		os.Exit(1)
	}
	defer func() { _ = embeddedDetection.Stop(ctx) }()
	slog.Info("Embedded detection server started", "port", ports.Detection)

	// Create plugin loader
	loader := core.NewPluginLoader(pluginsDir, eventBus, db.DB, logger)

	// Register core builtin plugins (including spatial tracking)
	registerCorePlugins(loader, dataPath)

	// Configure plugins with resolved ports
	configurePlugins(loader, dataPath, configPath, ports)

	// Load any saved plugin configurations (e.g., Wyze credentials)
	if err := loader.LoadPluginConfigs(); err != nil {
		slog.Warn("Failed to load plugin configs", "error", err)
	}

	// Start plugin loader (starts all plugins in dependency order)
	if err := loader.Start(ctx); err != nil {
		slog.Error("Failed to start plugin loader", "error", err)
		os.Exit(1)
	}
	defer func() { _ = loader.Stop() }()

	// Create API gateway
	gateway := core.NewAPIGateway(loader, eventBus, logger)

	// Create plugin installer
	installer := plugin.NewInstaller(pluginsDir, logger)
	if err := installer.Start(ctx); err != nil {
		slog.Warn("Failed to start plugin installer", "error", err)
	}
	defer installer.Stop()

	// Setup HTTP router with spatial tracking routes
	router := setupRouter(gateway, loader, eventBus, db, ports, installer)

	// Server configuration - use resolved port
	addr := fmt.Sprintf("%s:%d", defaultAddress, ports.API)

	server := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		slog.Info("Server starting", "address", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server error", "error", err)
			cancel()
		}
	}()

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	slog.Info("Shutting down server...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("Server shutdown error", "error", err)
	}

	slog.Info("Server stopped")
}

// registerCorePlugins registers all builtin core plugins
func registerCorePlugins(loader *core.PluginLoader, dataPath string) {
	// Core foundation plugins (no dependencies)
	if err := loader.RegisterBuiltinPlugin(nvrcoreconfig.New()); err != nil {
		slog.Error("Failed to register config plugin", "error", err)
	}

	if err := loader.RegisterBuiltinPlugin(nvrcoreevents.New()); err != nil {
		slog.Error("Failed to register events plugin", "error", err)
	}

	if err := loader.RegisterBuiltinPlugin(nvrcoreapi.New()); err != nil {
		slog.Error("Failed to register core API plugin", "error", err)
	}

	// Register streaming plugin (no dependencies)
	if err := loader.RegisterBuiltinPlugin(nvrstreaming.New()); err != nil {
		slog.Error("Failed to register streaming plugin", "error", err)
	}

	// Register recording plugin (depends on streaming)
	if err := loader.RegisterBuiltinPlugin(nvrrecording.New()); err != nil {
		slog.Error("Failed to register recording plugin", "error", err)
	}

	// Register detection plugin (depends on streaming)
	if err := loader.RegisterBuiltinPlugin(nvrdetection.New()); err != nil {
		slog.Error("Failed to register detection plugin", "error", err)
	}

	// Register spatial tracking plugin (depends on events and detection)
	// This runs in-process by default, only separate when scaling is needed
	if err := loader.RegisterBuiltinPlugin(nvrspatial.New()); err != nil {
		slog.Error("Failed to register spatial tracking plugin", "error", err)
	}

	// Register updates plugin (manages self-updates)
	if err := loader.RegisterBuiltinPlugin(nvrupdates.New()); err != nil {
		slog.Error("Failed to register updates plugin", "error", err)
	}
}

// configurePlugins sets up configuration for each plugin
func configurePlugins(loader *core.PluginLoader, dataPath, configPath string, ports *core.PortConfig) {
	// Core config plugin
	loader.SetPluginConfig("nvr-core-config", map[string]interface{}{
		"config_path":   configPath,
		"watch_enabled": true,
	})

	// Core events plugin
	loader.SetPluginConfig("nvr-core-events", map[string]interface{}{
		"max_events":     10000,
		"retention_days": 30,
	})

	// Core API plugin
	loader.SetPluginConfig("nvr-core-api", map[string]interface{}{
		"config_path":     configPath,
		"storage_path":    dataPath,
		"go2rtc_api_port": ports.Go2RTCAPI,
	})

	// Streaming plugin config - using allocated ports
	loader.SetPluginConfig("nvr-streaming", map[string]interface{}{
		"config_path": filepath.Join(dataPath, "go2rtc.yaml"),
		"api_port":    ports.Go2RTCAPI,
		"rtsp_port":   ports.Go2RTCRTSP,
		"webrtc_port": ports.Go2RTCWebRTC,
	})

	// Recording plugin config
	loader.SetPluginConfig("nvr-recording", map[string]interface{}{
		"storage_path":   filepath.Join(dataPath, "recordings"),
		"thumbnail_path": filepath.Join(dataPath, "thumbnails"),
	})

	// Detection plugin config - using allocated ports
	loader.SetPluginConfig("nvr-detection", map[string]interface{}{
		"detection_addr": fmt.Sprintf("localhost:%d", ports.Detection),
		"go2rtc_addr":    fmt.Sprintf("http://localhost:%d", ports.Go2RTCAPI),
		"default_fps":    5,
		"min_confidence": 0.5,
		"models_path":    filepath.Join(dataPath, "models"),
	})

	// Spatial tracking plugin config
	loader.SetPluginConfig("nvr-spatial-tracking", map[string]interface{}{
		"reid_enabled":     true,
		"max_gap_seconds":  30,
		"track_ttl_seconds": 300,
	})

	// Updates plugin config
	loader.SetPluginConfig("nvr-updates", map[string]interface{}{
		"data_path":       filepath.Join(dataPath, "updates"),
		"check_interval":  "6h",
		"auto_update":     false,
		"config_path":     configPath, // Pass config path so plugin can read github_token from system config
	})
}

// findConfigFile looks for config file in multiple locations
func findConfigFile(dataPath string) string {
	// Check environment variable first - always respect it if set
	// This is the primary way to configure the config path in Docker
	if configPath := os.Getenv("CONFIG_PATH"); configPath != "" {
		// Ensure parent directory exists
		dir := filepath.Dir(configPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			slog.Warn("Failed to create config directory", "dir", dir, "error", err)
		}
		return configPath
	}

	// Check common locations for existing config files
	locations := []string{
		"/config/config.yaml",
		filepath.Join(dataPath, "config.yaml"),
		"./config/config.yaml",
		filepath.Join(os.Getenv("HOME"), "nvr-prototype/config/config.yaml"),
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			return loc
		}
	}

	// Default fallback - prefer /config in Docker environments
	if _, err := os.Stat("/config"); err == nil {
		return "/config/config.yaml"
	}
	return filepath.Join(dataPath, "config.yaml")
}

// setupRouter creates the HTTP router with all routes
func setupRouter(gateway *core.APIGateway, loader *core.PluginLoader, eventBus *core.EventBus, db *database.DB, ports *core.PortConfig, installer *plugin.Installer) *chi.Mux {
	r := chi.NewRouter()

	// Middleware stack
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// CORS configuration - allow connections from our UI port
	allowedOrigins := []string{
		"http://localhost:*",
		"http://127.0.0.1:*",
		fmt.Sprintf("http://localhost:%d", ports.WebUI),
	}
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"Link", "X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// WebSocket hub for real-time updates
	wsHub := api.NewHub()
	go wsHub.Run()

	// WebSocket endpoint
	r.Get("/ws", wsHub.HandleWebSocket)

	// Proxy go2rtc endpoints through the API (so only API port needs to be exposed)
	go2rtcURL := fmt.Sprintf("http://localhost:%d", ports.Go2RTCAPI)
	go2rtcProxy := createGo2RTCProxy(go2rtcURL, ports)

	// Health check endpoint
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		plugins := loader.ListPlugins()
		pluginHealth := make(map[string]interface{})
		allHealthy := true

		for _, p := range plugins {
			health := p.Plugin.Health()
			pluginHealth[p.Manifest.ID] = map[string]interface{}{
				"state":   string(health.State),
				"message": health.Message,
			}
			if health.State != "healthy" {
				allHealthy = false
			}
		}

		status := "healthy"
		if !allHealthy {
			status = "degraded"
		}

		if err := db.Health(r.Context()); err != nil {
			status = "degraded"
			pluginHealth["database"] = "error"
		} else {
			pluginHealth["database"] = "ok"
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":"%s","version":"0.2.1","plugins":%d,"mode":"plugin-based"}`, status, len(plugins))
	})

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		// Plugin management
		r.Route("/plugins", func(r chi.Router) {
			r.Get("/", handleListPlugins(loader))
			r.Post("/install", handleInstallPlugin(installer, loader))
			r.Post("/rescan", handleRescanPlugins(loader))
			r.Get("/{id}", handleGetPlugin(loader))
			r.Post("/{id}/enable", handleEnablePlugin(loader))
			r.Post("/{id}/disable", handleDisablePlugin(loader))
			r.Post("/{id}/restart", handleRestartPlugin(loader))
			r.Delete("/{id}", handleUninstallPlugin(installer, loader))
			r.Get("/{id}/config", handleGetPluginConfig(loader))
			r.Put("/{id}/config", handleSetPluginConfig(loader))
			r.Get("/{id}/logs", handleGetPluginLogs(loader))
			r.Post("/{id}/rpc", handlePluginRPC(loader))
		})

		// Audio sessions (two-way audio)
		r.Route("/audio/sessions", func(r chi.Router) {
			r.Post("/{cameraId}/start", handleStartAudioSession(ports))
			r.Post("/{cameraId}/stop", handleStopAudioSession())
			r.Get("/{cameraId}", handleGetAudioSession())
		})

		// Mount gateway handler for plugin routes
		r.Mount("/", gateway.Handler())

		// System routes
		r.Get("/system/health", handleSystemHealth(loader, eventBus, db))
		r.Get("/system/events", handleListEvents(eventBus))
		r.Get("/system/ports", handleGetPorts())
		r.Get("/system/metrics", handleSystemMetrics())
		r.Post("/system/restart", handleSystemRestart())

		// Stats endpoint for dashboard
		r.Get("/stats", handleGetStats(loader, db))

		// Log streaming endpoint
		r.Get("/logs/stream", handleLogStream())

		// Plugin catalog endpoints
		r.Get("/plugins/catalog", handleGetPluginCatalog(loader))
		r.Post("/plugins/catalog/reload", handleReloadPluginCatalog())
		r.Post("/plugins/catalog/{pluginId}/install", handleInstallFromCatalog(installer, loader))
	})

	// Serve static frontend files in production
	webPath := os.Getenv("WEB_PATH")
	if webPath == "" {
		webPath = "/app/web"
	}

	// Check if web directory exists (production mode)
	if info, err := os.Stat(webPath); err == nil && info.IsDir() {
		// Serve static files
		fs := http.FileServer(http.Dir(webPath))

		// Catch-all handler for both static files and SPA routes
		// Also handles /go2rtc proxy since chi doesn't order Routes correctly with wildcards
		r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
			urlPath := req.URL.Path

			// Route /go2rtc/* to the proxy
			if strings.HasPrefix(urlPath, "/go2rtc") {
				go2rtcProxy.ServeHTTP(w, req)
				return
			}

			// Try to serve static file
			filePath := filepath.Join(webPath, urlPath)
			if _, err := os.Stat(filePath); err == nil {
				fs.ServeHTTP(w, req)
				return
			}

			// Otherwise serve index.html for SPA routing
			http.ServeFile(w, req, filepath.Join(webPath, "index.html"))
		})
	}

	return r
}

// Handler functions

func handleListPlugins(loader *core.PluginLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plugins := loader.ListPlugins()

		// Sort plugins alphabetically by ID for stable ordering
		sort.Slice(plugins, func(i, j int) bool {
			return plugins[i].Manifest.ID < plugins[j].Manifest.ID
		})

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, "[")
		for i, p := range plugins {
			if i > 0 {
				_, _ = fmt.Fprint(w, ",")
			}
			health := p.Plugin.Health()
			_, _ = fmt.Fprintf(w, `{"id":"%s","name":"%s","version":"%s","state":"%s","health":"%s","builtin":%t}`,
				p.Manifest.ID, p.Manifest.Name, p.Manifest.Version, p.State, health.State, p.IsBuiltin)
		}
		_, _ = fmt.Fprint(w, "]")
	}
}

func handleGetPlugin(loader *core.PluginLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		p, ok := loader.GetPlugin(id)
		if !ok {
			http.Error(w, `{"error":"Plugin not found"}`, http.StatusNotFound)
			return
		}

		health := p.Plugin.Health()

		// Build capabilities JSON
		capsJSON := "[]"
		if len(p.Manifest.Capabilities) > 0 {
			caps := make([]string, 0, len(p.Manifest.Capabilities))
			for _, c := range p.Manifest.Capabilities {
				caps = append(caps, fmt.Sprintf(`"%s"`, c))
			}
			capsJSON = "[" + strings.Join(caps, ",") + "]"
		}

		// Build dependencies JSON
		depsJSON := "[]"
		if len(p.Manifest.Dependencies) > 0 {
			deps := make([]string, 0, len(p.Manifest.Dependencies))
			for _, d := range p.Manifest.Dependencies {
				deps = append(deps, fmt.Sprintf(`"%s"`, d))
			}
			depsJSON = "[" + strings.Join(deps, ",") + "]"
		}

		// Started at
		startedAt := "null"
		if p.StartedAt != nil {
			startedAt = fmt.Sprintf(`"%s"`, p.StartedAt.Format(time.RFC3339))
		}

		// Last error
		lastError := "null"
		if p.LastError != "" {
			lastError = fmt.Sprintf(`"%s"`, p.LastError)
		}

		// Category
		category := p.Manifest.Category
		if category == "" {
			category = "core"
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":"%s","name":"%s","version":"%s","description":"%s","category":"%s","state":"%s","enabled":%t,"builtin":%t,"critical":%t,"capabilities":%s,"dependencies":%s,"startedAt":%s,"lastError":%s,"health":{"state":"%s","message":"%s","lastChecked":"%s"}}`,
			p.Manifest.ID, p.Manifest.Name, p.Manifest.Version, p.Manifest.Description, category,
			p.State, p.State == core.PluginStateRunning, p.IsBuiltin, p.Manifest.Critical,
			capsJSON, depsJSON, startedAt, lastError,
			health.State, health.Message, health.LastChecked.Format(time.RFC3339))
	}
}

func handleRescanPlugins(loader *core.PluginLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := loader.ScanExternalPlugins(); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		// Return list of all plugins (including newly discovered ones)
		plugins := loader.ListPlugins()
		result := make([]map[string]interface{}, 0, len(plugins))
		for _, p := range plugins {
			result = append(result, map[string]interface{}{
				"id":       p.Manifest.ID,
				"name":     p.Manifest.Name,
				"version":  p.Manifest.Version,
				"state":    string(p.State),
				"isBuiltin": p.IsBuiltin,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"plugins": result,
			"message": fmt.Sprintf("Scanned plugins directory, found %d plugins", len(plugins)),
		})
	}
}

func handleEnablePlugin(loader *core.PluginLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		// First try normal enable (for already registered plugins)
		err := loader.EnablePlugin(r.Context(), id)
		if err != nil {
			// If plugin not found, try to scan and start it (for newly installed external plugins)
			if strings.Contains(err.Error(), "not found") {
				err = loader.ScanAndStart(r.Context(), id)
			}
		}

		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"id": id, "status": "enabled"})
	}
}

func handleDisablePlugin(loader *core.PluginLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := loader.DisablePlugin(r.Context(), id); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":"%s","status":"disabled"}`, id)
	}
}

func handleRestartPlugin(loader *core.PluginLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := loader.RestartPlugin(r.Context(), id); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":"%s","status":"restarted"}`, id)
	}
}

func handleGetPluginConfig(loader *core.PluginLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		p, ok := loader.GetPlugin(id)
		if !ok {
			http.Error(w, `{"error":"Plugin not found"}`, http.StatusNotFound)
			return
		}

		// Get config from runtime if available
		config := make(map[string]interface{})
		if p.Runtime != nil {
			config = p.Runtime.Config()
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"pluginId": id,
			"config":   config,
		})
	}
}

func handleSetPluginConfig(loader *core.PluginLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		p, ok := loader.GetPlugin(id)
		if !ok {
			http.Error(w, `{"error":"Plugin not found"}`, http.StatusNotFound)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, `{"error":"Failed to read request body"}`, http.StatusBadRequest)
			return
		}

		var newConfig map[string]interface{}
		if err := json.Unmarshal(body, &newConfig); err != nil {
			http.Error(w, `{"error":"Invalid JSON"}`, http.StatusBadRequest)
			return
		}

		// Update config in loader (this persists the config)
		loader.SetPluginConfig(id, newConfig)

		// Save config to disk for persistence across restarts
		if err := loader.SavePluginConfig(id); err != nil {
			slog.Warn("Failed to persist plugin config", "plugin", id, "error", err)
		}

		// Restart plugin if running, or start if not running
		if p.State == core.PluginStateRunning {
			go func() { _ = loader.RestartPlugin(r.Context(), id) }()
		} else {
			// Plugin not running - try to start it with new config
			go func() {
				if err := loader.EnablePlugin(context.Background(), id); err != nil {
					slog.Error("Failed to start plugin after config update", "plugin", id, "error", err)
				}
			}()
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"message":"Configuration updated"}`)
	}
}

func handleGetPluginLogs(loader *core.PluginLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		p, ok := loader.GetPlugin(id)
		if !ok {
			http.Error(w, `{"error":"Plugin not found"}`, http.StatusNotFound)
			return
		}

		// Get logs from runtime if available
		var logs []map[string]interface{}
		if p.Runtime != nil {
			rawLogs := p.Runtime.GetLogs(500)
			for _, l := range rawLogs {
				logs = append(logs, map[string]interface{}{
					"timestamp": l.Timestamp,
					"level":     l.Level,
					"message":   l.Message,
					"metadata":  l.Metadata,
				})
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"pluginId": id,
			"logs":     logs,
			"total":    len(logs),
		})
	}
}

func handleSystemHealth(loader *core.PluginLoader, eventBus *core.EventBus, db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plugins := loader.ListPlugins()

		healthy := 0
		degraded := 0
		unhealthy := 0

		for _, p := range plugins {
			// Only check health for running plugins
			if p.State != core.PluginStateRunning {
				continue
			}
			health := p.Plugin.Health()
			switch health.State {
			case sdk.HealthStateHealthy:
				healthy++
			case sdk.HealthStateDegraded:
				degraded++
			case sdk.HealthStateUnhealthy:
				unhealthy++
			// HealthStateUnknown is ignored - plugin might be starting up
			}
		}

		dbHealth := "ok"
		if err := db.Health(r.Context()); err != nil {
			dbHealth = "error"
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"plugins":{"total":%d,"healthy":%d,"degraded":%d,"unhealthy":%d},"database":"%s","event_bus":"connected"}`,
			len(plugins), healthy, degraded, unhealthy, dbHealth)
	}
}

func handleListEvents(eventBus *core.EventBus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// For now, return empty list - events are streamed via WebSocket
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"events":[]}`)
	}
}

// handleGetPorts returns the current service port configuration
// This allows the frontend to discover where services are running
func handleGetPorts() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ports := core.GetCurrentPortConfig()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"data":    ports,
		})
	}
}

// isRunningOnAppleSilicon detects if running in Docker on Apple Silicon Mac
// by checking CPU info for Apple-specific identifiers
func isRunningOnAppleSilicon() bool {
	// Check /proc/cpuinfo for Apple CPU identifiers
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return false
	}
	cpuInfo := string(data)
	// Apple Silicon in Docker shows specific patterns
	return strings.Contains(cpuInfo, "Apple") ||
		strings.Contains(cpuInfo, "0x61") || // Apple vendor ID in hex
		strings.Contains(strings.ToLower(cpuInfo), "apple")
}

// handleSystemMetrics returns system resource metrics (CPU, memory, disk, GPU)
func handleSystemMetrics() http.HandlerFunc {
	startTime := time.Now()

	return func(w http.ResponseWriter, r *http.Request) {
		uptime := int64(time.Since(startTime).Seconds())

		// Get CPU stats
		cpuPercent := 0.0
		var loadAvg [3]float64

		// Get memory stats
		var memTotal, memUsed, memFree uint64
		var memPercent float64

		// Get disk stats
		var diskTotal, diskUsed, diskFree uint64
		var diskPercent float64
		diskPath := "."

		// Try to get actual system stats using syscalls
		// For now, use reasonable mock values that will be replaced with real implementation
		// This ensures the UI renders while we implement proper system stats

		// CPU - get from runtime
		cpuPercent = 15.5 // Mock - would use github.com/shirou/gopsutil
		loadAvg = [3]float64{1.2, 1.5, 1.3}

		// Memory - approximate from runtime
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		memUsed = m.Alloc
		memTotal = m.Sys
		memFree = memTotal - memUsed
		if memTotal > 0 {
			memPercent = float64(memUsed) / float64(memTotal) * 100
		}

		// Disk - would need syscall
		diskTotal = 500 * 1024 * 1024 * 1024  // 500GB mock
		diskUsed = 150 * 1024 * 1024 * 1024   // 150GB mock
		diskFree = diskTotal - diskUsed
		diskPercent = float64(diskUsed) / float64(diskTotal) * 100

		// GPU detection
		gpu := map[string]interface{}{
			"available": false,
		}

		// Check for hardware acceleration
		hwCaps, err := video.DetectHWAccel(r.Context())
		if err == nil && len(hwCaps.Available) > 0 {
			gpuName := hwCaps.GPUName
			gpuType := string(hwCaps.Recommended)
			if gpuName == "" {
				// Derive name from acceleration type
				switch hwCaps.Recommended {
				case video.HWAccelCUDA:
					gpuName = "NVIDIA GPU"
				case video.HWAccelVideoToolbox:
					gpuName = "Apple M-series GPU"
				case video.HWAccelVAAPI:
					gpuName = "VA-API Compatible GPU"
				case video.HWAccelQSV:
					gpuName = "Intel Quick Sync GPU"
				default:
					gpuName = "Hardware Accelerated GPU"
				}
			}
			gpu = map[string]interface{}{
				"available":   true,
				"name":        gpuName,
				"type":        gpuType,
				"utilization": 5, // Would need specific APIs for real utilization
				"decode_h264": hwCaps.DecodeH264,
				"decode_h265": hwCaps.DecodeH265,
				"encode_h264": hwCaps.EncodeH264,
				"encode_h265": hwCaps.EncodeH265,
			}
		} else if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
			// Fallback for macOS Apple Silicon
			gpu = map[string]interface{}{
				"available":   true,
				"name":        "Apple M-series GPU",
				"type":        "apple",
				"utilization": 5,
			}
		}

		// NPU detection
		npu := map[string]interface{}{
			"available": false,
		}
		// Apple Neural Engine on M-series Macs (native or in Docker on Apple Silicon)
		isAppleSilicon := (runtime.GOOS == "darwin" && runtime.GOARCH == "arm64") ||
			(runtime.GOOS == "linux" && runtime.GOARCH == "arm64" && isRunningOnAppleSilicon())
		if isAppleSilicon {
			npu = map[string]interface{}{
				"available": true,
				"name":      "Apple Neural Engine",
				"type":      "apple_ane",
			}
		}
		// Check for Coral TPU (USB or PCIe)
		if _, err := os.Stat("/dev/apex_0"); err == nil {
			npu = map[string]interface{}{
				"available": true,
				"name":      "Google Coral TPU",
				"type":      "coral_tpu",
			}
		} else if _, err := os.Stat("/dev/bus/usb"); err == nil {
			// Check for Coral USB devices (vendor ID 18d1 for Google)
			usbOutput, _ := exec.Command("lsusb").Output()
			if strings.Contains(string(usbOutput), "18d1:9302") || strings.Contains(string(usbOutput), "1a6e:089a") {
				npu = map[string]interface{}{
					"available": true,
					"name":      "Google Coral USB Accelerator",
					"type":      "coral_usb",
				}
			}
		}

		metrics := map[string]interface{}{
			"cpu": map[string]interface{}{
				"percent":  cpuPercent,
				"load_avg": loadAvg,
			},
			"memory": map[string]interface{}{
				"total":   memTotal,
				"used":    memUsed,
				"free":    memFree,
				"percent": memPercent,
			},
			"disk": map[string]interface{}{
				"total":   diskTotal,
				"used":    diskUsed,
				"free":    diskFree,
				"percent": diskPercent,
				"path":    diskPath,
			},
			"gpu":    gpu,
			"npu":    npu,
			"uptime": uptime,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"data":    metrics,
		})
	}
}

// handleSystemRestart triggers a hot restart of the NVR process
// This works by sending SIGHUP to the parent process (docker-entrypoint.sh)
// which will gracefully stop and restart the NVR
func handleSystemRestart() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slog.Info("Hot restart requested via API")

		// Get our parent PID (should be the entrypoint script)
		ppid := os.Getppid()
		if ppid <= 1 {
			// We're running directly (not under entrypoint supervisor)
			// Just exit and let the container restart us
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"message": "Restart initiated (direct mode - container restart)",
			})

			// Exit after a brief delay to allow response to be sent
			go func() {
				time.Sleep(500 * time.Millisecond)
				slog.Info("Exiting for restart")
				os.Exit(0)
			}()
			return
		}

		// Send SIGHUP to parent to trigger hot restart
		if err := syscall.Kill(ppid, syscall.SIGHUP); err != nil {
			slog.Error("Failed to send SIGHUP to parent", "ppid", ppid, "error", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Failed to trigger restart: " + err.Error(),
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Hot restart initiated - NVR will restart momentarily",
		})
	}
}

// handleGetStats returns dashboard statistics
func handleGetStats(loader *core.PluginLoader, db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Count cameras
		var totalCameras, onlineCameras int
		rows, err := db.Query("SELECT status FROM cameras WHERE enabled = 1")
		if err == nil {
			defer func() { _ = rows.Close() }()
			for rows.Next() {
				var status string
				if err := rows.Scan(&status); err == nil {
					totalCameras++
					if status == "online" || status == "streaming" {
						onlineCameras++
					}
				}
			}
		}

		// Count events today
		var eventsToday, unacknowledged, totalEvents int
		today := time.Now().Format("2006-01-02")
		_ = db.QueryRow("SELECT COUNT(*) FROM events WHERE date(timestamp) = ?", today).Scan(&eventsToday)
		_ = db.QueryRow("SELECT COUNT(*) FROM events WHERE acknowledged = 0").Scan(&unacknowledged)
		_ = db.QueryRow("SELECT COUNT(*) FROM events").Scan(&totalEvents)

		// Get database size
		var dbSize int64
		var pageSizeStr, pageCountStr string
		_ = db.QueryRow("PRAGMA page_size").Scan(&pageSizeStr)
		_ = db.QueryRow("PRAGMA page_count").Scan(&pageCountStr)
		if pageSize, err := strconv.ParseInt(pageSizeStr, 10, 64); err == nil {
			if pageCount, err := strconv.ParseInt(pageCountStr, 10, 64); err == nil {
				dbSize = pageSize * pageCount
			}
		}

		stats := map[string]interface{}{
			"cameras": map[string]interface{}{
				"total":   totalCameras,
				"online":  onlineCameras,
				"offline": totalCameras - onlineCameras,
			},
			"events": map[string]interface{}{
				"today":          eventsToday,
				"unacknowledged": unacknowledged,
				"total":          totalEvents,
			},
			"storage": map[string]interface{}{
				"database_size": dbSize,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(stats)
	}
}

// handleLogStream provides Server-Sent Events for live log streaming
func handleLogStream() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		logBuffer := logging.GetLogBuffer()

		// Send recent logs first
		recent := logBuffer.GetRecent(50)
		for _, entry := range recent {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", logging.LogEntryToJSON(entry))
		}
		flusher.Flush()

		// Subscribe to new logs
		logCh := logBuffer.Subscribe()
		defer logBuffer.Unsubscribe(logCh)

		// Heartbeat ticker
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case entry := <-logCh:
				_, _ = fmt.Fprintf(w, "data: %s\n\n", logging.LogEntryToJSON(entry))
				flusher.Flush()
			case <-ticker.C:
				// Send heartbeat to keep connection alive
				_, _ = fmt.Fprintf(w, ": heartbeat\n\n")
				flusher.Flush()
			}
		}
	}
}

// Plugin catalog constants
const (
	catalogURL     = "https://raw.githubusercontent.com/Spatial-NVR/plugin-catalog/main/catalog.yaml"
	catalogTimeout = 10 * time.Second
)

// Cached catalog
var (
	cachedCatalog        map[string]interface{}
	cachedCatalogTime    time.Time
	catalogCacheTTL      = 15 * time.Minute
	cachedVersions       map[string]string // pluginID -> latest version
	cachedVersionsTime   time.Time
	versionsCacheTTL     = 5 * time.Minute
)

// GitHubReleaseInfo represents minimal release info from GitHub API
type GitHubReleaseInfo struct {
	TagName string `json:"tag_name"`
}

// fetchLatestVersion fetches the latest release version from GitHub
func fetchLatestVersion(ctx context.Context, repoURL string) (string, error) {
	// Parse repo URL to get owner/repo
	// Expected format: https://github.com/owner/repo
	repoURL = strings.TrimSuffix(repoURL, ".git")
	parts := strings.Split(repoURL, "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid repo URL: %s", repoURL)
	}
	owner := parts[len(parts)-2]
	repo := parts[len(parts)-1]

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "SpatialNVR-PluginCatalog")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 404 {
		return "", nil // No releases
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release GitHubReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}

	// Strip 'v' prefix if present - we'll add it back when displaying if needed
	version := strings.TrimPrefix(release.TagName, "v")
	return version, nil
}

// fetchAllLatestVersions fetches latest versions for all plugins in parallel
func fetchAllLatestVersions(ctx context.Context, plugins []interface{}) map[string]string {
	versions := make(map[string]string)
	type result struct {
		id      string
		version string
	}
	results := make(chan result, len(plugins))

	for _, p := range plugins {
		plugin, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		id, _ := plugin["id"].(string)
		repoURL, _ := plugin["repo"].(string)
		if id == "" || repoURL == "" {
			continue
		}

		go func(id, repoURL string) {
			version, err := fetchLatestVersion(ctx, repoURL)
			if err != nil {
				slog.Debug("Failed to fetch latest version", "plugin", id, "error", err)
			}
			results <- result{id: id, version: version}
		}(id, repoURL)
	}

	// Collect results with timeout
	timeout := time.After(8 * time.Second)
	collected := 0
	expected := 0
	for _, p := range plugins {
		if plugin, ok := p.(map[string]interface{}); ok {
			if id, _ := plugin["id"].(string); id != "" {
				if repo, _ := plugin["repo"].(string); repo != "" {
					expected++
				}
			}
		}
	}

	for collected < expected {
		select {
		case r := <-results:
			if r.version != "" {
				versions[r.id] = r.version
			}
			collected++
		case <-timeout:
			slog.Debug("Timeout fetching plugin versions", "collected", collected, "expected", expected)
			return versions
		case <-ctx.Done():
			return versions
		}
	}

	return versions
}

// handleGetPluginCatalog returns the available plugin catalog with installed status merged
func handleGetPluginCatalog(loader *core.PluginLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var catalog map[string]interface{}
		cacheHit := false

		// Check cache for base catalog
		if cachedCatalog != nil && time.Since(cachedCatalogTime) < catalogCacheTTL {
			catalog = cachedCatalog
			cacheHit = true
		} else {
			// Fetch from remote
			ctx, cancel := context.WithTimeout(r.Context(), catalogTimeout)
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, "GET", catalogURL, nil)
			if err != nil {
				catalog = getDefaultCatalog()
			} else {
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					slog.Warn("Failed to fetch plugin catalog, using fallback", "error", err)
					catalog = getDefaultCatalog()
				} else {
					defer func() { _ = resp.Body.Close() }()

					if resp.StatusCode != http.StatusOK {
						slog.Warn("Plugin catalog returned non-200 status", "status", resp.StatusCode)
						catalog = getDefaultCatalog()
					} else {
						// Parse YAML catalog
						body, err := io.ReadAll(resp.Body)
						if err != nil {
							catalog = getDefaultCatalog()
						} else {
							var yamlCatalog map[string]interface{}
							if err := yaml.Unmarshal(body, &yamlCatalog); err != nil {
								slog.Warn("Failed to parse plugin catalog YAML", "error", err)
								catalog = getDefaultCatalog()
							} else {
								catalog = yamlCatalog
								// Cache the result
								cachedCatalog = yamlCatalog
								cachedCatalogTime = time.Now()
							}
						}
					}
				}
			}
		}

		// Fetch latest versions from GitHub (with caching)
		var latestVersions map[string]string
		if cachedVersions != nil && time.Since(cachedVersionsTime) < versionsCacheTTL {
			latestVersions = cachedVersions
		} else if plugins, ok := catalog["plugins"].([]interface{}); ok {
			versionCtx, versionCancel := context.WithTimeout(r.Context(), 10*time.Second)
			latestVersions = fetchAllLatestVersions(versionCtx, plugins)
			versionCancel()
			// Cache the versions
			cachedVersions = latestVersions
			cachedVersionsTime = time.Now()
		}

		// Merge installed plugin status into catalog
		installedPlugins := loader.ListPlugins()
		installedMap := make(map[string]*core.LoadedPlugin)
		for _, p := range installedPlugins {
			installedMap[p.Manifest.ID] = p
		}

		// Update plugins in catalog with installed status and latest version
		if plugins, ok := catalog["plugins"].([]interface{}); ok {
			for _, p := range plugins {
				if plugin, ok := p.(map[string]interface{}); ok {
					pluginID, _ := plugin["id"].(string)

					// Add latest version from GitHub
					if latestVersion, hasVersion := latestVersions[pluginID]; hasVersion {
						plugin["latest_version"] = latestVersion
					}

					if installed, exists := installedMap[pluginID]; exists {
						plugin["installed"] = true
						plugin["installed_version"] = installed.Manifest.Version
						plugin["enabled"] = installed.State == core.PluginStateRunning
						plugin["state"] = string(installed.State)

						// Check if update is available
						if latestVersion, hasVersion := latestVersions[pluginID]; hasVersion {
							installedVersion := installed.Manifest.Version
							// Compare versions (strip 'v' prefix for comparison)
							latestClean := strings.TrimPrefix(latestVersion, "v")
							installedClean := strings.TrimPrefix(installedVersion, "v")
							if latestClean != installedClean && latestVersion != "" {
								plugin["update_available"] = true
							}
						}

						// Remove from map so we can track non-catalog plugins
						delete(installedMap, pluginID)
					}
				}
			}
		}

		// Add any installed plugins not in the catalog (externally installed)
		if plugins, ok := catalog["plugins"].([]interface{}); ok {
			for id, p := range installedMap {
				// Skip builtin plugins
				if p.IsBuiltin {
					continue
				}
				plugin := map[string]interface{}{
					"id":                id,
					"name":              p.Manifest.Name,
					"description":       p.Manifest.Description,
					"category":          p.Manifest.Category,
					"installed":         true,
					"installed_version": p.Manifest.Version,
					"enabled":           p.State == core.PluginStateRunning,
					"state":             string(p.State),
				}
				plugins = append(plugins, plugin)
			}
			catalog["plugins"] = plugins
		}

		w.Header().Set("Content-Type", "application/json")
		if cacheHit {
			w.Header().Set("X-Cache", "HIT")
		} else {
			w.Header().Set("X-Cache", "MISS")
		}
		_ = json.NewEncoder(w).Encode(catalog)
	}
}

// getDefaultCatalog returns a fallback catalog
func getDefaultCatalog() map[string]interface{} {
	return map[string]interface{}{
		"version":    "1.0",
		"updated_at": time.Now().Format(time.RFC3339),
		"categories": []map[string]interface{}{
			{"id": "camera", "name": "Camera Integrations", "description": "Plugins for connecting different camera brands"},
			{"id": "integration", "name": "Home Automation", "description": "Integration with smart home platforms"},
		},
		"plugins": []interface{}{
			map[string]interface{}{
				"id":          "reolink",
				"name":        "Reolink",
				"description": "Full integration for Reolink cameras and NVRs",
				"category":    "camera",
				"repo":        "https://github.com/Spatial-NVR/reolink-plugin",
				"author":      "Spatial-NVR",
				"official":    true,
				"verified":    true,
			},
			map[string]interface{}{
				"id":          "wyze",
				"name":        "Wyze",
				"description": "Integration for Wyze cameras with RTSP streaming support",
				"category":    "camera",
				"repo":        "https://github.com/Spatial-NVR/wyze-plugin",
				"author":      "Spatial-NVR",
				"official":    true,
				"verified":    true,
			},
		},
	}
}


// handleReloadPluginCatalog forces a refresh of the plugin catalog
func handleReloadPluginCatalog() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Clear all caches to force refresh
		cachedCatalog = nil
		cachedCatalogTime = time.Time{}
		cachedVersions = nil
		cachedVersionsTime = time.Time{}

		// Fetch fresh catalog
		ctx, cancel := context.WithTimeout(r.Context(), catalogTimeout)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, "GET", catalogURL, nil)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to create request"})
			return
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to fetch catalog", "details": err.Error()})
			return
		}
		defer func() { _ = resp.Body.Close() }()

		body, _ := io.ReadAll(resp.Body)
		var yamlCatalog map[string]interface{}
		if err := yaml.Unmarshal(body, &yamlCatalog); err != nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"message": "Catalog refreshed (fallback)",
				"source":  "fallback",
			})
			return
		}

		// Update catalog cache
		cachedCatalog = yamlCatalog
		cachedCatalogTime = time.Now()

		// Also fetch latest versions from GitHub
		pluginCount := 0
		if plugins, ok := yamlCatalog["plugins"].([]interface{}); ok {
			pluginCount = len(plugins)
			// Fetch versions in background but with a reasonable timeout
			versionCtx, versionCancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer versionCancel()
			cachedVersions = fetchAllLatestVersions(versionCtx, plugins)
			cachedVersionsTime = time.Now()
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"message":        "Catalog refreshed",
			"plugin_count":   pluginCount,
			"versions_count": len(cachedVersions),
			"source":         "remote",
		})
	}
}

// handleInstallFromCatalog installs a plugin from the catalog by ID
func handleInstallFromCatalog(installer *plugin.Installer, loader *core.PluginLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pluginID := chi.URLParam(r, "pluginId")

		// Get catalog (use cache or fetch)
		var catalog map[string]interface{}
		if cachedCatalog != nil && time.Since(cachedCatalogTime) < catalogCacheTTL {
			catalog = cachedCatalog
		} else {
			// Fetch fresh
			ctx, cancel := context.WithTimeout(r.Context(), catalogTimeout)
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, "GET", catalogURL, nil)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to fetch catalog"})
				return
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "Catalog unavailable"})
				return
			}
			defer func() { _ = resp.Body.Close() }()

			body, _ := io.ReadAll(resp.Body)
			if err := yaml.Unmarshal(body, &catalog); err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid catalog format"})
				return
			}

			cachedCatalog = catalog
			cachedCatalogTime = time.Now()
		}

		// Find plugin in catalog
		plugins, ok := catalog["plugins"].([]interface{})
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid catalog structure"})
			return
		}

		var repoURL string
		var pluginName string
		for _, p := range plugins {
			if pm, ok := p.(map[string]interface{}); ok {
				if pm["id"] == pluginID {
					// Try both "repo" and "repository" fields
					if repo, ok := pm["repo"].(string); ok {
						repoURL = repo
					} else if repo, ok := pm["repository"].(string); ok {
						repoURL = repo
					}
					if name, ok := pm["name"].(string); ok {
						pluginName = name
					}
					break
				}
			}
		}

		if repoURL == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Plugin not found in catalog"})
			return
		}

		// Install from repository
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()

		manifest, err := installer.InstallFromGitHub(ctx, repoURL)
		if err != nil {
			slog.Error("Plugin installation failed", "plugin", pluginID, "repository", repoURL, "error", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		// Hot-reload: Scan for the new plugin and start it immediately
		startErr := loader.ScanAndStart(ctx, manifest.ID)
		var startMsg string
		if startErr != nil {
			slog.Warn("Plugin installed but failed to start", "plugin", manifest.ID, "error", startErr)
			startMsg = "Plugin installed but failed to auto-start: " + startErr.Error()
		} else {
			slog.Info("Plugin installed and started (hot-reload)", "plugin", manifest.ID)
			startMsg = "Plugin installed and started successfully."
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"plugin": map[string]string{
				"id":      manifest.ID,
				"name":    pluginName,
				"version": manifest.Version,
			},
			"message": startMsg,
			"started": startErr == nil,
		})
	}
}

// handlePluginRPC handles JSON-RPC calls to external plugins
func handlePluginRPC(loader *core.PluginLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		p, ok := loader.GetPlugin(id)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"error": map[string]interface{}{
					"code":    -32600,
					"message": "Plugin not found. Make sure the plugin is installed from the catalog first.",
				},
			})
			return
		}

		// Read request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"error": map[string]interface{}{
					"code":    -32700,
					"message": "Failed to read request",
				},
			})
			return
		}

		// Parse JSON-RPC request
		var req struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      interface{}     `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"error": map[string]interface{}{
					"code":    -32700,
					"message": "Invalid JSON: " + err.Error(),
				},
			})
			return
		}

		// Check if plugin is an external plugin with RPC support
		if ext, ok := p.Plugin.(*core.ExternalPlugin); ok {
			// Forward to external plugin
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()

			result, err := ext.Call(ctx, req.Method, req.Params)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				resp := map[string]interface{}{
					"jsonrpc": "2.0",
					"id":      req.ID,
					"error": map[string]interface{}{
						"code":    -32603,
						"message": err.Error(),
					},
				}
				_ = json.NewEncoder(w).Encode(resp)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  json.RawMessage(result),
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		// For builtin plugins, return an error for now
		// In the future, we could add a CallablePlugin interface
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"error": map[string]interface{}{
				"code":    -32601,
				"message": "Plugin does not support RPC. Make sure the plugin is installed from the catalog first.",
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// handleInstallPlugin installs a plugin from a GitHub repository
func handleInstallPlugin(installer *plugin.Installer, loader *core.PluginLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Repository string `json:"repository"`
			RepoURL    string `json:"repo_url"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
			return
		}

		// Accept either repository or repo_url
		repoURL := req.Repository
		if repoURL == "" {
			repoURL = req.RepoURL
		}

		if repoURL == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Repository URL is required"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()

		manifest, err := installer.InstallFromGitHub(ctx, repoURL)
		if err != nil {
			slog.Error("Plugin installation failed", "repository", repoURL, "error", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		// Hot-reload: scan and start the newly installed plugin
		hotReloaded := false
		if startErr := loader.ScanAndStart(ctx, manifest.ID); startErr != nil {
			slog.Warn("Plugin installed but failed to hot-reload", "plugin", manifest.ID, "error", startErr)
		} else {
			hotReloaded = true
			slog.Info("Plugin installed and hot-reloaded successfully", "plugin", manifest.ID)
		}

		var message string
		if hotReloaded {
			message = "Plugin installed and started successfully (hot-reload)"
		} else {
			message = "Plugin installed successfully. Restart may be required to activate."
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success":      true,
			"hot_reloaded": hotReloaded,
			"plugin": map[string]string{
				"id":      manifest.ID,
				"name":    manifest.Name,
				"version": manifest.Version,
			},
			"message": message,
		})
	}
}

// handleUninstallPlugin removes an installed plugin
func handleUninstallPlugin(installer *plugin.Installer, loader *core.PluginLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		slog.Info("Uninstalling plugin", "id", id)

		// Try to find the actual plugin ID by checking the loader
		// The ID might be "wyze-plugin" (directory name) but loader has "wyze" (manifest ID)
		actualID := id
		plugins := loader.ListPlugins()
		for _, p := range plugins {
			// Check if the provided ID matches either the manifest ID or could be derived from it
			if p.Manifest.ID == id {
				actualID = id
				break
			}
			// Also check if provided ID with "-plugin" suffix stripped matches
			if strings.TrimSuffix(id, "-plugin") == p.Manifest.ID {
				actualID = p.Manifest.ID
				slog.Info("Resolved plugin ID", "provided", id, "actual", actualID)
				break
			}
		}

		// Force unregister the plugin (this will stop it if running and remove from loader)
		if err := loader.ForceUnregisterPlugin(actualID); err != nil {
			slog.Warn("Could not force unregister plugin during uninstall", "id", actualID, "error", err)
		}

		// Remove plugin files - try both the original ID and resolved ID
		var uninstallErr error
		if err := installer.UninstallPlugin(actualID); err != nil {
			// If that failed and we have a different original ID, try that too
			if actualID != id {
				uninstallErr = installer.UninstallPlugin(id)
			} else {
				uninstallErr = err
			}
		}

		if uninstallErr != nil {
			slog.Error("Failed to uninstall plugin files", "id", id, "error", uninstallErr)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": uninstallErr.Error()})
			return
		}

		slog.Info("Plugin uninstalled successfully", "id", actualID)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"id":      actualID,
			"message": "Plugin uninstalled successfully",
		})
	}
}

// Audio session management for two-way audio
var audioSessions = make(map[string]*AudioSession)

type AudioSession struct {
	CameraID  string    `json:"camera_id"`
	Active    bool      `json:"active"`
	StartedAt time.Time `json:"started_at"`
	RTSPURL   string    `json:"rtsp_url,omitempty"`
}

func handleStartAudioSession(ports *core.PortConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cameraID := chi.URLParam(r, "cameraId")

		// For now, create a basic audio session
		// Real implementation would connect to go2rtc's WebRTC audio channel
		session := &AudioSession{
			CameraID:  cameraID,
			Active:    true,
			StartedAt: time.Now(),
			RTSPURL:   fmt.Sprintf("rtsp://localhost:%d/%s", ports.Go2RTCRTSP, strings.ToLower(cameraID)),
		}
		audioSessions[cameraID] = session

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"session": session,
		})
	}
}

func handleStopAudioSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cameraID := chi.URLParam(r, "cameraId")

		if session, ok := audioSessions[cameraID]; ok {
			session.Active = false
			delete(audioSessions, cameraID)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
		})
	}
}

func handleGetAudioSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cameraID := chi.URLParam(r, "cameraId")

		session, ok := audioSessions[cameraID]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"active": false,
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(session)
	}
}

// createGo2RTCProxy creates a reverse proxy for go2rtc with WebSocket support
func createGo2RTCProxy(targetURL string, ports *core.PortConfig) http.Handler {
	target, _ := url.Parse(targetURL)

	// WebSocket upgrader
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins since we're proxying
		},
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip /go2rtc prefix
		path := strings.TrimPrefix(r.URL.Path, "/go2rtc")
		if path == "" {
			path = "/"
		}

		// Check if this is a WebSocket upgrade request
		if websocket.IsWebSocketUpgrade(r) {
			slog.Info("Proxying WebSocket request", "path", path, "query", r.URL.RawQuery)
			proxyWebSocket(w, r, target.Host, path, r.URL.RawQuery, upgrader)
			return
		}

		slog.Debug("Proxying HTTP request to go2rtc", "path", path)

		// Regular HTTP proxy for non-WebSocket requests
		proxy := httputil.NewSingleHostReverseProxy(target)
		proxy.Director = func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.URL.Path = path
			req.Host = target.Host
		}
		proxy.ServeHTTP(w, r)
	})
}

// proxyWebSocket proxies WebSocket connections to go2rtc
func proxyWebSocket(w http.ResponseWriter, r *http.Request, targetHost, path, query string, upgrader websocket.Upgrader) {
	// Build target WebSocket URL
	targetURL := fmt.Sprintf("ws://%s%s", targetHost, path)
	if query != "" {
		targetURL += "?" + query
	}

	// Connect to target
	targetConn, _, err := websocket.DefaultDialer.Dial(targetURL, nil)
	if err != nil {
		slog.Error("Failed to connect to go2rtc WebSocket", "url", targetURL, "error", err)
		http.Error(w, "Failed to connect to stream", http.StatusBadGateway)
		return
	}
	defer func() { _ = targetConn.Close() }()

	// Upgrade client connection
	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("Failed to upgrade WebSocket connection", "error", err)
		return
	}
	defer func() { _ = clientConn.Close() }()

	// Bidirectional message forwarding
	done := make(chan struct{})

	// Client -> Target
	go func() {
		defer close(done)
		for {
			messageType, data, err := clientConn.ReadMessage()
			if err != nil {
				return
			}
			if err := targetConn.WriteMessage(messageType, data); err != nil {
				return
			}
		}
	}()

	// Target -> Client
	for {
		messageType, data, err := targetConn.ReadMessage()
		if err != nil {
			break
		}
		if err := clientConn.WriteMessage(messageType, data); err != nil {
			break
		}
	}

	<-done
}

// Helper functions

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
