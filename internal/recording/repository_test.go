package recording

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	return db
}

func setupTestRepo(t *testing.T) (*SQLiteRepository, *sql.DB) {
	db := setupTestDB(t)
	repo := NewSQLiteRepository(db)
	err := repo.InitSchema(context.Background())
	if err != nil {
		t.Fatalf("Failed to init schema: %v", err)
	}
	return repo, db
}

func TestNewSQLiteRepository(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	repo := NewSQLiteRepository(db)
	if repo == nil {
		t.Fatal("NewSQLiteRepository returned nil")
	}
}

func TestSQLiteRepository_InitSchema(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	repo := NewSQLiteRepository(db)
	err := repo.InitSchema(context.Background())
	if err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Verify table exists
	var name string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='recordings'").Scan(&name)
	if err != nil {
		t.Fatalf("Table not created: %v", err)
	}
}

func TestSQLiteRepository_Create(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer func() { _ = db.Close() }()

	segment := &Segment{
		CameraID:    "cam_1",
		StartTime:   time.Now(),
		EndTime:     time.Now().Add(time.Minute),
		Duration:    60,
		FilePath:    "/path/to/segment.ts",
		FileSize:    1024 * 1024,
		StorageTier: StorageTierHot,
		HasEvents:   true,
		EventCount:  5,
		Codec:       "h264",
		Resolution:  "1920x1080",
		Bitrate:     4000000,
	}

	err := repo.Create(context.Background(), segment)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if segment.ID == "" {
		t.Error("Expected ID to be set")
	}
	if segment.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}
}

func TestSQLiteRepository_Get(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer func() { _ = db.Close() }()

	// Create a segment
	segment := &Segment{
		CameraID:    "cam_1",
		StartTime:   time.Now().Truncate(time.Second),
		EndTime:     time.Now().Add(time.Minute).Truncate(time.Second),
		Duration:    60,
		FilePath:    "/path/to/segment.ts",
		FileSize:    1024,
		StorageTier: StorageTierHot,
	}
	err := repo.Create(context.Background(), segment)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Get the segment
	retrieved, err := repo.Get(context.Background(), segment.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.CameraID != segment.CameraID {
		t.Errorf("Expected CameraID %s, got %s", segment.CameraID, retrieved.CameraID)
	}
	if retrieved.Duration != segment.Duration {
		t.Errorf("Expected Duration %f, got %f", segment.Duration, retrieved.Duration)
	}
}

func TestSQLiteRepository_Get_NotFound(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer func() { _ = db.Close() }()

	_, err := repo.Get(context.Background(), "nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent segment")
	}
}

