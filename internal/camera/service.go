// Package camera provides camera management functionality
package camera

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/Spatial-NVR/SpatialNVR/internal/config"
	"github.com/Spatial-NVR/SpatialNVR/internal/database"
	"github.com/Spatial-NVR/SpatialNVR/internal/streaming"
)

// Status represents camera status
type Status string

const (
	StatusOnline   Status = "online"
	StatusOffline  Status = "offline"
	StatusError    Status = "error"
	StatusStarting Status = "starting"
)

// Camera represents a camera's current state
type Camera struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	Status             Status          `json:"status"`
	Enabled            bool            `json:"enabled"`
	Manufacturer       string          `json:"manufacturer,omitempty"`
	Model              string          `json:"model,omitempty"`
	StreamURL          string          `json:"stream_url,omitempty"`
	DisplayAspectRatio string          `json:"display_aspect_ratio,omitempty"`
	LastSeen           *time.Time      `json:"last_seen,omitempty"`
	FPSCurrent         *float64        `json:"fps_current,omitempty"`
	BitrateCurrent     *int            `json:"bitrate_current,omitempty"`
	ResolutionCurrent  string          `json:"resolution_current,omitempty"`
	ErrorMessage       string          `json:"error_message,omitempty"`
	Stats              json.RawMessage `json:"stats,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

// Service manages cameras
type Service struct {
	db        *database.DB
	cfg       *config.Config
	go2rtc    *streaming.Go2RTCManager
	logger    *slog.Logger
	mu        sync.RWMutex
	stopChan  chan struct{}
	cameras   map[string]*Camera
}

// NewService creates a new camera service
func NewService(db *database.DB, cfg *config.Config, go2rtc *streaming.Go2RTCManager) *Service {
	return &Service{
		db:       db,
		cfg:      cfg,
		go2rtc:   go2rtc,
		logger:   slog.Default().With("component", "camera-service"),
		stopChan: make(chan struct{}),
		cameras:  make(map[string]*Camera),
	}
}

// Start starts the camera service
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting camera service")

	// Load cameras from config
	if err := s.syncFromConfig(ctx); err != nil {
		return fmt.Errorf("failed to sync cameras from config: %w", err)
	}

	// Start health monitoring
	go s.healthMonitor(ctx)

	// Proactively warm up all camera streams for instant playback
	go s.warmupStreams(ctx)

	return nil
}

// Stop stops the camera service
func (s *Service) Stop() {
	close(s.stopChan)
}

// syncFromConfig syncs cameras from config to database
func (s *Service) syncFromConfig(ctx context.Context) error {
	for _, camCfg := range s.cfg.Cameras {
		cam := &Camera{
			ID:                 camCfg.ID,
			Name:               camCfg.Name,
			Status:             StatusOffline,
			Enabled:            camCfg.Enabled,
			Manufacturer:       camCfg.Manufacturer,
			Model:              camCfg.Model,
			StreamURL:          camCfg.Stream.URL,
			DisplayAspectRatio: camCfg.DisplayAspectRatio,
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
		}

		if err := s.upsertCameraDB(ctx, cam); err != nil {
			s.logger.Error("Failed to sync camera to database", "camera", cam.ID, "error", err)
			continue
		}

		s.mu.Lock()
		s.cameras[cam.ID] = cam
		s.mu.Unlock()
	}

	// Update go2rtc configuration
	return s.updateGo2RTCConfig()
}

// List returns all cameras
func (s *Service) List(ctx context.Context) ([]*Camera, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, status, last_seen, fps_current, bitrate_current, resolution_current,
		       stats, error_message, created_at, updated_at
		FROM cameras
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cameras []*Camera
	for rows.Next() {
		cam := &Camera{}
		var lastSeen sql.NullInt64
		var fpsCurrent sql.NullFloat64
		var bitrateCurrent sql.NullInt64
		var resolution sql.NullString
		var stats sql.NullString
		var errorMsg sql.NullString
		var createdAt, updatedAt int64

		if err := rows.Scan(
			&cam.ID, &cam.Status, &lastSeen, &fpsCurrent, &bitrateCurrent,
			&resolution, &stats, &errorMsg, &createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}

		if lastSeen.Valid {
			t := time.Unix(lastSeen.Int64, 0)
			cam.LastSeen = &t
		}
		if fpsCurrent.Valid {
			cam.FPSCurrent = &fpsCurrent.Float64
		}
		if bitrateCurrent.Valid {
			i := int(bitrateCurrent.Int64)
			cam.BitrateCurrent = &i
		}
		cam.ResolutionCurrent = resolution.String
		if stats.Valid {
			cam.Stats = json.RawMessage(stats.String)
		}
		cam.ErrorMessage = errorMsg.String
		cam.CreatedAt = time.Unix(createdAt, 0)
		cam.UpdatedAt = time.Unix(updatedAt, 0)

		// Get name and other config from config file
		if cfgCam := s.cfg.GetCamera(cam.ID); cfgCam != nil {
			cam.Name = cfgCam.Name
			cam.Enabled = cfgCam.Enabled
			cam.Manufacturer = cfgCam.Manufacturer
			cam.Model = cfgCam.Model
			cam.StreamURL = cfgCam.Stream.URL
		}

		cameras = append(cameras, cam)
	}

	return cameras, rows.Err()
}

// Get returns a camera by ID
func (s *Service) Get(ctx context.Context, id string) (*Camera, error) {
	cam := &Camera{ID: id}
	var lastSeen sql.NullInt64
	var fpsCurrent sql.NullFloat64
	var bitrateCurrent sql.NullInt64
	var resolution sql.NullString
	var stats sql.NullString
	var errorMsg sql.NullString
	var createdAt, updatedAt int64

	err := s.db.QueryRowContext(ctx, `
		SELECT status, last_seen, fps_current, bitrate_current, resolution_current,
		       stats, error_message, created_at, updated_at
		FROM cameras WHERE id = ?
	`, id).Scan(
		&cam.Status, &lastSeen, &fpsCurrent, &bitrateCurrent,
		&resolution, &stats, &errorMsg, &createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("camera not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	if lastSeen.Valid {
		t := time.Unix(lastSeen.Int64, 0)
		cam.LastSeen = &t
	}
	if fpsCurrent.Valid {
		cam.FPSCurrent = &fpsCurrent.Float64
	}
	if bitrateCurrent.Valid {
		i := int(bitrateCurrent.Int64)
		cam.BitrateCurrent = &i
	}
	cam.ResolutionCurrent = resolution.String
	if stats.Valid {
		cam.Stats = json.RawMessage(stats.String)
	}
	cam.ErrorMessage = errorMsg.String
	cam.CreatedAt = time.Unix(createdAt, 0)
	cam.UpdatedAt = time.Unix(updatedAt, 0)

	// Get config data
	if cfgCam := s.cfg.GetCamera(id); cfgCam != nil {
		cam.Name = cfgCam.Name
		cam.Enabled = cfgCam.Enabled
		cam.Manufacturer = cfgCam.Manufacturer
		cam.Model = cfgCam.Model
		cam.StreamURL = cfgCam.Stream.URL
	}

	return cam, nil
}

// Create creates a new camera
func (s *Service) Create(ctx context.Context, camCfg config.CameraConfig) (*Camera, error) {
	// Generate ID if not provided
	if camCfg.ID == "" {
		camCfg.ID = generateCameraID(camCfg.Name)
	}

	// Set defaults for new cameras
	camCfg.Enabled = true

	// Enable detection by default with sensible defaults
	if !camCfg.Detection.Enabled {
		camCfg.Detection.Enabled = true
	}
	if camCfg.Detection.FPS == 0 {
		camCfg.Detection.FPS = 5
	}

	// Enable recording by default
	if !camCfg.Recording.Enabled {
		camCfg.Recording.Enabled = true
	}
	if camCfg.Recording.PreBufferSeconds == 0 {
		camCfg.Recording.PreBufferSeconds = 5
	}
	if camCfg.Recording.PostBufferSeconds == 0 {
		camCfg.Recording.PostBufferSeconds = 5
	}
	if camCfg.Recording.Mode == "" {
		camCfg.Recording.Mode = "continuous"
	}
	if camCfg.Recording.SegmentDuration == 0 {
		camCfg.Recording.SegmentDuration = 300 // 5 minutes
	}
	if camCfg.Recording.Retention.DefaultDays == 0 {
		camCfg.Recording.Retention.DefaultDays = 7
	}

	// Enable audio by default
	if !camCfg.Audio.Enabled {
		camCfg.Audio.Enabled = true
	}

	// Enable motion detection by default
	if !camCfg.Motion.Enabled {
		camCfg.Motion.Enabled = true
		camCfg.Motion.Method = "frame_diff"
		camCfg.Motion.Threshold = 0.02
	}

	// Add to config
	if err := s.cfg.UpsertCamera(camCfg); err != nil {
		return nil, fmt.Errorf("failed to save camera config: %w", err)
	}

	// Create in database
	cam := &Camera{
		ID:                 camCfg.ID,
		Name:               camCfg.Name,
		Status:             StatusStarting,
		Enabled:            camCfg.Enabled,
		Manufacturer:       camCfg.Manufacturer,
		Model:              camCfg.Model,
		StreamURL:          camCfg.Stream.URL,
		DisplayAspectRatio: camCfg.DisplayAspectRatio,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	if err := s.upsertCameraDB(ctx, cam); err != nil {
		return nil, err
	}

	// Update go2rtc
	if err := s.updateGo2RTCConfig(); err != nil {
		s.logger.Error("Failed to update go2rtc config", "error", err)
	}

	s.mu.Lock()
	s.cameras[cam.ID] = cam
	s.mu.Unlock()

	return cam, nil
}

// Update updates a camera (legacy method, uses UpdateWithFields internally)
func (s *Service) Update(ctx context.Context, id string, camCfg config.CameraConfig) (*Camera, error) {
	return s.UpdateWithFields(ctx, id, camCfg, nil)
}

// UpdateWithFields updates a camera with knowledge of which fields were explicitly set
func (s *Service) UpdateWithFields(ctx context.Context, id string, camCfg config.CameraConfig, presentFields map[string]json.RawMessage) (*Camera, error) {
	// Get existing camera first
	existing, err := s.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("camera not found: %w", err)
	}

	// Get existing config to preserve settings
	existingCfg := s.cfg.GetCamera(id)

	// Start with existing config as base, then apply updates
	// This ensures we don't lose nested config like Recording, Detection, etc.
	var mergedCfg config.CameraConfig
	if existingCfg != nil {
		mergedCfg = *existingCfg
	}

	// Always set the ID
	mergedCfg.ID = id

	// Helper to check if a field was present in the request
	fieldPresent := func(name string) bool {
		if presentFields == nil {
			return false
		}
		_, ok := presentFields[name]
		return ok
	}

	// Update basic fields if provided in the request
	if fieldPresent("name") || camCfg.Name != "" {
		mergedCfg.Name = camCfg.Name
	}
	if mergedCfg.Name == "" {
		mergedCfg.Name = existing.Name
	}

	if fieldPresent("stream") || camCfg.Stream.URL != "" {
		mergedCfg.Stream = camCfg.Stream
	}
	if mergedCfg.Stream.URL == "" {
		mergedCfg.Stream.URL = existing.StreamURL
	}

	if fieldPresent("manufacturer") || camCfg.Manufacturer != "" {
		mergedCfg.Manufacturer = camCfg.Manufacturer
	}
	if mergedCfg.Manufacturer == "" {
		mergedCfg.Manufacturer = existing.Manufacturer
	}

	if fieldPresent("model") || camCfg.Model != "" {
		mergedCfg.Model = camCfg.Model
	}
	if mergedCfg.Model == "" {
		mergedCfg.Model = existing.Model
	}

	if fieldPresent("display_aspect_ratio") || camCfg.DisplayAspectRatio != "" {
		mergedCfg.DisplayAspectRatio = camCfg.DisplayAspectRatio
	}
	if mergedCfg.DisplayAspectRatio == "" {
		mergedCfg.DisplayAspectRatio = existing.DisplayAspectRatio
	}

	// Handle boolean field - enabled at camera level
	if fieldPresent("enabled") {
		mergedCfg.Enabled = camCfg.Enabled
	}

	// Handle nested configs - only update if the field was present in request
	if fieldPresent("recording") {
		mergedCfg.Recording = camCfg.Recording
	}

	if fieldPresent("detection") {
		mergedCfg.Detection = camCfg.Detection
	}

	if fieldPresent("motion") {
		mergedCfg.Motion = camCfg.Motion
	}

	if fieldPresent("audio") {
		mergedCfg.Audio = camCfg.Audio
	}

	if fieldPresent("ptz") {
		mergedCfg.PTZ = camCfg.PTZ
	}

	if fieldPresent("advanced") {
		mergedCfg.Advanced = camCfg.Advanced
	}

	if fieldPresent("location") {
		mergedCfg.Location = camCfg.Location
	}

	// Update config file with merged config
	if err := s.cfg.UpsertCamera(mergedCfg); err != nil {
		return nil, fmt.Errorf("failed to update camera config: %w", err)
	}

	// Update database
	cam := &Camera{
		ID:                 id,
		Name:               mergedCfg.Name,
		Status:             existing.Status,
		Enabled:            mergedCfg.Enabled,
		Manufacturer:       mergedCfg.Manufacturer,
		Model:              mergedCfg.Model,
		StreamURL:          mergedCfg.Stream.URL,
		DisplayAspectRatio: mergedCfg.DisplayAspectRatio,
		CreatedAt:          existing.CreatedAt,
		UpdatedAt:          time.Now(),
	}

	if err := s.upsertCameraDB(ctx, cam); err != nil {
		return nil, fmt.Errorf("failed to update camera in database: %w", err)
	}

	// Update go2rtc config
	if err := s.updateGo2RTCConfig(); err != nil {
		s.logger.Error("Failed to update go2rtc config", "error", err)
	}

	// Update in-memory cache
	s.mu.Lock()
	s.cameras[cam.ID] = cam
	s.mu.Unlock()

	return cam, nil
}

// Delete deletes a camera from all storage locations
// It attempts to remove from all sources even if some fail
func (s *Service) Delete(ctx context.Context, id string) error {
	var errors []string

	// Remove from config (may fail if camera was only in DB)
	if err := s.cfg.RemoveCamera(id); err != nil {
		s.logger.Warn("Failed to remove camera from config", "id", id, "error", err)
		errors = append(errors, fmt.Sprintf("config: %v", err))
	}

	// Remove from database
	result, err := s.db.ExecContext(ctx, "DELETE FROM cameras WHERE id = ?", id)
	if err != nil {
		s.logger.Error("Failed to remove camera from database", "id", id, "error", err)
		errors = append(errors, fmt.Sprintf("database: %v", err))
	} else {
		rows, _ := result.RowsAffected()
		s.logger.Info("Deleted camera from database", "id", id, "rows_affected", rows)
	}

	// Remove from memory
	s.mu.Lock()
	_, existed := s.cameras[id]
	delete(s.cameras, id)
	s.mu.Unlock()

	if existed {
		s.logger.Info("Removed camera from memory", "id", id)
	}

	// Update go2rtc config
	if err := s.updateGo2RTCConfig(); err != nil {
		s.logger.Error("Failed to update go2rtc config", "error", err)
	}

	// Return error only if we couldn't delete from ANY source
	if len(errors) > 0 && !existed {
		// Camera wasn't in memory either - truly not found
		return fmt.Errorf("camera not found in any storage: %s", id)
	}

	return nil
}

// GetConfig returns the full configuration for a camera
func (s *Service) GetConfig(ctx context.Context, id string) (*config.CameraConfig, error) {
	cfg := s.cfg.GetCamera(id)
	if cfg == nil {
		return nil, fmt.Errorf("camera not found: %s", id)
	}
	return cfg, nil
}

// GetSnapshot returns a snapshot from a camera
func (s *Service) GetSnapshot(ctx context.Context, id string) ([]byte, error) {
	// Get snapshot URL from go2rtc
	var url string
	if s.go2rtc != nil {
		// Use go2rtc manager's API URL
		streamName := strings.ToLower(strings.ReplaceAll(id, "-", "_"))
		url = fmt.Sprintf("%s/api/frame.jpeg?src=%s", s.go2rtc.APIURL(), streamName)
	} else {
		url = streaming.GetStreamURL(id, "mjpeg", streaming.DefaultGo2RTCPort)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get snapshot: %s", resp.Status)
	}

	// Read full image data with a limit
	limitedReader := io.LimitReader(resp.Body, 10*1024*1024) // 10MB max
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read snapshot: %w", err)
	}

	return data, nil
}

// upsertCameraDB inserts or updates a camera in the database
func (s *Service) upsertCameraDB(ctx context.Context, cam *Camera) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO cameras (id, status, last_seen, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status = excluded.status,
			updated_at = excluded.updated_at
	`, cam.ID, cam.Status, time.Now().Unix(), cam.CreatedAt.Unix(), cam.UpdatedAt.Unix())
	return err
}

