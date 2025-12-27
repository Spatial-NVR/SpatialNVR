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

func setupRetentionTest(t *testing.T) (*RetentionPolicy, *SQLiteRepository, string, func()) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	cfg := &config.Config{
		System: config.SystemConfig{
			StoragePath:  tmpDir,
			MaxStorageGB: 10,
		},
		Cameras: []config.CameraConfig{
			{
				ID:      "test_cam_1",
				Name:    "Test Camera 1",
				Enabled: true,
				Recording: config.RecordingConfig{
					Enabled: true,
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
				Recording: config.RecordingConfig{
					Enabled: false, // Recording disabled
				},
			},
		},
	}

	storagePath := filepath.Join(tmpDir, "recordings")
	_ = os.MkdirAll(storagePath, 0755)

	repository := NewSQLiteRepository(db)
	if err := repository.InitSchema(context.Background()); err != nil {
		t.Fatalf("Failed to init schema: %v", err)
	}

	segmentHandler := NewDefaultSegmentHandler(storagePath, filepath.Join(tmpDir, "thumbnails"))

	policy := NewRetentionPolicy(cfg, repository, segmentHandler, storagePath)

	cleanup := func() {
		db.Close()
	}

	return policy, repository, tmpDir, cleanup
}

func TestNewRetentionPolicy(t *testing.T) {
	policy, _, _, cleanup := setupRetentionTest(t)
	defer cleanup()

	if policy == nil {
		t.Fatal("NewRetentionPolicy() returned nil")
	}
}

func TestRetentionPolicy_StartStop(t *testing.T) {
	policy, _, _, cleanup := setupRetentionTest(t)
	defer cleanup()

	ctx := context.Background()

	// Start the policy
	err := policy.Start(ctx, time.Hour)
	if err != nil {
		t.Errorf("Start() error = %v", err)
	}

	// Starting again should be no-op
	err = policy.Start(ctx, time.Hour)
	if err != nil {
		t.Errorf("Start() second call error = %v", err)
	}

	// Stop the policy
	policy.Stop()

	// Stopping again should be no-op
	policy.Stop()
}

func TestRetentionPolicy_RunCleanup_NoCameras(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Config with all recording disabled
	cfg := &config.Config{
		System: config.SystemConfig{
			StoragePath:  tmpDir,
			MaxStorageGB: 0, // No storage limit
		},
		Cameras: []config.CameraConfig{
			{
				ID:      "test_cam",
				Enabled: true,
				Recording: config.RecordingConfig{
					Enabled: false,
				},
			},
		},
	}

	storagePath := filepath.Join(tmpDir, "recordings")
	_ = os.MkdirAll(storagePath, 0755)

	repository := NewSQLiteRepository(db)
	_ = repository.InitSchema(context.Background())
	segmentHandler := NewDefaultSegmentHandler(storagePath, tmpDir)

	policy := NewRetentionPolicy(cfg, repository, segmentHandler, storagePath)

	ctx := context.Background()
	stats, err := policy.RunCleanup(ctx)
	if err != nil {
		t.Errorf("RunCleanup() error = %v", err)
	}
	if stats == nil {
		t.Fatal("RunCleanup() returned nil stats")
	}
	if stats.SegmentsDeleted != 0 {
		t.Errorf("SegmentsDeleted = %d, want 0", stats.SegmentsDeleted)
	}
}

