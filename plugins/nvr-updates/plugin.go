package nvrupdates

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Spatial-NVR/SpatialNVR/internal/config"
	"github.com/Spatial-NVR/SpatialNVR/internal/updater"
	"github.com/Spatial-NVR/SpatialNVR/sdk"
)

// Version is set at build time (should match versions.json)
var Version = "0.0.6"

// Plugin implements the NVR updates plugin
type Plugin struct {
	sdk.BaseServicePlugin

	updater    *updater.Updater
	config     *PluginConfig
	configPath string // Path to the system config file
	logger     *slog.Logger

	mu sync.RWMutex
}

// PluginConfig holds the plugin configuration
type PluginConfig struct {
	CheckInterval      string `json:"check_interval"`   // e.g., "6h"
	AutoUpdate         bool   `json:"auto_update"`
	AutoUpdateTime     string `json:"auto_update_time"` // e.g., "03:00"
	IncludePrereleases bool   `json:"include_prereleases"`
	DataPath           string `json:"data_path"`
	GitHubToken        string `json:"github_token"` // GitHub token for private repos

	// Per-component settings
	Components map[string]ComponentConfig `json:"components"`
}

// ComponentConfig holds per-component settings
type ComponentConfig struct {
	Enabled    bool   `json:"enabled"`
	AutoUpdate bool   `json:"auto_update"`
	Channel    string `json:"channel"` // stable, beta
}

// DefaultConfig returns the default configuration
func DefaultConfig() *PluginConfig {
	return &PluginConfig{
		CheckInterval:      "6h",
		AutoUpdate:         false,
		AutoUpdateTime:     "03:00",
		IncludePrereleases: false,
		DataPath:           "/data/updates",
		Components: map[string]ComponentConfig{
			"nvr-core": {Enabled: true, AutoUpdate: false, Channel: "stable"},
			"web-ui":   {Enabled: true, AutoUpdate: false, Channel: "stable"},
			"go2rtc":   {Enabled: true, AutoUpdate: false, Channel: "stable"},
		},
	}
}

// New creates a new updates plugin
func New() *Plugin {
	p := &Plugin{
		config: DefaultConfig(),
		logger: slog.Default().With("plugin", "nvr-updates"),
	}
	p.SetManifest(sdk.PluginManifest{
		ID:          "nvr-updates",
		Name:        "Updates",
		Version:     Version,
		Description: "Manages system updates for SpatialNVR components",
		Category:    "core",
		Capabilities: []string{
			"updates",
		},
		Dependencies: []string{},
		Critical:     false,
	})
	return p
}

// Initialize initializes the plugin
func (p *Plugin) Initialize(ctx context.Context, rt *sdk.PluginRuntime) error {
	if err := p.BaseServicePlugin.Initialize(ctx, rt); err != nil {
		return err
	}

	// Get data path from runtime config
	if dataPath := rt.ConfigString("data_path", ""); dataPath != "" {
		p.config.DataPath = dataPath
	}
	if checkInterval := rt.ConfigString("check_interval", ""); checkInterval != "" {
		p.config.CheckInterval = checkInterval
	}
	p.config.AutoUpdate = rt.ConfigBool("auto_update", false)

	// Store config path for later use
	p.configPath = rt.ConfigString("config_path", "")

	// Get GitHub token from config or environment
	if token := rt.ConfigString("github_token", ""); token != "" {
		p.config.GitHubToken = token
	} else if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		p.config.GitHubToken = token
	} else {
		// Try to read from system config file
		p.config.GitHubToken = p.loadGitHubTokenFromConfig()
	}

	return nil
}

