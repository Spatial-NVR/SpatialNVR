package audio

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestNewTwoWayAudio(t *testing.T) {
	twa := NewTwoWayAudio()
	if twa == nil {
		t.Fatal("NewTwoWayAudio returned nil")
	}
	if twa.sessions == nil {
		t.Error("sessions map should be initialized")
	}
}

func TestTwoWayAudio_StartSession(t *testing.T) {
	twa := NewTwoWayAudio()
	ctx := context.Background()

	session, err := twa.StartSession(ctx, "cam_1", "user_1")
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	if session.CameraID != "cam_1" {
		t.Errorf("Expected CameraID cam_1, got %s", session.CameraID)
	}
	if session.UserID != "user_1" {
		t.Errorf("Expected UserID user_1, got %s", session.UserID)
	}
	if !session.Active {
		t.Error("Expected session to be active")
	}
	if session.ID == "" {
		t.Error("Expected session ID to be set")
	}
	if session.StartedAt.IsZero() {
		t.Error("Expected StartedAt to be set")
	}
}

func TestTwoWayAudio_StartSession_Duplicate(t *testing.T) {
	twa := NewTwoWayAudio()
	ctx := context.Background()

	// Start first session
	_, err := twa.StartSession(ctx, "cam_1", "user_1")
	if err != nil {
		t.Fatalf("First StartSession failed: %v", err)
	}

	// Try to start duplicate session for same camera
	_, err = twa.StartSession(ctx, "cam_1", "user_2")
	if err == nil {
		t.Error("Expected error for duplicate session")
	}
}

func TestTwoWayAudio_StopSession(t *testing.T) {
	twa := NewTwoWayAudio()
	ctx := context.Background()

	session, err := twa.StartSession(ctx, "cam_1", "user_1")
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	err = twa.StopSession(session.ID)
	if err != nil {
		t.Fatalf("StopSession failed: %v", err)
	}

	// Session should be removed
	sessions := twa.GetActiveSessions()
	for _, s := range sessions {
		if s.ID == session.ID {
			t.Error("Session should have been removed")
		}
	}
}

func TestTwoWayAudio_StopSession_NotFound(t *testing.T) {
	twa := NewTwoWayAudio()

	err := twa.StopSession("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent session")
	}
}

func TestTwoWayAudio_GetActiveSessions(t *testing.T) {
	twa := NewTwoWayAudio()
	ctx := context.Background()

	// Start multiple sessions for different cameras
	_, err := twa.StartSession(ctx, "cam_1", "user_1")
	if err != nil {
		t.Fatalf("StartSession 1 failed: %v", err)
	}
	_, err = twa.StartSession(ctx, "cam_2", "user_1")
	if err != nil {
		t.Fatalf("StartSession 2 failed: %v", err)
	}

	sessions := twa.GetActiveSessions()
	if len(sessions) != 2 {
		t.Errorf("Expected 2 sessions, got %d", len(sessions))
	}
}

func TestTwoWayAudio_SetStreamURLProvider(t *testing.T) {
	twa := NewTwoWayAudio()

	// Mock provider
	mockProvider := &mockStreamURLProvider{}
	twa.SetStreamURLProvider(mockProvider)

	if twa.streamURLs != mockProvider {
		t.Error("streamURLs should be set")
	}
}

type mockStreamURLProvider struct{}

func (m *mockStreamURLProvider) GetWebRTCURL(streamName string) string {
	return "webrtc://test/" + streamName
}

func (m *mockStreamURLProvider) GetBackchannelURL(streamName string) string {
	return "backchannel://test/" + streamName
}

func TestTwoWayAudio_Routes(t *testing.T) {
	twa := NewTwoWayAudio()
	router := twa.Routes()
	if router == nil {
		t.Error("Routes should return a router")
	}
}