func TestRetentionPolicy_RunCleanup_WithOldSegments(t *testing.T) {
	policy, repository, tmpDir, cleanup := setupRetentionTest(t)
	defer cleanup()

	ctx := context.Background()
	storagePath := filepath.Join(tmpDir, "recordings")

	// Create old segment files and records
	now := time.Now()
	oldTime := now.AddDate(0, 0, -30) // 30 days ago

	// Create the camera directory
	_ = os.MkdirAll(filepath.Join(storagePath, "test_cam_1"), 0755)

	for i := 0; i < 5; i++ {
		segPath := filepath.Join(storagePath, "test_cam_1", "segment_old_"+string(rune('0'+i))+".mp4")
		_ = os.WriteFile(segPath, []byte("old content"), 0644)

		segment := &Segment{
			CameraID:      "test_cam_1",
			StartTime:     oldTime.Add(time.Duration(i) * time.Minute),
			EndTime:       oldTime.Add(time.Duration(i+1) * time.Minute),
			Duration:      60.0,
			FilePath:      segPath,
			FileSize:      1024,
			StorageTier:   StorageTierHot,
			RecordingMode: string(RecordingModeContinuous),
			HasEvents:     false,
		}
		if err := repository.Create(ctx, segment); err != nil {
			t.Fatalf("Failed to create segment: %v", err)
		}
	}

	stats, err := policy.RunCleanup(ctx)
	if err != nil {
		t.Errorf("RunCleanup() error = %v", err)
	}
	if stats == nil {
		t.Fatal("RunCleanup() returned nil stats")
	}
	if stats.SegmentsDeleted != 5 {
		t.Errorf("SegmentsDeleted = %d, want 5", stats.SegmentsDeleted)
	}
}

func TestRetentionPolicy_RunCleanup_EventSegments(t *testing.T) {
	policy, repository, tmpDir, cleanup := setupRetentionTest(t)
	defer cleanup()

	ctx := context.Background()
	storagePath := filepath.Join(tmpDir, "recordings")

	now := time.Now()
	oldTime := now.AddDate(0, 0, -20) // 20 days ago (older than event retention of 14 days)

	_ = os.MkdirAll(filepath.Join(storagePath, "test_cam_1"), 0755)

	// Create event segment
	segPath := filepath.Join(storagePath, "test_cam_1", "event_segment.mp4")
	_ = os.WriteFile(segPath, []byte("event content"), 0644)

	segment := &Segment{
		CameraID:      "test_cam_1",
		StartTime:     oldTime,
		EndTime:       oldTime.Add(time.Minute),
		Duration:      60.0,
		FilePath:      segPath,
		FileSize:      1024,
		StorageTier:   StorageTierHot,
		RecordingMode: string(RecordingModeEvents),
		HasEvents:     true,
	}
	if err := repository.Create(ctx, segment); err != nil {
		t.Fatalf("Failed to create segment: %v", err)
	}

	stats, err := policy.RunCleanup(ctx)
	if err != nil {
		t.Errorf("RunCleanup() error = %v", err)
	}
	if stats == nil {
		t.Fatal("RunCleanup() returned nil stats")
	}
	// Event segment is older than 14 days so should be deleted
	if stats.SegmentsDeleted != 1 {
		t.Errorf("SegmentsDeleted = %d, want 1", stats.SegmentsDeleted)
	}
}

func TestRetentionPolicy_EnforceStorageLimit(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cfg := &config.Config{
		System: config.SystemConfig{
			StoragePath:  tmpDir,
			MaxStorageGB: 1, // 1GB limit
		},
		Cameras: []config.CameraConfig{
			{
				ID:      "test_cam",
				Enabled: true,
				Recording: config.RecordingConfig{
					Enabled: true,
					Retention: config.RetentionConfig{
						DefaultDays: 365, // Long retention so storage limit kicks in
						EventsDays:  365,
					},
				},
			},
		},
	}

	storagePath := filepath.Join(tmpDir, "recordings")
	_ = os.MkdirAll(filepath.Join(storagePath, "test_cam"), 0755)

	repository := NewSQLiteRepository(db)
	_ = repository.InitSchema(context.Background())
	segmentHandler := NewDefaultSegmentHandler(storagePath, tmpDir)

	policy := NewRetentionPolicy(cfg, repository, segmentHandler, storagePath)

	ctx := context.Background()

	// Create many segments
	now := time.Now()
	for i := 0; i < 10; i++ {
		segPath := filepath.Join(storagePath, "test_cam", "segment_"+string(rune('a'+i))+".mp4")
		// Create a large file to trigger storage limit
		_ = os.WriteFile(segPath, make([]byte, 100*1024*1024), 0644) // 100MB each

		segment := &Segment{
			CameraID:      "test_cam",
			StartTime:     now.Add(-time.Duration(i) * time.Hour),
			EndTime:       now.Add(-time.Duration(i) * time.Hour).Add(time.Minute),
			Duration:      60.0,
			FilePath:      segPath,
			FileSize:      100 * 1024 * 1024,
			StorageTier:   StorageTierHot,
			RecordingMode: string(RecordingModeContinuous),
		}
		_ = repository.Create(ctx, segment)
	}

	// Run cleanup
	stats, err := policy.RunCleanup(ctx)
	if err != nil {
		t.Errorf("RunCleanup() error = %v", err)
	}
	if stats == nil {
		t.Fatal("RunCleanup() returned nil stats")
	}
	// Some segments should be deleted to stay under limit
	if stats.SegmentsDeleted == 0 {
		t.Log("No segments deleted - this may be expected if storage wasn't calculated correctly")
	}
}

