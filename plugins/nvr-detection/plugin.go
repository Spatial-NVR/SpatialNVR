// Package nvrdetection provides the NVR Detection Plugin
// This plugin manages object detection for cameras using various backends
package nvrdetection

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Spatial-NVR/SpatialNVR/internal/detection"
	"github.com/Spatial-NVR/SpatialNVR/sdk"
)

// DetectionPlugin implements the detection service as a plugin
type DetectionPlugin struct {
	sdk.BaseServicePlugin

	client       *detection.Client
	frameGrabber detection.FrameGrabber

	// Configuration
	detectionAddr string
	go2rtcAddr    string
	defaultFPS    int
	minConfidence float64
	modelsPath    string
	defaultBackend string

	// Active detection streams
	streams map[string]context.CancelFunc

	// Error tracking for rate-limited logging
	lastErrorTime map[string]time.Time

	mu      sync.RWMutex
	started bool
	ctx     context.Context
	cancel  context.CancelFunc
}

// New creates a new DetectionPlugin instance
func New() *DetectionPlugin {
	p := &DetectionPlugin{
		streams: make(map[string]context.CancelFunc),
	}
	p.SetManifest(sdk.PluginManifest{
		ID:          "nvr-detection",
		Name:        "Detection Service",
		Version:     "1.0.0",
		Description: "Object detection service with multiple backend support",
		Category:    "core",
		Critical:    false,
		Dependencies: []string{
			"nvr-streaming",
		},
		Capabilities: []string{
			sdk.CapabilityDetection,
			sdk.CapabilityMotion,
		},
	})
	return p
}

// Initialize sets up the plugin
func (p *DetectionPlugin) Initialize(ctx context.Context, runtime *sdk.PluginRuntime) error {
	if err := p.BaseServicePlugin.Initialize(ctx, runtime); err != nil {
		return err
	}

	// Get configuration
	p.detectionAddr = runtime.ConfigString("detection_addr", "localhost:50051")
	p.go2rtcAddr = runtime.ConfigString("go2rtc_addr", "http://localhost:1984")
	p.defaultFPS = runtime.ConfigInt("default_fps", 5)
	p.minConfidence = runtime.ConfigFloat("min_confidence", 0.5)
	p.modelsPath = runtime.ConfigString("models_path", "/data/models")
	p.defaultBackend = runtime.ConfigString("default_backend", "onnx")
	p.lastErrorTime = make(map[string]time.Time)

	return nil
}

// Start starts the detection service
func (p *DetectionPlugin) Start(ctx context.Context) error {
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

	// Create detection client
	client, err := detection.NewClient(detection.ClientConfig{
		Address: p.detectionAddr,
		Timeout: 30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("failed to create detection client: %w", err)
	}
	p.client = client

	// Create frame grabber
	p.frameGrabber = detection.NewGo2RTCFrameGrabber(p.go2rtcAddr)

	// Store context for camera streams
	p.ctx, p.cancel = context.WithCancel(ctx)

	// Subscribe to events
	if err := p.subscribeToEvents(); err != nil {
		runtime.Logger().Warn("Failed to subscribe to events", "error", err)
	}

	p.mu.Lock()
	p.started = true
	p.mu.Unlock()

	p.SetHealthy("Detection service running")
	runtime.Logger().Info("Detection plugin started",
		"detection_addr", p.detectionAddr,
		"default_fps", p.defaultFPS)

	// Publish started event
	_ = p.PublishEvent(sdk.EventTypePluginStarted, map[string]string{
		"plugin_id": "nvr-detection",
	})

	return nil
}

// Stop stops the detection service
func (p *DetectionPlugin) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Stop all camera streams
	for cameraID, cancel := range p.streams {
		cancel()
		if p.frameGrabber != nil {
			_ = p.frameGrabber.StopStream(cameraID)
		}
	}
	p.streams = make(map[string]context.CancelFunc)

	if p.cancel != nil {
		p.cancel()
	}

	if p.client != nil {
		_ = p.client.Close()
	}

	if p.frameGrabber != nil {
		_ = p.frameGrabber.Close()
	}

	p.started = false
	p.SetHealth(sdk.HealthStateUnknown, "Detection service stopped")

	return nil
}

