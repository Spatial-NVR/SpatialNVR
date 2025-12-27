// Package nvrstreaming provides the NVR Streaming Plugin
// This plugin manages go2rtc for video streaming (WebRTC, RTSP, HLS)
package nvrstreaming

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Spatial-NVR/SpatialNVR/internal/streaming"
	"github.com/Spatial-NVR/SpatialNVR/sdk"
)

// StreamingPlugin implements the streaming service as a plugin
type StreamingPlugin struct {
	sdk.BaseServicePlugin

	manager    *streaming.Go2RTCManager
	generator  *streaming.ConfigGenerator
	configPath string
	binaryPath string
	apiPort    int
	rtspPort   int
	webrtcPort int

	// Track streams for cameras
	streams map[string]streaming.CameraStream

	mu      sync.RWMutex
	started bool
}

// New creates a new StreamingPlugin instance
func New() *StreamingPlugin {
	p := &StreamingPlugin{
		streams: make(map[string]streaming.CameraStream),
	}
	p.SetManifest(sdk.PluginManifest{
		ID:          "nvr-streaming",
		Name:        "Streaming Service",
		Version:     "1.0.0",
		Description: "Core video streaming service using go2rtc",
		Category:    "core",
		Critical:    true,
		Dependencies: []string{},
		Capabilities: []string{
			sdk.CapabilityStreaming,
			sdk.CapabilityTwoWayAudio,
		},
	})
	return p
}

// Initialize sets up the plugin
func (p *StreamingPlugin) Initialize(ctx context.Context, runtime *sdk.PluginRuntime) error {
	if err := p.BaseServicePlugin.Initialize(ctx, runtime); err != nil {
		return err
	}

	// Get configuration
	p.configPath = runtime.ConfigString("config_path", "/data/go2rtc.yaml")
	p.binaryPath = runtime.ConfigString("binary_path", "")
	p.apiPort = runtime.ConfigInt("api_port", streaming.DefaultGo2RTCPort)
	p.rtspPort = runtime.ConfigInt("rtsp_port", streaming.DefaultRTSPPort)
	p.webrtcPort = runtime.ConfigInt("webrtc_port", streaming.DefaultWebRTCPort)

	// Create config generator with ports
	p.generator = streaming.NewConfigGenerator().WithPorts(p.apiPort, p.rtspPort, p.webrtcPort)

	// Create go2rtc manager with the correct API port
	p.manager = streaming.NewGo2RTCManagerWithPort(p.configPath, p.binaryPath, p.apiPort)

	return nil
}

// Start starts the streaming service
func (p *StreamingPlugin) Start(ctx context.Context) error {
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

	// Generate initial config (empty streams)
	p.mu.RLock()
	streams := make([]streaming.CameraStream, 0, len(p.streams))
	for _, s := range p.streams {
		streams = append(streams, s)
	}
	p.mu.RUnlock()

	config := p.generator.Generate(streams)
	if err := p.generator.WriteToFile(config, p.configPath); err != nil {
		return fmt.Errorf("failed to write go2rtc config: %w", err)
	}

	// Start go2rtc
	if err := p.manager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start go2rtc: %w", err)
	}

	// Subscribe to events
	if err := p.subscribeToEvents(); err != nil {
		runtime.Logger().Warn("Failed to subscribe to events", "error", err)
	}

	p.mu.Lock()
	p.started = true
	p.mu.Unlock()

	p.SetHealthy("Streaming service running")
	runtime.Logger().Info("Streaming plugin started",
		"api_port", p.apiPort,
		"rtsp_port", p.rtspPort,
		"webrtc_port", p.webrtcPort)

	// Sync existing cameras from database
	go p.syncExistingCameras(ctx)

	// Publish started event
	p.PublishEvent(sdk.EventTypePluginStarted, map[string]string{
		"plugin_id": "nvr-streaming",
	})

	return nil
}

