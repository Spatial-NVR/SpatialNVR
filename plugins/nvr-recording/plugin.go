// Package nvrrecording provides the NVR Recording Plugin
// This plugin handles all video recording, storage, timeline, and playback functionality
package nvrrecording

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Spatial-NVR/SpatialNVR/internal/config"
	"github.com/Spatial-NVR/SpatialNVR/internal/recording"
	"github.com/Spatial-NVR/SpatialNVR/sdk"
)

// RecordingPlugin implements the recording service as a plugin
type RecordingPlugin struct {
	sdk.BaseServicePlugin

	service       *recording.Service
	storagePath   string
	thumbnailPath string

	mu      sync.RWMutex
	started bool
}

// New creates a new RecordingPlugin instance
func New() *RecordingPlugin {
	p := &RecordingPlugin{}
	p.SetManifest(sdk.PluginManifest{
		ID:          "nvr-recording",
		Name:        "Recording Service",
		Version:     "1.0.0",
		Description: "Core video recording, storage, timeline, and playback service",
		Category:    "core",
		Critical:    false,
		Dependencies: []string{
			"nvr-streaming",
		},
		Capabilities: []string{
			sdk.CapabilityRecording,
			sdk.CapabilityPlayback,
			sdk.CapabilityTimeline,
			sdk.CapabilityExport,
		},
	})
	return p
}

// Initialize sets up the plugin
func (p *RecordingPlugin) Initialize(ctx context.Context, runtime *sdk.PluginRuntime) error {
	if err := p.BaseServicePlugin.Initialize(ctx, runtime); err != nil {
		return err
	}

	// Get configuration
	storagePath := runtime.ConfigString("storage_path", "/data/recordings")
	thumbnailPath := runtime.ConfigString("thumbnail_path", "/data/thumbnails")

	p.storagePath = storagePath
	p.thumbnailPath = thumbnailPath

	return nil
}

// Start starts the recording service
func (p *RecordingPlugin) Start(ctx context.Context) error {
	runtime := p.Runtime()
	if runtime == nil {
		return fmt.Errorf("plugin not initialized")
	}

	// Create service
	db := runtime.DB()
	if db == nil {
		return fmt.Errorf("database not available")
	}

	svcConfig := recording.ServiceConfig{
		StoragePath:   p.storagePath,
		ThumbnailPath: p.thumbnailPath,
	}

	// Note: We need to pass a config object - in plugin mode, we'll get this from events
	// For now, create the service with minimal config
	svc, err := p.createService(db, svcConfig)
	if err != nil {
		return fmt.Errorf("failed to create recording service: %w", err)
	}

	p.mu.Lock()
	p.service = svc
	p.mu.Unlock()

	// Start the service
	if err := svc.Start(ctx); err != nil {
		return fmt.Errorf("failed to start recording service: %w", err)
	}

	// Subscribe to events
	if err := p.subscribeToEvents(); err != nil {
		runtime.Logger().Warn("Failed to subscribe to events", "error", err)
	}

	p.mu.Lock()
	p.started = true
	p.mu.Unlock()

	p.SetHealthy("Recording service running")
	runtime.Logger().Info("Recording plugin started",
		"storage_path", p.storagePath,
		"thumbnail_path", p.thumbnailPath)

	// Fetch existing cameras from config plugin after a short delay
	// (to ensure config plugin is ready)
	go func() {
		time.Sleep(2 * time.Second)
		p.loadExistingCameras(runtime)
	}()

	// Publish started event
	_ = p.PublishEvent(sdk.EventTypeRecordingStart, map[string]string{
		"plugin_id": "nvr-recording",
	})

	return nil
}

// loadExistingCameras fetches existing camera configs from the config plugin
func (p *RecordingPlugin) loadExistingCameras(runtime *sdk.PluginRuntime) {
	// Request cameras from config plugin via RPC
	data, err := runtime.Request("nvr-core-config", "get-cameras", nil, 5*time.Second)
	if err != nil {
		runtime.Logger().Warn("Failed to fetch existing cameras from config plugin", "error", err)
		return
	}

	var cameras []config.CameraConfig
	if err := json.Unmarshal(data, &cameras); err != nil {
		runtime.Logger().Error("Failed to unmarshal camera configs", "error", err)
		return
	}

	p.mu.RLock()
	svc := p.service
	p.mu.RUnlock()

	if svc == nil {
		return
	}

	// Update service with each camera config
	for _, cam := range cameras {
		svc.UpdateCameraConfig(cam)
	}

	runtime.Logger().Info("Loaded existing camera configs", "count", len(cameras))
}