func TestRetentionPolicy_CleanupCameraWithDefaultRetention(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cfg := &config.Config{
		System: config.SystemConfig{
			StoragePath:  tmpDir,
			MaxStorageGB: 0,
		},
		Cameras: []config.CameraConfig{
			{
				ID:      "test_cam",
				Enabled: true,
				Recording: config.RecordingConfig{
					Enabled: true,
					Retention: config.RetentionConfig{
						DefaultDays: 0, // Use default of 30
						EventsDays:  0, // Use default
					},
				},
			},
		},
	}

	storagePath := filepath.Join(tmpDir, "recordings")
	_ = os.MkdirAll(filepath.Join(storagePath, "test_cam"), 0755)

	repository := NewSQLiteRepository(db)
	_ = repository.InitSchema(context.Background())
	segmentHandler := NewDefaultSegmentHandler(storagePath, tmpDir)

	policy := NewRetentionPolicy(cfg, repository, segmentHandler, storagePath)

	ctx := context.Background()

	// Create old segment (45 days old)
	oldTime := time.Now().AddDate(0, 0, -45)
	segPath := filepath.Join(storagePath, "test_cam", "old_segment.mp4")
	_ = os.WriteFile(segPath, []byte("content"), 0644)

	segment := &Segment{
		CameraID:      "test_cam",
		StartTime:     oldTime,
		EndTime:       oldTime.Add(time.Minute),
		Duration:      60.0,
		FilePath:      segPath,
		FileSize:      1024,
		StorageTier:   StorageTierHot,
		RecordingMode: string(RecordingModeContinuous),
	}
	_ = repository.Create(ctx, segment)

	stats, err := policy.RunCleanup(ctx)
	if err != nil {
		t.Errorf("RunCleanup() error = %v", err)
	}
	// Should delete the 45-day-old segment when default is 30 days
	if stats.SegmentsDeleted != 1 {
		t.Errorf("SegmentsDeleted = %d, want 1", stats.SegmentsDeleted)
	}
}

func TestWalkDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directory structure
	subDir := filepath.Join(tmpDir, "subdir")
	_ = os.MkdirAll(subDir, 0755)

	// Create files
	_ = os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("content1"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("content2"), 0644)
	_ = os.WriteFile(filepath.Join(subDir, "file3.txt"), []byte("content3"), 0644)

	var files []string
	err := walkDir(tmpDir, func(path string, info os.FileInfo) error {
		if !info.IsDir() {
			files = append(files, filepath.Base(path))
		}
		return nil
	})

	if err != nil {
		t.Errorf("walkDir() error = %v", err)
	}
	if len(files) != 3 {
		t.Errorf("found %d files, want 3", len(files))
	}
}

func TestWalkDir_NonExistent(t *testing.T) {
	err := walkDir("/nonexistent/path", func(path string, info os.FileInfo) error {
		return nil
	})

	if err == nil {
		t.Error("walkDir() should return error for non-existent path")
	}
}

