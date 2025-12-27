package streaming

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewGo2RTCManager(t *testing.T) {
	manager := NewGo2RTCManager("/tmp/config.yaml", "/usr/bin/go2rtc")
	if manager == nil {
		t.Fatal("NewGo2RTCManager returned nil")
	}
	if manager.configPath != "/tmp/config.yaml" {
		t.Errorf("Expected config path /tmp/config.yaml, got %s", manager.configPath)
	}
	if manager.binaryPath != "/usr/bin/go2rtc" {
		t.Errorf("Expected binary path /usr/bin/go2rtc, got %s", manager.binaryPath)
	}
	if manager.apiURL != fmt.Sprintf("http://localhost:%d", DefaultGo2RTCPort) {
		t.Errorf("Unexpected API URL: %s", manager.apiURL)
	}
}

func TestGo2RTCManager_IsRunning(t *testing.T) {
	manager := NewGo2RTCManager("", "")
	if manager.IsRunning() {
		t.Error("New manager should not be running")
	}
}

func TestGo2RTCManager_GetAPIURL(t *testing.T) {
	manager := NewGo2RTCManager("", "")
	expected := fmt.Sprintf("http://localhost:%d", DefaultGo2RTCPort)
	if manager.GetAPIURL() != expected {
		t.Errorf("Expected API URL %s, got %s", expected, manager.GetAPIURL())
	}
}

func TestGo2RTCManager_GetWebRTCURL(t *testing.T) {
	manager := NewGo2RTCManager("", "")
	url := manager.GetWebRTCURL("camera1")
	expected := fmt.Sprintf("ws://localhost:%d/api/ws?src=camera1", DefaultGo2RTCPort)
	if url != expected {
		t.Errorf("Expected %s, got %s", expected, url)
	}
}

func TestGo2RTCManager_GetBackchannelURL(t *testing.T) {
	manager := NewGo2RTCManager("", "")
	url := manager.GetBackchannelURL("camera1")
	expected := fmt.Sprintf("ws://localhost:%d/api/ws?src=camera1&backchannel=1", DefaultGo2RTCPort)
	if url != expected {
		t.Errorf("Expected %s, got %s", expected, url)
	}
}

func TestGo2RTCManager_Stop_NotRunning(t *testing.T) {
	manager := NewGo2RTCManager("", "")
	// Stop when not running should not error
	err := manager.Stop()
	if err != nil {
		t.Errorf("Stop on non-running manager should not error: %v", err)
	}
}

func TestGo2RTCManager_Start_AlreadyRunning(t *testing.T) {
	manager := NewGo2RTCManager("", "")
	manager.running = true

	ctx := context.Background()
	err := manager.Start(ctx)
	if err == nil {
		t.Error("Start on running manager should error")
	}
	if err.Error() != "go2rtc is already running" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestGo2RTCManager_Reload_NotRunning(t *testing.T) {
	manager := NewGo2RTCManager("", "")
	err := manager.Reload()
	if err == nil {
		t.Error("Reload on non-running manager should error")
	}
	if err.Error() != "go2rtc is not running" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestGo2RTCManager_GetStreams_MockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/streams" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"camera1": {"url": "rtsp://localhost/stream1"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	manager := NewGo2RTCManager("", "")
	manager.apiURL = server.URL

	streams, err := manager.GetStreams()
	if err != nil {
		t.Fatalf("GetStreams failed: %v", err)
	}
	if streams == nil {
		t.Error("Expected non-nil streams")
	}
}

func TestGo2RTCManager_AddStream_MockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/streams" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	manager := NewGo2RTCManager("", "")
	manager.apiURL = server.URL

	err := manager.AddStream("test", "rtsp://localhost/stream")
	if err != nil {
		t.Errorf("AddStream failed: %v", err)
	}
}

func TestGo2RTCManager_AddStream_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid stream"))
	}))
	defer server.Close()

	manager := NewGo2RTCManager("", "")
	manager.apiURL = server.URL

	err := manager.AddStream("test", "rtsp://localhost/stream")
	if err == nil {
		t.Error("Expected error for bad request")
	}
}

func TestGo2RTCManager_RemoveStream_MockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/streams" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	manager := NewGo2RTCManager("", "")
	manager.apiURL = server.URL

	err := manager.RemoveStream("test")
	if err != nil {
		t.Errorf("RemoveStream failed: %v", err)
	}
}

