package detection

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func createTestJPEG() []byte {
	// Create a simple test image
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for x := 0; x < 100; x++ {
		for y := 0; y < 100; y++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}

	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80})
	return buf.Bytes()
}

func TestNewGo2RTCFrameGrabber(t *testing.T) {
	fg := NewGo2RTCFrameGrabber("http://localhost:1984")
	if fg == nil {
		t.Fatal("NewGo2RTCFrameGrabber returned nil")
	}
	if fg.baseURL != "http://localhost:1984" {
		t.Errorf("Expected baseURL 'http://localhost:1984', got '%s'", fg.baseURL)
	}
}

func TestGrabFrame(t *testing.T) {
	// Create a test server that serves a JPEG image
	jpegData := createTestJPEG()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check the request path
		if r.URL.Path != "/api/frame.jpeg" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jpegData)
	}))
	defer server.Close()

	fg := NewGo2RTCFrameGrabber(server.URL)

	frame, err := fg.GrabFrame(context.Background(), "test_camera")
	if err != nil {
		t.Fatalf("GrabFrame failed: %v", err)
	}

	if frame == nil {
		t.Fatal("GrabFrame returned nil frame")
	}

	if frame.CameraID != "test_camera" {
		t.Errorf("Expected CameraID 'test_camera', got '%s'", frame.CameraID)
	}

	if frame.Width != 100 || frame.Height != 100 {
		t.Errorf("Expected dimensions 100x100, got %dx%d", frame.Width, frame.Height)
	}

	if frame.Format != "jpeg" {
		t.Errorf("Expected format 'jpeg', got '%s'", frame.Format)
	}

	if frame.Image == nil {
		t.Error("Frame image should not be nil")
	}
}

func TestGrabFrame_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	fg := NewGo2RTCFrameGrabber(server.URL)

	_, err := fg.GrabFrame(context.Background(), "test_camera")
	if err == nil {
		t.Error("Expected error for server error response")
	}
}

func TestGrabFrame_InvalidJPEG(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not a valid jpeg"))
	}))
	defer server.Close()

	fg := NewGo2RTCFrameGrabber(server.URL)

	_, err := fg.GrabFrame(context.Background(), "test_camera")
	if err == nil {
		t.Error("Expected error for invalid JPEG data")
	}
}

func TestGrabFrame_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Delay response
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	fg := NewGo2RTCFrameGrabber(server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := fg.GrabFrame(ctx, "test_camera")
	if err == nil {
		t.Error("Expected error for cancelled context")
	}
}

func TestStartStream(t *testing.T) {
	// Create a test server that serves JPEG images
	jpegData := createTestJPEG()
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jpegData)
	}))
	defer server.Close()

	fg := NewGo2RTCFrameGrabber(server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	frameCh, err := fg.StartStream(ctx, "test_camera", 10)
	if err != nil {
		t.Fatalf("StartStream failed: %v", err)
	}

	// Collect some frames
	var frames []*Frame
	for frame := range frameCh {
		frames = append(frames, frame)
		if len(frames) >= 3 {
			break
		}
	}

	if len(frames) < 1 {
		t.Error("Expected at least 1 frame")
	}
}

func TestStartStream_ZeroFPS(t *testing.T) {
	jpegData := createTestJPEG()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jpegData)
	}))
	defer server.Close()

	fg := NewGo2RTCFrameGrabber(server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	// FPS of 0 should default to 5
	frameCh, err := fg.StartStream(ctx, "test_camera", 0)
	if err != nil {
		t.Fatalf("StartStream failed: %v", err)
	}

	// Wait for at least one frame
	select {
	case frame := <-frameCh:
		if frame == nil {
			t.Error("Received nil frame")
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Timeout waiting for frame")
	}
}

func TestStartStream_NegativeFPS(t *testing.T) {
	jpegData := createTestJPEG()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jpegData)
	}))
	defer server.Close()

	fg := NewGo2RTCFrameGrabber(server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	// Negative FPS should default to 5
	frameCh, err := fg.StartStream(ctx, "test_camera", -5)
	if err != nil {
		t.Fatalf("StartStream failed: %v", err)
	}

	// Wait for at least one frame
	select {
	case frame := <-frameCh:
		if frame == nil {
			t.Error("Received nil frame")
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Timeout waiting for frame")
	}
}

func TestStartStream_AlreadyRunning(t *testing.T) {
	jpegData := createTestJPEG()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jpegData)
	}))
	defer server.Close()

	fg := NewGo2RTCFrameGrabber(server.URL)

	ctx := context.Background()

	// Start first stream
	_, err := fg.StartStream(ctx, "test_camera", 5)
	if err != nil {
		t.Fatalf("First StartStream failed: %v", err)
	}

	// Try to start second stream for same camera - should fail
	_, err = fg.StartStream(ctx, "test_camera", 5)
	if err == nil {
		t.Error("Expected error for already running stream")
	}

	// Clean up
	_ = fg.StopStream("test_camera")
}

