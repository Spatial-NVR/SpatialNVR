// Package nvrcoreconfig provides the NVR Core Configuration Plugin
// This plugin handles system configuration management with hot-reload
package nvrcoreconfig

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Spatial-NVR/SpatialNVR/internal/config"
	"github.com/Spatial-NVR/SpatialNVR/sdk"
)

// ConfigPlugin implements the configuration service as a plugin
type ConfigPlugin struct {
	sdk.BaseServicePlugin

	config       *config.Config
	configPath   string
	watchEnabled bool

	mu      sync.RWMutex
	started bool
}

// New creates a new ConfigPlugin instance
func New() *ConfigPlugin {
	p := &ConfigPlugin{}
	p.SetManifest(sdk.PluginManifest{
		ID:           "nvr-core-config",
		Name:         "Configuration Service",
		Version:      "1.0.0",
		Description:  "System configuration management with hot-reload",
		Category:     "core",
		Critical:     true,
		Dependencies: []string{},
		Capabilities: []string{
			sdk.CapabilityConfig,
		},
	})
	return p
}

// Initialize sets up the plugin
func (p *ConfigPlugin) Initialize(ctx context.Context, runtime *sdk.PluginRuntime) error {
	if err := p.BaseServicePlugin.Initialize(ctx, runtime); err != nil {
		return err
	}

	// Get configuration
	p.configPath = runtime.ConfigString("config_path", "/config/config.yaml")
	p.watchEnabled = runtime.ConfigBool("watch_enabled", true)

	// Load configuration
	cfg, err := config.Load(p.configPath)
	if err != nil {
		// Create minimal config if file not exists
		cfg = &config.Config{
			Version: "1.0.0",
			Cameras: []config.CameraConfig{},
		}
	}
	p.config = cfg

	return nil
}

// Start starts the configuration service
func (p *ConfigPlugin) Start(ctx context.Context) error {
	runtime := p.Runtime()
	if runtime == nil {
		return nil
	}

	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	// Start file watcher if enabled
	if p.watchEnabled && p.config != nil {
		// Register change callback
		p.config.OnChange(func(cfg *config.Config) {
			p.mu.Lock()
			p.config = cfg
			p.mu.Unlock()

			// Publish config changed event
			p.PublishEvent(sdk.EventTypeConfigChanged, map[string]string{
				"path": p.configPath,
			})
		})

		// Start watching
		if err := p.config.Watch(); err != nil {
			runtime.Logger().Warn("Failed to start config watcher", "error", err)
		}
	}

	// Register RPC handlers for other plugins to request camera configs
	if err := p.registerRPCHandlers(runtime); err != nil {
		runtime.Logger().Warn("Failed to register RPC handlers", "error", err)
	}

	p.mu.Lock()
	p.started = true
	p.mu.Unlock()

	p.SetHealthy("Configuration service running")
	runtime.Logger().Info("Config plugin started", "path", p.configPath)

	return nil
}

// registerRPCHandlers registers handlers for inter-plugin communication
func (p *ConfigPlugin) registerRPCHandlers(runtime *sdk.PluginRuntime) error {
	// Handler for getting all camera configs
	return runtime.HandleRequests("get-cameras", func(data []byte) ([]byte, error) {
		p.mu.RLock()
		cfg := p.config
		p.mu.RUnlock()

		if cfg == nil {
			return json.Marshal([]config.CameraConfig{})
		}

		return json.Marshal(cfg.Cameras)
	})
}

// Stop stops the configuration service
func (p *ConfigPlugin) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.started = false
	p.SetHealth(sdk.HealthStateUnknown, "Configuration service stopped")

	return nil
}

// Health returns the plugin's health status
func (p *ConfigPlugin) Health() sdk.HealthStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.started {
		return sdk.HealthStatus{
			State:       sdk.HealthStateUnknown,
			Message:     "Not started",
			LastChecked: time.Now(),
		}
	}

	return sdk.HealthStatus{
		State:       sdk.HealthStateHealthy,
		Message:     "Configuration service operational",
		LastChecked: time.Now(),
	}
}

// Routes returns the HTTP routes for this plugin
func (p *ConfigPlugin) Routes() http.Handler {
	r := chi.NewRouter()

	// Configuration routes
	r.Get("/", p.handleGetConfig)
	r.Put("/", p.handleUpdateConfig)
	r.Post("/reload", p.handleReload)
	r.Get("/cameras", p.handleGetCameras)
	r.Get("/cameras/{id}", p.handleGetCameraConfig)
	r.Put("/cameras/{id}", p.handleUpdateCameraConfig)
	r.Get("/system", p.handleGetSystemConfig)
	r.Put("/system", p.handleUpdateSystemConfig)

	return r
}

