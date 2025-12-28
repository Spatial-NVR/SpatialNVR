// Package nvrcoreconfig provides the NVR Core Configuration Plugin
// This plugin handles system configuration management with hot-reload
package nvrcoreconfig

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
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
		// IMPORTANT: Set the path so Save() knows where to write
		cfg.SetPath(p.configPath)
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
			_ = p.PublishEvent(sdk.EventTypeConfigChanged, map[string]string{
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

	// Return full config (no passwords) matching what the frontend expects
	safeConfig := map[string]interface{}{
		"version":      cfg.Version,
		"camera_count": len(cfg.Cameras),
		"system": map[string]interface{}{
			"name":           cfg.System.Name,
			"timezone":       cfg.System.Timezone,
			"storage_path":   cfg.System.StoragePath,
			"max_storage_gb": cfg.System.MaxStorageGB,
			"updates": map[string]interface{}{
				"github_token": maskToken(cfg.System.Updates.GitHubToken),
			},
		},
		"storage": map[string]interface{}{
			"retention": map[string]interface{}{
				"default_days": cfg.Storage.Retention.DefaultDays,
			},
		},
		"detection": map[string]interface{}{
			"backend": cfg.Detectors.YOLO12.Type,
			"fps":     5, // Default detection FPS
			"objects": map[string]interface{}{
				"enabled":    cfg.Detectors.YOLO12.Model != "",
				"model":      cfg.Detectors.YOLO12.Model,
				"confidence": cfg.Detectors.YOLO12.Confidence,
				"classes":    cfg.Detectors.YOLO12.Classes,
			},
			"faces": map[string]interface{}{
				"enabled":    cfg.Detectors.FaceRecognition.Model != "",
				"model":      cfg.Detectors.FaceRecognition.Model,
				"confidence": cfg.Detectors.FaceRecognition.Confidence,
			},
			"lpr": map[string]interface{}{
				"enabled":    cfg.Detectors.LPR.Type != "",
				"model":      cfg.Detectors.LPR.Type,
				"confidence": cfg.Detectors.LPR.Confidence,
			},
		},
		"preferences": cfg.Preferences,
	}

	p.respondJSON(w, safeConfig)
}

func (p *ConfigPlugin) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		p.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	p.mu.Lock()
	cfg := p.config
	if cfg == nil {
		p.mu.Unlock()
		p.respondError(w, http.StatusServiceUnavailable, "Configuration not loaded")
		return
	}

	// Apply system updates
	if system, ok := updates["system"].(map[string]interface{}); ok {
		if name, ok := system["name"].(string); ok {
			cfg.System.Name = name
		}
		if tz, ok := system["timezone"].(string); ok {
			cfg.System.Timezone = tz
		}
		if maxStorage, ok := system["max_storage_gb"].(float64); ok {
			cfg.System.MaxStorageGB = int(maxStorage)
		}
		// Handle updates.github_token
		if updatesConfig, ok := system["updates"].(map[string]interface{}); ok {
			if token, ok := updatesConfig["github_token"].(string); ok {
				// Only update if it's a new token (not the masked version)
				if token != "" && !strings.Contains(token, "****") {
					cfg.System.Updates.GitHubToken = token
				}
			}
		}
	}

	// Apply storage updates
	if storage, ok := updates["storage"].(map[string]interface{}); ok {
		if retention, ok := storage["retention"].(map[string]interface{}); ok {
			if days, ok := retention["default_days"].(float64); ok {
				cfg.Storage.Retention.DefaultDays = int(days)
			}
		}
	}

	// Apply detection updates
	if detection, ok := updates["detection"].(map[string]interface{}); ok {
		if backend, ok := detection["backend"].(string); ok {
			cfg.Detectors.YOLO12.Type = backend
		}
		if objects, ok := detection["objects"].(map[string]interface{}); ok {
			if model, ok := objects["model"].(string); ok {
				cfg.Detectors.YOLO12.Model = model
			}
			if conf, ok := objects["confidence"].(float64); ok {
				cfg.Detectors.YOLO12.Confidence = conf
			}
			if classes, ok := objects["classes"].([]interface{}); ok {
				cfg.Detectors.YOLO12.Classes = make([]string, 0, len(classes))
				for _, c := range classes {
					if s, ok := c.(string); ok {
						cfg.Detectors.YOLO12.Classes = append(cfg.Detectors.YOLO12.Classes, s)
					}
				}
			}
		}
		if faces, ok := detection["faces"].(map[string]interface{}); ok {
			if model, ok := faces["model"].(string); ok {
				cfg.Detectors.FaceRecognition.Model = model
			}
			if conf, ok := faces["confidence"].(float64); ok {
				cfg.Detectors.FaceRecognition.Confidence = conf
			}
		}
		if lpr, ok := detection["lpr"].(map[string]interface{}); ok {
			if model, ok := lpr["model"].(string); ok {
				cfg.Detectors.LPR.Type = model
			}
			if conf, ok := lpr["confidence"].(float64); ok {
				cfg.Detectors.LPR.Confidence = conf
			}
		}
	}

	// Apply preferences updates
	if prefs, ok := updates["preferences"].(map[string]interface{}); ok {
		if ui, ok := prefs["ui"].(map[string]interface{}); ok {
			if theme, ok := ui["theme"].(string); ok {
				cfg.Preferences.UI.Theme = theme
			}
			if lang, ok := ui["language"].(string); ok {
				cfg.Preferences.UI.Language = lang
			}
			if dash, ok := ui["dashboard"].(map[string]interface{}); ok {
				if cols, ok := dash["grid_columns"].(float64); ok {
					cfg.Preferences.UI.Dashboard.GridColumns = int(cols)
				}
				if showFps, ok := dash["show_fps"].(bool); ok {
					cfg.Preferences.UI.Dashboard.ShowFPS = showFps
				}
			}
		}
		if timeline, ok := prefs["timeline"].(map[string]interface{}); ok {
			if hours, ok := timeline["default_range_hours"].(float64); ok {
				cfg.Preferences.Timeline.DefaultRangeHours = int(hours)
			}
			if interval, ok := timeline["thumbnail_interval_seconds"].(float64); ok {
				cfg.Preferences.Timeline.ThumbnailIntervalSeconds = int(interval)
			}
		}
		if events, ok := prefs["events"].(map[string]interface{}); ok {
			if days, ok := events["auto_acknowledge_after_days"].(float64); ok {
				cfg.Preferences.Events.AutoAcknowledgeAfterDays = int(days)
			}
			if group, ok := events["group_similar_events"].(bool); ok {
				cfg.Preferences.Events.GroupSimilarEvents = group
			}
			if window, ok := events["group_window_seconds"].(float64); ok {
				cfg.Preferences.Events.GroupWindowSeconds = int(window)
			}
		}
	}

	p.mu.Unlock()

	// Save configuration to disk
	if err := cfg.Save(); err != nil {
		p.respondError(w, http.StatusInternalServerError, "Failed to save configuration: "+err.Error())
		return
	}

	p.respondJSON(w, map[string]string{"status": "updated"})

	// Publish config changed event
	_ = p.PublishEvent(sdk.EventTypeConfigChanged, map[string]string{
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
	_ = p.PublishEvent(sdk.EventTypeConfigChanged, map[string]string{
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
	_ = p.PublishEvent(sdk.EventTypeCameraUpdated, map[string]string{
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
	_ = p.PublishEvent(sdk.EventTypeConfigChanged, map[string]string{
		"path": p.configPath,
	})
}

// Helper methods

func (p *ConfigPlugin) respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (p *ConfigPlugin) respondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

// maskToken returns a masked version of the token for display (shows first 4 and last 4 chars)
func maskToken(token string) string {
	if token == "" {
		return ""
	}
	if len(token) <= 8 {
		return "********"
	}
	return token[:4] + "****" + token[len(token)-4:]
}

// Ensure ConfigPlugin implements the sdk.Plugin interface
var _ sdk.Plugin = (*ConfigPlugin)(nil)
var _ sdk.ServicePlugin = (*ConfigPlugin)(nil)

// Prevent unused function warning
var _ = New