// Health returns the plugin's health status
func (p *DetectionPlugin) Health() sdk.HealthStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.started {
		return sdk.HealthStatus{
			State:       sdk.HealthStateUnknown,
			Message:     "Not started",
			LastChecked: time.Now(),
		}
	}

	// Check if detection service is connected
	if p.client != nil {
		status, _ := p.client.GetStatus(context.Background())
		if status != nil && status.Connected {
			return sdk.HealthStatus{
				State:       sdk.HealthStateHealthy,
				Message:     "Detection service connected",
				LastChecked: time.Now(),
				Details: map[string]string{
					"stream_count":    fmt.Sprintf("%d", len(p.streams)),
					"processed_count": fmt.Sprintf("%d", status.ProcessedCount),
					"avg_latency_ms":  fmt.Sprintf("%.2f", status.AvgLatencyMs),
				},
			}
		}
	}

	return sdk.HealthStatus{
		State:       sdk.HealthStateDegraded,
		Message:     "Detection service not connected",
		LastChecked: time.Now(),
	}
}

// Routes returns the HTTP routes for this plugin
// When mounted at /models by the gateway, "/" handles model listing
func (p *DetectionPlugin) Routes() http.Handler {
	r := chi.NewRouter()

	// Root route for model listing (when mounted at /models)
	r.Get("/", p.handleListModels)
	r.Post("/", p.handleLoadModel)

	// Detection endpoints
	r.Post("/detect", p.handleDetect)
	r.Post("/detect/async", p.handleDetectAsync)

	// Camera detection control
	r.Post("/cameras/{cameraId}/start", p.handleStartCamera)
	r.Post("/cameras/{cameraId}/stop", p.handleStopCamera)
	r.Get("/cameras/{cameraId}/status", p.handleCameraStatus)

	// Models (also accessible via legacy /models path)
	r.Get("/models", p.handleListModels)
	r.Post("/models", p.handleLoadModel)
	r.Delete("/models/{modelId}", p.handleUnloadModel)
	r.Delete("/{modelId}", p.handleUnloadModel)

	// Backends
	r.Get("/backends", p.handleListBackends)

	// Motion detection
	r.Get("/motion", p.handleGetMotionStatus)
	r.Post("/motion/config", p.handleConfigureMotion)
	r.Post("/motion/reset", p.handleResetMotion)

	// Service status
	r.Get("/status", p.handleGetStatus)

	return r
}

// EventSubscriptions returns events this plugin subscribes to
func (p *DetectionPlugin) EventSubscriptions() []string {
	return []string{
		sdk.EventTypeCameraAdded,
		sdk.EventTypeCameraRemoved,
		sdk.EventTypeCameraUpdated,
		sdk.EventTypeConfigChanged,
	}
}

