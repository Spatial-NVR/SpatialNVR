package recording

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Spatial-NVR/SpatialNVR/internal/config"
	_ "github.com/mattn/go-sqlite3"
)

func setupServiceTestDB(t *testing.T) (*sql.DB, string, func()) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	cleanup := func() {
		db.Close()
	}

	return db, tmpDir, cleanup
}

func testConfig(storagePath string) *config.Config {
	return &config.Config{
		System: config.SystemConfig{
			StoragePath:  storagePath,
			MaxStorageGB: 100,
		},
		Cameras: []config.CameraConfig{
			{
				ID:      "test_cam_1",
				Name:    "Test Camera 1",
				Enabled: true,
				Stream: config.StreamConfig{
					URL: "rtsp://localhost:554/stream1",
				},
				Recording: config.RecordingConfig{
					Enabled:         true,
					SegmentDuration: 60,
					Retention: config.RetentionConfig{
						DefaultDays: 7,
						EventsDays:  14,
					},
				},
			},
			{
				ID:      "test_cam_2",
				Name:    "Test Camera 2",
				Enabled: true,
				Stream: config.StreamConfig{
					URL: "rtsp://localhost:554/stream2",
				},
				Recording: config.RecordingConfig{
					Enabled:         false, // Recording disabled
					SegmentDuration: 60,
				},
			},
		},
	}
}