// syncExistingCameras loads cameras from the API and adds their streams
func (p *StreamingPlugin) syncExistingCameras(ctx context.Context) {
	runtime := p.Runtime()
	if runtime == nil {
		return
	}

	// Wait for API and go2rtc to be fully ready
	time.Sleep(1 * time.Second)

	// Fetch cameras from local API
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://localhost:8080/api/v1/cameras")
	if err != nil {
		runtime.Logger().Error("Failed to fetch cameras for sync", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		runtime.Logger().Error("Failed to fetch cameras", "status", resp.StatusCode)
		return
	}

	var cameras []struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		StreamURL string `json:"stream_url"`
		Enabled   bool   `json:"enabled"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&cameras); err != nil {
		runtime.Logger().Error("Failed to decode cameras response", "error", err)
		return
	}

	syncCount := 0
	for _, cam := range cameras {
		if !cam.Enabled || cam.StreamURL == "" {
			continue
		}

		// Add to go2rtc
		if err := p.AddStream(cam.ID, cam.Name, cam.StreamURL, "", "", ""); err != nil {
			runtime.Logger().Warn("Failed to sync camera stream", "camera", cam.ID, "error", err)
		} else {
			syncCount++
			runtime.Logger().Info("Synced camera stream", "camera", cam.ID, "name", cam.Name)
		}
	}

	if syncCount > 0 {
		runtime.Logger().Info("Synced existing cameras to go2rtc", "count", syncCount)
	}
}

// Stop stops the streaming service
func (p *StreamingPlugin) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.manager != nil {
		if err := p.manager.Stop(); err != nil {
			return err
		}
	}

	p.started = false
	p.SetHealth(sdk.HealthStateUnknown, "Streaming service stopped")

	return nil
}

// Health returns the plugin's health status
func (p *StreamingPlugin) Health() sdk.HealthStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.started {
		return sdk.HealthStatus{
			State:       sdk.HealthStateUnknown,
			Message:     "Not started",
			LastChecked: time.Now(),
		}
	}

	// Check if go2rtc is running
	if p.manager == nil || !p.manager.IsRunning() {
		return sdk.HealthStatus{
			State:       sdk.HealthStateUnhealthy,
			Message:     "go2rtc not running",
			LastChecked: time.Now(),
		}
	}

	return sdk.HealthStatus{
		State:       sdk.HealthStateHealthy,
		Message:     "Streaming service operational",
		LastChecked: time.Now(),
		Details: map[string]string{
			"api_url":      p.manager.GetAPIURL(),
			"stream_count": fmt.Sprintf("%d", len(p.streams)),
		},
	}
}

// Routes returns the HTTP routes for this plugin
func (p *StreamingPlugin) Routes() http.Handler {
	r := chi.NewRouter()

	// Stream management
	r.Get("/streams", p.handleListStreams)
	r.Post("/streams", p.handleAddStream)
	r.Get("/streams/{name}", p.handleGetStream)
	r.Delete("/streams/{name}", p.handleRemoveStream)

	// Stream URLs
	r.Get("/streams/{name}/url", p.handleGetStreamURL)
	r.Get("/streams/{name}/webrtc", p.handleGetWebRTCURL)
	r.Get("/streams/{name}/rtsp", p.handleGetRTSPURL)
	r.Get("/streams/{name}/hls", p.handleGetHLSURL)

	// go2rtc management
	r.Get("/status", p.handleGetStatus)
	r.Post("/reload", p.handleReload)
	r.Post("/restart", p.handleRestart)

	// Proxy to go2rtc API (for direct access)
	r.Get("/go2rtc/*", p.handleGo2RTCProxy)

	return r
}

// EventSubscriptions returns events this plugin subscribes to
func (p *StreamingPlugin) EventSubscriptions() []string {
	return []string{
		sdk.EventTypeCameraAdded,
		sdk.EventTypeCameraRemoved,
		sdk.EventTypeCameraUpdated,
		sdk.EventTypeConfigChanged,
	}
}

// HandleEvent processes incoming events
func (p *StreamingPlugin) HandleEvent(ctx context.Context, event *sdk.Event) error {
	switch event.Type {
	case sdk.EventTypeCameraAdded:
		return p.handleCameraAdded(event)

	case sdk.EventTypeCameraRemoved:
		return p.handleCameraRemoved(event)

	case sdk.EventTypeCameraUpdated:
		return p.handleCameraUpdated(event)
	}

	return nil
}

// OnConfigChange handles configuration changes
func (p *StreamingPlugin) OnConfigChange(config map[string]interface{}) {
	needsRestart := false

	if apiPort, ok := config["api_port"].(int); ok && apiPort != p.apiPort {
		p.apiPort = apiPort
		needsRestart = true
	}
	if rtspPort, ok := config["rtsp_port"].(int); ok && rtspPort != p.rtspPort {
		p.rtspPort = rtspPort
		needsRestart = true
	}
	if webrtcPort, ok := config["webrtc_port"].(int); ok && webrtcPort != p.webrtcPort {
		p.webrtcPort = webrtcPort
		needsRestart = true
	}

	if needsRestart {
		p.generator = streaming.NewConfigGenerator().WithPorts(p.apiPort, p.rtspPort, p.webrtcPort)
		go p.regenerateConfigAndReload()
	}
}

// AddStream adds a camera stream
func (p *StreamingPlugin) AddStream(id, name, url, username, password, subURL string) error {
	p.mu.Lock()
	p.streams[id] = streaming.CameraStream{
		ID:       id,
		Name:     name,
		URL:      url,
		Username: username,
		Password: password,
		SubURL:   subURL,
	}
	p.mu.Unlock()

	return p.regenerateConfigAndReload()
}

// RemoveStream removes a camera stream
func (p *StreamingPlugin) RemoveStream(id string) error {
	p.mu.Lock()
	delete(p.streams, id)
	p.mu.Unlock()

	return p.regenerateConfigAndReload()
}

// GetStreamURL returns the URL for a stream
func (p *StreamingPlugin) GetStreamURL(cameraID, format string) string {
	return streaming.GetStreamURL(cameraID, format, p.apiPort)
}

// GetWebRTCURL returns the WebRTC WebSocket URL for a camera
func (p *StreamingPlugin) GetWebRTCURL(cameraID string) string {
	return p.manager.GetWebRTCURL(cameraID)
}

// Private methods

func (p *StreamingPlugin) subscribeToEvents() error {
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

func (p *StreamingPlugin) handleCameraAdded(event *sdk.Event) error {
	cameraID, _ := event.Data["camera_id"].(string)
	name, _ := event.Data["name"].(string)
	mainStream, _ := event.Data["main_stream"].(string)
	subStream, _ := event.Data["sub_stream"].(string)
	username, _ := event.Data["username"].(string)
	password, _ := event.Data["password"].(string)

	if cameraID == "" || mainStream == "" {
		return nil
	}

	return p.AddStream(cameraID, name, mainStream, username, password, subStream)
}

func (p *StreamingPlugin) handleCameraRemoved(event *sdk.Event) error {
	cameraID, _ := event.Data["camera_id"].(string)
	if cameraID == "" {
		return nil
	}

	return p.RemoveStream(cameraID)
}

func (p *StreamingPlugin) handleCameraUpdated(event *sdk.Event) error {
	// Remove and re-add with updated info
	if err := p.handleCameraRemoved(event); err != nil {
		return err
	}
	return p.handleCameraAdded(event)
}

func (p *StreamingPlugin) regenerateConfigAndReload() error {
	p.mu.RLock()
	streams := make([]streaming.CameraStream, 0, len(p.streams))
	for _, s := range p.streams {
		streams = append(streams, s)
	}
	p.mu.RUnlock()

	config := p.generator.Generate(streams)
	if err := p.generator.WriteToFile(config, p.configPath); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return p.manager.Reload()
}

// HTTP Handlers

func (p *StreamingPlugin) handleListStreams(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	streams := make([]map[string]interface{}, 0, len(p.streams))
	for _, s := range p.streams {
		streams = append(streams, map[string]interface{}{
			"id":          s.ID,
			"name":        s.Name,
			"has_sub":     s.SubURL != "",
			"webrtc_url":  p.manager.GetWebRTCURL(s.ID),
			"rtsp_url":    streaming.GetStreamURL(s.ID, "rtsp", p.apiPort),
			"hls_url":     streaming.GetStreamURL(s.ID, "hls", p.apiPort),
		})
	}
	p.mu.RUnlock()

	p.respondJSON(w, map[string]interface{}{
		"streams": streams,
		"count":   len(streams),
	})
}

func (p *StreamingPlugin) handleAddStream(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		URL      string `json:"url"`
		Username string `json:"username"`
		Password string `json:"password"`
		SubURL   string `json:"sub_url"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		p.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.ID == "" || req.URL == "" {
		p.respondError(w, http.StatusBadRequest, "id and url are required")
		return
	}

	if err := p.AddStream(req.ID, req.Name, req.URL, req.Username, req.Password, req.SubURL); err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	p.respondJSON(w, map[string]interface{}{
		"id":         req.ID,
		"status":     "added",
		"webrtc_url": p.manager.GetWebRTCURL(req.ID),
	})
}

func (p *StreamingPlugin) handleGetStream(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	p.mu.RLock()
	stream, exists := p.streams[name]
	p.mu.RUnlock()

	if !exists {
		p.respondError(w, http.StatusNotFound, "Stream not found")
		return
	}

	// Get stream info from go2rtc
	info, _ := p.manager.GetStreamInfo(name)

	p.respondJSON(w, map[string]interface{}{
		"id":          stream.ID,
		"name":        stream.Name,
		"has_sub":     stream.SubURL != "",
		"webrtc_url":  p.manager.GetWebRTCURL(stream.ID),
		"rtsp_url":    streaming.GetStreamURL(stream.ID, "rtsp", p.apiPort),
		"hls_url":     streaming.GetStreamURL(stream.ID, "hls", p.apiPort),
		"info":        info,
	})
}

func (p *StreamingPlugin) handleRemoveStream(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	p.mu.RLock()
	_, exists := p.streams[name]
	p.mu.RUnlock()

	if !exists {
		p.respondError(w, http.StatusNotFound, "Stream not found")
		return
	}

	if err := p.RemoveStream(name); err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (p *StreamingPlugin) handleGetStreamURL(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "hls"
	}

	p.mu.RLock()
	_, exists := p.streams[name]
	p.mu.RUnlock()

	if !exists {
		p.respondError(w, http.StatusNotFound, "Stream not found")
		return
	}

	p.respondJSON(w, map[string]interface{}{
		"stream_id": name,
		"format":    format,
		"url":       streaming.GetStreamURL(name, format, p.apiPort),
	})
}

func (p *StreamingPlugin) handleGetWebRTCURL(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	p.mu.RLock()
	_, exists := p.streams[name]
	p.mu.RUnlock()

	if !exists {
		p.respondError(w, http.StatusNotFound, "Stream not found")
		return
	}

	// Check if backchannel (two-way audio) is requested
	backchannel := r.URL.Query().Get("backchannel") == "true"

	var url string
	if backchannel {
		url = p.manager.GetBackchannelURL(name)
	} else {
		url = p.manager.GetWebRTCURL(name)
	}

	p.respondJSON(w, map[string]interface{}{
		"stream_id":   name,
		"url":         url,
		"backchannel": backchannel,
	})
}

func (p *StreamingPlugin) handleGetRTSPURL(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	p.mu.RLock()
	_, exists := p.streams[name]
	p.mu.RUnlock()

	if !exists {
		p.respondError(w, http.StatusNotFound, "Stream not found")
		return
	}

	p.respondJSON(w, map[string]interface{}{
		"stream_id": name,
		"url":       streaming.GetStreamURL(name, "rtsp", p.apiPort),
	})
}

func (p *StreamingPlugin) handleGetHLSURL(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	p.mu.RLock()
	_, exists := p.streams[name]
	p.mu.RUnlock()

	if !exists {
		p.respondError(w, http.StatusNotFound, "Stream not found")
		return
	}

	p.respondJSON(w, map[string]interface{}{
		"stream_id": name,
		"url":       streaming.GetStreamURL(name, "hls", p.apiPort),
	})
}

func (p *StreamingPlugin) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	started := p.started
	streamCount := len(p.streams)
	p.mu.RUnlock()

	running := false
	apiURL := ""
	if p.manager != nil {
		running = p.manager.IsRunning()
		apiURL = p.manager.GetAPIURL()
	}

	p.respondJSON(w, map[string]interface{}{
		"started":      started,
		"running":      running,
		"api_url":      apiURL,
		"api_port":     p.apiPort,
		"rtsp_port":    p.rtspPort,
		"webrtc_port":  p.webrtcPort,
		"stream_count": streamCount,
	})
}

