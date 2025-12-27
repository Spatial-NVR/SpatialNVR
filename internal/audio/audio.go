// Package audio provides two-way audio functionality
package audio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

// Broadcaster interface for sending WebSocket messages
type Broadcaster interface {
	Broadcast(msg interface{})
	BroadcastToCamera(cameraID string, msg interface{})
}

// EventCreator interface for creating doorbell events
type EventCreator interface {
	CreateDoorbellEvent(ctx context.Context, cameraID string, thumbnail string) (interface{}, error)
}

// StreamURLProvider interface for getting WebRTC stream URLs from go2rtc
type StreamURLProvider interface {
	GetWebRTCURL(streamName string) string
	GetBackchannelURL(streamName string) string
}

// TwoWayAudio handles two-way audio communication with cameras
type TwoWayAudio struct {
	sessions   map[string]*AudioSession
	mu         sync.RWMutex
	logger     *slog.Logger
	upgrader   websocket.Upgrader
	streamURLs StreamURLProvider
}

// SetStreamURLProvider sets the stream URL provider (usually go2rtc manager)
func (t *TwoWayAudio) SetStreamURLProvider(p StreamURLProvider) {
	t.streamURLs = p
}

// AudioSession represents an active two-way audio session
type AudioSession struct {
	ID        string    `json:"id"`
	CameraID  string    `json:"camera_id"`
	UserID    string    `json:"user_id"`
	StartedAt time.Time `json:"started_at"`
	Active    bool      `json:"active"`
	conn      *websocket.Conn
	cancel    context.CancelFunc
}

// AudioCapabilities represents camera audio capabilities
type AudioCapabilities struct {
	HasMicrophone bool   `json:"has_microphone"`
	HasSpeaker    bool   `json:"has_speaker"`
	TwoWayAudio   bool   `json:"two_way_audio"`
	AudioCodec    string `json:"audio_codec"` // pcm, g711, aac, opus
	SampleRate    int    `json:"sample_rate"`
	Channels      int    `json:"channels"`
}

// NewTwoWayAudio creates a new two-way audio handler
func NewTwoWayAudio() *TwoWayAudio {
	return &TwoWayAudio{
		sessions: make(map[string]*AudioSession),
		logger:   slog.Default().With("component", "two_way_audio"),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// Routes returns the audio routes
func (t *TwoWayAudio) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/capabilities/{cameraId}", t.handleGetCapabilities)
	r.Get("/sessions", t.handleListSessions)
	r.Post("/sessions/{cameraId}/start", t.handleStartSession)
	r.Post("/sessions/{sessionId}/stop", t.handleStopSession)
	r.Get("/sessions/{sessionId}/stream", t.handleWebSocket)

	return r
}

// GetActiveSessions returns all active audio sessions
func (t *TwoWayAudio) GetActiveSessions() []*AudioSession {
	t.mu.RLock()
	defer t.mu.RUnlock()

	sessions := make([]*AudioSession, 0)
	for _, s := range t.sessions {
		if s.Active {
			sessions = append(sessions, s)
		}
	}
	return sessions
}

// StartSession starts a two-way audio session with a camera
func (t *TwoWayAudio) StartSession(ctx context.Context, cameraID, userID string) (*AudioSession, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Check if session already exists for this camera
	for _, s := range t.sessions {
		if s.CameraID == cameraID && s.Active {
			return nil, fmt.Errorf("audio session already active for camera %s", cameraID)
		}
	}

	sessionID := fmt.Sprintf("audio_%s_%d", cameraID, time.Now().UnixNano())
	ctx, cancel := context.WithCancel(ctx)

	session := &AudioSession{
		ID:        sessionID,
		CameraID:  cameraID,
		UserID:    userID,
		StartedAt: time.Now(),
		Active:    true,
		cancel:    cancel,
	}

	t.sessions[sessionID] = session

	t.logger.Info("Audio session started",
		"session_id", sessionID,
		"camera_id", cameraID,
		"user_id", userID,
	)

	// Auto-stop after 5 minutes
	go func() {
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Minute):
			_ = t.StopSession(sessionID)
		}
	}()

	return session, nil
}