// Start starts the plugin
func (p *Plugin) Start(ctx context.Context) error {
	// Parse check interval
	interval, err := time.ParseDuration(p.config.CheckInterval)
	if err != nil {
		interval = 6 * time.Hour
	}

	// Create updater
	p.updater = updater.NewUpdater(updater.Config{
		CheckInterval:      interval,
		AutoUpdate:         p.config.AutoUpdate,
		AutoUpdateTime:     p.config.AutoUpdateTime,
		IncludePrereleases: p.config.IncludePrereleases,
		DataPath:           p.config.DataPath,
		GitHubToken:        p.config.GitHubToken,
	}, p.logger)

	// Register core components
	p.registerComponents()

	// Set up callbacks
	p.updater.SetOnUpdateAvailable(func(component, current, latest string) {
		p.logger.Info("Update available", "component", component, "current", current, "latest", latest)

		// Publish event using BasePlugin method
		_ = p.PublishEvent("update.available", map[string]interface{}{
			"component":       component,
			"current_version": current,
			"latest_version":  latest,
		})
	})

	p.updater.SetOnUpdateComplete(func(component, version string) {
		p.logger.Info("Update complete", "component", component, "version", version)

		// Publish event using BasePlugin method
		_ = p.PublishEvent("update.complete", map[string]interface{}{
			"component": component,
			"version":   version,
		})
	})

	p.updater.SetOnRestartNeeded(func() {
		p.logger.Info("Update installed, triggering hot-restart")

		// Publish event before restart
		_ = p.PublishEvent("system.restarting", map[string]interface{}{
			"reason": "update_installed",
		})

		// Give a moment for the event to be published and response to be sent
		go func() {
			time.Sleep(500 * time.Millisecond)
			p.triggerRestart()
		}()
	})

	// Start updater
	if err := p.updater.Start(ctx); err != nil {
		return fmt.Errorf("failed to start updater: %w", err)
	}

	p.SetHealthy("Updates plugin started")
	p.logger.Info("Updates plugin started", "check_interval", p.config.CheckInterval)
	return nil
}

// Stop stops the plugin
func (p *Plugin) Stop(ctx context.Context) error {
	if p.updater != nil {
		p.updater.Stop()
	}
	return nil
}

// registerComponents registers the updatable components
func (p *Plugin) registerComponents() {
	arch := runtime.GOARCH

	// NVR Core binary
	p.updater.RegisterComponent(updater.Component{
		Name:           "nvr-core",
		CurrentVersion: Version,
		Repository:     "Spatial-NVR/SpatialNVR",
		AssetPattern:   fmt.Sprintf("spatialnvr-linux-%s", arch),
		InstallPath:    "/data/bin/nvr",
		AutoUpdate:     p.config.Components["nvr-core"].AutoUpdate,
	})

	// Web UI assets
	p.updater.RegisterComponent(updater.Component{
		Name:           "web-ui",
		CurrentVersion: Version,
		Repository:     "Spatial-NVR/SpatialNVR",
		AssetPattern:   "web-ui",
		InstallPath:    "/data/web",
		AutoUpdate:     p.config.Components["web-ui"].AutoUpdate,
	})

	// go2rtc
	p.updater.RegisterComponent(updater.Component{
		Name:           "go2rtc",
		CurrentVersion: "1.9.13", // TODO: detect actual version
		Repository:     "AlexxIT/go2rtc",
		AssetPattern:   fmt.Sprintf("go2rtc_linux_%s", arch),
		InstallPath:    "/data/bin/go2rtc",
		AutoUpdate:     p.config.Components["go2rtc"].AutoUpdate,
	})
}

// Routes returns the HTTP routes for this plugin
func (p *Plugin) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/", p.handleGetUpdates)
	r.Post("/check", p.handleCheckUpdates)
	r.Post("/{component}", p.handleUpdate)
	r.Post("/all", p.handleUpdateAll)
	r.Get("/status", p.handleGetStatus)
	r.Get("/config", p.handleGetConfig)
	r.Put("/config", p.handleSetConfig)
	return r
}

