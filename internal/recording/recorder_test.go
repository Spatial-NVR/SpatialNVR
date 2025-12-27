package recording

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Spatial-NVR/SpatialNVR/internal/config"
)

func TestSanitizeStreamName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"front-door", "front_door"},
		{"back door", "back_door"},
		{"camera.1", "camera_1"},
		{"path/to/camera", "path_to_camera"},
		{"path\\to\\camera", "path_to_camera"},
		{"FRONT DOOR", "front_door"},
		{"Simple", "simple"},
		{"already_valid", "already_valid"},
		{"camera-1.test", "camera_1_test"},
		{"mixed-case_name.with/parts", "mixed_case_name_with_parts"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeStreamName(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeStreamName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNewRecorder(t *testing.T) {
	cfg := &config.CameraConfig{
		ID:   "test_camera",
		Name: "Test Camera",
		Recording: config.RecordingConfig{
			Enabled:         true,
			SegmentDuration: 60,
		},
	}

	tmpDir := t.TempDir()
	handler := NewDefaultSegmentHandler(tmpDir, tmpDir)

	recorder := NewRecorder(
		"test_camera",
		cfg,
		tmpDir,
		handler,
		func(segment *Segment) {},
	)

	if recorder == nil {
		t.Fatal("NewRecorder() returned nil")
	}
	if recorder.cameraID != "test_camera" {
		t.Errorf("cameraID = %q, want %q", recorder.cameraID, "test_camera")
	}
	if recorder.state != RecorderStateIdle {
		t.Errorf("initial state = %v, want %v", recorder.state, RecorderStateIdle)
	}
}

func TestRecorder_Status(t *testing.T) {
	cfg := &config.CameraConfig{
		ID:   "test_camera",
		Name: "Test Camera",
		Recording: config.RecordingConfig{
			SegmentDuration: 60,
		},
	}

	tmpDir := t.TempDir()
	handler := NewDefaultSegmentHandler(tmpDir, tmpDir)

	recorder := NewRecorder(
		"test_camera",
		cfg,
		tmpDir,
		handler,
		nil,
	)

	status := recorder.Status()

	if status == nil {
		t.Fatal("Status() returned nil")
	}
	if status.CameraID != "test_camera" {
		t.Errorf("CameraID = %q, want %q", status.CameraID, "test_camera")
	}
	if status.State != RecorderStateIdle {
		t.Errorf("State = %v, want %v", status.State, RecorderStateIdle)
	}
	if status.SegmentStart != nil {
		t.Error("SegmentStart should be nil for idle recorder")
	}
	if status.Uptime != 0 {
		t.Errorf("Uptime = %f, want 0", status.Uptime)
	}
}

func TestRecorder_Stop_WhenNotRunning(t *testing.T) {
	cfg := &config.CameraConfig{
		ID:   "test_camera",
		Name: "Test Camera",
		Recording: config.RecordingConfig{
			SegmentDuration: 60,
		},
	}

	tmpDir := t.TempDir()
	handler := NewDefaultSegmentHandler(tmpDir, tmpDir)

	recorder := NewRecorder(
		"test_camera",
		cfg,
		tmpDir,
		handler,
		nil,
	)

	// Stop should be a no-op when not running
	err := recorder.Stop()
	if err != nil {
		t.Errorf("Stop() error = %v, want nil", err)
	}
	if recorder.state != RecorderStateIdle {
		t.Errorf("state = %v, want %v", recorder.state, RecorderStateIdle)
	}
}

func TestRecorder_BuildFFmpegArgs(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name            string
		segmentDuration int
		hwAccel         string
		wantContains    []string
	}{
		{
			name:            "default segment duration",
			segmentDuration: 0,
			hwAccel:         "",
			wantContains:    []string{"-segment_time", "60"},
		},
		{
			name:            "custom segment duration",
			segmentDuration: 120,
			hwAccel:         "",
			wantContains:    []string{"-segment_time", "120"},
		},
		{
			name:            "with hardware acceleration",
			segmentDuration: 60,
			hwAccel:         "videotoolbox",
			wantContains:    []string{"-segment_time", "60"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.CameraConfig{
				ID:   "test_camera",
				Name: "Test Camera",
				Stream: config.StreamConfig{
					URL: "rtsp://localhost:554/stream",
				},
				Recording: config.RecordingConfig{
					SegmentDuration: tt.segmentDuration,
				},
				Advanced: config.AdvancedConfig{
					HWAccel: tt.hwAccel,
				},
			}

			handler := NewDefaultSegmentHandler(tmpDir, tmpDir)
			recorder := NewRecorder("test_camera", cfg, tmpDir, handler, nil)

			args := recorder.buildFFmpegArgs()

			for _, want := range tt.wantContains {
				found := false
				for _, arg := range args {
					if arg == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("buildFFmpegArgs() missing expected arg %q", want)
				}
			}
		})
	}
}

func TestRecorder_ParseTimeFromFilename(t *testing.T) {
	cfg := &config.CameraConfig{
		ID:   "test_camera",
		Name: "Test Camera",
	}

	tmpDir := t.TempDir()
	handler := NewDefaultSegmentHandler(tmpDir, tmpDir)
	recorder := NewRecorder("test_camera", cfg, tmpDir, handler, nil)

	tests := []struct {
		filename     string
		expectedTime string // Empty if should return zero time
	}{
		{
			filename:     "/path/to/camera/2024-01-15_10-30-00.mp4",
			expectedTime: "2024-01-15 10:30:00",
		},
		{
			filename:     "/path/to/camera/2023-12-31_23-59-59.mp4",
			expectedTime: "2023-12-31 23:59:59",
		},
		{
			filename:     "/path/to/camera/invalid.mp4",
			expectedTime: "", // Should return zero time
		},
		{
			filename:     "/path/to/camera/2024-13-45_25-61-61.mp4", // Invalid date
			expectedTime: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := recorder.parseTimeFromFilename(tt.filename)

			if tt.expectedTime == "" {
				if !result.IsZero() {
					t.Errorf("parseTimeFromFilename(%q) = %v, want zero time", tt.filename, result)
				}
			} else {
				expected, _ := time.ParseInLocation("2006-01-02 15:04:05", tt.expectedTime, time.Local)
				if !result.Equal(expected) {
					t.Errorf("parseTimeFromFilename(%q) = %v, want %v", tt.filename, result, expected)
				}
			}
		})
	}
}