func TestNewService(t *testing.T) {
	db, tmpDir, cleanup := setupServiceTestDB(t)
	defer cleanup()

	cfg := testConfig(tmpDir)

	tests := []struct {
		name      string
		svcConfig ServiceConfig
		wantErr   bool
	}{
		{
			name:      "default config",
			svcConfig: ServiceConfig{},
			wantErr:   false,
		},
		{
			name: "custom paths",
			svcConfig: ServiceConfig{
				StoragePath:   filepath.Join(tmpDir, "recordings"),
				ThumbnailPath: filepath.Join(tmpDir, "thumbnails"),
			},
			wantErr: false,
		},
		{
			name: "custom retention interval",
			svcConfig: ServiceConfig{
				RetentionInterval: 30 * time.Minute,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := NewService(cfg, db, tt.svcConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewService() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && svc == nil {
				t.Error("NewService() returned nil service")
			}
		})
	}
}

func TestNewService_DirectoryCreation(t *testing.T) {
	db, tmpDir, cleanup := setupServiceTestDB(t)
	defer cleanup()

	cfg := testConfig(tmpDir)
	storagePath := filepath.Join(tmpDir, "nested", "recordings")
	thumbnailPath := filepath.Join(tmpDir, "nested", "thumbnails")

	svc, err := NewService(cfg, db, ServiceConfig{
		StoragePath:   storagePath,
		ThumbnailPath: thumbnailPath,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if svc == nil {
		t.Fatal("NewService() returned nil")
	}

	// Verify directories were created
	if _, err := os.Stat(storagePath); os.IsNotExist(err) {
		t.Error("Storage path was not created")
	}
	if _, err := os.Stat(thumbnailPath); os.IsNotExist(err) {
		t.Error("Thumbnail path was not created")
	}
}

func TestService_StartStop(t *testing.T) {
	db, tmpDir, cleanup := setupServiceTestDB(t)
	defer cleanup()

	cfg := testConfig(tmpDir)
	// Disable cameras to avoid FFmpeg dependency in tests
	for i := range cfg.Cameras {
		cfg.Cameras[i].Recording.Enabled = false
	}

	svc, err := NewService(cfg, db, ServiceConfig{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	ctx := context.Background()

	// Test Start
	if err := svc.Start(ctx); err != nil {
		t.Errorf("Start() error = %v", err)
	}

	// Starting again should be no-op
	if err := svc.Start(ctx); err != nil {
		t.Errorf("Start() second call error = %v", err)
	}

	// Test Stop
	if err := svc.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	// Stopping again should be no-op
	if err := svc.Stop(); err != nil {
		t.Errorf("Stop() second call error = %v", err)
	}
}

func TestService_StartCamera_NotFound(t *testing.T) {
	db, tmpDir, cleanup := setupServiceTestDB(t)
	defer cleanup()

	cfg := testConfig(tmpDir)
	svc, err := NewService(cfg, db, ServiceConfig{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer svc.Stop()

	// Try to start a camera that doesn't exist
	err = svc.StartCamera("nonexistent_camera")
	if err == nil {
		t.Error("StartCamera() should return error for non-existent camera")
	}
}

func TestService_StopCamera_NotRunning(t *testing.T) {
	db, tmpDir, cleanup := setupServiceTestDB(t)
	defer cleanup()

	cfg := testConfig(tmpDir)
	svc, err := NewService(cfg, db, ServiceConfig{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer svc.Stop()

	// Try to stop a camera that isn't running - should be no-op
	err = svc.StopCamera("test_cam_1")
	if err != nil {
		t.Errorf("StopCamera() error = %v, expected nil for non-running camera", err)
	}
}

func TestService_GetRecorderStatus_NotRunning(t *testing.T) {
	db, tmpDir, cleanup := setupServiceTestDB(t)
	defer cleanup()

	cfg := testConfig(tmpDir)
	// Disable recording to ensure no recorder is started
	for i := range cfg.Cameras {
		cfg.Cameras[i].Recording.Enabled = false
	}

	svc, err := NewService(cfg, db, ServiceConfig{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer svc.Stop()

	// Camera exists but recorder is not running
	status, err := svc.GetRecorderStatus("test_cam_1")
	if err != nil {
		t.Errorf("GetRecorderStatus() error = %v", err)
	}
	if status == nil {
		t.Fatal("GetRecorderStatus() returned nil status")
	}
	if status.State != RecorderStateIdle {
		t.Errorf("Expected state Idle, got %v", status.State)
	}
}

func TestService_GetAllRecorderStatus(t *testing.T) {
	db, tmpDir, cleanup := setupServiceTestDB(t)
	defer cleanup()

	cfg := testConfig(tmpDir)
	svc, err := NewService(cfg, db, ServiceConfig{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer svc.Stop()

	statuses := svc.GetAllRecorderStatus()
	if statuses == nil {
		t.Error("GetAllRecorderStatus() returned nil")
	}
}

func TestService_GetStorageStats(t *testing.T) {
	db, tmpDir, cleanup := setupServiceTestDB(t)
	defer cleanup()

	cfg := testConfig(tmpDir)
	svc, err := NewService(cfg, db, ServiceConfig{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer svc.Stop()

	stats, err := svc.GetStorageStats(ctx)
	if err != nil {
		t.Errorf("GetStorageStats() error = %v", err)
	}
	if stats == nil {
		t.Error("GetStorageStats() returned nil")
	}
}

func TestService_SegmentOperations(t *testing.T) {
	db, tmpDir, cleanup := setupServiceTestDB(t)
	defer cleanup()

	cfg := testConfig(tmpDir)
	svc, err := NewService(cfg, db, ServiceConfig{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer svc.Stop()

	// Create a test segment file
	segmentPath := filepath.Join(tmpDir, "recordings", "test_cam_1", "test_segment.mp4")
	os.MkdirAll(filepath.Dir(segmentPath), 0755)
	os.WriteFile(segmentPath, []byte("test video content"), 0644)

	// Create a test segment in the database
	segment := &Segment{
		CameraID:      "test_cam_1",
		StartTime:     time.Now().Add(-time.Minute),
		EndTime:       time.Now(),
		Duration:      60.0,
		FilePath:      segmentPath,
		FileSize:      1024,
		StorageTier:   StorageTierHot,
		RecordingMode: string(RecordingModeContinuous),
	}

	err = svc.repository.Create(ctx, segment)
	if err != nil {
		t.Fatalf("Failed to create test segment: %v", err)
	}

	// Test GetSegment
	retrieved, err := svc.GetSegment(ctx, segment.ID)
	if err != nil {
		t.Errorf("GetSegment() error = %v", err)
	}
	if retrieved == nil {
		t.Error("GetSegment() returned nil")
	}

	// Test ListSegments
	segments, count, err := svc.ListSegments(ctx, ListOptions{CameraID: "test_cam_1"})
	if err != nil {
		t.Errorf("ListSegments() error = %v", err)
	}
	if len(segments) != 1 || count != 1 {
		t.Errorf("ListSegments() returned %d segments, expected 1", len(segments))
	}

	// Test DeleteSegment
	err = svc.DeleteSegment(ctx, segment.ID)
	if err != nil {
		t.Errorf("DeleteSegment() error = %v", err)
	}

	// Verify deletion
	_, err = svc.GetSegment(ctx, segment.ID)
	if err == nil {
		t.Error("Segment should not exist after deletion")
	}
}

func TestService_GetTimeline(t *testing.T) {
	db, tmpDir, cleanup := setupServiceTestDB(t)
	defer cleanup()

	cfg := testConfig(tmpDir)
	svc, err := NewService(cfg, db, ServiceConfig{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer svc.Stop()

	end := time.Now()
	start := end.Add(-24 * time.Hour)

	timeline, err := svc.GetTimeline(ctx, "test_cam_1", start, end)
	if err != nil {
		t.Errorf("GetTimeline() error = %v", err)
	}
	if timeline == nil {
		t.Error("GetTimeline() returned nil")
	}
}

func TestService_GetTimelineSegments(t *testing.T) {
	db, tmpDir, cleanup := setupServiceTestDB(t)
	defer cleanup()

	cfg := testConfig(tmpDir)
	svc, err := NewService(cfg, db, ServiceConfig{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer svc.Stop()

	end := time.Now()
	start := end.Add(-24 * time.Hour)

	segments, err := svc.GetTimelineSegments(ctx, "test_cam_1", start, end)
	if err != nil {
		t.Errorf("GetTimelineSegments() error = %v", err)
	}
	// Should return empty slice, not nil
	if segments == nil {
		t.Error("GetTimelineSegments() returned nil, expected empty slice")
	}
}

func TestService_ExportSegments_NoSegments(t *testing.T) {
	db, tmpDir, cleanup := setupServiceTestDB(t)
	defer cleanup()

	cfg := testConfig(tmpDir)
	svc, err := NewService(cfg, db, ServiceConfig{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer svc.Stop()

	end := time.Now()
	start := end.Add(-time.Hour)
	outputPath := filepath.Join(tmpDir, "export.mp4")

	err = svc.ExportSegments(ctx, "test_cam_1", start, end, outputPath)
	if err == nil {
		t.Error("ExportSegments() should return error when no segments found")
	}
}

func TestService_RunRetention(t *testing.T) {
	db, tmpDir, cleanup := setupServiceTestDB(t)
	defer cleanup()

	cfg := testConfig(tmpDir)
	svc, err := NewService(cfg, db, ServiceConfig{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer svc.Stop()

	stats, err := svc.RunRetention(ctx)
	if err != nil {
		t.Errorf("RunRetention() error = %v", err)
	}
	if stats == nil {
		t.Error("RunRetention() returned nil stats")
	}
}

func TestService_OnConfigChange(t *testing.T) {
	db, tmpDir, cleanup := setupServiceTestDB(t)
	defer cleanup()

	cfg := testConfig(tmpDir)
	// Start with recording disabled
	for i := range cfg.Cameras {
		cfg.Cameras[i].Recording.Enabled = false
	}

	svc, err := NewService(cfg, db, ServiceConfig{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer svc.Stop()

	// Create new config with different camera settings
	newCfg := testConfig(tmpDir)
	newCfg.Cameras = append(newCfg.Cameras, config.CameraConfig{
		ID:      "test_cam_3",
		Name:    "Test Camera 3",
		Enabled: true,
		Recording: config.RecordingConfig{
			Enabled: false, // Keep recording disabled to avoid FFmpeg
		},
	})

	// Should not panic
	svc.OnConfigChange(newCfg)
}

func TestService_TriggerEventRecording_NotRunning(t *testing.T) {
	db, tmpDir, cleanup := setupServiceTestDB(t)
	defer cleanup()

	cfg := testConfig(tmpDir)
	// Disable cameras so event trigger will try to start
	for i := range cfg.Cameras {
		cfg.Cameras[i].Recording.Enabled = false
	}

	svc, err := NewService(cfg, db, ServiceConfig{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer svc.Stop()

	// Try to trigger event recording on a camera that doesn't exist
	err = svc.TriggerEventRecording("nonexistent_camera", "event_123")
	if err == nil {
		t.Error("TriggerEventRecording() should return error for non-existent camera")
	}
}

func TestService_GenerateThumbnail_SegmentNotFound(t *testing.T) {
	db, tmpDir, cleanup := setupServiceTestDB(t)
	defer cleanup()

	cfg := testConfig(tmpDir)
	svc, err := NewService(cfg, db, ServiceConfig{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer svc.Stop()

	_, err = svc.GenerateThumbnail(ctx, "nonexistent_segment_id")
	if err == nil {
		t.Error("GenerateThumbnail() should return error for non-existent segment")
	}
}

func TestService_GenerateThumbnail_FileNotFound(t *testing.T) {
	db, tmpDir, cleanup := setupServiceTestDB(t)
	defer cleanup()

	cfg := testConfig(tmpDir)
	svc, err := NewService(cfg, db, ServiceConfig{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer svc.Stop()

	// Create a segment record without the actual file
	segment := &Segment{
		CameraID:      "test_cam_1",
		StartTime:     time.Now().Add(-time.Minute),
		EndTime:       time.Now(),
		Duration:      60.0,
		FilePath:      filepath.Join(tmpDir, "nonexistent.mp4"),
		FileSize:      1024,
		StorageTier:   StorageTierHot,
		RecordingMode: string(RecordingModeContinuous),
	}

	err = svc.repository.Create(ctx, segment)
	if err != nil {
		t.Fatalf("Failed to create test segment: %v", err)
	}

	_, err = svc.GenerateThumbnail(ctx, segment.ID)
	if err == nil {
		t.Error("GenerateThumbnail() should return error when segment file not found")
	}
}

func TestService_GetPlaybackInfo(t *testing.T) {
	db, tmpDir, cleanup := setupServiceTestDB(t)
	defer cleanup()

	cfg := testConfig(tmpDir)
	svc, err := NewService(cfg, db, ServiceConfig{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer svc.Stop()

	// Should return empty when no segments exist
	url, offset, err := svc.GetPlaybackInfo(ctx, "test_cam_1", time.Now())
	if err != nil {
		// An error is acceptable if no segments found
		return
	}
	_ = url
	_ = offset
}