// Stop stops the recording service
func (p *RecordingPlugin) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.service != nil {
		if err := p.service.Stop(); err != nil {
			return err
		}
	}

	p.started = false
	p.SetHealth(sdk.HealthStateUnknown, "Recording service stopped")

	return nil
}

// Health returns the plugin's health status
func (p *RecordingPlugin) Health() sdk.HealthStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.started {
		return sdk.HealthStatus{
			State:       sdk.HealthStateUnknown,
			Message:     "Not started",
			LastChecked: time.Now(),
		}
	}

	// Check if service is running
	if p.service == nil {
		return sdk.HealthStatus{
			State:       sdk.HealthStateUnhealthy,
			Message:     "Service not initialized",
			LastChecked: time.Now(),
		}
	}

	return sdk.HealthStatus{
		State:       sdk.HealthStateHealthy,
		Message:     "Recording service operational",
		LastChecked: time.Now(),
	}
}

// Routes returns the HTTP routes for this plugin
func (p *RecordingPlugin) Routes() http.Handler {
	r := chi.NewRouter()

	// Segments
	r.Get("/", p.handleListSegments)
	r.Get("/segments", p.handleListSegments)
	r.Get("/segments/{id}", p.handleGetSegment)
	r.Get("/segments/{id}/stream", p.handleStreamSegment)
	r.Get("/segments/{id}/download", p.handleDownloadSegment)
	r.Get("/segments/{id}/thumbnail", p.handleGetThumbnail)
	r.Delete("/segments/{id}", p.handleDeleteSegment)

	// Timeline
	r.Get("/timeline/{cameraId}", p.handleGetTimeline)
	r.Get("/timeline/{cameraId}/segments", p.handleGetTimelineSegments)
	r.Get("/timeline/{cameraId}/stream", p.handleStreamFromTimestamp)

	// Camera recording control
	r.Post("/cameras/{cameraId}/start", p.handleStartRecording)
	r.Post("/cameras/{cameraId}/stop", p.handleStopRecording)
	r.Post("/cameras/{cameraId}/restart", p.handleRestartRecording)

	// Status
	r.Get("/status", p.handleGetAllStatus)
	r.Get("/status/{cameraId}", p.handleGetStatus)
	r.Get("/storage", p.handleGetStorageStats)

	// Playback
	r.Get("/playback/{cameraId}", p.handleGetPlaybackInfo)

	// Export
	r.Post("/export", p.handleExportSegments)

	// Retention
	r.Post("/retention/run", p.handleRunRetention)

	return r
}

// EventSubscriptions returns events this plugin subscribes to
func (p *RecordingPlugin) EventSubscriptions() []string {
	return []string{
		sdk.EventTypeCameraAdded,
		sdk.EventTypeCameraRemoved,
		sdk.EventTypeCameraUpdated,
		sdk.EventTypeConfigChanged,
		sdk.EventTypeDetection,
	}
}

// HandleEvent processes incoming events
func (p *RecordingPlugin) HandleEvent(ctx context.Context, event *sdk.Event) error {
	switch event.Type {
	case sdk.EventTypeCameraAdded, sdk.EventTypeCameraUpdated:
		// Extract camera config from event and update recording service
		p.handleCameraConfigEvent(event)

	case sdk.EventTypeCameraRemoved:
		// Stop recording for removed camera
		if cameraID, ok := event.Data["camera_id"].(string); ok {
			p.mu.RLock()
			svc := p.service
			p.mu.RUnlock()
			if svc != nil {
				svc.RemoveCameraConfig(cameraID)
			}
		}

	case sdk.EventTypeDetection:
		// Trigger event recording
		if cameraID := event.CameraID; cameraID != "" {
			p.mu.RLock()
			svc := p.service
			p.mu.RUnlock()
			if svc != nil {
				go func() {
					if err := svc.TriggerEventRecording(cameraID, event.ID); err != nil {
						p.Runtime().Logger().Debug("Failed to trigger event recording",
							"camera_id", cameraID, "error", err)
					}
				}()
			}
		}
	}

	return nil
}

