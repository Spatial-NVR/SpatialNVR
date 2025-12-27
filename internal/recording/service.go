package recording

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Spatial-NVR/SpatialNVR/internal/config"
)

// Service manages video recording for all cameras
type Service struct {
	mu              sync.RWMutex
	config          *config.Config
	db              *sql.DB
	repository      *SQLiteRepository
	segmentHandler  *DefaultSegmentHandler
	timelineBuilder *TimelineBuilder
	retentionPolicy *RetentionPolicy

	recorders     map[string]*Recorder
	storagePath   string
	thumbnailPath string
	running       bool
	ctx           context.Context
	cancel        context.CancelFunc
	logger        *slog.Logger
}

// ServiceConfig holds service configuration
type ServiceConfig struct {
	StoragePath       string
	ThumbnailPath     string
	RetentionInterval time.Duration
}

// NewService creates a new recording service
func NewService(cfg *config.Config, db *sql.DB, svcConfig ServiceConfig) (*Service, error) {
	// Set defaults
	if svcConfig.StoragePath == "" {
		svcConfig.StoragePath = filepath.Join(cfg.System.StoragePath, "recordings")
	}
	if svcConfig.ThumbnailPath == "" {
		svcConfig.ThumbnailPath = filepath.Join(cfg.System.StoragePath, "thumbnails")
	}
	if svcConfig.RetentionInterval == 0 {
		svcConfig.RetentionInterval = time.Hour
	}

	// Create directories
	for _, dir := range []string{svcConfig.StoragePath, svcConfig.ThumbnailPath} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	repository := NewSQLiteRepository(db)
	segmentHandler := NewDefaultSegmentHandler(svcConfig.StoragePath, svcConfig.ThumbnailPath)
	timelineBuilder := NewTimelineBuilder(repository)

	svc := &Service{
		config:          cfg,
		db:              db,
		repository:      repository,
		segmentHandler:  segmentHandler,
		timelineBuilder: timelineBuilder,
		recorders:       make(map[string]*Recorder),
		storagePath:     svcConfig.StoragePath,
		thumbnailPath:   svcConfig.ThumbnailPath,
		logger:          slog.Default().With("component", "recording_service"),
	}

	// Create retention policy
	svc.retentionPolicy = NewRetentionPolicy(cfg, repository, segmentHandler, svcConfig.StoragePath)

	return svc, nil
}

// Start starts the recording service
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	s.ctx, s.cancel = context.WithCancel(ctx)

	// Initialize schema
	if err := s.repository.InitSchema(ctx); err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Start retention policy
	if err := s.retentionPolicy.Start(s.ctx, time.Hour); err != nil {
		s.logger.Error("Failed to start retention policy", "error", err)
	}

	// Start recorders for enabled cameras
	for _, camera := range s.config.Cameras {
		if camera.Enabled && camera.Recording.Enabled {
			if err := s.startRecorder(camera); err != nil {
				s.logger.Error("Failed to start recorder", "camera", camera.ID, "error", err)
			}
		}
	}

	s.running = true
	s.logger.Info("Recording service started", "cameras", len(s.recorders))

	return nil
}

// Stop stops the recording service
func (s *Service) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	// Stop retention policy
	s.retentionPolicy.Stop()

	// Stop all recorders
	var wg sync.WaitGroup
	for id, recorder := range s.recorders {
		wg.Add(1)
		go func(id string, r *Recorder) {
			defer wg.Done()
			if err := r.Stop(); err != nil {
				s.logger.Error("Failed to stop recorder", "camera", id, "error", err)
			}
		}(id, recorder)
	}
	wg.Wait()

	if s.cancel != nil {
		s.cancel()
	}

	s.running = false
	s.logger.Info("Recording service stopped")

	return nil
}

// StartCamera starts recording for a specific camera
func (s *Service) StartCamera(cameraID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already running
	if _, exists := s.recorders[cameraID]; exists {
		return nil
	}

	// Find camera config
	camCfg := s.config.GetCamera(cameraID)
	if camCfg == nil {
		return fmt.Errorf("camera not found: %s", cameraID)
	}

	return s.startRecorder(*camCfg)
}