// handleGetUpdates returns all components and their update status
func (p *Plugin) handleGetUpdates(w http.ResponseWriter, r *http.Request) {
	if p.updater == nil {
		http.Error(w, "Updater not initialized", http.StatusServiceUnavailable)
		return
	}

	components := p.updater.GetComponents()
	pending := p.updater.GetPendingUpdates()

	response := map[string]interface{}{
		"components":      components,
		"pending_updates": len(pending),
		"needs_restart":   p.updater.NeedsRestart(),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// handleCheckUpdates triggers an update check
func (p *Plugin) handleCheckUpdates(w http.ResponseWriter, r *http.Request) {
	if p.updater == nil {
		http.Error(w, "Updater not initialized", http.StatusServiceUnavailable)
		return
	}

	ctx := r.Context()

	// Check all components
	components := p.updater.GetComponents()
	for _, c := range components {
		if err := p.updater.CheckUpdate(ctx, c.Name); err != nil {
			p.logger.Error("Failed to check update", "component", c.Name, "error", err)
		}
	}

	// Return updated status
	p.handleGetUpdates(w, r)
}

// handleUpdate updates a specific component
func (p *Plugin) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if p.updater == nil {
		http.Error(w, "Updater not initialized", http.StatusServiceUnavailable)
		return
	}

	component := chi.URLParam(r, "component")
	ctx := r.Context()

	if err := p.updater.Update(ctx, component); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	status := p.updater.GetUpdateStatus(component)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

// handleUpdateAll updates all components with updates available
func (p *Plugin) handleUpdateAll(w http.ResponseWriter, r *http.Request) {
	if p.updater == nil {
		http.Error(w, "Updater not initialized", http.StatusServiceUnavailable)
		return
	}

	ctx := r.Context()

	if err := p.updater.UpdateAll(ctx); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	status := p.updater.GetAllUpdateStatus()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

// handleGetStatus returns the current update status
func (p *Plugin) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	if p.updater == nil {
		http.Error(w, "Updater not initialized", http.StatusServiceUnavailable)
		return
	}

	status := p.updater.GetAllUpdateStatus()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

// handleGetConfig returns the plugin configuration
func (p *Plugin) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p.config)
}

// handleSetConfig updates the plugin configuration
func (p *Plugin) handleSetConfig(w http.ResponseWriter, r *http.Request) {
	var newConfig PluginConfig
	if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	p.mu.Lock()
	p.config = &newConfig
	p.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p.config)
}

// triggerRestart sends SIGHUP to the parent process (docker-entrypoint.sh) to trigger a hot-restart
func (p *Plugin) triggerRestart() {
	// Get parent PID (docker-entrypoint.sh)
	ppid := os.Getppid()

	if ppid <= 1 {
		// Running directly (not under entrypoint), exit and let container restart policy handle it
		p.logger.Info("No parent process to signal, exiting for restart")
		os.Exit(0)
		return
	}

	// Send SIGHUP to parent to trigger hot-restart
	p.logger.Info("Sending SIGHUP to parent process", "ppid", ppid)
	if err := syscall.Kill(ppid, syscall.SIGHUP); err != nil {
		p.logger.Error("Failed to send SIGHUP to parent", "error", err)
		// Fallback: exit and let container restart
		os.Exit(0)
	}
}

// Health returns the health status
func (p *Plugin) Health() sdk.HealthStatus {
	if p.updater == nil {
		return sdk.HealthStatus{
			State:       sdk.HealthStateUnknown,
			Message:     "Updater not initialized",
			LastChecked: time.Now(),
		}
	}

	pending := p.updater.GetPendingUpdates()

	message := "No updates available"
	if len(pending) > 0 {
		message = fmt.Sprintf("%d update(s) available", len(pending))
	}
	if p.updater.NeedsRestart() {
		message = "Restart required to apply updates"
	}

	return sdk.HealthStatus{
		State:       sdk.HealthStateHealthy,
		Message:     message,
		LastChecked: time.Now(),
		Details: map[string]string{
			"pending_updates": strconv.Itoa(len(pending)),
			"needs_restart":   strconv.FormatBool(p.updater.NeedsRestart()),
		},
	}
}

// EventSubscriptions returns events this plugin subscribes to
func (p *Plugin) EventSubscriptions() []string {
	return []string{sdk.EventTypeConfigChanged}
}

// HandleEvent handles events from the event bus
func (p *Plugin) HandleEvent(eventType string, data interface{}) {
	if eventType == sdk.EventTypeConfigChanged {
		// Reload GitHub token from config
		newToken := p.loadGitHubTokenFromConfig()
		if newToken != "" && newToken != p.config.GitHubToken {
			p.mu.Lock()
			p.config.GitHubToken = newToken
			// Update the updater's config if running
			if p.updater != nil {
				p.updater.SetGitHubToken(newToken)
			}
			p.mu.Unlock()
			p.logger.Info("GitHub token updated from config")
		}
	}
}

// loadGitHubTokenFromConfig reads the GitHub token from the system config file
func (p *Plugin) loadGitHubTokenFromConfig() string {
	if p.configPath == "" {
		return ""
	}

	cfg, err := config.Load(p.configPath)
	if err != nil {
		p.logger.Debug("Could not load config for GitHub token", "error", err)
		return ""
	}

	return cfg.System.Updates.GitHubToken
}