// updateGo2RTCConfig updates the go2rtc configuration
func (s *Service) updateGo2RTCConfig() error {
	var streams []streaming.CameraStream

	for _, cam := range s.cfg.Cameras {
		if !cam.Enabled {
			continue
		}
		streams = append(streams, streaming.CameraStream{
			ID:       cam.ID,
			Name:     cam.Name,
			URL:      cam.Stream.URL,
			Username: cam.Stream.Username,
			Password: cam.Stream.Password,
			SubURL:   cam.Stream.SubURL,
		})
	}

	generator := streaming.NewConfigGenerator()
	config := generator.Generate(streams)

	configPath := s.cfg.System.StoragePath + "/go2rtc.yaml"
	if err := generator.WriteToFile(config, configPath); err != nil {
		return err
	}

	// Reload go2rtc if running
	if s.go2rtc != nil && s.go2rtc.IsRunning() {
		return s.go2rtc.Reload()
	}

	return nil
}

// healthMonitor monitors camera health
func (s *Service) healthMonitor(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.checkCameraHealth(ctx)
		}
	}
}

// warmupStreams proactively starts all camera streams for instant playback
// This connects to all cameras immediately so video is available instantly when users view them
func (s *Service) warmupStreams(ctx context.Context) {
	// Wait a bit for go2rtc to fully initialize
	time.Sleep(3 * time.Second)

	s.logger.Info("Warming up camera streams for instant playback")

	s.mu.RLock()
	cameras := make([]*Camera, 0, len(s.cameras))
	for _, cam := range s.cameras {
		cameras = append(cameras, cam)
	}
	s.mu.RUnlock()

	// Start each camera stream
	for _, cam := range cameras {
		if err := s.startCameraStream(ctx, cam.ID); err != nil {
			s.logger.Warn("Failed to warm up camera stream", "camera", cam.ID, "error", err)
		} else {
			s.logger.Info("Camera stream warmed up", "camera", cam.ID)
		}
	}

	// Keep streams alive with periodic keep-alive pings
	go s.streamKeepAlive(ctx)
}