func (p *StreamingPlugin) handleReload(w http.ResponseWriter, r *http.Request) {
	if err := p.regenerateConfigAndReload(); err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	p.respondJSON(w, map[string]string{
		"status": "reloaded",
	})
}

func (p *StreamingPlugin) handleRestart(w http.ResponseWriter, r *http.Request) {
	if p.manager == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Manager not available")
		return
	}

	if err := p.manager.Restart(); err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	p.respondJSON(w, map[string]string{
		"status": "restarted",
	})
}

func (p *StreamingPlugin) handleGo2RTCProxy(w http.ResponseWriter, r *http.Request) {
	// Proxy requests to go2rtc API
	if p.manager == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Manager not available")
		return
	}

	// Get the path after /go2rtc/
	path := chi.URLParam(r, "*")
	targetURL := fmt.Sprintf("%s/%s", p.manager.GetAPIURL(), path)

	// Create proxy request
	proxyReq, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Copy headers
	for key, values := range r.Header {
		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}

	// Execute request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(proxyReq)
	if err != nil {
		p.respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Copy status and body
	w.WriteHeader(resp.StatusCode)
	http.MaxBytesReader(w, resp.Body, 10*1024*1024) // 10MB limit
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
}

// Helper methods

func (p *StreamingPlugin) respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (p *StreamingPlugin) respondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

// Ensure StreamingPlugin implements the sdk.Plugin interface
var _ sdk.Plugin = (*StreamingPlugin)(nil)
var _ sdk.ServicePlugin = (*StreamingPlugin)(nil)

// Prevent unused function warning
var _ = New