func TestSQLiteRepository_Update(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer func() { _ = db.Close() }()

	// Create a segment
	segment := &Segment{
		CameraID:    "cam_1",
		StartTime:   time.Now(),
		EndTime:     time.Now().Add(time.Minute),
		Duration:    60,
		FilePath:    "/path/to/segment.ts",
		FileSize:    1024,
		StorageTier: StorageTierHot,
	}
	err := repo.Create(context.Background(), segment)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update the segment
	segment.Duration = 120
	segment.HasEvents = true
	err = repo.Update(context.Background(), segment)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify update
	retrieved, err := repo.Get(context.Background(), segment.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved.Duration != 120 {
		t.Errorf("Expected Duration 120, got %f", retrieved.Duration)
	}
	if !retrieved.HasEvents {
		t.Error("Expected HasEvents to be true")
	}
}

func TestSQLiteRepository_Update_NotFound(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer func() { _ = db.Close() }()

	segment := &Segment{
		ID:          "nonexistent",
		CameraID:    "cam_1",
		StartTime:   time.Now(),
		EndTime:     time.Now().Add(time.Minute),
		Duration:    60,
		FilePath:    "/path/to/segment.ts",
		StorageTier: StorageTierHot,
	}

	err := repo.Update(context.Background(), segment)
	if err == nil {
		t.Error("Expected error for nonexistent segment")
	}
}

func TestSQLiteRepository_Delete(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer func() { _ = db.Close() }()

	// Create a segment
	segment := &Segment{
		CameraID:    "cam_1",
		StartTime:   time.Now(),
		EndTime:     time.Now().Add(time.Minute),
		Duration:    60,
		FilePath:    "/path/to/segment.ts",
		StorageTier: StorageTierHot,
	}
	err := repo.Create(context.Background(), segment)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Delete the segment
	err = repo.Delete(context.Background(), segment.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deletion
	_, err = repo.Get(context.Background(), segment.ID)
	if err == nil {
		t.Error("Expected error for deleted segment")
	}
}

func TestSQLiteRepository_Delete_NotFound(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer func() { _ = db.Close() }()

	err := repo.Delete(context.Background(), "nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent segment")
	}
}

func TestSQLiteRepository_List(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer func() { _ = db.Close() }()

	// Create multiple segments
	for i := 0; i < 5; i++ {
		segment := &Segment{
			CameraID:    "cam_1",
			StartTime:   time.Now().Add(time.Duration(i) * time.Hour),
			EndTime:     time.Now().Add(time.Duration(i)*time.Hour + time.Minute),
			Duration:    60,
			FilePath:    "/path/to/segment.ts",
			StorageTier: StorageTierHot,
		}
		if err := repo.Create(context.Background(), segment); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// List all
	segments, total, err := repo.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
	if len(segments) != 5 {
		t.Errorf("Expected 5 segments, got %d", len(segments))
	}
}

func TestSQLiteRepository_List_WithFilters(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer func() { _ = db.Close() }()

	// Create segments for different cameras
	for _, camID := range []string{"cam_1", "cam_1", "cam_2"} {
		segment := &Segment{
			CameraID:    camID,
			StartTime:   time.Now(),
			EndTime:     time.Now().Add(time.Minute),
			Duration:    60,
			FilePath:    "/path/to/segment.ts",
			StorageTier: StorageTierHot,
		}
		if err := repo.Create(context.Background(), segment); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// List with camera filter
	segments, total, err := repo.List(context.Background(), ListOptions{CameraID: "cam_1"})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if total != 2 {
		t.Errorf("Expected total 2, got %d", total)
	}
	if len(segments) != 2 {
		t.Errorf("Expected 2 segments, got %d", len(segments))
	}
}

func TestSQLiteRepository_List_WithTimeRange(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer func() { _ = db.Close() }()

	now := time.Now()

	// Create segments at different times
	for i := 0; i < 5; i++ {
		segment := &Segment{
			CameraID:    "cam_1",
			StartTime:   now.Add(time.Duration(i) * time.Hour),
			EndTime:     now.Add(time.Duration(i)*time.Hour + 30*time.Minute),
			Duration:    1800,
			FilePath:    "/path/to/segment.ts",
			StorageTier: StorageTierHot,
		}
		if err := repo.Create(context.Background(), segment); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// List with time filter (should get segments 2, 3, 4)
	startTime := now.Add(2 * time.Hour)
	_, total, err := repo.List(context.Background(), ListOptions{StartTime: &startTime})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if total != 3 {
		t.Errorf("Expected total 3, got %d", total)
	}
}

func TestSQLiteRepository_List_WithPagination(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer func() { _ = db.Close() }()

	// Create 10 segments
	for i := 0; i < 10; i++ {
		segment := &Segment{
			CameraID:    "cam_1",
			StartTime:   time.Now().Add(time.Duration(i) * time.Hour),
			EndTime:     time.Now().Add(time.Duration(i)*time.Hour + time.Minute),
			Duration:    60,
			FilePath:    "/path/to/segment.ts",
			StorageTier: StorageTierHot,
		}
		if err := repo.Create(context.Background(), segment); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// List with pagination
	segments, total, err := repo.List(context.Background(), ListOptions{Limit: 3, Offset: 2})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if total != 10 {
		t.Errorf("Expected total 10, got %d", total)
	}
	if len(segments) != 3 {
		t.Errorf("Expected 3 segments, got %d", len(segments))
	}
}

func TestSQLiteRepository_List_WithOrder(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer func() { _ = db.Close() }()

	now := time.Now()

	// Create segments at different times
	for i := 0; i < 3; i++ {
		segment := &Segment{
			CameraID:    "cam_1",
			StartTime:   now.Add(time.Duration(i) * time.Hour),
			EndTime:     now.Add(time.Duration(i)*time.Hour + time.Minute),
			Duration:    60,
			FilePath:    "/path/to/segment.ts",
			StorageTier: StorageTierHot,
		}
		if err := repo.Create(context.Background(), segment); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// List with descending order
	segments, _, err := repo.List(context.Background(), ListOptions{OrderBy: "start_time", OrderDesc: true})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	// First segment should be the newest
	if segments[0].StartTime.Before(segments[1].StartTime) {
		t.Error("Expected descending order")
	}
}

func TestSQLiteRepository_DeleteBefore(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer func() { _ = db.Close() }()

	now := time.Now()

	// Create segments at i=0,1,2,3,4 hours ago
	// Each segment is 30 min long, so end_times are at i-0.5 hours ago
	// i=0: ends at now+30min (new)
	// i=1: ends at now-30min (1 hour ago start, ends 30min ago)
	// i=2: ends at now-1.5h (2 hours ago start, ends 1.5h ago)
	// i=3: ends at now-2.5h (3 hours ago start, ends 2.5h ago)
	// i=4: ends at now-3.5h (4 hours ago start, ends 3.5h ago)
	for i := 0; i < 5; i++ {
		segment := &Segment{
			CameraID:    "cam_1",
			StartTime:   now.Add(-time.Duration(i) * time.Hour),
			EndTime:     now.Add(-time.Duration(i)*time.Hour + 30*time.Minute),
			Duration:    1800,
			FilePath:    "/path/to/segment.ts",
			StorageTier: StorageTierHot,
		}
		if err := repo.Create(context.Background(), segment); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// Delete segments whose end_time is before cutoff (2 hours ago)
	// Segments 3 and 4 end before 2 hours ago (at 2.5h and 3.5h ago respectively)
	cutoff := now.Add(-2 * time.Hour)
	deleted, err := repo.DeleteBefore(context.Background(), "cam_1", cutoff)
	if err != nil {
		t.Fatalf("DeleteBefore failed: %v", err)
	}

	if deleted != 2 {
		t.Errorf("Expected 2 deleted, got %d", deleted)
	}
}

func TestSQLiteRepository_UpdateTier(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer func() { _ = db.Close() }()

	// Create segments
	var ids []string
	for i := 0; i < 3; i++ {
		segment := &Segment{
			CameraID:    "cam_1",
			StartTime:   time.Now(),
			EndTime:     time.Now().Add(time.Minute),
			Duration:    60,
			FilePath:    "/path/to/segment.ts",
			StorageTier: StorageTierHot,
		}
		if err := repo.Create(context.Background(), segment); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		ids = append(ids, segment.ID)
	}

	// Update tier
	err := repo.UpdateTier(context.Background(), ids, StorageTierWarm)
	if err != nil {
		t.Fatalf("UpdateTier failed: %v", err)
	}

	// Verify update
	for _, id := range ids {
		segment, err := repo.Get(context.Background(), id)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if segment.StorageTier != StorageTierWarm {
			t.Errorf("Expected tier warm, got %s", segment.StorageTier)
		}
	}
}

func TestSQLiteRepository_UpdateTier_Empty(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer func() { _ = db.Close() }()

	// Should not error on empty list
	err := repo.UpdateTier(context.Background(), []string{}, StorageTierWarm)
	if err != nil {
		t.Fatalf("UpdateTier failed: %v", err)
	}
}

func TestSQLiteRepository_GetByTimeRange(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer func() { _ = db.Close() }()

	now := time.Now()

	// Create segments
	for i := 0; i < 5; i++ {
		segment := &Segment{
			CameraID:    "cam_1",
			StartTime:   now.Add(time.Duration(i) * time.Hour),
			EndTime:     now.Add(time.Duration(i)*time.Hour + 30*time.Minute),
			Duration:    1800,
			FilePath:    "/path/to/segment.ts",
			StorageTier: StorageTierHot,
		}
		if err := repo.Create(context.Background(), segment); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// Get segments in range (should overlap with segments 1, 2, 3)
	start := now.Add(1 * time.Hour)
	end := now.Add(4 * time.Hour)
	segments, err := repo.GetByTimeRange(context.Background(), "cam_1", start, end)
	if err != nil {
		t.Fatalf("GetByTimeRange failed: %v", err)
	}

	if len(segments) != 3 {
		t.Errorf("Expected 3 segments, got %d", len(segments))
	}
}

func TestSQLiteRepository_GetOldestSegments(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer func() { _ = db.Close() }()

	now := time.Now()

	// Create segments at different times
	for i := 0; i < 5; i++ {
		segment := &Segment{
			CameraID:    "cam_1",
			StartTime:   now.Add(time.Duration(i) * time.Hour),
			EndTime:     now.Add(time.Duration(i)*time.Hour + time.Minute),
			Duration:    60,
			FilePath:    "/path/to/segment.ts",
			StorageTier: StorageTierHot,
		}
		if err := repo.Create(context.Background(), segment); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// Get oldest 2 segments
	segments, err := repo.GetOldestSegments(context.Background(), "cam_1", 2)
	if err != nil {
		t.Fatalf("GetOldestSegments failed: %v", err)
	}

	if len(segments) != 2 {
		t.Errorf("Expected 2 segments, got %d", len(segments))
	}

	// First should be oldest
	if segments[0].StartTime.After(segments[1].StartTime) {
		t.Error("Expected segments in ascending order by start time")
	}
}

func TestSQLiteRepository_GetTotalSize(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer func() { _ = db.Close() }()

	// Create segments with known sizes
	for i := 0; i < 3; i++ {
		segment := &Segment{
			CameraID:    "cam_1",
			StartTime:   time.Now(),
			EndTime:     time.Now().Add(time.Minute),
			Duration:    60,
			FilePath:    "/path/to/segment.ts",
			FileSize:    1000,
			StorageTier: StorageTierHot,
		}
		if err := repo.Create(context.Background(), segment); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	size, err := repo.GetTotalSize(context.Background(), "cam_1")
	if err != nil {
		t.Fatalf("GetTotalSize failed: %v", err)
	}

	if size != 3000 {
		t.Errorf("Expected size 3000, got %d", size)
	}
}

func TestSQLiteRepository_GetSegmentCount(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer func() { _ = db.Close() }()

	// Create segments
	for i := 0; i < 5; i++ {
		segment := &Segment{
			CameraID:    "cam_1",
			StartTime:   time.Now(),
			EndTime:     time.Now().Add(time.Minute),
			Duration:    60,
			FilePath:    "/path/to/segment.ts",
			StorageTier: StorageTierHot,
		}
		if err := repo.Create(context.Background(), segment); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	count, err := repo.GetSegmentCount(context.Background(), "cam_1")
	if err != nil {
		t.Fatalf("GetSegmentCount failed: %v", err)
	}

	if count != 5 {
		t.Errorf("Expected count 5, got %d", count)
	}
}

func TestSQLiteRepository_GetStorageByCamera(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer func() { _ = db.Close() }()

	// Create segments for different cameras
	for _, camID := range []string{"cam_1", "cam_1", "cam_2"} {
		segment := &Segment{
			CameraID:    camID,
			StartTime:   time.Now(),
			EndTime:     time.Now().Add(time.Minute),
			Duration:    60,
			FilePath:    "/path/to/segment.ts",
			FileSize:    1000,
			StorageTier: StorageTierHot,
		}
		if err := repo.Create(context.Background(), segment); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	storage, err := repo.GetStorageByCamera(context.Background())
	if err != nil {
		t.Fatalf("GetStorageByCamera failed: %v", err)
	}

	if storage["cam_1"] != 2000 {
		t.Errorf("Expected cam_1 size 2000, got %d", storage["cam_1"])
	}
	if storage["cam_2"] != 1000 {
		t.Errorf("Expected cam_2 size 1000, got %d", storage["cam_2"])
	}
}

func TestSQLiteRepository_GetStorageByTier(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer func() { _ = db.Close() }()

	// Create segments for different tiers
	tiers := []StorageTier{StorageTierHot, StorageTierHot, StorageTierWarm}
	for _, tier := range tiers {
		segment := &Segment{
			CameraID:    "cam_1",
			StartTime:   time.Now(),
			EndTime:     time.Now().Add(time.Minute),
			Duration:    60,
			FilePath:    "/path/to/segment.ts",
			FileSize:    1000,
			StorageTier: tier,
		}
		if err := repo.Create(context.Background(), segment); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	storage, err := repo.GetStorageByTier(context.Background())
	if err != nil {
		t.Fatalf("GetStorageByTier failed: %v", err)
	}

	if storage[StorageTierHot] != 2000 {
		t.Errorf("Expected hot size 2000, got %d", storage[StorageTierHot])
	}
	if storage[StorageTierWarm] != 1000 {
		t.Errorf("Expected warm size 1000, got %d", storage[StorageTierWarm])
	}
}

func TestSQLiteRepository_List_WithHasEvents(t *testing.T) {
	repo, db := setupTestRepo(t)
	defer func() { _ = db.Close() }()

	// Create segments with and without events
	for _, hasEvents := range []bool{true, false, true} {
		segment := &Segment{
			CameraID:    "cam_1",
			StartTime:   time.Now(),
			EndTime:     time.Now().Add(time.Minute),
			Duration:    60,
			FilePath:    "/path/to/segment.ts",
			StorageTier: StorageTierHot,
			HasEvents:   hasEvents,
		}
		if err := repo.Create(context.Background(), segment); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// List with hasEvents filter
	hasEvents := true
	segments, total, err := repo.List(context.Background(), ListOptions{HasEvents: &hasEvents})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if total != 2 {
		t.Errorf("Expected total 2, got %d", total)
	}
	if len(segments) != 2 {
		t.Errorf("Expected 2 segments, got %d", len(segments))
	}
}