// startCameraStream requests go2rtc to start a camera stream by requesting a frame
func (s *Service) startCameraStream(ctx context.Context, cameraID string) error {
	// go2rtc uses lowercase stream names with underscores
	streamName := strings.ToLower(strings.ReplaceAll(cameraID, "-", "_"))

	// Request a single JPEG frame - this forces go2rtc to connect to the source
	// The frame endpoint is more reliable at triggering stream startup than just querying info
	var url string
	if s.go2rtc != nil {
		url = fmt.Sprintf("%s/api/frame.jpeg?src=%s", s.go2rtc.APIURL(), streamName)
	} else {
		url = fmt.Sprintf("http://localhost:%d/api/frame.jpeg?src=%s", streaming.DefaultGo2RTCPort, streamName)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read and discard the frame data
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("go2rtc frame request returned %d", resp.StatusCode)
	}

	return nil
}

// streamKeepAlive periodically touches streams to keep them active
func (s *Service) streamKeepAlive(ctx context.Context) {
	// Keep-alive every 60 seconds to prevent stream timeout
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.touchAllStreams(ctx)
		}
	}
}

// touchAllStreams pings all camera streams to keep them active
func (s *Service) touchAllStreams(ctx context.Context) {
	s.mu.RLock()
	cameras := make([]*Camera, 0, len(s.cameras))
	for _, cam := range s.cameras {
		cameras = append(cameras, cam)
	}
	s.mu.RUnlock()

	for _, cam := range cameras {
		_ = s.startCameraStream(ctx, cam.ID)
	}
}