// handleCameraConfigEvent extracts camera config from an event and updates the service
func (p *RecordingPlugin) handleCameraConfigEvent(event *sdk.Event) {
	cameraID, ok := event.Data["camera_id"].(string)
	if !ok {
		return
	}

	p.mu.RLock()
	svc := p.service
	p.mu.RUnlock()

	if svc == nil {
		return
	}

	// Try to extract full config from event
	configData, hasConfig := event.Data["config"]
	if !hasConfig {
		p.Runtime().Logger().Warn("Camera event missing config data, cannot configure recording",
			"camera_id", cameraID)
		return
	}

	// Convert config data to CameraConfig
	camCfg, err := p.extractCameraConfig(configData)
	if err != nil {
		p.Runtime().Logger().Error("Failed to extract camera config from event",
			"camera_id", cameraID, "error", err)
		return
	}

	// Ensure camera ID is set
	camCfg.ID = cameraID

	// Update service with camera config
	svc.UpdateCameraConfig(camCfg)

	p.Runtime().Logger().Info("Camera config received via event",
		"camera_id", cameraID,
		"recording_enabled", camCfg.Recording.Enabled)
}

// extractCameraConfig converts event config data to a CameraConfig struct
func (p *RecordingPlugin) extractCameraConfig(data interface{}) (config.CameraConfig, error) {
	var camCfg config.CameraConfig

	// The config may come as a map or as the actual struct
	// Try JSON marshal/unmarshal to convert
	jsonData, err := json.Marshal(data)
	if err != nil {
		return camCfg, fmt.Errorf("failed to marshal config data: %w", err)
	}

	if err := json.Unmarshal(jsonData, &camCfg); err != nil {
		return camCfg, fmt.Errorf("failed to unmarshal config data: %w", err)
	}

	return camCfg, nil
}

// OnConfigChange handles configuration changes
func (p *RecordingPlugin) OnConfigChange(config map[string]interface{}) {
	// Update paths if changed
	if storagePath, ok := config["storage_path"].(string); ok {
		p.mu.Lock()
		p.storagePath = storagePath
		p.mu.Unlock()
	}
	if thumbnailPath, ok := config["thumbnail_path"].(string); ok {
		p.mu.Lock()
		p.thumbnailPath = thumbnailPath
		p.mu.Unlock()
	}
}

// createService creates the recording service
func (p *RecordingPlugin) createService(db *sql.DB, svcConfig recording.ServiceConfig) (*recording.Service, error) {
	// Create directories
	for _, dir := range []string{svcConfig.StoragePath, svcConfig.ThumbnailPath} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// For plugin mode, we create a minimal config wrapper
	// The full config comes via events
	return recording.NewServiceFromPlugin(db, svcConfig)
}

// subscribeToEvents subscribes to relevant events
func (p *RecordingPlugin) subscribeToEvents() error {
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

func (p *RecordingPlugin) handleListSegments(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	svc := p.service
	p.mu.RUnlock()

	if svc == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Service not available")
		return
	}

	ctx := r.Context()
	opts := recording.ListOptions{
		Limit:  50,
		Offset: 0,
	}

	// Parse query parameters
	if v := r.URL.Query().Get("camera_id"); v != "" {
		opts.CameraID = v
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if limit, err := strconv.Atoi(v); err == nil && limit > 0 && limit <= 100 {
			opts.Limit = limit
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if offset, err := strconv.Atoi(v); err == nil && offset >= 0 {
			opts.Offset = offset
		}
	}
	if v := r.URL.Query().Get("start_time"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			opts.StartTime = &t
		}
	}
	if v := r.URL.Query().Get("end_time"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			opts.EndTime = &t
		}
	}

	segments, total, err := svc.ListSegments(ctx, opts)
	if err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	p.respondJSON(w, map[string]interface{}{
		"data":  segments,
		"total": total,
		"page":  (opts.Offset / opts.Limit) + 1,
		"limit": opts.Limit,
	})
}