// StopSession stops an audio session
func (t *TwoWayAudio) StopSession(sessionID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	session, exists := t.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.Active = false
	if session.cancel != nil {
		session.cancel()
	}
	if session.conn != nil {
		_ = session.conn.Close()
	}

	delete(t.sessions, sessionID)

	t.logger.Info("Audio session stopped", "session_id", sessionID)
	return nil
}

func (t *TwoWayAudio) handleGetCapabilities(w http.ResponseWriter, r *http.Request) {
	cameraID := chi.URLParam(r, "cameraId")

	// In a real implementation, query the camera for its capabilities
	// For now, return default capabilities
	caps := AudioCapabilities{
		HasMicrophone: true,
		HasSpeaker:    true,
		TwoWayAudio:   true,
		AudioCodec:    "pcm",
		SampleRate:    16000,
		Channels:      1,
	}

	_ = cameraID // Would use this to query actual camera

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    caps,
	})
}

func (t *TwoWayAudio) handleListSessions(w http.ResponseWriter, r *http.Request) {
	sessions := t.GetActiveSessions()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    sessions,
	})
}

func (t *TwoWayAudio) handleStartSession(w http.ResponseWriter, r *http.Request) {
	cameraID := chi.URLParam(r, "cameraId")
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "anonymous"
	}

	session, err := t.StartSession(r.Context(), cameraID, userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Build response with session info and stream URLs
	response := map[string]interface{}{
		"session":       session,
		"websocket_url": fmt.Sprintf("/api/v1/audio/sessions/%s/stream", session.ID),
	}

	// Add go2rtc WebRTC URLs if available
	if t.streamURLs != nil {
		// Use sanitized camera ID as stream name (matches go2rtc stream names)
		streamName := sanitizeStreamName(cameraID)
		response["webrtc_url"] = t.streamURLs.GetWebRTCURL(streamName)
		response["backchannel_url"] = t.streamURLs.GetBackchannelURL(streamName)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    response,
	})
}

// sanitizeStreamName ensures the stream name is valid for go2rtc
func sanitizeStreamName(name string) string {
	// Convert to lowercase and replace special characters with underscores
	result := ""
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
			result += string(c)
		} else if c >= 'A' && c <= 'Z' {
			result += string(c + 32) // lowercase
		} else {
			result += "_"
		}
	}
	return result
}

func (t *TwoWayAudio) handleStopSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionId")

	if err := t.StopSession(sessionID); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}

func (t *TwoWayAudio) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionId")

	t.mu.Lock()
	session, exists := t.sessions[sessionID]
	if !exists || !session.Active {
		t.mu.Unlock()
		http.Error(w, "Session not found or inactive", http.StatusNotFound)
		return
	}
	t.mu.Unlock()

	conn, err := t.upgrader.Upgrade(w, r, nil)
	if err != nil {
		t.logger.Error("WebSocket upgrade failed", "error", err)
		return
	}

	t.mu.Lock()
	session.conn = conn
	t.mu.Unlock()

	defer func() {
		_ = conn.Close()
		_ = t.StopSession(sessionID)
	}()

	t.logger.Info("WebSocket connected for audio session", "session_id", sessionID)

	// Handle bidirectional audio
	// In a real implementation:
	// - Read audio from WebSocket and forward to camera via RTSP/ONVIF backchannel
	// - Read audio from camera and forward to WebSocket
	
	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				t.logger.Error("WebSocket read error", "error", err)
			}
			break
		}

		if messageType == websocket.BinaryMessage {
			// Audio data from client - forward to camera
			// This would involve:
			// 1. Decoding the audio (if encoded)
			// 2. Re-encoding to camera's format
			// 3. Sending via ONVIF backchannel or RTSP
			t.logger.Debug("Received audio data", "bytes", len(data))
		}
	}
}

// DoorbellHandler handles doorbell-specific functionality
type DoorbellHandler struct {
	audio        *TwoWayAudio
	broadcaster  Broadcaster
	eventCreator EventCreator
	logger       *slog.Logger
	callbacks    []func(cameraID string) // Ring callbacks
	mu           sync.RWMutex
}

// NewDoorbellHandler creates a new doorbell handler
func NewDoorbellHandler(audio *TwoWayAudio) *DoorbellHandler {
	return &DoorbellHandler{
		audio:     audio,
		logger:    slog.Default().With("component", "doorbell"),
		callbacks: make([]func(cameraID string), 0),
	}
}