// StreamStats holds stream statistics from go2rtc
type StreamStats struct {
	Producers []ProducerStats `json:"producers"`
	Consumers []ConsumerStats `json:"consumers"`
}

// ProducerStats holds producer (source) statistics
type ProducerStats struct {
	URL       string        `json:"url"`
	Recv      int64         `json:"bytes_recv"`
	Medias    []string      `json:"medias"` // go2rtc returns strings like "video, recvonly, H264"
	Tracks    []TrackStats  `json:"tracks,omitempty"` // detailed track info if available
}

// TrackStats holds detailed track statistics from go2rtc
type TrackStats struct {
	Codec  string `json:"codec,omitempty"`
	Type   string `json:"type,omitempty"`   // "video" or "audio"
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	FPS    float64 `json:"fps,omitempty"`
}

// ConsumerStats holds consumer statistics
type ConsumerStats struct {
	Send   int64    `json:"bytes_send"`
	Medias []string `json:"medias"`
}

// CameraHealth holds detailed health information for a camera
type CameraHealth struct {
	Status     Status   `json:"status"`
	FPS        float64  `json:"fps,omitempty"`
	Bitrate    int      `json:"bitrate,omitempty"`    // bits per second
	Resolution string   `json:"resolution,omitempty"`
	Codec      string   `json:"codec,omitempty"`
	BytesRecv  int64    `json:"bytes_recv,omitempty"`
	LastCheck  time.Time `json:"last_check"`
}