func (p *RecordingPlugin) handleGetSegment(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	svc := p.service
	p.mu.RUnlock()

	if svc == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Service not available")
		return
	}

	id := chi.URLParam(r, "id")
	segment, err := svc.GetSegment(r.Context(), id)
	if err != nil {
		p.respondError(w, http.StatusNotFound, "Segment not found")
		return
	}

	p.respondJSON(w, segment)
}

func (p *RecordingPlugin) handleDeleteSegment(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	svc := p.service
	p.mu.RUnlock()

	if svc == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Service not available")
		return
	}

	id := chi.URLParam(r, "id")
	if err := svc.DeleteSegment(r.Context(), id); err != nil {
		p.respondError(w, http.StatusNotFound, "Segment not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (p *RecordingPlugin) handleStreamSegment(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	svc := p.service
	p.mu.RUnlock()

	if svc == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Service not available")
		return
	}

	id := chi.URLParam(r, "id")
	segment, err := svc.GetSegment(r.Context(), id)
	if err != nil {
		p.respondError(w, http.StatusNotFound, "Segment not found")
		return
	}

	// Check file exists
	if _, err := os.Stat(segment.FilePath); err != nil {
		p.respondError(w, http.StatusNotFound, "Segment file not found")
		return
	}

	// Serve file
	w.Header().Set("Content-Type", "video/mp4")
	http.ServeFile(w, r, segment.FilePath)
}

func (p *RecordingPlugin) handleDownloadSegment(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	svc := p.service
	p.mu.RUnlock()

	if svc == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Service not available")
		return
	}

	id := chi.URLParam(r, "id")
	segment, err := svc.GetSegment(r.Context(), id)
	if err != nil {
		p.respondError(w, http.StatusNotFound, "Segment not found")
		return
	}

	// Check file exists
	if _, err := os.Stat(segment.FilePath); err != nil {
		p.respondError(w, http.StatusNotFound, "Segment file not found")
		return
	}

	// Set download headers
	filename := filepath.Base(segment.FilePath)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Type", "video/mp4")
	http.ServeFile(w, r, segment.FilePath)
}

func (p *RecordingPlugin) handleGetThumbnail(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	svc := p.service
	p.mu.RUnlock()

	if svc == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Service not available")
		return
	}

	id := chi.URLParam(r, "id")
	thumbPath, err := svc.GenerateThumbnail(r.Context(), id)
	if err != nil {
		p.respondError(w, http.StatusNotFound, "Thumbnail not available")
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeFile(w, r, thumbPath)
}

func (p *RecordingPlugin) handleGetTimeline(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	svc := p.service
	p.mu.RUnlock()

	if svc == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Service not available")
		return
	}

	cameraID := chi.URLParam(r, "cameraId")
	start, end := p.parseTimeRange(r)

	timeline, err := svc.GetTimeline(r.Context(), cameraID, start, end)
	if err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	p.respondJSON(w, timeline)
}

func (p *RecordingPlugin) handleGetTimelineSegments(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	svc := p.service
	p.mu.RUnlock()

	if svc == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Service not available")
		return
	}

	cameraID := chi.URLParam(r, "cameraId")
	start, end := p.parseTimeRange(r)

	segments, err := svc.GetTimelineSegments(r.Context(), cameraID, start, end)
	if err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	p.respondJSON(w, map[string]interface{}{
		"camera_id": cameraID,
		"segments":  segments,
		"start":     start.Format(time.RFC3339),
		"end":       end.Format(time.RFC3339),
	})
}

