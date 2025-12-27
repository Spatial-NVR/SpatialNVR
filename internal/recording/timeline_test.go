package recording

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func setupTimelineTestRepo(t *testing.T) (*SQLiteRepository, *sql.DB) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	repo := NewSQLiteRepository(db)
	err = repo.InitSchema(context.Background())
	if err != nil {
		t.Fatalf("Failed to init schema: %v", err)
	}
	return repo, db
}

func TestNewTimelineBuilder(t *testing.T) {
	repo, db := setupTimelineTestRepo(t)
	defer func() { _ = db.Close() }()

	builder := NewTimelineBuilder(repo)
	if builder == nil {
		t.Fatal("NewTimelineBuilder returned nil")
	}
}

func TestTimelineBuilder_BuildTimeline_Empty(t *testing.T) {
	repo, db := setupTimelineTestRepo(t)
	defer func() { _ = db.Close() }()

	builder := NewTimelineBuilder(repo)

	start := time.Now()
	end := start.Add(time.Hour)

	timeline, err := builder.BuildTimeline(context.Background(), "cam_1", start, end)
	if err != nil {
		t.Fatalf("BuildTimeline failed: %v", err)
	}

	if timeline.CameraID != "cam_1" {
		t.Errorf("Expected camera_id cam_1, got %s", timeline.CameraID)
	}

	// Should have single gap segment
	if len(timeline.Segments) != 1 {
		t.Errorf("Expected 1 segment, got %d", len(timeline.Segments))
	}
	if timeline.Segments[0].Type != "gap" {
		t.Errorf("Expected gap segment, got %s", timeline.Segments[0].Type)
	}
}

