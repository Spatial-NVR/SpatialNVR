package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/Spatial-NVR/SpatialNVR/internal/recording"
)

// RecordingService defines the interface for recording operations
type RecordingService interface {
	ListSegments(ctx context.Context, opts recording.ListOptions) ([]recording.Segment, int, error)
	GetSegment(ctx context.Context, id string) (*recording.Segment, error)
	DeleteSegment(ctx context.Context, id string) error
	StartCamera(cameraID string) error
	StopCamera(cameraID string) error
	RestartCamera(cameraID string) error
	GetRecorderStatus(cameraID string) (*recording.RecorderStatus, error)
	GetAllRecorderStatus() map[string]*recording.RecorderStatus
	GetStorageStats(ctx context.Context) (*recording.StorageStats, error)
	GetPlaybackInfo(ctx context.Context, cameraID string, timestamp time.Time) (string, float64, error)
	ExportSegments(ctx context.Context, cameraID string, start, end time.Time, outputPath string) error
	RunRetention(ctx context.Context) (*recording.RetentionStats, error)
	GetTimeline(ctx context.Context, cameraID string, start, end time.Time) (*recording.Timeline, error)
	GetTimelineSegments(ctx context.Context, cameraID string, start, end time.Time) ([]*recording.TimelineSegment, error)
	GenerateThumbnail(ctx context.Context, segmentID string) (string, error)
}

// RecordingHandler handles recording API endpoints
type RecordingHandler struct {
	service RecordingService
}

// NewRecordingHandler creates a new recording handler
func NewRecordingHandler(service RecordingService) *RecordingHandler {
	return &RecordingHandler{
		service: service,
	}
}

// Routes returns the recording routes
func (h *RecordingHandler) Routes() chi.Router {
	r := chi.NewRouter()

	// Segments
	r.Get("/", h.ListSegments)
	r.Get("/{id}", h.GetSegment)
	r.Get("/{id}/stream", h.StreamSegment)
	r.Get("/{id}/download", h.DownloadSegment)
	r.Get("/{id}/thumbnail", h.GetThumbnail)
	r.Delete("/{id}", h.DeleteSegment)

	// Timeline
	r.Get("/timeline/{cameraId}", h.GetTimeline)
	r.Get("/timeline/{cameraId}/segments", h.GetTimelineSegments)
	r.Get("/timeline/{cameraId}/stream", h.StreamFromTimestamp)

	// Camera recording control
	r.Post("/cameras/{cameraId}/start", h.StartRecording)
	r.Post("/cameras/{cameraId}/stop", h.StopRecording)
	r.Post("/cameras/{cameraId}/restart", h.RestartRecording)

	// Status
	r.Get("/status", h.GetAllStatus)
	r.Get("/status/{cameraId}", h.GetStatus)
	r.Get("/storage", h.GetStorageStats)

	// Playback
	r.Get("/playback/{cameraId}", h.GetPlaybackInfo)

	// Export
	r.Post("/export", h.ExportSegments)

	// Retention
	r.Post("/retention/run", h.RunRetention)

	return r
}

// ListSegments lists recorded segments with filtering
func (h *RecordingHandler) ListSegments(w http.ResponseWriter, r *http.Request) {
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
	if v := r.URL.Query().Get("has_events"); v != "" {
		hasEvents := v == "true"
		opts.HasEvents = &hasEvents
	}
	if v := r.URL.Query().Get("order_by"); v != "" {
		opts.OrderBy = v
	}
	if v := r.URL.Query().Get("order_desc"); v == "true" {
		opts.OrderDesc = true
	}

	segments, total, err := h.service.ListSegments(ctx, opts)
	if err != nil {
		InternalError(w, err.Error())
		return
	}

	page := (opts.Offset / opts.Limit) + 1
	List(w, segments, total, page, opts.Limit)
}

// GetSegment returns a specific segment
func (h *RecordingHandler) GetSegment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	segment, err := h.service.GetSegment(ctx, id)
	if err != nil {
		NotFound(w, "Segment not found")
		return
	}

	OK(w, segment)
}