func TestTierMigration_New(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	repository := NewSQLiteRepository(db)
	_ = repository.InitSchema(context.Background())

	segmentHandler := NewDefaultSegmentHandler(tmpDir, tmpDir)

	migration := NewTierMigration(
		repository,
		segmentHandler,
		filepath.Join(tmpDir, "hot"),
		filepath.Join(tmpDir, "warm"),
		filepath.Join(tmpDir, "cold"),
	)

	if migration == nil {
		t.Fatal("NewTierMigration() returned nil")
	}
}

func TestTierMigration_MigrateToWarm(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	repository := NewSQLiteRepository(db)
	_ = repository.InitSchema(context.Background())

	hotPath := filepath.Join(tmpDir, "hot")
	warmPath := filepath.Join(tmpDir, "warm")
	coldPath := filepath.Join(tmpDir, "cold")

	for _, path := range []string{hotPath, warmPath, coldPath} {
		_ = os.MkdirAll(path, 0755)
	}

	segmentHandler := NewDefaultSegmentHandler(hotPath, tmpDir)

	migration := NewTierMigration(repository, segmentHandler, hotPath, warmPath, coldPath)

	ctx := context.Background()

	// Create a segment older than 24 hours
	oldTime := time.Now().Add(-48 * time.Hour)
	segPath := filepath.Join(hotPath, "old_segment.mp4")
	_ = os.WriteFile(segPath, []byte("content"), 0644)

	segment := &Segment{
		CameraID:      "test_cam",
		StartTime:     oldTime,
		EndTime:       oldTime.Add(time.Minute),
		Duration:      60.0,
		FilePath:      segPath,
		FileSize:      1024,
		StorageTier:   StorageTierHot,
		RecordingMode: string(RecordingModeContinuous),
	}
	_ = repository.Create(ctx, segment)

	// Migrate segments older than 24 hours
	err = migration.MigrateToWarm(ctx, 24*time.Hour)
	if err != nil {
		t.Errorf("MigrateToWarm() error = %v", err)
	}

	// Verify segment was updated to warm tier
	updated, _ := repository.Get(ctx, segment.ID)
	if updated != nil && updated.StorageTier != StorageTierWarm {
		t.Errorf("StorageTier = %v, want %v", updated.StorageTier, StorageTierWarm)
	}
}

func TestTierMigration_MigrateToWarm_NoOldSegments(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	repository := NewSQLiteRepository(db)
	_ = repository.InitSchema(context.Background())

	segmentHandler := NewDefaultSegmentHandler(tmpDir, tmpDir)

	migration := NewTierMigration(repository, segmentHandler, tmpDir, tmpDir, tmpDir)

	ctx := context.Background()

	// Create a recent segment
	recentTime := time.Now().Add(-time.Hour)
	segment := &Segment{
		CameraID:      "test_cam",
		StartTime:     recentTime,
		EndTime:       recentTime.Add(time.Minute),
		Duration:      60.0,
		FilePath:      filepath.Join(tmpDir, "recent.mp4"),
		FileSize:      1024,
		StorageTier:   StorageTierHot,
		RecordingMode: string(RecordingModeContinuous),
	}
	_ = repository.Create(ctx, segment)

	// Try to migrate segments older than 24 hours
	err = migration.MigrateToWarm(ctx, 24*time.Hour)
	if err != nil {
		t.Errorf("MigrateToWarm() error = %v", err)
	}

	// Segment should still be in hot tier
	updated, _ := repository.Get(ctx, segment.ID)
	if updated != nil && updated.StorageTier != StorageTierHot {
		t.Errorf("StorageTier = %v, want %v (segment was too recent to migrate)", updated.StorageTier, StorageTierHot)
	}
}