func TestGo2RTCManager_GetStreamInfo_MockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/streams" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name": "camera1", "status": "active"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	manager := NewGo2RTCManager("", "")
	manager.apiURL = server.URL

	info, err := manager.GetStreamInfo("camera1")
	if err != nil {
		t.Fatalf("GetStreamInfo failed: %v", err)
	}
	if info == nil {
		t.Error("Expected non-nil info")
	}
	if info["name"] != "camera1" {
		t.Errorf("Expected name 'camera1', got %v", info["name"])
	}
}

func TestLogWriter(t *testing.T) {
	writer := &logWriter{
		logger: slog.Default(),
		level:  slog.LevelInfo,
	}

	// Test with trailing newline
	n, err := writer.Write([]byte("test message\n"))
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}
	if n != 13 {
		t.Errorf("Expected 13 bytes written, got %d", n)
	}

	// Test without trailing newline
	n, err = writer.Write([]byte("test"))
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}
	if n != 4 {
		t.Errorf("Expected 4 bytes written, got %d", n)
	}

	// Test empty message
	n, err = writer.Write([]byte(""))
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}
	if n != 0 {
		t.Errorf("Expected 0 bytes written, got %d", n)
	}

	// Test just newline
	n, err = writer.Write([]byte("\n"))
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}
	if n != 1 {
		t.Errorf("Expected 1 byte written, got %d", n)
	}
}

// Config tests
func TestNewConfigGenerator(t *testing.T) {
	gen := NewConfigGenerator()
	if gen == nil {
		t.Fatal("NewConfigGenerator returned nil")
	}
	if gen.apiPort != DefaultGo2RTCPort {
		t.Errorf("Expected API port %d, got %d", DefaultGo2RTCPort, gen.apiPort)
	}
	if gen.rtspPort != DefaultRTSPPort {
		t.Errorf("Expected RTSP port %d, got %d", DefaultRTSPPort, gen.rtspPort)
	}
	if gen.webrtcPort != DefaultWebRTCPort {
		t.Errorf("Expected WebRTC port %d, got %d", DefaultWebRTCPort, gen.webrtcPort)
	}
}

func TestConfigGenerator_WithPorts(t *testing.T) {
	gen := NewConfigGenerator().WithPorts(8080, 8554, 8555)
	if gen.apiPort != 8080 {
		t.Errorf("Expected API port 8080, got %d", gen.apiPort)
	}
	if gen.rtspPort != 8554 {
		t.Errorf("Expected RTSP port 8554, got %d", gen.rtspPort)
	}
	if gen.webrtcPort != 8555 {
		t.Errorf("Expected WebRTC port 8555, got %d", gen.webrtcPort)
	}
}

func TestConfigGenerator_Generate(t *testing.T) {
	gen := NewConfigGenerator()

	cameras := []CameraStream{
		{
			ID:       "camera1",
			Name:     "Front Door",
			URL:      "rtsp://192.168.1.100/stream",
			Username: "admin",
			Password: "password",
		},
		{
			ID:       "camera2",
			Name:     "Back Yard",
			URL:      "rtsp://192.168.1.101/stream",
			SubURL:   "rtsp://192.168.1.101/sub",
		},
	}

	config := gen.Generate(cameras)
	if config == nil {
		t.Fatal("Generate returned nil")
	}

	// Check API config
	if config.API.Listen != fmt.Sprintf(":%d", DefaultGo2RTCPort) {
		t.Errorf("Unexpected API listen: %s", config.API.Listen)
	}
	if config.API.Origin != "*" {
		t.Errorf("Expected origin '*', got %s", config.API.Origin)
	}

	// Check RTSP config
	if config.RTSP.Listen != fmt.Sprintf(":%d", DefaultRTSPPort) {
		t.Errorf("Unexpected RTSP listen: %s", config.RTSP.Listen)
	}

	// Check streams
	if len(config.Streams) != 3 { // camera1, camera2, camera2_sub
		t.Errorf("Expected 3 streams, got %d", len(config.Streams))
	}

	// Check camera1 stream has credentials and audio transcoding
	if streams, ok := config.Streams["camera1"]; ok {
		if len(streams) != 2 {
			t.Errorf("Expected 2 sources for camera1 (main + opus transcode), got %d", len(streams))
		}
		// URL should contain credentials
		if streams[0] != "rtsp://admin:password@192.168.1.100/stream" {
			t.Errorf("Unexpected stream URL: %s", streams[0])
		}
		// Second source should be opus transcode
		if len(streams) > 1 && streams[1] != "ffmpeg:camera1#audio=opus" {
			t.Errorf("Unexpected opus transcode source: %s", streams[1])
		}
	} else {
		t.Error("camera1 stream not found")
	}

	// Check camera2 has sub stream
	if _, ok := config.Streams["camera2_sub"]; !ok {
		t.Error("camera2_sub stream not found")
	}
}

