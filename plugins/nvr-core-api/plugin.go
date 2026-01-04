// Package nvrcoreapi provides the NVR Core API Plugin
// This plugin handles camera CRUD operations and system management
package nvrcoreapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Spatial-NVR/SpatialNVR/internal/camera"
	"github.com/Spatial-NVR/SpatialNVR/internal/config"
	"github.com/Spatial-NVR/SpatialNVR/internal/database"
	"github.com/Spatial-NVR/SpatialNVR/internal/streaming"
	"github.com/Spatial-NVR/SpatialNVR/sdk"
)

// CoreAPIPlugin implements the core API service as a plugin
type CoreAPIPlugin struct {
	sdk.BaseServicePlugin

	cameraService *camera.Service
	config        *config.Config
	db            *database.DB
	go2rtc        *streaming.Go2RTCManager

	configPath  string
	storagePath string

	mu      sync.RWMutex
	started bool
}

// New creates a new CoreAPIPlugin instance
func New() *CoreAPIPlugin {
	p := &CoreAPIPlugin{}
	p.SetManifest(sdk.PluginManifest{
		ID:           "nvr-core-api",
		Name:         "Core API",
		Version:      "1.0.0",
		Description:  "Core camera management and system API",
		Category:     "core",
		Critical:     true,
		Dependencies: []string{},
		Capabilities: []string{
			sdk.CapabilityCamera,
		},
	})
	return p
}

// Initialize sets up the plugin
func (p *CoreAPIPlugin) Initialize(ctx context.Context, runtime *sdk.PluginRuntime) error {
	if err := p.BaseServicePlugin.Initialize(ctx, runtime); err != nil {
		return err
	}

	// Get configuration
	p.configPath = runtime.ConfigString("config_path", "/config/config.yaml")
	p.storagePath = runtime.ConfigString("storage_path", "/data")

	// Load configuration
	cfg, err := config.Load(p.configPath)
	if err != nil {
		// Create minimal config if file not exists
		cfg = &config.Config{
			Version: "1.0.0",
			System: config.SystemConfig{
				StoragePath: p.storagePath,
			},
			Cameras: []config.CameraConfig{},
		}
		// Set the path so saving works
		cfg.SetPath(p.configPath)
	}
	// Override storage path with runtime value (from DATA_PATH env var)
	// This ensures container/env paths take precedence over config file values
	cfg.System.StoragePath = p.storagePath
	p.config = cfg

	// Get database from runtime
	db := runtime.DB()
	if db != nil {
		p.db = &database.DB{DB: db}
	}

	// Create go2rtc manager for camera service
	// Use configured ports (set by main.go from PortConfig)
	go2rtcConfigPath := p.storagePath + "/go2rtc.yaml"
	go2rtcAPIPort := runtime.ConfigInt("go2rtc_api_port", streaming.DefaultGo2RTCPort)
	p.go2rtc = streaming.NewGo2RTCManagerWithPort(go2rtcConfigPath, "", go2rtcAPIPort)

	return nil
}

// Start starts the core API service
func (p *CoreAPIPlugin) Start(ctx context.Context) error {
	runtime := p.Runtime()
	if runtime == nil {
		return fmt.Errorf("plugin not initialized")
	}

	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	// Create camera service
	if p.db != nil {
		p.cameraService = camera.NewService(p.db, p.config, p.go2rtc)
		if err := p.cameraService.Start(ctx); err != nil {
			return fmt.Errorf("failed to start camera service: %w", err)
		}
	}

	// Subscribe to events
	if err := p.subscribeToEvents(); err != nil {
		runtime.Logger().Warn("Failed to subscribe to events", "error", err)
	}

	p.mu.Lock()
	p.started = true
	p.mu.Unlock()

	p.SetHealthy("Core API running")
	runtime.Logger().Info("Core API plugin started")

	// Publish started event
	_ = p.PublishEvent(sdk.EventTypePluginStarted, map[string]string{
		"plugin_id": "nvr-core-api",
	})

	return nil
}

// Stop stops the core API service
func (p *CoreAPIPlugin) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cameraService != nil {
		p.cameraService.Stop()
	}

	p.started = false
	p.SetHealth(sdk.HealthStateUnknown, "Core API stopped")

	return nil
}

// Health returns the plugin's health status
func (p *CoreAPIPlugin) Health() sdk.HealthStatus {
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
		Message:     "Core API operational",
		LastChecked: time.Now(),
	}
}