// DeleteSegment deletes a segment
func (h *RecordingHandler) DeleteSegment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if err := h.service.DeleteSegment(ctx, id); err != nil {
		NotFound(w, "Segment not found")
		return
	}

	NoContent(w)
}

// GetTimeline returns timeline data for a camera
func (h *RecordingHandler) GetTimeline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cameraID := chi.URLParam(r, "cameraId")

	// Parse time range
	start, end, err := parseTimeRange(r)
	if err != nil {
		BadRequest(w, err.Error())
		return
	}

	timeline, err := h.service.GetTimeline(ctx, cameraID, start, end)
	if err != nil {
		InternalError(w, err.Error())
		return
	}

	OK(w, timeline)
}

// GetTimelineSegments returns timeline segments for a camera
func (h *RecordingHandler) GetTimelineSegments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cameraID := chi.URLParam(r, "cameraId")

	start, end, err := parseTimeRange(r)
	if err != nil {
		BadRequest(w, err.Error())
		return
	}

	segments, err := h.service.GetTimelineSegments(ctx, cameraID, start, end)
	if err != nil {
		InternalError(w, err.Error())
		return
	}

	OK(w, map[string]interface{}{
		"camera_id": cameraID,
		"segments":  segments,
	})
}

// StartRecording starts recording for a camera
func (h *RecordingHandler) StartRecording(w http.ResponseWriter, r *http.Request) {
	cameraID := chi.URLParam(r, "cameraId")

	if err := h.service.StartCamera(cameraID); err != nil {
		InternalError(w, err.Error())
		return
	}

	OK(w, map[string]string{
		"message":   "Recording started",
		"camera_id": cameraID,
	})
}

// StopRecording stops recording for a camera
func (h *RecordingHandler) StopRecording(w http.ResponseWriter, r *http.Request) {
	cameraID := chi.URLParam(r, "cameraId")

	if err := h.service.StopCamera(cameraID); err != nil {
		InternalError(w, err.Error())
		return
	}

	OK(w, map[string]string{
		"message":   "Recording stopped",
		"camera_id": cameraID,
	})
}

// RestartRecording restarts recording for a camera
func (h *RecordingHandler) RestartRecording(w http.ResponseWriter, r *http.Request) {
	cameraID := chi.URLParam(r, "cameraId")

	if err := h.service.RestartCamera(cameraID); err != nil {
		InternalError(w, err.Error())
		return
	}

	OK(w, map[string]string{
		"message":   "Recording restarted",
		"camera_id": cameraID,
	})
}

// GetStatus returns recorder status for a camera
func (h *RecordingHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	cameraID := chi.URLParam(r, "cameraId")

	status, err := h.service.GetRecorderStatus(cameraID)
	if err != nil {
		NotFound(w, "Camera not found")
		return
	}

	OK(w, status)
}

// GetAllStatus returns status of all recorders
func (h *RecordingHandler) GetAllStatus(w http.ResponseWriter, r *http.Request) {
	status := h.service.GetAllRecorderStatus()
	OK(w, status)
}

// GetStorageStats returns storage statistics
func (h *RecordingHandler) GetStorageStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	stats, err := h.service.GetStorageStats(ctx)
	if err != nil {
		InternalError(w, err.Error())
		return
	}

	OK(w, stats)
}

// GetPlaybackInfo returns playback information for a timestamp
func (h *RecordingHandler) GetPlaybackInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cameraID := chi.URLParam(r, "cameraId")

	// Parse timestamp
	timestampStr := r.URL.Query().Get("timestamp")
	if timestampStr == "" {
		BadRequest(w, "timestamp parameter is required")
		return
	}

	timestamp, err := time.Parse(time.RFC3339, timestampStr)
	if err != nil {
		BadRequest(w, "invalid timestamp format")
		return
	}

	filePath, offset, err := h.service.GetPlaybackInfo(ctx, cameraID, timestamp)
	if err != nil {
		NotFound(w, "No recording found for the specified timestamp")
		return
	}

	OK(w, map[string]interface{}{
		"file_path": filePath,
		"offset":    offset,
	})
}