// StopCamera stops recording for a specific camera
func (s *Service) StopCamera(cameraID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	recorder, exists := s.recorders[cameraID]
	if !exists {
		return nil
	}

	if err := recorder.Stop(); err != nil {
		return err
	}

	delete(s.recorders, cameraID)
	return nil
}

// RestartCamera restarts recording for a specific camera
func (s *Service) RestartCamera(cameraID string) error {
	if err := s.StopCamera(cameraID); err != nil {
		return err
	}
	return s.StartCamera(cameraID)
}

// TriggerEventRecording triggers event-based recording
func (s *Service) TriggerEventRecording(cameraID, eventID string) error {
	s.mu.RLock()
	recorder, exists := s.recorders[cameraID]
	s.mu.RUnlock()

	if !exists {
		// Start recording if not already running
		if err := s.StartCamera(cameraID); err != nil {
			return err
		}
	}

	// Mark current segment as having events
	// This would be enhanced to use the ring buffer for pre-event footage
	s.logger.Info("Event recording triggered", "camera", cameraID, "event", eventID)

	_ = recorder // TODO: Implement event marking on current segment
	return nil
}

// startRecorder starts a recorder for a camera (must hold lock)
func (s *Service) startRecorder(camera config.CameraConfig) error {
	recorder := NewRecorder(
		camera.ID,
		&camera,
		s.storagePath,
		s.segmentHandler,
		s.onSegmentComplete,
	)

	if err := recorder.Start(s.ctx); err != nil {
		return err
	}

	s.recorders[camera.ID] = recorder
	s.logger.Info("Recorder started", "camera", camera.ID)

	return nil
}

// onSegmentComplete handles completed segment callbacks
func (s *Service) onSegmentComplete(segment *Segment) {
	ctx := context.Background()

	// Generate thumbnail
	if thumbPath, err := s.segmentHandler.GenerateThumbnailAuto(segment.FilePath); err == nil {
		segment.Thumbnail = thumbPath
	} else {
		s.logger.Warn("Failed to generate thumbnail", "segment", segment.ID, "error", err)
	}

	// Save to database
	if err := s.repository.Create(ctx, segment); err != nil {
		s.logger.Error("Failed to save segment", "segment", segment.ID, "error", err)
		return
	}

	s.logger.Debug("Segment saved", "id", segment.ID, "camera", segment.CameraID)
}

// GetSegment returns a segment by ID
func (s *Service) GetSegment(ctx context.Context, id string) (*Segment, error) {
	return s.repository.Get(ctx, id)
}

// ListSegments lists segments with filtering
func (s *Service) ListSegments(ctx context.Context, opts ListOptions) ([]Segment, int, error) {
	return s.repository.List(ctx, opts)
}

// DeleteSegment deletes a segment
func (s *Service) DeleteSegment(ctx context.Context, id string) error {
	segment, err := s.repository.Get(ctx, id)
	if err != nil {
		return err
	}

	// Delete file
	if err := s.segmentHandler.Delete(segment); err != nil {
		s.logger.Warn("Failed to delete segment file", "id", id, "error", err)
	}

	// Delete from database
	return s.repository.Delete(ctx, id)
}

// GetTimeline returns timeline data for a camera
func (s *Service) GetTimeline(ctx context.Context, cameraID string, start, end time.Time) (*Timeline, error) {
	return s.timelineBuilder.BuildTimeline(ctx, cameraID, start, end)
}

// GetTimelineSegments returns timeline segments for a camera
func (s *Service) GetTimelineSegments(ctx context.Context, cameraID string, start, end time.Time) ([]TimelineSegment, error) {
	return s.timelineBuilder.GetTimelineSegments(ctx, cameraID, start, end)
}

// GetRecorderStatus returns the status of a camera recorder
func (s *Service) GetRecorderStatus(cameraID string) (*RecorderStatus, error) {
	s.mu.RLock()
	recorder, exists := s.recorders[cameraID]
	s.mu.RUnlock()

	if !exists {
		return &RecorderStatus{
			CameraID: cameraID,
			State:    RecorderStateIdle,
		}, nil
	}

	return recorder.Status(), nil
}

// GetAllRecorderStatus returns status of all recorders
func (s *Service) GetAllRecorderStatus() map[string]*RecorderStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*RecorderStatus)
	for id, recorder := range s.recorders {
		result[id] = recorder.Status()
	}
	return result
}