// Routes returns the HTTP routes for this plugin
// When mounted via gateway.routeToPlugin, the prefix is already stripped
func (p *CoreAPIPlugin) Routes() http.Handler {
	r := chi.NewRouter()

	// Camera routes - mounted at /api/v1/cameras by gateway
	r.Get("/", p.handleListCameras)
	r.Post("/", p.handleCreateCamera)
	r.Get("/{id}", p.handleGetCamera)
	r.Put("/{id}", p.handleUpdateCamera)
	r.Delete("/{id}", p.handleDeleteCamera)
	r.Get("/{id}/config", p.handleGetCameraConfig)
	r.Get("/{id}/snapshot", p.handleGetSnapshot)
	r.Get("/{id}/stream", p.handleGetStreamURLs)

	// Plugin capability routes - for plugin-managed cameras
	r.Get("/{id}/capabilities", p.handleGetCapabilities)
	r.Get("/{id}/ptz/presets", p.handleGetPTZPresets)
	r.Post("/{id}/ptz/control", p.handlePTZControl)
	r.Get("/{id}/protocols", p.handleGetProtocols)
	r.Put("/{id}/protocol", p.handleSetProtocol)
	r.Get("/{id}/device-info", p.handleGetDeviceInfo)

	// System routes - these won't be accessible via /cameras route
	// They're accessible via /api/v1/plugins/nvr-core-api/system/...
	r.Get("/system/info", p.handleSystemInfo)
	r.Get("/system/config", p.handleGetConfig)

	return r
}

// EventSubscriptions returns events this plugin subscribes to
func (p *CoreAPIPlugin) EventSubscriptions() []string {
	return []string{
		sdk.EventTypeConfigChanged,
	}
}

// HandleEvent processes incoming events
func (p *CoreAPIPlugin) HandleEvent(ctx context.Context, event *sdk.Event) error {
	switch event.Type {
	case sdk.EventTypeConfigChanged:
		// Reload configuration
		cfg, err := config.Load(p.configPath)
		if err == nil {
			p.config = cfg
		}
	}
	return nil
}

// Private methods

func (p *CoreAPIPlugin) subscribeToEvents() error {
	runtime := p.Runtime()
	if runtime == nil {
		return nil
	}

	return runtime.SubscribeEvents(func(event *sdk.Event) {
		ctx := context.Background()
		if err := p.HandleEvent(ctx, event); err != nil {
			runtime.Logger().Error("Failed to handle event", "type", event.Type, "error", err)
		}
	}, p.EventSubscriptions()...)
}

// HTTP Handlers

func (p *CoreAPIPlugin) handleListCameras(w http.ResponseWriter, r *http.Request) {
	if p.cameraService == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Camera service not available")
		return
	}

	cameras, err := p.cameraService.List(r.Context())
	if err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	p.respondJSON(w, cameras)
}

func (p *CoreAPIPlugin) handleCreateCamera(w http.ResponseWriter, r *http.Request) {
	if p.cameraService == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Camera service not available")
		return
	}

	var camCfg config.CameraConfig
	if err := json.NewDecoder(r.Body).Decode(&camCfg); err != nil {
		p.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	cam, err := p.cameraService.Create(r.Context(), camCfg)
	if err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Publish camera added event with full config for other plugins
	_ = p.PublishEvent(sdk.EventTypeCameraAdded, map[string]interface{}{
		"camera_id":   cam.ID,
		"name":        cam.Name,
		"main_stream": cam.StreamURL,
		"config":      camCfg, // Include full config for recording plugin
	})

	w.WriteHeader(http.StatusCreated)
	p.respondJSON(w, cam)
}

func (p *CoreAPIPlugin) handleGetCamera(w http.ResponseWriter, r *http.Request) {
	if p.cameraService == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Camera service not available")
		return
	}

	id := chi.URLParam(r, "id")
	cam, err := p.cameraService.Get(r.Context(), id)
	if err != nil {
		p.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	p.respondJSON(w, cam)
}

func (p *CoreAPIPlugin) handleUpdateCamera(w http.ResponseWriter, r *http.Request) {
	if p.cameraService == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Camera service not available")
		return
	}

	id := chi.URLParam(r, "id")

	// Read body into bytes so we can decode twice
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		p.respondError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	// First, decode into a map to track which fields were present
	var rawFields map[string]json.RawMessage
	if err := json.Unmarshal(bodyBytes, &rawFields); err != nil {
		p.respondError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Then decode into the proper struct
	var camCfg config.CameraConfig
	if err := json.Unmarshal(bodyBytes, &camCfg); err != nil {
		p.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	cam, err := p.cameraService.UpdateWithFields(r.Context(), id, camCfg, rawFields)
	if err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Publish camera updated event with full config for other plugins
	_ = p.PublishEvent(sdk.EventTypeCameraUpdated, map[string]interface{}{
		"camera_id":   cam.ID,
		"name":        cam.Name,
		"main_stream": cam.StreamURL,
		"config":      camCfg, // Include full config for recording plugin
	})

	p.respondJSON(w, cam)
}

func (p *CoreAPIPlugin) handleDeleteCamera(w http.ResponseWriter, r *http.Request) {
	if p.cameraService == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Camera service not available")
		return
	}

	id := chi.URLParam(r, "id")

	if err := p.cameraService.Delete(r.Context(), id); err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Publish camera removed event
	_ = p.PublishEvent(sdk.EventTypeCameraRemoved, map[string]string{
		"camera_id": id,
	})

	w.WriteHeader(http.StatusNoContent)
}