func TestRecorder_SetError(t *testing.T) {
	cfg := &config.CameraConfig{
		ID:   "test_camera",
		Name: "Test Camera",
	}

	tmpDir := t.TempDir()
	handler := NewDefaultSegmentHandler(tmpDir, tmpDir)
	recorder := NewRecorder("test_camera", cfg, tmpDir, handler, nil)

	testError := "test error message"
	recorder.setError(context.DeadlineExceeded)

	status := recorder.Status()
	if status.State != RecorderStateError {
		t.Errorf("state = %v, want %v", status.State, RecorderStateError)
	}
	if status.LastError == "" {
		t.Error("LastError should not be empty after setError")
	}
	if status.LastErrorTime == nil {
		t.Error("LastErrorTime should not be nil after setError")
	}

	_ = testError
}

func TestRecorder_Start_DirectoryCreation(t *testing.T) {
	cfg := &config.CameraConfig{
		ID:   "test_camera",
		Name: "Test Camera",
		Stream: config.StreamConfig{
			URL: "rtsp://localhost:554/stream",
		},
		Recording: config.RecordingConfig{
			SegmentDuration: 60,
		},
	}

	tmpDir := t.TempDir()
	storagePath := filepath.Join(tmpDir, "recordings")
	handler := NewDefaultSegmentHandler(storagePath, tmpDir)

	recorder := NewRecorder("test_camera", cfg, storagePath, handler, nil)

	ctx, cancel := context.WithCancel(context.Background())

	// Start the recorder briefly
	err := recorder.Start(ctx)
	if err != nil {
		t.Errorf("Start() error = %v", err)
	}

	// Wait briefly for directory creation
	time.Sleep(50 * time.Millisecond)

	// Stop immediately
	cancel()
	_ = recorder.Stop()

	// Check if camera directory was created
	cameraDir := filepath.Join(storagePath, "test_camera")
	if _, err := os.Stat(cameraDir); os.IsNotExist(err) {
		t.Error("Camera directory was not created")
	}
}