// GetStorageStats returns storage statistics
func (s *Service) GetStorageStats(ctx context.Context) (*StorageStats, error) {
	byCamera, err := s.repository.GetStorageByCamera(ctx)
	if err != nil {
		return nil, err
	}

	byTier, err := s.repository.GetStorageByTier(ctx)
	if err != nil {
		return nil, err
	}

	var totalUsed int64
	var segmentCount int
	for _, size := range byCamera {
		totalUsed += size
	}

	// Count segments
	_, count, err := s.repository.List(ctx, ListOptions{Limit: 0})
	if err == nil {
		segmentCount = count
	}

	// Get disk space info
	var totalBytes, availableBytes int64
	// Note: Would use syscall.Statfs on Unix systems

	return &StorageStats{
		TotalBytes:     totalBytes,
		UsedBytes:      totalUsed,
		AvailableBytes: availableBytes,
		SegmentCount:   segmentCount,
		ByCamera:       byCamera,
		ByTier:         byTier,
	}, nil
}

// RunRetention runs retention cleanup manually
func (s *Service) RunRetention(ctx context.Context) (*RetentionStats, error) {
	return s.retentionPolicy.RunCleanup(ctx)
}

// GetPlaybackInfo returns playback information for a timestamp
func (s *Service) GetPlaybackInfo(ctx context.Context, cameraID string, timestamp time.Time) (string, float64, error) {
	return s.timelineBuilder.GetPlaybackURL(ctx, cameraID, timestamp)
}

// ExportSegments exports segments to a file
func (s *Service) ExportSegments(ctx context.Context, cameraID string, start, end time.Time, outputPath string) error {
	segments, err := s.repository.GetByTimeRange(ctx, cameraID, start, end)
	if err != nil {
		return err
	}

	if len(segments) == 0 {
		return fmt.Errorf("no segments found in the specified range")
	}

	// Collect segment paths
	var paths []string
	for _, seg := range segments {
		paths = append(paths, seg.FilePath)
	}

	// Merge segments
	return s.segmentHandler.MergeSegments(paths, outputPath)
}

// GenerateThumbnail generates a thumbnail for a segment if it doesn't exist
func (s *Service) GenerateThumbnail(ctx context.Context, id string) (string, error) {
	segment, err := s.repository.Get(ctx, id)
	if err != nil {
		return "", err
	}

	// If thumbnail already exists and file is present, return it
	if segment.Thumbnail != "" {
		if _, err := os.Stat(segment.Thumbnail); err == nil {
			return segment.Thumbnail, nil
		}
	}

	// Check if segment file exists
	if _, err := os.Stat(segment.FilePath); err != nil {
		return "", fmt.Errorf("segment file not found: %w", err)
	}

	// Generate thumbnail
	thumbPath, err := s.segmentHandler.GenerateThumbnailAuto(segment.FilePath)
	if err != nil {
		return "", fmt.Errorf("failed to generate thumbnail: %w", err)
	}

	// Update segment with thumbnail path
	segment.Thumbnail = thumbPath
	if err := s.repository.Update(ctx, segment); err != nil {
		s.logger.Warn("Failed to update segment with thumbnail", "id", id, "error", err)
	}

	return thumbPath, nil
}

// NewServiceFromPlugin creates a recording service for plugin mode
// In plugin mode, camera configs come via events instead of a config file
func NewServiceFromPlugin(db *sql.DB, svcConfig ServiceConfig) (*Service, error) {
	// Set defaults
	if svcConfig.StoragePath == "" {
		svcConfig.StoragePath = "/data/recordings"
	}
	if svcConfig.ThumbnailPath == "" {
		svcConfig.ThumbnailPath = "/data/thumbnails"
	}
	if svcConfig.RetentionInterval == 0 {
		svcConfig.RetentionInterval = time.Hour
	}

	// Create directories
	for _, dir := range []string{svcConfig.StoragePath, svcConfig.ThumbnailPath} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	repository := NewSQLiteRepository(db)
	segmentHandler := NewDefaultSegmentHandler(svcConfig.StoragePath, svcConfig.ThumbnailPath)
	timelineBuilder := NewTimelineBuilder(repository)

	// Create an empty config for plugin mode
	cfg := &config.Config{
		Cameras: []config.CameraConfig{},
		Storage: config.StorageConfig{
			Retention: config.StorageRetention{
				DefaultDays: 30,
			},
		},
	}

	svc := &Service{
		config:          cfg,
		db:              db,
		repository:      repository,
		segmentHandler:  segmentHandler,
		timelineBuilder: timelineBuilder,
		recorders:       make(map[string]*Recorder),
		storagePath:     svcConfig.StoragePath,
		thumbnailPath:   svcConfig.ThumbnailPath,
		logger:          slog.Default().With("component", "recording_service"),
	}

	// Create retention policy
	svc.retentionPolicy = NewRetentionPolicy(cfg, repository, segmentHandler, svcConfig.StoragePath)

	return svc, nil
}