func (p *CoreAPIPlugin) handleGetCameraConfig(w http.ResponseWriter, r *http.Request) {
	if p.cameraService == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Camera service not available")
		return
	}

	id := chi.URLParam(r, "id")
	cfg, err := p.cameraService.GetConfig(r.Context(), id)
	if err != nil {
		p.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	// Build stream config with roles
	streamResponse := map[string]interface{}{
		"url":          cfg.Stream.URL,
		"sub_url":      cfg.Stream.SubURL,
		"username":     cfg.Stream.Username,
		"has_password": cfg.Stream.Password != "",
	}
	// Include roles if configured
	if cfg.Stream.Roles != nil {
		streamResponse["roles"] = cfg.Stream.Roles
	} else {
		// Default roles if not set
		streamResponse["roles"] = map[string]string{
			"detect": "sub",
			"record": "main",
			"audio":  "main",
			"motion": "sub",
		}
	}

	// Create a sanitized response (hide password)
	response := map[string]interface{}{
		"id":                   cfg.ID,
		"name":                 cfg.Name,
		"enabled":              cfg.Enabled,
		"manufacturer":         cfg.Manufacturer,
		"model":                cfg.Model,
		"display_aspect_ratio": cfg.DisplayAspectRatio,
		"stream":               streamResponse,
		"recording":            cfg.Recording,
		"detection":            cfg.Detection,
		"motion":               cfg.Motion,
		"audio":                cfg.Audio,
		"ptz":                  cfg.PTZ,
		"advanced":             cfg.Advanced,
		"location":             cfg.Location,
	}

	p.respondJSON(w, response)
}

func (p *CoreAPIPlugin) handleGetSnapshot(w http.ResponseWriter, r *http.Request) {
	if p.cameraService == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Camera service not available")
		return
	}

	id := chi.URLParam(r, "id")

	data, err := p.cameraService.GetSnapshot(r.Context(), id)
	if err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	_, _ = w.Write(data)
}

func (p *CoreAPIPlugin) handleGetStreamURLs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	p.respondJSON(w, map[string]interface{}{
		"camera_id": id,
		"webrtc":    streaming.GetStreamURL(id, "webrtc", streaming.DefaultGo2RTCPort),
		"rtsp":      streaming.GetStreamURL(id, "rtsp", streaming.DefaultGo2RTCPort),
		"hls":       streaming.GetStreamURL(id, "hls", streaming.DefaultGo2RTCPort),
		"mse":       streaming.GetStreamURL(id, "mse", streaming.DefaultGo2RTCPort),
	})
}

func (p *CoreAPIPlugin) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	p.respondJSON(w, map[string]interface{}{
		"name":         "NVR System",
		"version":      "0.2.1",
		"architecture": "plugin-based",
		"mode":         "production",
	})
}

func (p *CoreAPIPlugin) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	if p.config == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Configuration not available")
		return
	}

	// Return sanitized config (no passwords)
	safeConfig := map[string]interface{}{
		"camera_count": len(p.config.Cameras),
		"system": map[string]interface{}{
			"storage_path": p.config.System.StoragePath,
		},
	}

	p.respondJSON(w, safeConfig)
}

// Plugin capability handlers

func (p *CoreAPIPlugin) handleGetCapabilities(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Get camera config to check for plugin association
	cfg := p.config.GetCamera(id)
	if cfg == nil {
		p.respondError(w, http.StatusNotFound, "Camera not found")
		return
	}

	// If camera has a plugin_id, capabilities come from the plugin
	// For now, return defaults. Frontend can call plugin RPC directly for plugin-managed cameras.
	if cfg.PluginID != "" {
		// Return a response indicating this is a plugin-managed camera
		// The frontend should call the plugin's get_capabilities RPC
		p.respondJSON(w, map[string]interface{}{
			"plugin_id":        cfg.PluginID,
			"plugin_camera_id": cfg.PluginCamID,
			"has_ptz":          cfg.PTZ.Enabled,
			"has_audio":        cfg.Audio.Enabled,
			"has_two_way_audio": cfg.Audio.TwoWay,
			"has_snapshot":     true,
			"device_type":      "camera",
			"protocols":        []string{"source"}, // Plugin will provide actual protocols
			"current_protocol": "source",
			"is_plugin_managed": true,
		})
		return
	}

	// Manual camera - return basic capabilities from config
	p.respondJSON(w, map[string]interface{}{
		"has_ptz":          cfg.PTZ.Enabled,
		"has_audio":        cfg.Audio.Enabled,
		"has_two_way_audio": cfg.Audio.TwoWay,
		"has_snapshot":     true,
		"device_type":      "camera",
		"is_doorbell":      false,
		"is_nvr":           false,
		"is_battery":       false,
		"has_ai_detection": cfg.Detection.Enabled,
		"ai_types":         cfg.Detection.Models,
		"protocols":        []string{"source"},
		"current_protocol": "source",
		"is_plugin_managed": false,
	})
}