func TestRecorder_Start_AlreadyRunning(t *testing.T) {
	cfg := &config.CameraConfig{
		ID:   "test_camera",
		Name: "Test Camera",
		Stream: config.StreamConfig{
			URL: "rtsp://localhost:554/stream",
		},
		Recording: config.RecordingConfig{
			SegmentDuration: 60,
		},
	}

	tmpDir := t.TempDir()
	handler := NewDefaultSegmentHandler(tmpDir, tmpDir)
	recorder := NewRecorder("test_camera", cfg, tmpDir, handler, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Manually set state to running
	recorder.mu.Lock()
	recorder.state = RecorderStateRunning
	recorder.mu.Unlock()

	// Start should be a no-op
	err := recorder.Start(ctx)
	if err != nil {
		t.Errorf("Start() error = %v when already running", err)
	}
}

func TestRecorder_ProcessCompletedSegment(t *testing.T) {
	cfg := &config.CameraConfig{
		ID:   "test_camera",
		Name: "Test Camera",
	}

	tmpDir := t.TempDir()
	handler := NewDefaultSegmentHandler(tmpDir, tmpDir)

	var receivedSegment *Segment
	callback := func(segment *Segment) {
		receivedSegment = segment
	}

	recorder := NewRecorder("test_camera", cfg, tmpDir, handler, callback)

	// Create a test file
	testFile := filepath.Join(tmpDir, "2024-01-15_10-30-00.mp4")
	testContent := []byte("test video content")
	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Process the segment
	recorder.processCompletedSegment(testFile)

	// Wait for callback
	time.Sleep(600 * time.Millisecond)

	if receivedSegment == nil {
		t.Fatal("Callback was not called")
	}
	if receivedSegment.CameraID != "test_camera" {
		t.Errorf("CameraID = %q, want %q", receivedSegment.CameraID, "test_camera")
	}
	if receivedSegment.FilePath != testFile {
		t.Errorf("FilePath = %q, want %q", receivedSegment.FilePath, testFile)
	}
}

func TestRecorder_ProcessCompletedSegment_FileNotExist(t *testing.T) {
	cfg := &config.CameraConfig{
		ID:   "test_camera",
		Name: "Test Camera",
	}

	tmpDir := t.TempDir()
	handler := NewDefaultSegmentHandler(tmpDir, tmpDir)

	var callbackCalled bool
	callback := func(segment *Segment) {
		callbackCalled = true
	}

	recorder := NewRecorder("test_camera", cfg, tmpDir, handler, callback)

	// Process a non-existent file
	recorder.processCompletedSegment("/nonexistent/file.mp4")

	time.Sleep(600 * time.Millisecond)

	if callbackCalled {
		t.Error("Callback should not be called for non-existent file")
	}
}

func TestRecorder_StatusWithTiming(t *testing.T) {
	cfg := &config.CameraConfig{
		ID:   "test_camera",
		Name: "Test Camera",
	}

	tmpDir := t.TempDir()
	handler := NewDefaultSegmentHandler(tmpDir, tmpDir)
	recorder := NewRecorder("test_camera", cfg, tmpDir, handler, nil)

	// Set timing information
	now := time.Now()
	recorder.mu.Lock()
	recorder.startTime = now.Add(-time.Minute)
	recorder.segmentStart = now.Add(-10 * time.Second)
	recorder.currentSegment = "/path/to/segment.mp4"
	recorder.bytesWritten = 1024 * 1024
	recorder.segmentsCount = 5
	recorder.lastError = "previous error"
	recorder.lastErrorTime = now.Add(-30 * time.Second)
	recorder.mu.Unlock()

	status := recorder.Status()

	if status.Uptime < 59 || status.Uptime > 61 {
		t.Errorf("Uptime = %f, want ~60", status.Uptime)
	}
	if status.SegmentStart == nil {
		t.Error("SegmentStart should not be nil")
	}
	if status.CurrentSegment != "/path/to/segment.mp4" {
		t.Errorf("CurrentSegment = %q, want /path/to/segment.mp4", status.CurrentSegment)
	}
	if status.BytesWritten != 1024*1024 {
		t.Errorf("BytesWritten = %d, want %d", status.BytesWritten, 1024*1024)
	}
	if status.SegmentsCreated != 5 {
		t.Errorf("SegmentsCreated = %d, want 5", status.SegmentsCreated)
	}
	if status.LastError != "previous error" {
		t.Errorf("LastError = %q, want 'previous error'", status.LastError)
	}
	if status.LastErrorTime == nil {
		t.Error("LastErrorTime should not be nil")
	}
}

func TestRecorderState_Values(t *testing.T) {
	// Test that all recorder state constants have expected string values
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