func TestStopStream(t *testing.T) {
	jpegData := createTestJPEG()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jpegData)
	}))
	defer server.Close()

	fg := NewGo2RTCFrameGrabber(server.URL)

	// Start stream
	_, err := fg.StartStream(context.Background(), "test_camera", 5)
	if err != nil {
		t.Fatalf("StartStream failed: %v", err)
	}

	// Stop stream
	err = fg.StopStream("test_camera")
	if err != nil {
		t.Errorf("StopStream failed: %v", err)
	}

	// Stop non-existent stream should not error
	err = fg.StopStream("nonexistent")
	if err != nil {
		t.Errorf("StopStream for non-existent should not error: %v", err)
	}
}

func TestClose(t *testing.T) {
	jpegData := createTestJPEG()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jpegData)
	}))
	defer server.Close()

	fg := NewGo2RTCFrameGrabber(server.URL)

	// Start multiple streams
	_, _ = fg.StartStream(context.Background(), "camera1", 5)
	_, _ = fg.StartStream(context.Background(), "camera2", 5)

	// Close should stop all streams
	err := fg.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Streams map should be empty
	fg.mu.RLock()
	count := len(fg.streams)
	fg.mu.RUnlock()

	if count != 0 {
		t.Errorf("Expected 0 streams after Close, got %d", count)
	}
}

func TestNewRTSPFrameGrabber(t *testing.T) {
	fg := NewRTSPFrameGrabber()
	if fg == nil {
		t.Fatal("NewRTSPFrameGrabber returned nil")
	}
}

func TestRTSPFrameGrabber_NotImplemented(t *testing.T) {
	fg := NewRTSPFrameGrabber()

	// GrabFrame should return not implemented error
	_, err := fg.GrabFrame(context.Background(), "test")
	if err == nil {
		t.Error("Expected not implemented error")
	}

	// StartStream should return not implemented error
	_, err = fg.StartStream(context.Background(), "test", 5)
	if err == nil {
		t.Error("Expected not implemented error")
	}
}

func TestRTSPFrameGrabber_StopAndClose(t *testing.T) {
	fg := NewRTSPFrameGrabber()

	// StopStream should not error for non-existent stream
	err := fg.StopStream("nonexistent")
	if err != nil {
		t.Errorf("StopStream should not error: %v", err)
	}

	// Close should not error
	err = fg.Close()
	if err != nil {
		t.Errorf("Close should not error: %v", err)
	}
}

func TestImageToBytes(t *testing.T) {
	// Create a test image
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for x := 0; x < 100; x++ {
		for y := 0; y < 100; y++ {
			img.Set(x, y, color.RGBA{R: 255, G: 128, B: 0, A: 255})
		}
	}

	data, err := ImageToBytes(img, 80)
	if err != nil {
		t.Fatalf("ImageToBytes failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("ImageToBytes returned empty data")
	}

	// Verify it's valid JPEG
	_, err = jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Errorf("Result is not valid JPEG: %v", err)
	}
}

func TestImageToRGB(t *testing.T) {
	// Create a 2x2 test image with known colors
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})   // Red
	img.Set(1, 0, color.RGBA{R: 0, G: 255, B: 0, A: 255})   // Green
	img.Set(0, 1, color.RGBA{R: 0, G: 0, B: 255, A: 255})   // Blue
	img.Set(1, 1, color.RGBA{R: 255, G: 255, B: 255, A: 255}) // White

	rgb := ImageToRGB(img)

	// Should be 2*2*3 = 12 bytes
	if len(rgb) != 12 {
		t.Errorf("Expected 12 bytes, got %d", len(rgb))
	}

	// Check first pixel (red)
	if rgb[0] != 255 || rgb[1] != 0 || rgb[2] != 0 {
		t.Errorf("First pixel should be red, got R=%d G=%d B=%d", rgb[0], rgb[1], rgb[2])
	}

	// Check second pixel (green)
	if rgb[3] != 0 || rgb[4] != 255 || rgb[5] != 0 {
		t.Errorf("Second pixel should be green, got R=%d G=%d B=%d", rgb[3], rgb[4], rgb[5])
	}
}

func TestCameraIDNormalization(t *testing.T) {
	// Test that camera IDs with spaces and uppercase are handled
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The src parameter should be lowercase with underscores
		src := r.URL.Query().Get("src")
		if src != "front_door" {
			t.Errorf("Expected src='front_door', got '%s'", src)
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(createTestJPEG())
	}))
	defer server.Close()

	fg := NewGo2RTCFrameGrabber(server.URL)

	// Camera ID with space and mixed case
	_, err := fg.GrabFrame(context.Background(), "Front Door")
	if err != nil {
		t.Logf("GrabFrame returned error (expected if normalization works): %v", err)
	}
}