func (p *RecordingPlugin) handleStreamFromTimestamp(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	svc := p.service
	p.mu.RUnlock()

	if svc == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Service not available")
		return
	}

	cameraID := chi.URLParam(r, "cameraId")
	// Support both "t" (frontend uses this) and "timestamp" for backwards compat
	timestampStr := r.URL.Query().Get("t")
	if timestampStr == "" {
		timestampStr = r.URL.Query().Get("timestamp")
	}

	var timestamp time.Time
	if timestampStr != "" {
		var err error
		timestamp, err = time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			// Try Unix timestamp as fallback
			if unix, err2 := strconv.ParseInt(timestampStr, 10, 64); err2 == nil {
				timestamp = time.Unix(unix, 0)
			} else {
				p.respondError(w, http.StatusBadRequest, "Invalid timestamp format")
				return
			}
		}
	} else {
		p.respondError(w, http.StatusBadRequest, "timestamp parameter 't' is required")
		return
	}

	filePath, offset, err := svc.GetPlaybackInfo(r.Context(), cameraID, timestamp)
	if err != nil {
		p.respondError(w, http.StatusNotFound, "No recording found at timestamp")
		return
	}

	// Check if file exists
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		p.respondError(w, http.StatusNotFound, "Recording file not found")
		return
	}

	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		p.respondError(w, http.StatusInternalServerError, "Failed to open recording file")
		return
	}
	defer file.Close()

	// Set content type based on extension
	contentType := "video/mp4"
	ext := filepath.Ext(filePath)
	switch ext {
	case ".mkv":
		contentType = "video/x-matroska"
	case ".webm":
		contentType = "video/webm"
	case ".ts":
		contentType = "video/mp2t"
	}

	// Add custom headers for timeline info
	w.Header().Set("X-Segment-Offset", fmt.Sprintf("%.3f", offset))

	// Handle range requests for seeking
	fileSize := fileInfo.Size()
	rangeHeader := r.Header.Get("Range")

	if rangeHeader == "" {
		// No range request, serve entire file
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", strconv.FormatInt(fileSize, 10))
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusOK)
		io.Copy(w, file)
		return
	}

	// Parse range header (format: "bytes=start-end")
	rangeHeader = strings.TrimPrefix(rangeHeader, "bytes=")
	parts := strings.Split(rangeHeader, "-")
	if len(parts) != 2 {
		p.respondError(w, http.StatusBadRequest, "Invalid range header")
		return
	}

	var start, end int64
	if parts[0] != "" {
		start, _ = strconv.ParseInt(parts[0], 10, 64)
	}

	if parts[1] != "" {
		end, _ = strconv.ParseInt(parts[1], 10, 64)
	} else {
		// If no end specified, serve to end of file (but limit chunk size)
		end = start + 10*1024*1024 // 10MB chunks
		if end >= fileSize {
			end = fileSize - 1
		}
	}

	// Validate range
	if start >= fileSize || end >= fileSize || start > end {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", fileSize))
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}

	// Seek to start position
	file.Seek(start, 0)

	// Set headers for partial content
	contentLength := end - start + 1
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileSize))
	w.Header().Set("Accept-Ranges", "bytes")
	w.WriteHeader(http.StatusPartialContent)

	io.CopyN(w, file, contentLength)
}

func (p *RecordingPlugin) handleStartRecording(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	svc := p.service
	p.mu.RUnlock()

	if svc == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Service not available")
		return
	}

	cameraID := chi.URLParam(r, "cameraId")
	if err := svc.StartCamera(cameraID); err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	p.respondJSON(w, map[string]interface{}{
		"camera_id": cameraID,
		"status":    "recording",
	})
}

func (p *RecordingPlugin) handleStopRecording(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	svc := p.service
	p.mu.RUnlock()

	if svc == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Service not available")
		return
	}

	cameraID := chi.URLParam(r, "cameraId")
	if err := svc.StopCamera(cameraID); err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	p.respondJSON(w, map[string]interface{}{
		"camera_id": cameraID,
		"status":    "stopped",
	})
}

func (p *RecordingPlugin) handleRestartRecording(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	svc := p.service
	p.mu.RUnlock()

	if svc == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Service not available")
		return
	}

	cameraID := chi.URLParam(r, "cameraId")
	if err := svc.RestartCamera(cameraID); err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	p.respondJSON(w, map[string]interface{}{
		"camera_id": cameraID,
		"status":    "recording",
	})
}

func (p *RecordingPlugin) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	svc := p.service
	p.mu.RUnlock()

	if svc == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Service not available")
		return
	}

	cameraID := chi.URLParam(r, "cameraId")
	status, err := svc.GetRecorderStatus(cameraID)
	if err != nil {
		p.respondError(w, http.StatusNotFound, "Camera not found")
		return
	}

	p.respondJSON(w, status)
}