func (p *CoreAPIPlugin) handleGetPTZPresets(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	cfg := p.config.GetCamera(id)
	if cfg == nil {
		p.respondError(w, http.StatusNotFound, "Camera not found")
		return
	}

	// If plugin-managed, frontend should call plugin RPC directly
	if cfg.PluginID != "" {
		p.respondJSON(w, map[string]interface{}{
			"plugin_id":        cfg.PluginID,
			"plugin_camera_id": cfg.PluginCamID,
			"presets":          []interface{}{}, // Plugin will provide
		})
		return
	}

	// Manual camera - return presets from config
	var presets []map[string]interface{}
	for _, preset := range cfg.PTZ.Presets {
		presets = append(presets, map[string]interface{}{
			"id":   preset.ID,
			"name": preset.Name,
		})
	}

	p.respondJSON(w, presets)
}

func (p *CoreAPIPlugin) handlePTZControl(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	cfg := p.config.GetCamera(id)
	if cfg == nil {
		p.respondError(w, http.StatusNotFound, "Camera not found")
		return
	}

	// PTZ control requires a plugin for the actual camera control
	if cfg.PluginID == "" {
		p.respondError(w, http.StatusBadRequest, "PTZ control requires a plugin-managed camera")
		return
	}

	// For plugin cameras, frontend should call plugin RPC directly
	// Return plugin info so frontend knows which plugin to call
	p.respondJSON(w, map[string]interface{}{
		"plugin_id":        cfg.PluginID,
		"plugin_camera_id": cfg.PluginCamID,
		"message":          "Use plugin RPC endpoint for PTZ control",
	})
}

func (p *CoreAPIPlugin) handleGetProtocols(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	cfg := p.config.GetCamera(id)
	if cfg == nil {
		p.respondError(w, http.StatusNotFound, "Camera not found")
		return
	}

	if cfg.PluginID != "" {
		// Plugin camera - protocols come from plugin
		p.respondJSON(w, map[string]interface{}{
			"plugin_id":        cfg.PluginID,
			"plugin_camera_id": cfg.PluginCamID,
			"protocols":        []interface{}{}, // Plugin will provide
		})
		return
	}

	// Manual camera - only has the configured source
	p.respondJSON(w, []map[string]interface{}{
		{
			"id":          "source",
			"name":        "Source Stream",
			"description": "Original stream URL",
			"stream_url":  cfg.Stream.URL,
		},
	})
}

func (p *CoreAPIPlugin) handleSetProtocol(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	cfg := p.config.GetCamera(id)
	if cfg == nil {
		p.respondError(w, http.StatusNotFound, "Camera not found")
		return
	}

	if cfg.PluginID == "" {
		p.respondError(w, http.StatusBadRequest, "Protocol switching requires a plugin-managed camera")
		return
	}

	// For plugin cameras, frontend should call plugin RPC directly
	p.respondJSON(w, map[string]interface{}{
		"plugin_id":        cfg.PluginID,
		"plugin_camera_id": cfg.PluginCamID,
		"message":          "Use plugin RPC endpoint to change protocol",
	})
}

func (p *CoreAPIPlugin) handleGetDeviceInfo(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	cfg := p.config.GetCamera(id)
	if cfg == nil {
		p.respondError(w, http.StatusNotFound, "Camera not found")
		return
	}

	if cfg.PluginID != "" {
		// Plugin camera - device info comes from plugin
		p.respondJSON(w, map[string]interface{}{
			"plugin_id":        cfg.PluginID,
			"plugin_camera_id": cfg.PluginCamID,
			"model":            cfg.Model,
			"manufacturer":     cfg.Manufacturer,
		})
		return
	}

	// Manual camera - return what we know from config
	p.respondJSON(w, map[string]interface{}{
		"model":         cfg.Model,
		"manufacturer":  cfg.Manufacturer,
		"channel_count": 1,
	})
}

// Helper methods

func (p *CoreAPIPlugin) respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (p *CoreAPIPlugin) respondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

// Ensure CoreAPIPlugin implements the sdk.Plugin interface
var _ sdk.Plugin = (*CoreAPIPlugin)(nil)
var _ sdk.ServicePlugin = (*CoreAPIPlugin)(nil)

// Prevent unused function warning
var _ = New