// ExportRequest represents an export request
type ExportRequest struct {
	CameraID   string `json:"camera_id"`
	StartTime  string `json:"start_time"`
	EndTime    string `json:"end_time"`
	OutputPath string `json:"output_path,omitempty"`
}

// ExportSegments exports segments to a file
func (h *RecordingHandler) ExportSegments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req ExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Invalid request body")
		return
	}

	start, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		BadRequest(w, "Invalid start_time format")
		return
	}

	end, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		BadRequest(w, "Invalid end_time format")
		return
	}

	outputPath := req.OutputPath
	if outputPath == "" {
		outputPath = "/tmp/export_" + req.CameraID + "_" + start.Format("20060102_150405") + ".mp4"
	}

	if err := h.service.ExportSegments(ctx, req.CameraID, start, end, outputPath); err != nil {
		InternalError(w, err.Error())
		return
	}

	OK(w, map[string]string{
		"message":     "Export completed",
		"output_path": outputPath,
	})
}

// RunRetention runs retention cleanup
func (h *RecordingHandler) RunRetention(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	stats, err := h.service.RunRetention(ctx)
	if err != nil {
		InternalError(w, err.Error())
		return
	}

	OK(w, stats)
}

// StreamSegment streams a video segment with range request support
func (h *RecordingHandler) StreamSegment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	segment, err := h.service.GetSegment(ctx, id)
	if err != nil {
		NotFound(w, "Segment not found")
		return
	}

	// Check if file exists
	fileInfo, err := os.Stat(segment.FilePath)
	if err != nil {
		NotFound(w, "Recording file not found")
		return
	}

	// Open the file
	file, err := os.Open(segment.FilePath)
	if err != nil {
		InternalError(w, "Failed to open recording file")
		return
	}
	defer file.Close()

	// Set content type based on file extension
	contentType := "video/mp4"
	ext := strings.ToLower(filepath.Ext(segment.FilePath))
	switch ext {
	case ".mp4":
		contentType = "video/mp4"
	case ".mkv":
		contentType = "video/x-matroska"
	case ".webm":
		contentType = "video/webm"
	case ".ts":
		contentType = "video/mp2t"
	}

	// Handle range requests for video seeking
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
		BadRequest(w, "Invalid range header")
		return
	}

	var start, end int64
	if parts[0] != "" {
		start, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			BadRequest(w, "Invalid range start")
			return
		}
	}

	if parts[1] != "" {
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			BadRequest(w, "Invalid range end")
			return
		}
	} else {
		// If no end specified, serve to end of file (but limit chunk size for efficiency)
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
	if _, err := file.Seek(start, 0); err != nil {
		InternalError(w, "Failed to seek in file")
		return
	}

	// Set headers for partial content
	contentLength := end - start + 1
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileSize))
	w.Header().Set("Accept-Ranges", "bytes")
	w.WriteHeader(http.StatusPartialContent)

	// Copy the requested range
	io.CopyN(w, file, contentLength)
}

// DownloadSegment serves a video segment as a download
func (h *RecordingHandler) DownloadSegment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	segment, err := h.service.GetSegment(ctx, id)
	if err != nil {
		NotFound(w, "Segment not found")
		return
	}

	// Check if file exists
	fileInfo, err := os.Stat(segment.FilePath)
	if err != nil {
		NotFound(w, "Recording file not found")
		return
	}

	// Open the file
	file, err := os.Open(segment.FilePath)
	if err != nil {
		InternalError(w, "Failed to open recording file")
		return
	}
	defer file.Close()

	// Generate a nice filename
	filename := fmt.Sprintf("%s_%s.mp4",
		segment.CameraID,
		segment.StartTime.Format("2006-01-02_15-04-05"))

	// Set headers for download
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))
	w.WriteHeader(http.StatusOK)

	io.Copy(w, file)
}