// OnConfigChange handles configuration changes
func (s *Service) OnConfigChange(cfg *config.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = cfg

	// Update recorders based on new config
	for _, camera := range cfg.Cameras {
		_, running := s.recorders[camera.ID]

		shouldRun := camera.Enabled && camera.Recording.Enabled

		if shouldRun && !running {
			// Start new recorder
			if err := s.startRecorder(camera); err != nil {
				s.logger.Error("Failed to start recorder on config change", "camera", camera.ID, "error", err)
			}
		} else if !shouldRun && running {
			// Stop recorder
			if recorder, exists := s.recorders[camera.ID]; exists {
				if err := recorder.Stop(); err != nil {
					s.logger.Error("Failed to stop recorder on config change", "camera", camera.ID, "error", err)
				}
				delete(s.recorders, camera.ID)
			}
		}
	}
}

// UpdateCameraConfig updates or adds a single camera's configuration
// This is used by the plugin mode to receive camera configs via events
func (s *Service) UpdateCameraConfig(camCfg config.CameraConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update config's camera list
	found := false
	for i, cam := range s.config.Cameras {
		if cam.ID == camCfg.ID {
			s.config.Cameras[i] = camCfg
			found = true
			break
		}
	}
	if !found {
		s.config.Cameras = append(s.config.Cameras, camCfg)
	}

	// Start or stop recorder based on new config
	_, running := s.recorders[camCfg.ID]
	shouldRun := camCfg.Enabled && camCfg.Recording.Enabled

	if shouldRun && !running {
		if err := s.startRecorder(camCfg); err != nil {
			s.logger.Error("Failed to start recorder after config update", "camera", camCfg.ID, "error", err)
		}
	} else if !shouldRun && running {
		if recorder, exists := s.recorders[camCfg.ID]; exists {
			if err := recorder.Stop(); err != nil {
				s.logger.Error("Failed to stop recorder after config update", "camera", camCfg.ID, "error", err)
			}
			delete(s.recorders, camCfg.ID)
		}
	} else if shouldRun && running {
		// Restart recorder if config changed for a running camera
		if recorder, exists := s.recorders[camCfg.ID]; exists {
			if err := recorder.Stop(); err != nil {
				s.logger.Warn("Failed to stop recorder for restart", "camera", camCfg.ID, "error", err)
			}
			delete(s.recorders, camCfg.ID)
		}
		if err := s.startRecorder(camCfg); err != nil {
			s.logger.Error("Failed to restart recorder after config update", "camera", camCfg.ID, "error", err)
		}
	}

	s.logger.Info("Camera config updated", "camera", camCfg.ID,
		"enabled", camCfg.Enabled,
		"recording_enabled", camCfg.Recording.Enabled,
		"running", s.recorders[camCfg.ID] != nil)
}

// RemoveCameraConfig removes a camera's configuration
func (s *Service) RemoveCameraConfig(cameraID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop recorder if running
	if recorder, exists := s.recorders[cameraID]; exists {
		if err := recorder.Stop(); err != nil {
			s.logger.Warn("Failed to stop recorder during removal", "camera", cameraID, "error", err)
		}
		delete(s.recorders, cameraID)
	}

	// Remove from config
	for i, cam := range s.config.Cameras {
		if cam.ID == cameraID {
			s.config.Cameras = append(s.config.Cameras[:i], s.config.Cameras[i+1:]...)
			break
		}
	}

	s.logger.Info("Camera config removed", "camera", cameraID)
}