func TestTimelineBuilder_BuildTimeline_WithSegments(t *testing.T) {
	repo, db := setupTimelineTestRepo(t)
	defer func() { _ = db.Close() }()

	builder := NewTimelineBuilder(repo)

	now := time.Now().Truncate(time.Second)

	// Create segments
	for i := 0; i < 3; i++ {
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

	start := now
	end := now.Add(3 * time.Hour)

	timeline, err := builder.BuildTimeline(context.Background(), "cam_1", start, end)
	if err != nil {
		t.Fatalf("BuildTimeline failed: %v", err)
	}

	// Should have recording and gap segments
	recordingCount := 0
	gapCount := 0
	for _, seg := range timeline.Segments {
		switch seg.Type {
		case "recording":
			recordingCount++
		case "gap":
			gapCount++
		}
	}

	if recordingCount != 3 {
		t.Errorf("Expected 3 recording segments, got %d", recordingCount)
	}
}

func TestTimelineBuilder_BuildTimeline_WithEvents(t *testing.T) {
	repo, db := setupTimelineTestRepo(t)
	defer func() { _ = db.Close() }()

	builder := NewTimelineBuilder(repo)

	now := time.Now().Truncate(time.Second)

	// Create segment with events
	segment := &Segment{
		CameraID:    "cam_1",
		StartTime:   now,
		EndTime:     now.Add(30 * time.Minute),
		Duration:    1800,
		FilePath:    "/path/to/segment.ts",
		StorageTier: StorageTierHot,
		HasEvents:   true,
		EventCount:  5,
	}
	if err := repo.Create(context.Background(), segment); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	timeline, err := builder.BuildTimeline(context.Background(), "cam_1", now, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("BuildTimeline failed: %v", err)
	}

	// Find recording segment
	var recordingSeg *TimelineSegment
	for i := range timeline.Segments {
		if timeline.Segments[i].Type == "recording" {
			recordingSeg = &timeline.Segments[i]
			break
		}
	}

	if recordingSeg == nil {
		t.Fatal("Expected recording segment")
	}
	if !recordingSeg.HasEvents {
		t.Error("Expected HasEvents to be true")
	}
	if recordingSeg.EventCount != 5 {
		t.Errorf("Expected EventCount 5, got %d", recordingSeg.EventCount)
	}
}

func TestTimelineBuilder_GetTimelineSegments(t *testing.T) {
	repo, db := setupTimelineTestRepo(t)
	defer func() { _ = db.Close() }()

	builder := NewTimelineBuilder(repo)

	now := time.Now()
	segments, err := builder.GetTimelineSegments(context.Background(), "cam_1", now, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("GetTimelineSegments failed: %v", err)
	}

	if len(segments) != 1 {
		t.Errorf("Expected 1 segment, got %d", len(segments))
	}
}

func TestTimelineBuilder_GetCoverage(t *testing.T) {
	repo, db := setupTimelineTestRepo(t)
	defer func() { _ = db.Close() }()

	builder := NewTimelineBuilder(repo)

	now := time.Now().Truncate(time.Second)

	// Create segment covering 30 minutes of a 1 hour window
	segment := &Segment{
		CameraID:    "cam_1",
		StartTime:   now,
		EndTime:     now.Add(30 * time.Minute),
		Duration:    1800,
		FilePath:    "/path/to/segment.ts",
		StorageTier: StorageTierHot,
	}
	if err := repo.Create(context.Background(), segment); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	coverage, err := builder.GetCoverage(context.Background(), "cam_1", now, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("GetCoverage failed: %v", err)
	}

	// Should be approximately 50%
	if coverage < 45 || coverage > 55 {
		t.Errorf("Expected coverage around 50%%, got %f%%", coverage)
	}
}

func TestTimelineBuilder_GetCoverage_Empty(t *testing.T) {
	repo, db := setupTimelineTestRepo(t)
	defer func() { _ = db.Close() }()

	builder := NewTimelineBuilder(repo)

	now := time.Now()
	coverage, err := builder.GetCoverage(context.Background(), "cam_1", now, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("GetCoverage failed: %v", err)
	}

	if coverage != 0 {
		t.Errorf("Expected coverage 0, got %f", coverage)
	}
}

func TestTimelineBuilder_GetCoverage_ZeroDuration(t *testing.T) {
	repo, db := setupTimelineTestRepo(t)
	defer func() { _ = db.Close() }()

	builder := NewTimelineBuilder(repo)

	now := time.Now()
	coverage, err := builder.GetCoverage(context.Background(), "cam_1", now, now)
	if err != nil {
		t.Fatalf("GetCoverage failed: %v", err)
	}

	if coverage != 0 {
		t.Errorf("Expected coverage 0 for zero duration, got %f", coverage)
	}
}

func TestTimelineBuilder_GetEventTimeline(t *testing.T) {
	repo, db := setupTimelineTestRepo(t)
	defer func() { _ = db.Close() }()

	builder := NewTimelineBuilder(repo)

	now := time.Now().Truncate(time.Second)

	// Create segments with and without events
	for i, hasEvents := range []bool{true, false, true} {
		segment := &Segment{
			CameraID:    "cam_1",
			StartTime:   now.Add(time.Duration(i) * time.Hour),
			EndTime:     now.Add(time.Duration(i)*time.Hour + 30*time.Minute),
			Duration:    1800,
			FilePath:    "/path/to/segment.ts",
			StorageTier: StorageTierHot,
			HasEvents:   hasEvents,
			EventCount:  i + 1,
		}
		if err := repo.Create(context.Background(), segment); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	segments, err := builder.GetEventTimeline(context.Background(), "cam_1", now, now.Add(4*time.Hour))
	if err != nil {
		t.Fatalf("GetEventTimeline failed: %v", err)
	}

	if len(segments) != 2 {
		t.Errorf("Expected 2 event segments, got %d", len(segments))
	}
}

func TestTimelineBuilder_GetDailyStats(t *testing.T) {
	repo, db := setupTimelineTestRepo(t)
	defer func() { _ = db.Close() }()

	builder := NewTimelineBuilder(repo)

	today := time.Now().Truncate(24 * time.Hour)

	// Create segments for today
	for i := 0; i < 3; i++ {
		segment := &Segment{
			CameraID:    "cam_1",
			StartTime:   today.Add(time.Duration(i) * time.Hour),
			EndTime:     today.Add(time.Duration(i)*time.Hour + time.Hour),
			Duration:    3600,
			FilePath:    "/path/to/segment.ts",
			FileSize:    1000000,
			StorageTier: StorageTierHot,
			HasEvents:   i%2 == 0,
			EventCount:  i + 1,
		}
		if err := repo.Create(context.Background(), segment); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	stats, err := builder.GetDailyStats(context.Background(), "cam_1", today)
	if err != nil {
		t.Fatalf("GetDailyStats failed: %v", err)
	}

	if stats.SegmentCount != 3 {
		t.Errorf("Expected SegmentCount 3, got %d", stats.SegmentCount)
	}
	if stats.TotalDuration != 3*3600 {
		t.Errorf("Expected TotalDuration 10800, got %f", stats.TotalDuration)
	}
	if stats.TotalSize != 3000000 {
		t.Errorf("Expected TotalSize 3000000, got %d", stats.TotalSize)
	}
	if stats.EventCount != 4 { // 1 + 3 from segments 0 and 2
		t.Errorf("Expected EventCount 4, got %d", stats.EventCount)
	}
}

func TestTimelineBuilder_GetWeeklyStats(t *testing.T) {
	repo, db := setupTimelineTestRepo(t)
	defer func() { _ = db.Close() }()

	builder := NewTimelineBuilder(repo)

	weekStart := time.Now().Truncate(24 * time.Hour)

	// Create a segment for day 0
	segment := &Segment{
		CameraID:    "cam_1",
		StartTime:   weekStart,
		EndTime:     weekStart.Add(time.Hour),
		Duration:    3600,
		FilePath:    "/path/to/segment.ts",
		FileSize:    1000,
		StorageTier: StorageTierHot,
	}
	if err := repo.Create(context.Background(), segment); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	stats, err := builder.GetWeeklyStats(context.Background(), "cam_1", weekStart)
	if err != nil {
		t.Fatalf("GetWeeklyStats failed: %v", err)
	}

	if len(stats) != 7 {
		t.Errorf("Expected 7 daily stats, got %d", len(stats))
	}

	// First day should have data
	if stats[0].SegmentCount != 1 {
		t.Errorf("Expected first day segment count 1, got %d", stats[0].SegmentCount)
	}
}

func TestTimelineBuilder_FindSegmentsContaining(t *testing.T) {
	repo, db := setupTimelineTestRepo(t)
	defer func() { _ = db.Close() }()

	builder := NewTimelineBuilder(repo)

	now := time.Now().Truncate(time.Second)

	// Create a segment
	segment := &Segment{
		CameraID:    "cam_1",
		StartTime:   now,
		EndTime:     now.Add(time.Hour),
		Duration:    3600,
		FilePath:    "/path/to/segment.ts",
		StorageTier: StorageTierHot,
	}
	if err := repo.Create(context.Background(), segment); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Find segment containing a point in the middle
	midpoint := now.Add(30 * time.Minute)
	segments, err := builder.FindSegmentsContaining(context.Background(), "cam_1", midpoint)
	if err != nil {
		t.Fatalf("FindSegmentsContaining failed: %v", err)
	}

	if len(segments) != 1 {
		t.Errorf("Expected 1 segment, got %d", len(segments))
	}
}

func TestTimelineBuilder_FindSegmentsContaining_NotFound(t *testing.T) {
	repo, db := setupTimelineTestRepo(t)
	defer func() { _ = db.Close() }()

	builder := NewTimelineBuilder(repo)

	segments, err := builder.FindSegmentsContaining(context.Background(), "cam_1", time.Now())
	if err != nil {
		t.Fatalf("FindSegmentsContaining failed: %v", err)
	}

	if len(segments) != 0 {
		t.Errorf("Expected 0 segments, got %d", len(segments))
	}
}

func TestTimelineBuilder_GetPlaybackURL(t *testing.T) {
	repo, db := setupTimelineTestRepo(t)
	defer func() { _ = db.Close() }()

	builder := NewTimelineBuilder(repo)

	now := time.Now().Truncate(time.Second)

	// Create a segment
	segment := &Segment{
		CameraID:    "cam_1",
		StartTime:   now,
		EndTime:     now.Add(time.Hour),
		Duration:    3600,
		FilePath:    "/path/to/segment.ts",
		StorageTier: StorageTierHot,
	}
	if err := repo.Create(context.Background(), segment); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Get playback URL for 30 minutes in
	playbackTime := now.Add(30 * time.Minute)
	path, offset, err := builder.GetPlaybackURL(context.Background(), "cam_1", playbackTime)
	if err != nil {
		t.Fatalf("GetPlaybackURL failed: %v", err)
	}

	if path != "/path/to/segment.ts" {
		t.Errorf("Expected path /path/to/segment.ts, got %s", path)
	}
	if offset != 1800 { // 30 minutes in seconds
		t.Errorf("Expected offset 1800, got %f", offset)
	}
}

func TestTimelineBuilder_GetPlaybackURL_NotFound(t *testing.T) {
	repo, db := setupTimelineTestRepo(t)
	defer func() { _ = db.Close() }()

	builder := NewTimelineBuilder(repo)

	_, _, err := builder.GetPlaybackURL(context.Background(), "cam_1", time.Now())
	if err != ErrNoSegmentFound {
		t.Errorf("Expected ErrNoSegmentFound, got %v", err)
	}
}

func TestTimelineError(t *testing.T) {
	err := TimelineError("test error")
	if err.Error() != "test error" {
		t.Errorf("Expected 'test error', got '%s'", err.Error())
	}
}

func TestMergeTimelines_Empty(t *testing.T) {
	result := MergeTimelines(nil)
	if result != nil {
		t.Error("Expected nil for empty input")
	}

	result = MergeTimelines([]*Timeline{})
	if result != nil {
		t.Error("Expected nil for empty slice")
	}
}

func TestMergeTimelines_Single(t *testing.T) {
	now := time.Now()
	timeline := &Timeline{
		CameraID:  "cam_1",
		StartTime: now,
		EndTime:   now.Add(time.Hour),
		TotalSize: 1000,
		Segments: []TimelineSegment{
			{
				StartTime: now,
				EndTime:   now.Add(30 * time.Minute),
				Type:      "recording",
			},
		},
	}

	result := MergeTimelines([]*Timeline{timeline})
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if result.CameraID != "all" {
		t.Errorf("Expected CameraID 'all', got %s", result.CameraID)
	}
}

func TestMergeTimelines_Multiple(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	timeline1 := &Timeline{
		CameraID:  "cam_1",
		StartTime: now,
		EndTime:   now.Add(time.Hour),
		TotalSize: 1000,
		Segments: []TimelineSegment{
			{
				StartTime: now,
				EndTime:   now.Add(30 * time.Minute),
				Type:      "recording",
			},
		},
	}

	timeline2 := &Timeline{
		CameraID:  "cam_2",
		StartTime: now,
		EndTime:   now.Add(time.Hour),
		TotalSize: 2000,
		Segments: []TimelineSegment{
			{
				StartTime: now.Add(15 * time.Minute),
				EndTime:   now.Add(45 * time.Minute),
				Type:      "recording",
			},
		},
	}

	result := MergeTimelines([]*Timeline{timeline1, timeline2})
	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if result.TotalSize != 3000 {
		t.Errorf("Expected TotalSize 3000, got %d", result.TotalSize)
	}
}