func (p *RecordingPlugin) handleGetAllStatus(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	svc := p.service
	p.mu.RUnlock()

	if svc == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Service not available")
		return
	}

	statuses := svc.GetAllRecorderStatus()
	p.respondJSON(w, statuses)
}

func (p *RecordingPlugin) handleGetStorageStats(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	svc := p.service
	p.mu.RUnlock()

	if svc == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Service not available")
		return
	}

	stats, err := svc.GetStorageStats(r.Context())
	if err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Wrap in success format expected by frontend
	p.respondJSON(w, map[string]interface{}{
		"success": true,
		"data":    stats,
	})
}

func (p *RecordingPlugin) handleGetPlaybackInfo(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	svc := p.service
	p.mu.RUnlock()

	if svc == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Service not available")
		return
	}

	cameraID := chi.URLParam(r, "cameraId")
	timestampStr := r.URL.Query().Get("timestamp")

	var timestamp time.Time
	if timestampStr != "" {
		var err error
		timestamp, err = time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			p.respondError(w, http.StatusBadRequest, "Invalid timestamp format")
			return
		}
	} else {
		timestamp = time.Now()
	}

	url, offset, err := svc.GetPlaybackInfo(r.Context(), cameraID, timestamp)
	if err != nil {
		p.respondError(w, http.StatusNotFound, "No recording found")
		return
	}

	p.respondJSON(w, map[string]interface{}{
		"camera_id": cameraID,
		"url":       url,
		"offset":    offset,
		"timestamp": timestamp.Format(time.RFC3339),
	})
}

func (p *RecordingPlugin) handleExportSegments(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	svc := p.service
	p.mu.RUnlock()

	if svc == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Service not available")
		return
	}

	var req struct {
		CameraID  string `json:"camera_id"`
		StartTime string `json:"start_time"`
		EndTime   string `json:"end_time"`
		Format    string `json:"format"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		p.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	start, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		p.respondError(w, http.StatusBadRequest, "Invalid start_time format")
		return
	}

	end, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		p.respondError(w, http.StatusBadRequest, "Invalid end_time format")
		return
	}

	// Create temporary export file
	exportDir := filepath.Join(p.storagePath, "exports")
	_ = os.MkdirAll(exportDir, 0755)
	outputPath := filepath.Join(exportDir, fmt.Sprintf("export_%s_%d.mp4", req.CameraID, time.Now().Unix()))

	if err := svc.ExportSegments(r.Context(), req.CameraID, start, end, outputPath); err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Return download URL
	p.respondJSON(w, map[string]interface{}{
		"camera_id":   req.CameraID,
		"start_time":  req.StartTime,
		"end_time":    req.EndTime,
		"output_path": outputPath,
		"status":      "completed",
	})
}

func (p *RecordingPlugin) handleRunRetention(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	svc := p.service
	p.mu.RUnlock()

	if svc == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Service not available")
		return
	}

	stats, err := svc.RunRetention(r.Context())
	if err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	p.respondJSON(w, stats)
}

// Helper methods

func (p *RecordingPlugin) parseTimeRange(r *http.Request) (time.Time, time.Time) {
	var start, end time.Time

	if startStr := r.URL.Query().Get("start"); startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			start = t
		}
	}
	if start.IsZero() {
		start = time.Now().Add(-24 * time.Hour)
	}

	if endStr := r.URL.Query().Get("end"); endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			end = t
		}
	}
	if end.IsZero() {
		end = time.Now()
	}

	return start, end
}

func (p *RecordingPlugin) respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (p *RecordingPlugin) respondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

// Ensure RecordingPlugin implements the sdk.Plugin interface
var _ sdk.Plugin = (*RecordingPlugin)(nil)
var _ sdk.ServicePlugin = (*RecordingPlugin)(nil)

// Unexported for preventing direct usage - plugins are loaded by the plugin loader
// This file is compiled as part of the main binary for builtin plugins
// or as a separate binary for external plugins
var _ = New // Prevent unused function warning in IDE

// For compatibility, we need to add a constructor function to recording package
// that works without the full config.Config

func init() {
	// Register any initialization needed
}