// HandleEvent processes incoming events
func (p *DetectionPlugin) HandleEvent(ctx context.Context, event *sdk.Event) error {
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
func (p *DetectionPlugin) OnConfigChange(config map[string]interface{}) {
	if fps, ok := config["default_fps"].(int); ok {
		p.defaultFPS = fps
	}
	if conf, ok := config["min_confidence"].(float64); ok {
		p.minConfidence = conf
	}
}

// StartCamera starts detection for a camera
func (p *DetectionPlugin) StartCamera(cameraID string, fps int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.streams[cameraID]; exists {
		return nil // Already running
	}

	if fps <= 0 {
		fps = p.defaultFPS
	}

	ctx, cancel := context.WithCancel(p.ctx)
	p.streams[cameraID] = cancel

	// Start frame stream
	frameCh, err := p.frameGrabber.StartStream(ctx, cameraID, fps)
	if err != nil {
		cancel()
		delete(p.streams, cameraID)
		return err
	}

	// Process frames in background
	go p.processFrames(ctx, cameraID, frameCh)

	p.Runtime().Logger().Info("Started detection", "camera", cameraID, "fps", fps)
	return nil
}

// StopCamera stops detection for a camera
func (p *DetectionPlugin) StopCamera(cameraID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	cancel, exists := p.streams[cameraID]
	if !exists {
		return nil
	}

	cancel()
	if p.frameGrabber != nil {
		_ = p.frameGrabber.StopStream(cameraID)
	}
	delete(p.streams, cameraID)

	p.Runtime().Logger().Info("Stopped detection", "camera", cameraID)
	return nil
}

// Private methods

func (p *DetectionPlugin) subscribeToEvents() error {
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

func (p *DetectionPlugin) handleCameraAdded(event *sdk.Event) error {
	cameraID, _ := event.Data["camera_id"].(string)
	if cameraID == "" {
		return nil
	}

	// Extract detection settings from nested config object
	detectionEnabled, fps := p.extractDetectionSettings(event.Data)

	if !detectionEnabled {
		p.Runtime().Logger().Debug("Detection not enabled for camera", "camera_id", cameraID)
		return nil
	}

	p.Runtime().Logger().Info("Starting detection for camera", "camera_id", cameraID, "fps", fps)
	return p.StartCamera(cameraID, fps)
}

// extractDetectionSettings extracts detection enabled/fps from event data
// Handles both flat fields (legacy) and nested config object
func (p *DetectionPlugin) extractDetectionSettings(data map[string]interface{}) (enabled bool, fps int) {
	// Try flat fields first (legacy format)
	if enabled, ok := data["detection_enabled"].(bool); ok {
		fps, _ := data["detection_fps"].(int)
		return enabled, fps
	}

	// Try nested config object
	if configData, ok := data["config"]; ok {
		// Handle config.CameraConfig struct
		if cfg, ok := configData.(map[string]interface{}); ok {
			if detection, ok := cfg["detection"].(map[string]interface{}); ok {
				enabled, _ = detection["enabled"].(bool)
				fps, _ = detection["fps"].(int)
				return enabled, fps
			}
		}
		// Handle typed config.CameraConfig (when passed directly in-process)
		// Use reflection-free approach by checking for Detection field
		if cfgBytes, err := json.Marshal(configData); err == nil {
			var cfgMap map[string]interface{}
			if json.Unmarshal(cfgBytes, &cfgMap) == nil {
				if detection, ok := cfgMap["detection"].(map[string]interface{}); ok {
					enabled, _ = detection["enabled"].(bool)
					fpsFloat, _ := detection["fps"].(float64) // JSON numbers are float64
					fps = int(fpsFloat)
					return enabled, fps
				}
			}
		}
	}

	return false, 0
}

func (p *DetectionPlugin) handleCameraRemoved(event *sdk.Event) error {
	cameraID, _ := event.Data["camera_id"].(string)
	if cameraID == "" {
		return nil
	}

	return p.StopCamera(cameraID)
}

func (p *DetectionPlugin) handleCameraUpdated(event *sdk.Event) error {
	cameraID, _ := event.Data["camera_id"].(string)
	if cameraID == "" {
		return nil
	}

	// Extract detection settings from nested config object
	detectionEnabled, fps := p.extractDetectionSettings(event.Data)

	p.mu.RLock()
	_, running := p.streams[cameraID]
	p.mu.RUnlock()

	if detectionEnabled && !running {
		p.Runtime().Logger().Info("Starting detection for camera", "camera_id", cameraID, "fps", fps)
		return p.StartCamera(cameraID, fps)
	} else if !detectionEnabled && running {
		p.Runtime().Logger().Info("Stopping detection for camera", "camera_id", cameraID)
		return p.StopCamera(cameraID)
	}

	return nil
}

func (p *DetectionPlugin) processFrames(ctx context.Context, cameraID string, frameCh <-chan *detection.Frame) {
	for {
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-frameCh:
			if !ok {
				return
			}

			p.processFrame(ctx, cameraID, frame)
		}
	}
}

func (p *DetectionPlugin) processFrame(ctx context.Context, cameraID string, frame *detection.Frame) {
	resp, err := p.client.Detect(ctx, &detection.DetectRequest{
		CameraID:      cameraID,
		Frame:         frame,
		MinConfidence: p.minConfidence,
	})
	if err != nil {
		// Log detection errors (rate-limited to avoid spam)
		p.logDetectionError(cameraID, err)
		return
	}

	// Publish detection events
	for _, det := range resp.Detections {
		p.Runtime().Logger().Debug("Detection found",
			"camera_id", cameraID,
			"object_type", det.ObjectType,
			"label", det.Label,
			"confidence", det.Confidence,
		)

		_ = p.PublishEvent(sdk.EventTypeDetection, map[string]interface{}{
			"camera_id":   cameraID,
			"object_type": string(det.ObjectType),
			"label":       det.Label,
			"confidence":  det.Confidence,
			"bbox": map[string]float64{
				"x":      det.BoundingBox.X,
				"y":      det.BoundingBox.Y,
				"width":  det.BoundingBox.Width,
				"height": det.BoundingBox.Height,
			},
			"track_id":  det.TrackID,
			"timestamp": det.Timestamp.Format(time.RFC3339),
		})
	}
}

// logDetectionError logs detection errors with rate limiting to avoid log spam
func (p *DetectionPlugin) logDetectionError(cameraID string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Initialize error tracking map if needed
	if p.lastErrorTime == nil {
		p.lastErrorTime = make(map[string]time.Time)
	}

	// Only log once per minute per camera
	lastTime, exists := p.lastErrorTime[cameraID]
	if !exists || time.Since(lastTime) > time.Minute {
		p.Runtime().Logger().Error("Detection failed", "camera_id", cameraID, "error", err)
		p.lastErrorTime[cameraID] = time.Now()
	}
}

// HTTP Handlers

func (p *DetectionPlugin) handleDetect(w http.ResponseWriter, r *http.Request) {
	if p.client == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Detection service not available")
		return
	}

	var req struct {
		CameraID      string  `json:"camera_id"`
		ImageData     string  `json:"image_data"` // Base64 encoded
		MinConfidence float64 `json:"min_confidence"`
		Objects       []string `json:"objects"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		p.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Decode base64 image
	imageBytes, err := base64.StdEncoding.DecodeString(req.ImageData)
	if err != nil {
		p.respondError(w, http.StatusBadRequest, "Invalid image data")
		return
	}

	minConf := req.MinConfidence
	if minConf <= 0 {
		minConf = p.minConfidence
	}

	resp, err := p.client.Detect(r.Context(), &detection.DetectRequest{
		CameraID: req.CameraID,
		Frame: &detection.Frame{
			CameraID:  req.CameraID,
			Timestamp: time.Now(),
			Data:      imageBytes,
			Format:    "jpeg",
		},
		MinConfidence: minConf,
		Objects:       req.Objects,
	})
	if err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	p.respondJSON(w, resp)
}

func (p *DetectionPlugin) handleDetectAsync(w http.ResponseWriter, r *http.Request) {
	if p.client == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Detection service not available")
		return
	}

	var req struct {
		CameraID      string  `json:"camera_id"`
		ImageData     string  `json:"image_data"`
		MinConfidence float64 `json:"min_confidence"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		p.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	imageBytes, err := base64.StdEncoding.DecodeString(req.ImageData)
	if err != nil {
		p.respondError(w, http.StatusBadRequest, "Invalid image data")
		return
	}

	minConf := req.MinConfidence
	if minConf <= 0 {
		minConf = p.minConfidence
	}

	// Queue async detection
	_ = p.client.DetectAsync(&detection.DetectRequest{
		CameraID: req.CameraID,
		Frame: &detection.Frame{
			CameraID:  req.CameraID,
			Timestamp: time.Now(),
			Data:      imageBytes,
			Format:    "jpeg",
		},
		MinConfidence: minConf,
	})

	p.respondJSON(w, map[string]string{
		"status": "queued",
	})
}

func (p *DetectionPlugin) handleStartCamera(w http.ResponseWriter, r *http.Request) {
	cameraID := chi.URLParam(r, "cameraId")

	var req struct {
		FPS int `json:"fps"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if err := p.StartCamera(cameraID, req.FPS); err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	p.respondJSON(w, map[string]interface{}{
		"camera_id": cameraID,
		"status":    "started",
	})
}

func (p *DetectionPlugin) handleStopCamera(w http.ResponseWriter, r *http.Request) {
	cameraID := chi.URLParam(r, "cameraId")

	if err := p.StopCamera(cameraID); err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	p.respondJSON(w, map[string]interface{}{
		"camera_id": cameraID,
		"status":    "stopped",
	})
}

func (p *DetectionPlugin) handleCameraStatus(w http.ResponseWriter, r *http.Request) {
	cameraID := chi.URLParam(r, "cameraId")

	p.mu.RLock()
	_, running := p.streams[cameraID]
	p.mu.RUnlock()

	p.respondJSON(w, map[string]interface{}{
		"camera_id": cameraID,
		"running":   running,
	})
}

func (p *DetectionPlugin) handleListModels(w http.ResponseWriter, r *http.Request) {
	if p.client == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Detection service not available")
		return
	}

	models, err := p.client.GetModels(r.Context())
	if err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	p.respondJSON(w, map[string]interface{}{
		"models": models,
	})
}

func (p *DetectionPlugin) handleLoadModel(w http.ResponseWriter, r *http.Request) {
	if p.client == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Detection service not available")
		return
	}

	var req struct {
		Path    string `json:"path"`
		Type    string `json:"type"`
		Backend string `json:"backend"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		p.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	modelID, err := p.client.LoadModel(r.Context(), req.Path, detection.ModelType(req.Type), detection.BackendType(req.Backend))
	if err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	p.respondJSON(w, map[string]string{
		"model_id": modelID,
		"status":   "loaded",
	})
}

func (p *DetectionPlugin) handleUnloadModel(w http.ResponseWriter, r *http.Request) {
	if p.client == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Detection service not available")
		return
	}

	modelID := chi.URLParam(r, "modelId")

	if err := p.client.UnloadModel(r.Context(), modelID); err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (p *DetectionPlugin) handleListBackends(w http.ResponseWriter, r *http.Request) {
	if p.client == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Detection service not available")
		return
	}

	backends, err := p.client.GetBackends(r.Context())
	if err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	p.respondJSON(w, map[string]interface{}{
		"backends": backends,
	})
}

func (p *DetectionPlugin) handleGetMotionStatus(w http.ResponseWriter, r *http.Request) {
	if p.client == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Detection service not available")
		return
	}

	status, err := p.client.GetMotionStatus(r.Context())
	if err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	p.respondJSON(w, status)
}