// GetThumbnail serves the thumbnail image for a segment
func (h *RecordingHandler) GetThumbnail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	// Try to generate thumbnail if it doesn't exist
	thumbnailPath, err := h.service.GenerateThumbnail(ctx, id)
	if err != nil {
		NotFound(w, "Thumbnail not available: "+err.Error())
		return
	}

	// Check if file exists
	fileInfo, err := os.Stat(thumbnailPath)
	if err != nil {
		NotFound(w, "Thumbnail file not found")
		return
	}

	// Use the generated path
	segment := &struct{ Thumbnail string }{Thumbnail: thumbnailPath}

	// Open the file
	file, err := os.Open(segment.Thumbnail)
	if err != nil {
		InternalError(w, "Failed to open thumbnail file")
		return
	}
	defer file.Close()

	// Determine content type
	contentType := "image/jpeg"
	ext := strings.ToLower(filepath.Ext(segment.Thumbnail))
	switch ext {
	case ".png":
		contentType = "image/png"
	case ".webp":
		contentType = "image/webp"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))
	w.Header().Set("Cache-Control", "public, max-age=86400") // Cache for 24 hours
	w.WriteHeader(http.StatusOK)

	io.Copy(w, file)
}

// parseTimeRange parses start and end time from query parameters
func parseTimeRange(r *http.Request) (time.Time, time.Time, error) {
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	var start, end time.Time
	var err error

	if startStr != "" {
		start, err = time.Parse(time.RFC3339, startStr)
		if err != nil {
			return start, end, err
		}
	} else {
		start = time.Now().Add(-24 * time.Hour)
	}

	if endStr != "" {
		end, err = time.Parse(time.RFC3339, endStr)
		if err != nil {
			return start, end, err
		}
	} else {
		end = time.Now()
	}

	return start, end, nil
}

// StreamFromTimestamp streams video starting from a specific timestamp
// This provides seamless playback across segment boundaries
func (h *RecordingHandler) StreamFromTimestamp(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cameraID := chi.URLParam(r, "cameraId")

	// Parse timestamp
	timestampStr := r.URL.Query().Get("t")
	if timestampStr == "" {
		BadRequest(w, "timestamp parameter 't' is required")
		return
	}

	timestamp, err := time.Parse(time.RFC3339, timestampStr)
	if err != nil {
		// Try Unix timestamp
		if unix, err2 := strconv.ParseInt(timestampStr, 10, 64); err2 == nil {
			timestamp = time.Unix(unix, 0)
		} else {
			BadRequest(w, "invalid timestamp format")
			return
		}
	}

	// Get playback info for the timestamp
	filePath, offset, err := h.service.GetPlaybackInfo(ctx, cameraID, timestamp)
	if err != nil {
		NotFound(w, "No recording found for the specified timestamp")
		return
	}

	// Check if file exists
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		NotFound(w, "Recording file not found")
		return
	}

	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		InternalError(w, "Failed to open recording file")
		return
	}
	defer file.Close()

	// For now, we stream the segment that contains the timestamp
	// TODO: Implement on-the-fly segment stitching for seamless playback
	// The frontend can handle segment transitions by tracking time and requesting next segment

	// Set content type
	contentType := "video/mp4"
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".mkv":
		contentType = "video/x-matroska"
	case ".webm":
		contentType = "video/webm"
	case ".ts":
		contentType = "video/mp2t"
	}

	// Handle range requests for seeking
	fileSize := fileInfo.Size()
	rangeHeader := r.Header.Get("Range")

	// Add custom headers for timeline info
	w.Header().Set("X-Segment-Start", timestamp.Format(time.RFC3339))
	w.Header().Set("X-Segment-Offset", fmt.Sprintf("%.3f", offset))

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
		BadRequest(w, "Invalid range header")
		return
	}

	var start, end int64
	if parts[0] != "" {
		start, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			BadRequest(w, "Invalid range start")
			return
		}
	}

	if parts[1] != "" {
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			BadRequest(w, "Invalid range end")
			return
		}
	} else {
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
	if _, err := file.Seek(start, 0); err != nil {
		InternalError(w, "Failed to seek in file")
		return
	}

	// Set headers for partial content
	contentLength := end - start + 1
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileSize))
	w.Header().Set("Accept-Ranges", "bytes")
	w.WriteHeader(http.StatusPartialContent)

	io.CopyN(w, file, contentLength)
}