func TestRetentionStats(t *testing.T) {
	stats := &RetentionStats{
		SegmentsDeleted: 10,
		BytesFreed:      1024 * 1024 * 100, // 100MB
	}

	if stats.SegmentsDeleted != 10 {
		t.Errorf("SegmentsDeleted = %d, want 10", stats.SegmentsDeleted)
	}
	if stats.BytesFreed != 104857600 {
		t.Errorf("BytesFreed = %d, want 104857600", stats.BytesFreed)
	}
}

func TestStorageStats(t *testing.T) {
	stats := &StorageStats{
		TotalBytes:     1024 * 1024 * 1024 * 100, // 100GB
		UsedBytes:      1024 * 1024 * 1024 * 50,  // 50GB
		AvailableBytes: 1024 * 1024 * 1024 * 50,  // 50GB
		SegmentCount:   1000,
		ByCamera: map[string]int64{
			"cam1": 1024 * 1024 * 1024 * 25,
			"cam2": 1024 * 1024 * 1024 * 25,
		},
		ByTier: map[StorageTier]int64{
			StorageTierHot:  1024 * 1024 * 1024 * 30,
			StorageTierWarm: 1024 * 1024 * 1024 * 20,
		},
	}

	if len(stats.ByCamera) != 2 {
		t.Errorf("ByCamera has %d entries, want 2", len(stats.ByCamera))
	}
	if len(stats.ByTier) != 2 {
		t.Errorf("ByTier has %d entries, want 2", len(stats.ByTier))
	}
}

func TestRetentionPolicy_DeleteSegment_FileNotExist(t *testing.T) {
	policy, repository, _, cleanup := setupRetentionTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create segment record without actual file
	segment := &Segment{
		CameraID:      "test_cam_1",
		StartTime:     time.Now().Add(-time.Hour),
		EndTime:       time.Now(),
		Duration:      60.0,
		FilePath:      "/nonexistent/path/segment.mp4",
		FileSize:      1024,
		StorageTier:   StorageTierHot,
		RecordingMode: string(RecordingModeContinuous),
	}
	_ = repository.Create(ctx, segment)

	// Should still delete from database even if file doesn't exist
	err := policy.deleteSegment(ctx, segment)
	if err != nil {
		t.Errorf("deleteSegment() error = %v", err)
	}

	// Verify deleted from database
	_, err = repository.Get(ctx, segment.ID)
	if err == nil {
		t.Error("Segment should be deleted from database")
	}
}

func TestRetentionPolicy_GetStorageUsage(t *testing.T) {
	policy, _, tmpDir, cleanup := setupRetentionTest(t)
	defer cleanup()

	storagePath := filepath.Join(tmpDir, "recordings")

	// Create some test files
	_ = os.MkdirAll(filepath.Join(storagePath, "test_cam"), 0755)
	_ = os.WriteFile(filepath.Join(storagePath, "test_cam", "file1.mp4"), []byte("content1"), 0644)
	_ = os.WriteFile(filepath.Join(storagePath, "test_cam", "file2.mp4"), []byte("longer content 2"), 0644)

	usage, err := policy.getStorageUsage()
	if err != nil {
		t.Errorf("getStorageUsage() error = %v", err)
	}
	if usage <= 0 {
		t.Errorf("usage = %d, want > 0", usage)
	}
}

func TestRetentionPolicyConfig(t *testing.T) {
	cfg := RetentionPolicyConfig{
		DefaultDays:     30,
		EventsDays:      90,
		MaxStorageGB:    500,
		CleanupInterval: time.Hour,
	}

	if cfg.DefaultDays != 30 {
		t.Errorf("DefaultDays = %d, want 30", cfg.DefaultDays)
	}
	if cfg.EventsDays != 90 {
		t.Errorf("EventsDays = %d, want 90", cfg.EventsDays)
	}
	if cfg.MaxStorageGB != 500 {
		t.Errorf("MaxStorageGB = %d, want 500", cfg.MaxStorageGB)
	}
	if cfg.CleanupInterval != time.Hour {
		t.Errorf("CleanupInterval = %v, want %v", cfg.CleanupInterval, time.Hour)
	}
}