// SetBroadcaster sets the WebSocket broadcaster for ring notifications
func (d *DoorbellHandler) SetBroadcaster(b Broadcaster) {
	d.broadcaster = b
}

// SetEventCreator sets the event creator for storing doorbell events
func (d *DoorbellHandler) SetEventCreator(e EventCreator) {
	d.eventCreator = e
}

// Routes returns doorbell routes
func (d *DoorbellHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Post("/{cameraId}/ring", d.handleRing)
	r.Post("/{cameraId}/answer", d.handleAnswer)
	r.Get("/{cameraId}/snapshot", d.handleSnapshot)

	return r
}

// OnRing registers a callback for doorbell rings
func (d *DoorbellHandler) OnRing(fn func(cameraID string)) {
	d.mu.Lock()
	d.callbacks = append(d.callbacks, fn)
	d.mu.Unlock()
}

// NotifyRing notifies all callbacks of a doorbell ring
func (d *DoorbellHandler) NotifyRing(cameraID string) {
	d.mu.RLock()
	callbacks := make([]func(string), len(d.callbacks))
	copy(callbacks, d.callbacks)
	d.mu.RUnlock()

	for _, fn := range callbacks {
		go fn(cameraID)
	}
}

func (d *DoorbellHandler) handleRing(w http.ResponseWriter, r *http.Request) {
	cameraID := chi.URLParam(r, "cameraId")

	d.logger.Info("Doorbell ring received", "camera_id", cameraID)

	// Create doorbell event in database
	if d.eventCreator != nil {
		if _, err := d.eventCreator.CreateDoorbellEvent(r.Context(), cameraID, ""); err != nil {
			d.logger.Warn("Failed to create doorbell event", "error", err)
		}
	}

	// Broadcast ring notification via WebSocket
	if d.broadcaster != nil {
		snapshotURL := fmt.Sprintf("/api/v1/cameras/%s/snapshot", cameraID)
		d.broadcaster.BroadcastToCamera(cameraID, map[string]interface{}{
			"type":      "doorbell",
			"timestamp": time.Now(),
			"data": map[string]interface{}{
				"camera_id":    cameraID,
				"action":       "ring",
				"snapshot_url": snapshotURL,
			},
		})
	}

	// Notify legacy callbacks
	d.NotifyRing(cameraID)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Ring notification sent",
	})
}

func (d *DoorbellHandler) handleAnswer(w http.ResponseWriter, r *http.Request) {
	cameraID := chi.URLParam(r, "cameraId")
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "anonymous"
	}

	// Start two-way audio session
	session, err := d.audio.StartSession(r.Context(), cameraID, userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	d.logger.Info("Doorbell answered", "camera_id", cameraID, "user_id", userID)

	// Broadcast answered notification via WebSocket
	if d.broadcaster != nil {
		d.broadcaster.BroadcastToCamera(cameraID, map[string]interface{}{
			"type":      "doorbell",
			"timestamp": time.Now(),
			"data": map[string]interface{}{
				"camera_id":  cameraID,
				"session_id": session.ID,
				"user_id":    userID,
				"action":     "answered",
			},
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"session_id":    session.ID,
			"websocket_url": fmt.Sprintf("/api/v1/audio/sessions/%s/stream", session.ID),
		},
	})
}

func (d *DoorbellHandler) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	cameraID := chi.URLParam(r, "cameraId")

	// In a real implementation, get snapshot from camera
	// For now, redirect to the camera snapshot endpoint
	http.Redirect(w, r, fmt.Sprintf("/api/v1/cameras/%s/snapshot", cameraID), http.StatusTemporaryRedirect)
}

// AudioStreamProxy proxies audio from camera to client
type AudioStreamProxy struct {
	logger *slog.Logger
}

// NewAudioStreamProxy creates a new audio stream proxy
func NewAudioStreamProxy() *AudioStreamProxy {
	return &AudioStreamProxy{
		logger: slog.Default().With("component", "audio_proxy"),
	}
}

// ProxyAudio proxies audio from a camera to an HTTP response
func (p *AudioStreamProxy) ProxyAudio(w http.ResponseWriter, r *http.Request, audioURL string) error {
	ctx := r.Context()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, audioURL, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	// Copy headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	return err
}
