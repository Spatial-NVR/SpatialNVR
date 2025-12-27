package recording

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewDefaultSegmentHandler(t *testing.T) {
	tmpDir := t.TempDir()

	handler := NewDefaultSegmentHandler(tmpDir, filepath.Join(tmpDir, "thumbs"))

	if handler == nil {
		t.Fatal("NewDefaultSegmentHandler() returned nil")
	}
	if handler.storagePath != tmpDir {
		t.Errorf("storagePath = %q, want %q", handler.storagePath, tmpDir)
	}
}

func TestDefaultSegmentHandler_CreatePath(t *testing.T) {
	tmpDir := t.TempDir()
	handler := NewDefaultSegmentHandler(tmpDir, filepath.Join(tmpDir, "thumbs"))

	testTime := time.Date(2024, 1, 15, 10, 30, 45, 0, time.Local)
	path := handler.CreatePath("front_door", testTime)

	expected := filepath.Join(tmpDir, "front_door", "2024-01-15_10-30-45.mp4")
	if path != expected {
		t.Errorf("CreatePath() = %q, want %q", path, expected)
	}
}

func TestDefaultSegmentHandler_CreatePath_DifferentCameras(t *testing.T) {
	tmpDir := t.TempDir()
	handler := NewDefaultSegmentHandler(tmpDir, filepath.Join(tmpDir, "thumbs"))

	testTime := time.Date(2024, 6, 20, 15, 45, 0, 0, time.Local)

	cameras := []string{"cam1", "back_yard", "garage_camera"}

	for _, cam := range cameras {
		path := handler.CreatePath(cam, testTime)
		expectedDir := filepath.Join(tmpDir, cam)
		if filepath.Dir(path) != expectedDir {
			t.Errorf("CreatePath(%q) directory = %q, want %q", cam, filepath.Dir(path), expectedDir)
		}
	}
}

func TestDefaultSegmentHandler_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	handler := NewDefaultSegmentHandler(tmpDir, filepath.Join(tmpDir, "thumbs"))

	// Create test files
	segmentPath := filepath.Join(tmpDir, "test_segment.mp4")
	thumbnailPath := filepath.Join(tmpDir, "thumbs", "test_segment.jpg")

	_ = os.MkdirAll(filepath.Join(tmpDir, "thumbs"), 0755)
	_ = os.WriteFile(segmentPath, []byte("video content"), 0644)
	_ = os.WriteFile(thumbnailPath, []byte("image content"), 0644)

	segment := &Segment{
		FilePath:  segmentPath,
		Thumbnail: thumbnailPath,
	}

	err := handler.Delete(segment)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	// Verify files are deleted
	if _, err := os.Stat(segmentPath); !os.IsNotExist(err) {
		t.Error("Segment file should be deleted")
	}
	if _, err := os.Stat(thumbnailPath); !os.IsNotExist(err) {
		t.Error("Thumbnail file should be deleted")
	}
}

func TestDefaultSegmentHandler_Delete_FileNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	handler := NewDefaultSegmentHandler(tmpDir, filepath.Join(tmpDir, "thumbs"))

	segment := &Segment{
		FilePath:  filepath.Join(tmpDir, "nonexistent.mp4"),
		Thumbnail: "",
	}

	// Should not return error if file doesn't exist
	err := handler.Delete(segment)
	if err != nil {
		t.Errorf("Delete() error = %v, want nil for non-existent file", err)
	}
}

func TestDefaultSegmentHandler_Delete_NoThumbnail(t *testing.T) {
	tmpDir := t.TempDir()
	handler := NewDefaultSegmentHandler(tmpDir, filepath.Join(tmpDir, "thumbs"))

	// Create test file
	segmentPath := filepath.Join(tmpDir, "test_segment.mp4")
	_ = os.WriteFile(segmentPath, []byte("video content"), 0644)

	segment := &Segment{
		FilePath:  segmentPath,
		Thumbnail: "", // No thumbnail
	}

	err := handler.Delete(segment)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	if _, err := os.Stat(segmentPath); !os.IsNotExist(err) {
		t.Error("Segment file should be deleted")
	}
}