// checkCameraHealth checks the health of all cameras
func (s *Service) checkCameraHealth(ctx context.Context) {
	// Get stream stats from go2rtc
	streams, err := s.fetchGo2RTCStreams(ctx)
	if err != nil {
		s.logger.Warn("Failed to fetch go2rtc streams", "error", err)
		return
	}

	s.logger.Info("Checking camera health", "stream_count", len(streams), "camera_count", len(s.cameras))

	s.mu.RLock()
	cameras := make([]*Camera, 0, len(s.cameras))
	for _, cam := range s.cameras {
		cameras = append(cameras, cam)
	}
	s.mu.RUnlock()

	for _, cam := range cameras {
		health := s.checkSingleCameraHealth(ctx, cam.ID, streams)
		s.logger.Info("Camera health check result",
			"camera", cam.ID,
			"status", health.Status,
			"bytes_recv", health.BytesRecv,
			"codec", health.Codec)
		s.updateCameraHealth(ctx, cam.ID, health)
	}
}

// fetchGo2RTCStreams gets all stream stats from go2rtc
func (s *Service) fetchGo2RTCStreams(ctx context.Context) (map[string]StreamStats, error) {
	var url string
	if s.go2rtc != nil {
		url = fmt.Sprintf("%s/api/streams", s.go2rtc.APIURL())
	} else {
		url = fmt.Sprintf("http://localhost:%d/api/streams", streaming.DefaultGo2RTCPort)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("go2rtc returned status %d", resp.StatusCode)
	}

	var streams map[string]StreamStats
	if err := json.NewDecoder(resp.Body).Decode(&streams); err != nil {
		return nil, err
	}

	return streams, nil
}