func TestConfigGenerator_Generate_EmptyCameras(t *testing.T) {
	gen := NewConfigGenerator()
	config := gen.Generate([]CameraStream{})

	if config == nil {
		t.Fatal("Generate returned nil")
	}
	if len(config.Streams) != 0 {
		t.Errorf("Expected 0 streams, got %d", len(config.Streams))
	}
}

func TestConfigGenerator_WriteToFile(t *testing.T) {
	gen := NewConfigGenerator()
	config := gen.Generate([]CameraStream{
		{ID: "test", URL: "rtsp://localhost/test"},
	})

	tmpFile := filepath.Join(t.TempDir(), "go2rtc.yaml")
	err := gen.WriteToFile(config, tmpFile)
	if err != nil {
		t.Fatalf("WriteToFile failed: %v", err)
	}

	// Read and verify file exists and has content
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if len(data) == 0 {
		t.Error("File is empty")
	}

	// Check for header
	content := string(data)
	if !strings.HasPrefix(content, "# go2rtc configuration") {
		t.Errorf("Missing config header, got: %s", content[:min(50, len(content))])
	}
}

func TestConfigGenerator_buildStreamURL(t *testing.T) {
	gen := NewConfigGenerator()

	tests := []struct {
		name     string
		url      string
		username string
		password string
		expected string
	}{
		{
			name:     "no credentials",
			url:      "rtsp://192.168.1.100/stream",
			username: "",
			password: "",
			expected: "rtsp://192.168.1.100/stream",
		},
		{
			name:     "with credentials",
			url:      "rtsp://192.168.1.100/stream",
			username: "admin",
			password: "pass",
			expected: "rtsp://admin:pass@192.168.1.100/stream",
		},
		{
			name:     "username only",
			url:      "rtsp://192.168.1.100/stream",
			username: "admin",
			password: "",
			expected: "rtsp://admin@192.168.1.100/stream",
		},
		{
			name:     "http url with credentials",
			url:      "http://192.168.1.100/stream",
			username: "admin",
			password: "pass",
			expected: "http://admin:pass@192.168.1.100/stream",
		},
		{
			name:     "https url with credentials",
			url:      "https://192.168.1.100/stream",
			username: "admin",
			password: "pass",
			expected: "https://admin:pass@192.168.1.100/stream",
		},
		{
			name:     "unknown scheme",
			url:      "tcp://192.168.1.100/stream",
			username: "admin",
			password: "pass",
			expected: "tcp://192.168.1.100/stream", // unchanged
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gen.buildStreamURL(tt.url, tt.username, tt.password)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestSanitizeStreamName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"camera1", "camera1"},
		{"Camera1", "camera1"},
		{"front-door", "front_door"},
		{"back.yard", "back_yard"},
		{"camera 1", "camera_1"},
		{"cam/1", "cam_1"},
		{"cam\\1", "cam_1"},
		{"Front-Door.1", "front_door_1"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeStreamName(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeStreamName(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGetStreamURL(t *testing.T) {
	tests := []struct {
		cameraID string
		format   string
		apiPort  int
		expected string
	}{
		{"camera1", "rtsp", DefaultGo2RTCPort, fmt.Sprintf("rtsp://localhost:%d/camera1", DefaultRTSPPort)},
		{"camera1", "webrtc", DefaultGo2RTCPort, fmt.Sprintf("http://localhost:%d/api/webrtc?src=camera1", DefaultGo2RTCPort)},
		{"camera1", "hls", DefaultGo2RTCPort, fmt.Sprintf("http://localhost:%d/api/stream.m3u8?src=camera1", DefaultGo2RTCPort)},
		{"camera1", "mse", DefaultGo2RTCPort, fmt.Sprintf("http://localhost:%d/api/ws?src=camera1", DefaultGo2RTCPort)},
		{"camera1", "mjpeg", DefaultGo2RTCPort, fmt.Sprintf("http://localhost:%d/api/frame.jpeg?src=camera1", DefaultGo2RTCPort)},
		{"camera1", "unknown", DefaultGo2RTCPort, fmt.Sprintf("http://localhost:%d/api/stream.m3u8?src=camera1", DefaultGo2RTCPort)},
		{"Front-Door", "webrtc", 8080, "http://localhost:8080/api/webrtc?src=front_door"},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("%s_%s", tt.cameraID, tt.format)
		t.Run(name, func(t *testing.T) {
			result := GetStreamURL(tt.cameraID, tt.format, tt.apiPort)
			if result != tt.expected {
				t.Errorf("GetStreamURL(%q, %q, %d) = %q, expected %q", tt.cameraID, tt.format, tt.apiPort, result, tt.expected)
			}
		})
	}
}

func TestDefaultPorts(t *testing.T) {
	if DefaultGo2RTCPort != 1984 {
		t.Errorf("Expected DefaultGo2RTCPort 1984, got %d", DefaultGo2RTCPort)
	}
	if DefaultRTSPPort != 8554 {
		t.Errorf("Expected DefaultRTSPPort 8554, got %d", DefaultRTSPPort)
	}
	if DefaultWebRTCPort != 8555 {
		t.Errorf("Expected DefaultWebRTCPort 8555, got %d", DefaultWebRTCPort)
	}
}

func TestGo2RTCManager_GetStreams_Error(t *testing.T) {
	manager := NewGo2RTCManager("", "")
	// Use an invalid URL that will cause connection error
	manager.apiURL = "http://127.0.0.1:1"

	_, err := manager.GetStreams()
	if err == nil {
		t.Error("Expected error for connection failure")
	}
}

func TestGo2RTCManager_AddStream_ConnectionError(t *testing.T) {
	manager := NewGo2RTCManager("", "")
	manager.apiURL = "http://127.0.0.1:1"

	err := manager.AddStream("test", "rtsp://localhost/stream")
	if err == nil {
		t.Error("Expected error for connection failure")
	}
}

func TestGo2RTCManager_RemoveStream_ConnectionError(t *testing.T) {
	manager := NewGo2RTCManager("", "")
	manager.apiURL = "http://127.0.0.1:1"

	err := manager.RemoveStream("test")
	if err == nil {
		t.Error("Expected error for connection failure")
	}
}

func TestGo2RTCManager_GetStreamInfo_Error(t *testing.T) {
	manager := NewGo2RTCManager("", "")
	manager.apiURL = "http://127.0.0.1:1"

	_, err := manager.GetStreamInfo("camera1")
	if err == nil {
		t.Error("Expected error for connection failure")
	}
}

func TestGo2RTCManager_GetStreamInfo_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	manager := NewGo2RTCManager("", "")
	manager.apiURL = server.URL

	_, err := manager.GetStreamInfo("camera1")
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestGo2RTCManager_Reload_APISuccess(t *testing.T) {
	reloadCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/config/reload" && r.Method == http.MethodPost {
			reloadCalled = true
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	manager := NewGo2RTCManager("", "")
	manager.apiURL = server.URL
	manager.running = true

	err := manager.Reload()
	if err != nil {
		t.Errorf("Reload failed: %v", err)
	}
	if !reloadCalled {
		t.Error("Reload API was not called")
	}
}

func TestGo2RTCManager_Reload_APIFailure_TriggersRestart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/config/reload" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	manager := NewGo2RTCManager("", "")
	manager.apiURL = server.URL
	manager.running = true

	// Reload will fail and try to restart (which will also fail because no binary)
	err := manager.Reload()
	// Error expected since restart will fail
	if err == nil {
		t.Error("Expected error when restart fails")
	}
}

func TestGo2RTCManager_Start_NoBinary(t *testing.T) {
	manager := NewGo2RTCManager("/tmp/test.yaml", "")

	ctx := context.Background()
	err := manager.Start(ctx)
	if err == nil {
		t.Error("Expected error when binary not found")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

func TestGo2RTCManager_Start_InvalidBinary(t *testing.T) {
	// Create a temp file that's not executable
	tmpFile := filepath.Join(t.TempDir(), "fake_go2rtc")
	_ = os.WriteFile(tmpFile, []byte("not a binary"), 0644)

	manager := NewGo2RTCManager("", tmpFile)

	ctx := context.Background()
	err := manager.Start(ctx)
	if err == nil {
		t.Error("Expected error when binary is invalid")
	}
}

func TestGo2RTCManager_findBinary_NotFound(t *testing.T) {
	manager := NewGo2RTCManager("", "")

	_, err := manager.findBinary()
	if err == nil {
		t.Error("Expected error when binary not found")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' in error, got: %v", err)
	}
}

func TestGo2RTCManager_findBinary_InPath(t *testing.T) {
	// Create a fake binary in temp dir and add to PATH
	tmpDir := t.TempDir()
	fakeBinary := filepath.Join(tmpDir, "go2rtc")
	_ = os.WriteFile(fakeBinary, []byte("fake"), 0755)

	// Save original PATH and restore after test
	origPath := os.Getenv("PATH")
	_ = os.Setenv("PATH", tmpDir+":"+origPath)
	defer func() { _ = os.Setenv("PATH", origPath) }()

	manager := NewGo2RTCManager("", "")
	path, err := manager.findBinary()
	if err != nil {
		t.Errorf("findBinary failed: %v", err)
	}
	if path == "" {
		t.Error("Expected non-empty path")
	}
}

func TestGo2RTCManager_findBinary_LocalBin(t *testing.T) {
	// Save current directory
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()

	// Create temp directory and change to it
	tmpDir := t.TempDir()
	_ = os.Chdir(tmpDir)

	// Create bin/go2rtc
	_ = os.Mkdir("bin", 0755)
	fakeBinary := filepath.Join("bin", "go2rtc")
	_ = os.WriteFile(fakeBinary, []byte("fake"), 0755)

	manager := NewGo2RTCManager("", "")
	path, err := manager.findBinary()
	if err != nil {
		t.Errorf("findBinary failed: %v", err)
	}
	if path == "" {
		t.Error("Expected non-empty path")
	}
	if !strings.Contains(path, "go2rtc") {
		t.Errorf("Expected path to contain go2rtc, got: %s", path)
	}
}

func TestConfigGenerator_WriteToFile_MarshalError(t *testing.T) {
	gen := NewConfigGenerator()
	// Create a valid config
	config := gen.Generate([]CameraStream{})

	// Try to write to an invalid path
	err := gen.WriteToFile(config, "/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestConfigGenerator_buildStreamURL_UserOnly_HTTP(t *testing.T) {
	gen := NewConfigGenerator()

	// Test HTTP with username only (no password)
	result := gen.buildStreamURL("http://192.168.1.100/stream", "admin", "")
	expected := "http://admin@192.168.1.100/stream"
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestConfigGenerator_buildStreamURL_UserOnly_HTTPS(t *testing.T) {
	gen := NewConfigGenerator()

	// Test HTTPS with username only
	result := gen.buildStreamURL("https://192.168.1.100/stream", "admin", "")
	expected := "https://admin@192.168.1.100/stream"
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestGo2RTCManager_Restart_Error(t *testing.T) {
	manager := NewGo2RTCManager("", "")
	manager.running = false

	// Restart should fail because Start will fail (no binary)
	err := manager.Restart()
	if err == nil {
		t.Error("Expected error when restart fails to start")
	}
}

func TestGo2RTCManager_Stop_WithNilCmd(t *testing.T) {
	manager := NewGo2RTCManager("", "")
	manager.running = true
	// cmd is nil so Stop returns early (no-op for safety)
	err := manager.Stop()
	if err != nil {
		t.Errorf("Stop should not error when cmd is nil: %v", err)
	}
	// running stays true because we can't actually stop anything (no process to stop)
	// This is the expected behavior when cmd or cmd.Process is nil
}

func TestLogWriter_EmptyAfterTrim(t *testing.T) {
	writer := &logWriter{
		logger: slog.Default(),
		level:  slog.LevelInfo,
	}

	// Test with only newline - should result in empty string after trim
	n, err := writer.Write([]byte("\n"))
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}
	if n != 1 {
		t.Errorf("Expected 1 byte written, got %d", n)
	}
}

func TestGo2RTCManager_waitForReady_ImmediateSuccess(t *testing.T) {
	apiCallCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCallCount++
		if r.URL.Path == "/api" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	manager := NewGo2RTCManager("", "")
	manager.apiURL = server.URL

	ctx := context.Background()
	err := manager.waitForReady(ctx, 5*time.Second)
	if err != nil {
		t.Errorf("waitForReady failed: %v", err)
	}
	if apiCallCount == 0 {
		t.Error("API should have been called")
	}
}

func TestGo2RTCManager_waitForReady_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always return not found to simulate not ready
		http.NotFound(w, r)
	}))
	defer server.Close()

	manager := NewGo2RTCManager("", "")
	manager.apiURL = server.URL

	ctx := context.Background()
	err := manager.waitForReady(ctx, 200*time.Millisecond)
	if err == nil {
		t.Error("Expected timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("Expected timeout error, got: %v", err)
	}
}

func TestGo2RTCManager_waitForReady_ContextCanceled(t *testing.T) {
	manager := NewGo2RTCManager("", "")
	manager.apiURL = "http://127.0.0.1:1" // Invalid URL

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := manager.waitForReady(ctx, 5*time.Second)
	if err == nil {
		t.Error("Expected error when context is canceled")
	}
}

func TestGo2RTCManager_waitForReady_EventualSuccess(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.URL.Path == "/api" {
			// Succeed after a few attempts
			if callCount >= 3 {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	manager := NewGo2RTCManager("", "")
	manager.apiURL = server.URL

	ctx := context.Background()
	err := manager.waitForReady(ctx, 5*time.Second)
	if err != nil {
		t.Errorf("waitForReady failed: %v", err)
	}
	if callCount < 3 {
		t.Errorf("Expected at least 3 calls, got %d", callCount)
	}
}

func TestGo2RTCManager_Start_WithConfigPath(t *testing.T) {
	// Create a temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	_ = os.WriteFile(configPath, []byte("api:\n  listen: :1984\n"), 0644)

	manager := NewGo2RTCManager(configPath, "")

	ctx := context.Background()
	// Will fail because no binary, but tests config path handling
	err := manager.Start(ctx)
	if err == nil {
		t.Error("Expected error when binary not found")
	}
}

func TestGo2RTCManager_Start_EmptyConfigPath(t *testing.T) {
	manager := NewGo2RTCManager("", "")

	ctx := context.Background()
	err := manager.Start(ctx)
	if err == nil {
		t.Error("Expected error when binary not found")
	}
}

func TestConfigGenerator_Generate_MultipleStreams(t *testing.T) {
	gen := NewConfigGenerator()

	cameras := []CameraStream{
		{ID: "cam1", URL: "rtsp://host1/stream1"},
		{ID: "cam2", URL: "rtsp://host2/stream2", SubURL: "rtsp://host2/sub"},
		{ID: "cam3", URL: "rtsp://host3/stream3", Username: "admin", Password: "pass"},
	}

	config := gen.Generate(cameras)

	if len(config.Streams) != 4 { // cam1, cam2, cam2_sub, cam3
		t.Errorf("Expected 4 streams, got %d", len(config.Streams))
	}

	// Verify credentials are added (2 sources: main + opus transcode)
	if streams, ok := config.Streams["cam3"]; ok {
		if len(streams) != 2 || !strings.Contains(streams[0], "admin:pass@") {
			t.Errorf("Credentials not properly added: %v", streams)
		}
	}
}

func TestGo2RTCManager_RemoveStream_RequestCreationError(t *testing.T) {
	manager := NewGo2RTCManager("", "")
	// Use invalid URL to cause request creation error
	manager.apiURL = "://invalid"

	err := manager.RemoveStream("test")
	if err == nil {
		t.Error("Expected error for invalid URL")
	}
}

func TestGo2RTCManager_findBinary_CurrentDir(t *testing.T) {
	// Save current directory
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()

	// Create temp directory and change to it
	tmpDir := t.TempDir()
	_ = os.Chdir(tmpDir)

	// Create ./go2rtc (not in bin/)
	fakeBinary := "go2rtc"
	_ = os.WriteFile(fakeBinary, []byte("fake"), 0755)

	manager := NewGo2RTCManager("", "")
	path, err := manager.findBinary()
	if err != nil {
		t.Errorf("findBinary failed: %v", err)
	}
	if path == "" {
		t.Error("Expected non-empty path")
	}
}

func TestGo2RTCManager_monitor_NilCmd(t *testing.T) {
	manager := NewGo2RTCManager("", "")
	manager.cmd = nil

	// monitor should return early when cmd is nil
	// This runs synchronously so we just call it
	manager.monitor()

	// No panic means success
}

func TestConfigGenerator_WriteToFile_Success(t *testing.T) {
	gen := NewConfigGenerator()
	config := gen.Generate([]CameraStream{
		{ID: "test1", URL: "rtsp://localhost/stream1"},
		{ID: "test2", URL: "rtsp://localhost/stream2", SubURL: "rtsp://localhost/sub"},
	})

	tmpFile := filepath.Join(t.TempDir(), "test_config.yaml")
	err := gen.WriteToFile(config, tmpFile)
	if err != nil {
		t.Errorf("WriteToFile failed: %v", err)
	}

	// Verify file content
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "test1") {
		t.Error("Config should contain test1 stream")
	}
	if !strings.Contains(content, "test2_sub") {
		t.Error("Config should contain test2_sub stream")
	}
}