func TestDefaultSegmentHandler_CalculateChecksum(t *testing.T) {
	tmpDir := t.TempDir()
	handler := NewDefaultSegmentHandler(tmpDir, filepath.Join(tmpDir, "thumbs"))

	// Create test file with known content
	testFile := filepath.Join(tmpDir, "test.mp4")
	content := []byte("test video content for checksum")
	_ = os.WriteFile(testFile, content, 0644)

	checksum, err := handler.CalculateChecksum(testFile)
	if err != nil {
		t.Errorf("CalculateChecksum() error = %v", err)
	}

	// SHA256 hash should be 64 characters hex
	if len(checksum) != 64 {
		t.Errorf("Checksum length = %d, want 64", len(checksum))
	}

	// Should be consistent
	checksum2, _ := handler.CalculateChecksum(testFile)
	if checksum != checksum2 {
		t.Error("Checksum should be deterministic")
	}
}

func TestDefaultSegmentHandler_CalculateChecksum_FileNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	handler := NewDefaultSegmentHandler(tmpDir, filepath.Join(tmpDir, "thumbs"))

	_, err := handler.CalculateChecksum(filepath.Join(tmpDir, "nonexistent.mp4"))
	if err == nil {
		t.Error("CalculateChecksum() should return error for non-existent file")
	}
}

func TestDefaultSegmentHandler_ExtractMetadata_FileNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	handler := NewDefaultSegmentHandler(tmpDir, filepath.Join(tmpDir, "thumbs"))

	_, err := handler.ExtractMetadata(filepath.Join(tmpDir, "nonexistent.mp4"))
	if err == nil {
		t.Error("ExtractMetadata() should return error for non-existent file")
	}
}

func TestDefaultSegmentHandler_ValidateSegment_FileNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	handler := NewDefaultSegmentHandler(tmpDir, filepath.Join(tmpDir, "thumbs"))

	err := handler.ValidateSegment(filepath.Join(tmpDir, "nonexistent.mp4"))
	if err == nil {
		t.Error("ValidateSegment() should return error for non-existent file")
	}
}

func TestDefaultSegmentHandler_ValidateSegment_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	handler := NewDefaultSegmentHandler(tmpDir, filepath.Join(tmpDir, "thumbs"))

	// Create empty file
	emptyFile := filepath.Join(tmpDir, "empty.mp4")
	_ = os.WriteFile(emptyFile, []byte{}, 0644)

	err := handler.ValidateSegment(emptyFile)
	if err == nil {
		t.Error("ValidateSegment() should return error for empty file")
	}
}

func TestDefaultSegmentHandler_MergeSegments_NoSegments(t *testing.T) {
	tmpDir := t.TempDir()
	handler := NewDefaultSegmentHandler(tmpDir, filepath.Join(tmpDir, "thumbs"))

	err := handler.MergeSegments([]string{}, filepath.Join(tmpDir, "output.mp4"))
	if err == nil {
		t.Error("MergeSegments() should return error when no segments provided")
	}
}

func TestDefaultSegmentHandler_GetStreamInfo_FileNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	handler := NewDefaultSegmentHandler(tmpDir, filepath.Join(tmpDir, "thumbs"))

	_, err := handler.GetStreamInfo(filepath.Join(tmpDir, "nonexistent.mp4"))
	if err == nil {
		t.Error("GetStreamInfo() should return error for non-existent file")
	}
}