// checkSingleCameraHealth checks health of a single camera
func (s *Service) checkSingleCameraHealth(ctx context.Context, id string, streams map[string]StreamStats) CameraHealth {
	health := CameraHealth{
		Status:    StatusOffline,
		LastCheck: time.Now(),
	}

	// Check if stream exists in go2rtc
	if streams == nil {
		return health
	}

	// go2rtc uses lowercase stream names with underscores
	streamName := strings.ToLower(strings.ReplaceAll(id, "-", "_"))
	stats, exists := streams[streamName]
	if !exists {
		s.logger.Info("Stream not found in go2rtc", "camera", id, "stream_name", streamName, "available", getStreamNames(streams))
		return health
	}

	// Check if we have active producers (sources)
	if len(stats.Producers) == 0 {
		health.Status = StatusOffline
		return health
	}

	// Get stats from the first producer
	producer := stats.Producers[0]
	health.Status = StatusOnline
	health.BytesRecv = producer.Recv

	// Extract video codec and resolution from tracks if available
	for _, track := range producer.Tracks {
		if track.Type == "video" || strings.HasPrefix(track.Codec, "H26") {
			if track.Codec != "" {
				health.Codec = track.Codec
			}
			if track.Width > 0 && track.Height > 0 {
				health.Resolution = fmt.Sprintf("%dx%d", track.Width, track.Height)
			}
			if track.FPS > 0 {
				health.FPS = track.FPS
			}
			break
		}
	}

	// Fallback: extract video codec from media strings (format: "video, recvonly, H264")
	if health.Codec == "" {
		for _, media := range producer.Medias {
			if strings.HasPrefix(media, "video") {
				parts := strings.Split(media, ", ")
				if len(parts) >= 3 {
					health.Codec = parts[2]
				}
				break
			}
		}
	}

	// Calculate bitrate from bytes received (approximate)
	if producer.Recv > 0 {
		health.Bitrate = int(producer.Recv * 8 / 30) // rough estimate over 30s
	}

	return health
}