func TestSanitizeStreamName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"camera1", "camera1"},
		{"Camera1", "camera1"},
		{"cam-1", "cam_1"},
		{"cam.1.front", "cam_1_front"},
		{"cam 1", "cam_1"},
		{"CAM_1", "cam_1"},
		{"123", "123"},
		{"a-B_c.D e", "a_b_c_d_e"},
	}

	for _, tt := range tests {
		result := sanitizeStreamName(tt.input)
		if result != tt.expected {
			t.Errorf("sanitizeStreamName(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestNewDoorbellHandler(t *testing.T) {
	twa := NewTwoWayAudio()
	db := NewDoorbellHandler(twa)
	if db == nil {
		t.Fatal("NewDoorbellHandler returned nil")
	}
	if db.audio != twa {
		t.Error("audio should be set")
	}
}

func TestDoorbellHandler_SetBroadcaster(t *testing.T) {
	twa := NewTwoWayAudio()
	db := NewDoorbellHandler(twa)

	mock := &mockBroadcaster{}
	db.SetBroadcaster(mock)

	if db.broadcaster != mock {
		t.Error("broadcaster should be set")
	}
}

type mockBroadcaster struct{}

func (m *mockBroadcaster) Broadcast(msg interface{})                          {}
func (m *mockBroadcaster) BroadcastToCamera(cameraID string, msg interface{}) {}

func TestDoorbellHandler_SetEventCreator(t *testing.T) {
	twa := NewTwoWayAudio()
	db := NewDoorbellHandler(twa)

	mock := &mockEventCreator{}
	db.SetEventCreator(mock)

	if db.eventCreator != mock {
		t.Error("eventCreator should be set")
	}
}

type mockEventCreator struct{}

func (m *mockEventCreator) CreateDoorbellEvent(ctx context.Context, cameraID string, thumbnail string) (interface{}, error) {
	return nil, nil
}

func TestDoorbellHandler_Routes(t *testing.T) {
	twa := NewTwoWayAudio()
	db := NewDoorbellHandler(twa)
	router := db.Routes()
	if router == nil {
		t.Error("Routes should return a router")
	}
}

func TestDoorbellHandler_OnRing(t *testing.T) {
	twa := NewTwoWayAudio()
	db := NewDoorbellHandler(twa)

	called := false
	db.OnRing(func(cameraID string) {
		called = true
		if cameraID != "cam_1" {
			t.Errorf("Expected cam_1, got %s", cameraID)
		}
	})

	db.NotifyRing("cam_1")

	// Give time for goroutine
	time.Sleep(10 * time.Millisecond)

	if !called {
		t.Error("OnRing callback should have been called")
	}
}

func TestDoorbellHandler_NotifyRing_MultipleCallbacks(t *testing.T) {
	twa := NewTwoWayAudio()
	db := NewDoorbellHandler(twa)

	callCount := 0
	for i := 0; i < 3; i++ {
		db.OnRing(func(cameraID string) {
			callCount++
		})
	}

	db.NotifyRing("cam_1")

	// Give time for goroutines
	time.Sleep(50 * time.Millisecond)

	if callCount != 3 {
		t.Errorf("Expected 3 callbacks, got %d", callCount)
	}
}

func TestNewAudioStreamProxy(t *testing.T) {
	proxy := NewAudioStreamProxy()
	if proxy == nil {
		t.Fatal("NewAudioStreamProxy returned nil")
	}
}

func TestAudioCapabilities(t *testing.T) {
	caps := AudioCapabilities{
		HasMicrophone: true,
		HasSpeaker:    true,
		TwoWayAudio:   true,
		AudioCodec:    "opus",
		SampleRate:    48000,
		Channels:      2,
	}

	if !caps.HasMicrophone {
		t.Error("HasMicrophone should be true")
	}
	if !caps.HasSpeaker {
		t.Error("HasSpeaker should be true")
	}
	if !caps.TwoWayAudio {
		t.Error("TwoWayAudio should be true")
	}
	if caps.AudioCodec != "opus" {
		t.Errorf("Expected codec opus, got %s", caps.AudioCodec)
	}
	if caps.SampleRate != 48000 {
		t.Errorf("Expected sample rate 48000, got %d", caps.SampleRate)
	}
	if caps.Channels != 2 {
		t.Errorf("Expected channels 2, got %d", caps.Channels)
	}
}

func TestAudioSession(t *testing.T) {
	session := AudioSession{
		ID:        "session_1",
		CameraID:  "cam_1",
		UserID:    "user_1",
		StartedAt: time.Now(),
		Active:    true,
	}

	if session.ID != "session_1" {
		t.Errorf("Expected ID session_1, got %s", session.ID)
	}
	if session.CameraID != "cam_1" {
		t.Errorf("Expected CameraID cam_1, got %s", session.CameraID)
	}
	if !session.Active {
		t.Error("Expected Active to be true")
	}
}

// HTTP Handler Tests

func TestHandleGetCapabilities(t *testing.T) {
	twa := NewTwoWayAudio()

	req := httptest.NewRequest("GET", "/api/v1/audio/capabilities/cam_1", nil)
	w := httptest.NewRecorder()

	// Add chi URL params
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("cameraId", "cam_1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	twa.handleGetCapabilities(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["success"] != true {
		t.Error("Expected success to be true")
	}

	data := response["data"].(map[string]interface{})
	if data["two_way_audio"] != true {
		t.Error("Expected two_way_audio to be true")
	}
}

func TestHandleListSessions(t *testing.T) {
	twa := NewTwoWayAudio()

	// Start a session first
	twa.StartSession(context.Background(), "cam_1", "user_1")

	req := httptest.NewRequest("GET", "/api/v1/audio/sessions", nil)
	w := httptest.NewRecorder()

	twa.handleListSessions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["success"] != true {
		t.Error("Expected success to be true")
	}

	data := response["data"].([]interface{})
	if len(data) != 1 {
		t.Errorf("Expected 1 session, got %d", len(data))
	}
}

func TestHandleStartSession(t *testing.T) {
	twa := NewTwoWayAudio()

	req := httptest.NewRequest("POST", "/api/v1/audio/cameras/cam_1/start", nil)
	req.Header.Set("X-User-ID", "test_user")
	w := httptest.NewRecorder()

	// Add chi URL params
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("cameraId", "cam_1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	twa.handleStartSession(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["success"] != true {
		t.Error("Expected success to be true")
	}
}

func TestHandleStartSession_WithStreamURLProvider(t *testing.T) {
	twa := NewTwoWayAudio()
	twa.SetStreamURLProvider(&mockStreamURLProvider{})

	req := httptest.NewRequest("POST", "/api/v1/audio/cameras/cam_2/start", nil)
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("cameraId", "cam_2")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	twa.handleStartSession(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	json.NewDecoder(w.Body).Decode(&response)

	data := response["data"].(map[string]interface{})
	if data["webrtc_url"] == nil {
		t.Error("Expected webrtc_url to be set")
	}
	if data["backchannel_url"] == nil {
		t.Error("Expected backchannel_url to be set")
	}
}

func TestHandleStartSession_Duplicate(t *testing.T) {
	twa := NewTwoWayAudio()

	// Start first session
	twa.StartSession(context.Background(), "cam_1", "user_1")

	// Try to start duplicate via handler
	req := httptest.NewRequest("POST", "/api/v1/audio/cameras/cam_1/start", nil)
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("cameraId", "cam_1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	twa.handleStartSession(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestHandleStopSession(t *testing.T) {
	twa := NewTwoWayAudio()

	// Start a session first
	session, _ := twa.StartSession(context.Background(), "cam_1", "user_1")

	req := httptest.NewRequest("POST", "/api/v1/audio/sessions/"+session.ID+"/stop", nil)
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("sessionId", session.ID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	twa.handleStopSession(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestHandleStopSession_NotFound(t *testing.T) {
	twa := NewTwoWayAudio()

	req := httptest.NewRequest("POST", "/api/v1/audio/sessions/nonexistent/stop", nil)
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("sessionId", "nonexistent")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	twa.handleStopSession(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}