func TestSegment_Fields(t *testing.T) {
	now := time.Now()
	segment := &Segment{
		ID:             "seg_123",
		CameraID:       "cam_1",
		StartTime:      now.Add(-time.Minute),
		EndTime:        now,
		Duration:       60.0,
		FilePath:       "/path/to/segment.mp4",
		FileSize:       1024 * 1024,
		StorageTier:    StorageTierHot,
		HasEvents:      true,
		EventCount:     3,
		Codec:          "h264",
		Resolution:     "1920x1080",
		Bitrate:        4000000,
		Thumbnail:      "/path/to/thumb.jpg",
		Checksum:       "abc123",
		RecordingMode:  string(RecordingModeContinuous),
		TriggerEventID: "event_456",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if segment.ID != "seg_123" {
		t.Errorf("ID = %q, want %q", segment.ID, "seg_123")
	}
	if segment.CameraID != "cam_1" {
		t.Errorf("CameraID = %q, want %q", segment.CameraID, "cam_1")
	}
	if segment.Duration != 60.0 {
		t.Errorf("Duration = %f, want 60.0", segment.Duration)
	}
	if segment.FileSize != 1024*1024 {
		t.Errorf("FileSize = %d, want %d", segment.FileSize, 1024*1024)
	}
	if segment.StorageTier != StorageTierHot {
		t.Errorf("StorageTier = %v, want %v", segment.StorageTier, StorageTierHot)
	}
	if !segment.HasEvents {
		t.Error("HasEvents should be true")
	}
	if segment.EventCount != 3 {
		t.Errorf("EventCount = %d, want 3", segment.EventCount)
	}
}

func TestSegmentMetadata_Fields(t *testing.T) {
	now := time.Now()
	metadata := &SegmentMetadata{
		Duration:   120.5,
		Codec:      "hevc",
		Resolution: "3840x2160",
		Bitrate:    8000000,
		FileSize:   2048 * 1024,
		StartTime:  now.Add(-2 * time.Minute),
		EndTime:    now,
	}

	if metadata.Duration != 120.5 {
		t.Errorf("Duration = %f, want 120.5", metadata.Duration)
	}
	if metadata.Codec != "hevc" {
		t.Errorf("Codec = %q, want %q", metadata.Codec, "hevc")
	}
	if metadata.Resolution != "3840x2160" {
		t.Errorf("Resolution = %q, want %q", metadata.Resolution, "3840x2160")
	}
}

func TestTimelineSegment_Fields(t *testing.T) {
	now := time.Now()
	ts := TimelineSegment{
		StartTime:  now.Add(-time.Hour),
		EndTime:    now,
		Type:       "recording",
		HasEvents:  true,
		EventCount: 5,
		SegmentIDs: []string{"seg_1", "seg_2", "seg_3"},
	}

	if ts.Type != "recording" {
		t.Errorf("Type = %q, want %q", ts.Type, "recording")
	}
	if !ts.HasEvents {
		t.Error("HasEvents should be true")
	}
	if len(ts.SegmentIDs) != 3 {
		t.Errorf("len(SegmentIDs) = %d, want 3", len(ts.SegmentIDs))
	}
}

func TestTimeline_Fields(t *testing.T) {
	now := time.Now()
	timeline := &Timeline{
		CameraID:   "cam_1",
		StartTime:  now.Add(-24 * time.Hour),
		EndTime:    now,
		Segments:   []TimelineSegment{{Type: "recording"}},
		TotalSize:  1024 * 1024 * 1024,
		TotalHours: 23.5,
	}

	if timeline.CameraID != "cam_1" {
		t.Errorf("CameraID = %q, want %q", timeline.CameraID, "cam_1")
	}
	if len(timeline.Segments) != 1 {
		t.Errorf("len(Segments) = %d, want 1", len(timeline.Segments))
	}
	if timeline.TotalHours != 23.5 {
		t.Errorf("TotalHours = %f, want 23.5", timeline.TotalHours)
	}
}

func TestStreamInfo_Fields(t *testing.T) {
	info := &StreamInfo{
		Codec:      "h264",
		Width:      1920,
		Height:     1080,
		FPS:        29.97,
		Bitrate:    5000000,
		Duration:   300.5,
		HasAudio:   true,
		AudioCodec: "aac",
	}

	if info.Codec != "h264" {
		t.Errorf("Codec = %q, want %q", info.Codec, "h264")
	}
	if info.Width != 1920 {
		t.Errorf("Width = %d, want 1920", info.Width)
	}
	if info.Height != 1080 {
		t.Errorf("Height = %d, want 1080", info.Height)
	}
	if info.FPS != 29.97 {
		t.Errorf("FPS = %f, want 29.97", info.FPS)
	}
	if !info.HasAudio {
		t.Error("HasAudio should be true")
	}
	if info.AudioCodec != "aac" {
		t.Errorf("AudioCodec = %q, want %q", info.AudioCodec, "aac")
	}
}

func TestFFmpegConfig_Fields(t *testing.T) {
	config := &FFmpegConfig{
		InputURL:        "rtsp://localhost:554/stream",
		OutputPath:      "/path/to/output.mp4",
		SegmentDuration: 60,
		Codec:           "copy",
		HWAccel:         "videotoolbox",
		ExtraArgs:       []string{"-preset", "fast"},
	}

	if config.InputURL != "rtsp://localhost:554/stream" {
		t.Errorf("InputURL = %q, want expected", config.InputURL)
	}
	if config.SegmentDuration != 60 {
		t.Errorf("SegmentDuration = %d, want 60", config.SegmentDuration)
	}
	if len(config.ExtraArgs) != 2 {
		t.Errorf("len(ExtraArgs) = %d, want 2", len(config.ExtraArgs))
	}
}

func TestStorageTier_Constants(t *testing.T) {
	tests := []struct {
		tier     StorageTier
		expected string
	}{
		{StorageTierHot, "hot"},
		{StorageTierWarm, "warm"},
		{StorageTierCold, "cold"},
	}

	for _, tt := range tests {
		if string(tt.tier) != tt.expected {
			t.Errorf("StorageTier(%v) = %q, want %q", tt.tier, string(tt.tier), tt.expected)
		}
	}
}

func TestRecordingMode_Constants(t *testing.T) {
	tests := []struct {
		mode     RecordingMode
		expected string
	}{
		{RecordingModeContinuous, "continuous"},
		{RecordingModeMotion, "motion"},
		{RecordingModeEvents, "events"},
	}

	for _, tt := range tests {
		if string(tt.mode) != tt.expected {
			t.Errorf("RecordingMode(%v) = %q, want %q", tt.mode, string(tt.mode), tt.expected)
		}
	}
}

func TestRecorderState_Constants(t *testing.T) {
	tests := []struct {
		state    RecorderState
		expected string
	}{
		{RecorderStateIdle, "idle"},
		{RecorderStateStarting, "starting"},
		{RecorderStateRunning, "running"},
		{RecorderStateStopping, "stopping"},
		{RecorderStateError, "error"},
	}

	for _, tt := range tests {
		if string(tt.state) != tt.expected {
			t.Errorf("RecorderState(%v) = %q, want %q", tt.state, string(tt.state), tt.expected)
		}
	}
}

func TestListOptions_Fields(t *testing.T) {
	now := time.Now()
	startTime := now.Add(-24 * time.Hour)
	endTime := now
	hasEvents := true
	tier := StorageTierHot

	opts := ListOptions{
		CameraID:  "cam_1",
		StartTime: &startTime,
		EndTime:   &endTime,
		HasEvents: &hasEvents,
		Tier:      &tier,
		Limit:     100,
		Offset:    20,
		OrderBy:   "start_time",
		OrderDesc: true,
	}

	if opts.CameraID != "cam_1" {
		t.Errorf("CameraID = %q, want %q", opts.CameraID, "cam_1")
	}
	if opts.Limit != 100 {
		t.Errorf("Limit = %d, want 100", opts.Limit)
	}
	if opts.Offset != 20 {
		t.Errorf("Offset = %d, want 20", opts.Offset)
	}
	if opts.OrderBy != "start_time" {
		t.Errorf("OrderBy = %q, want %q", opts.OrderBy, "start_time")
	}
	if !opts.OrderDesc {
		t.Error("OrderDesc should be true")
	}
}