// updateCameraHealth updates a camera's health in the database and memory
func (s *Service) updateCameraHealth(ctx context.Context, id string, health CameraHealth) {
	now := time.Now()

	// Only update if status changed or periodically update stats
	var fpsVal, bitrateVal interface{}
	var resolutionVal interface{}
	if health.FPS > 0 {
		fpsVal = health.FPS
	}
	if health.Bitrate > 0 {
		bitrateVal = health.Bitrate
	}
	if health.Resolution != "" {
		resolutionVal = health.Resolution
	}

	_, err := s.db.ExecContext(ctx, `
		UPDATE cameras
		SET status = ?,
		    last_seen = ?,
		    fps_current = ?,
		    bitrate_current = ?,
		    resolution_current = ?,
		    updated_at = ?
		WHERE id = ?
	`, health.Status, now.Unix(), fpsVal, bitrateVal, resolutionVal, now.Unix(), id)

	if err != nil {
		s.logger.Error("Failed to update camera health", "camera", id, "error", err)
		return
	}

	s.mu.Lock()
	if cam, ok := s.cameras[id]; ok {
		cam.Status = health.Status
		cam.LastSeen = &now
		if health.FPS > 0 {
			cam.FPSCurrent = &health.FPS
		}
		if health.Bitrate > 0 {
			cam.BitrateCurrent = &health.Bitrate
		}
		cam.ResolutionCurrent = health.Resolution
	}
	s.mu.Unlock()
}

// pingCamera checks if a camera is responding (simplified version)
func (s *Service) pingCamera(ctx context.Context, id string) Status {
	streams, err := s.fetchGo2RTCStreams(ctx)
	if err != nil {
		return StatusOffline
	}

	health := s.checkSingleCameraHealth(ctx, id, streams)
	return health.Status
}

// updateCameraStatus updates a camera's status in the database (legacy, use updateCameraHealth)
func (s *Service) updateCameraStatus(ctx context.Context, id string, status Status) {
	s.updateCameraHealth(ctx, id, CameraHealth{
		Status:    status,
		LastCheck: time.Now(),
	})
}

// generateCameraID generates a unique camera ID
func generateCameraID(name string) string {
	// Create a short unique ID
	uid := uuid.New().String()[:8]
	return fmt.Sprintf("%s_%s", sanitizeName(name), uid)
}

// sanitizeName sanitizes a name for use in IDs
func sanitizeName(name string) string {
	result := ""
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			result += string(c)
		} else if c == ' ' || c == '-' || c == '_' {
			result += "_"
		}
	}
	if len(result) > 20 {
		result = result[:20]
	}
	return result
}

// getStreamNames returns a list of stream names from a map
func getStreamNames(streams map[string]StreamStats) []string {
	names := make([]string, 0, len(streams))
	for name := range streams {
		names = append(names, name)
	}
	return names
}