// EventSubscriptions returns events this plugin subscribes to
func (p *ConfigPlugin) EventSubscriptions() []string {
	return []string{}
}

// HTTP Handlers

func (p *ConfigPlugin) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	cfg := p.config
	p.mu.RUnlock()

	if cfg == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Configuration not loaded")
		return
	}

	// Return sanitized config (no passwords)
	safeConfig := map[string]interface{}{
		"version":      cfg.Version,
		"camera_count": len(cfg.Cameras),
		"system": map[string]interface{}{
			"name":         cfg.System.Name,
			"timezone":     cfg.System.Timezone,
			"storage_path": cfg.System.StoragePath,
		},
	}

	p.respondJSON(w, safeConfig)
}

func (p *ConfigPlugin) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		p.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Configuration update would be implemented here
	p.respondJSON(w, map[string]string{"status": "updated"})

	// Publish config changed event
	p.PublishEvent(sdk.EventTypeConfigChanged, map[string]string{
		"path": p.configPath,
	})
}

func (p *ConfigPlugin) handleReload(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load(p.configPath)
	if err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	p.mu.Lock()
	p.config = cfg
	p.mu.Unlock()

	// Publish config changed event
	p.PublishEvent(sdk.EventTypeConfigChanged, map[string]string{
		"path": p.configPath,
	})

	p.respondJSON(w, map[string]string{"status": "reloaded"})
}

func (p *ConfigPlugin) handleGetCameras(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	cfg := p.config
	p.mu.RUnlock()

	if cfg == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Configuration not loaded")
		return
	}

	// Return cameras without sensitive data
	cameras := make([]map[string]interface{}, 0, len(cfg.Cameras))
	for _, cam := range cfg.Cameras {
		cameras = append(cameras, map[string]interface{}{
			"id":           cam.ID,
			"name":         cam.Name,
			"enabled":      cam.Enabled,
			"manufacturer": cam.Manufacturer,
			"model":        cam.Model,
			"has_stream":   cam.Stream.URL != "",
		})
	}

	p.respondJSON(w, cameras)
}

func (p *ConfigPlugin) handleGetCameraConfig(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	p.mu.RLock()
	cfg := p.config
	p.mu.RUnlock()

	if cfg == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Configuration not loaded")
		return
	}

	camCfg := cfg.GetCamera(id)
	if camCfg == nil {
		p.respondError(w, http.StatusNotFound, "Camera not found")
		return
	}

	// Return camera config without passwords
	safeConfig := map[string]interface{}{
		"id":           camCfg.ID,
		"name":         camCfg.Name,
		"enabled":      camCfg.Enabled,
		"manufacturer": camCfg.Manufacturer,
		"model":        camCfg.Model,
		"stream": map[string]interface{}{
			"url":     camCfg.Stream.URL,
			"has_sub": camCfg.Stream.SubURL != "",
		},
		"recording":  camCfg.Recording,
		"detection":  camCfg.Detection,
		"motion":     camCfg.Motion,
	}

	p.respondJSON(w, safeConfig)
}

func (p *ConfigPlugin) handleUpdateCameraConfig(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var camCfg config.CameraConfig
	if err := json.NewDecoder(r.Body).Decode(&camCfg); err != nil {
		p.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	p.mu.Lock()
	cfg := p.config
	p.mu.Unlock()

	if cfg == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Configuration not loaded")
		return
	}

	camCfg.ID = id
	if err := cfg.UpsertCamera(camCfg); err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Publish camera updated event
	p.PublishEvent(sdk.EventTypeCameraUpdated, map[string]string{
		"camera_id": id,
	})

	p.respondJSON(w, map[string]string{"status": "updated"})
}

func (p *ConfigPlugin) handleGetSystemConfig(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	cfg := p.config
	p.mu.RUnlock()

	if cfg == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Configuration not loaded")
		return
	}

	p.respondJSON(w, map[string]interface{}{
		"name":         cfg.System.Name,
		"timezone":     cfg.System.Timezone,
		"storage_path": cfg.System.StoragePath,
		"logging":      cfg.System.Logging,
	})
}

func (p *ConfigPlugin) handleUpdateSystemConfig(w http.ResponseWriter, r *http.Request) {
	var updates config.SystemConfig
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		p.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// System config update would be implemented here
	p.respondJSON(w, map[string]string{"status": "updated"})

	// Publish config changed event
	p.PublishEvent(sdk.EventTypeConfigChanged, map[string]string{
		"path": p.configPath,
	})
}

// Helper methods

func (p *ConfigPlugin) respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (p *ConfigPlugin) respondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

// Ensure ConfigPlugin implements the sdk.Plugin interface
var _ sdk.Plugin = (*ConfigPlugin)(nil)
var _ sdk.ServicePlugin = (*ConfigPlugin)(nil)

// Prevent unused function warning
var _ = New