func (p *DetectionPlugin) handleConfigureMotion(w http.ResponseWriter, r *http.Request) {
	if p.client == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Detection service not available")
		return
	}

	var config detection.MotionConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		p.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := p.client.ConfigureMotion(r.Context(), config); err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	p.respondJSON(w, map[string]string{
		"status": "configured",
	})
}

func (p *DetectionPlugin) handleResetMotion(w http.ResponseWriter, r *http.Request) {
	if p.client == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Detection service not available")
		return
	}

	var req struct {
		CameraID string `json:"camera_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if err := p.client.ResetMotion(r.Context(), req.CameraID); err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	p.respondJSON(w, map[string]string{
		"status": "reset",
	})
}

func (p *DetectionPlugin) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	started := p.started
	streamCount := len(p.streams)
	p.mu.RUnlock()

	var serviceStatus *detection.ServiceStatus
	if p.client != nil {
		serviceStatus, _ = p.client.GetStatus(r.Context())
	}

	response := map[string]interface{}{
		"started":      started,
		"stream_count": streamCount,
		"default_fps":  p.defaultFPS,
		"min_confidence": p.minConfidence,
	}

	if serviceStatus != nil {
		response["connected"] = serviceStatus.Connected
		response["processed_count"] = serviceStatus.ProcessedCount
		response["error_count"] = serviceStatus.ErrorCount
		response["avg_latency_ms"] = serviceStatus.AvgLatencyMs
	} else {
		response["connected"] = false
	}

	p.respondJSON(w, response)
}

// Helper methods

func (p *DetectionPlugin) respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (p *DetectionPlugin) respondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

// Ensure DetectionPlugin implements the sdk.Plugin interface
var _ sdk.Plugin = (*DetectionPlugin)(nil)
var _ sdk.ServicePlugin = (*DetectionPlugin)(nil)

// Prevent unused function warning
var _ = New
