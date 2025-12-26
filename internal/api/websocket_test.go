package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestNewHub(t *testing.T) {
	hub := NewHub()
	if hub == nil {
		t.Fatal("NewHub returned nil")
	}
	if hub.clients == nil {
		t.Error("clients map should be initialized")
	}
	if hub.broadcast == nil {
		t.Error("broadcast channel should be initialized")
	}
	if hub.register == nil {
		t.Error("register channel should be initialized")
	}
	if hub.unregister == nil {
		t.Error("unregister channel should be initialized")
	}
}

func TestHub_ClientCount(t *testing.T) {
	hub := NewHub()
	if hub.ClientCount() != 0 {
		t.Errorf("Expected 0 clients, got %d", hub.ClientCount())
	}
}

func TestNewHubBroadcaster(t *testing.T) {
	hub := NewHub()
	broadcaster := NewHubBroadcaster(hub)
	if broadcaster == nil {
		t.Fatal("NewHubBroadcaster returned nil")
	}
	if broadcaster.hub != hub {
		t.Error("broadcaster hub should match input")
	}
}

func TestMessageType_Constants(t *testing.T) {
	tests := []struct {
		msgType  MessageType
		expected string
	}{
		{MessageTypeEvent, "event"},
		{MessageTypeCameraState, "camera_state"},
		{MessageTypeDetection, "detection"},
		{MessageTypeStats, "stats"},
		{MessageTypePing, "ping"},
		{MessageTypePong, "pong"},
		{MessageTypeSubscribe, "subscribe"},
		{MessageTypeUnsubscribe, "unsubscribe"},
		{MessageTypeDoorbell, "doorbell"},
		{MessageTypeAudioState, "audio_state"},
	}

	for _, tt := range tests {
		if string(tt.msgType) != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, string(tt.msgType))
		}
	}
}

func TestEventMessage(t *testing.T) {
	msg := EventMessage("evt1", "cam1", "motion", "person", 0.95)
	if msg.Type != MessageTypeEvent {
		t.Errorf("Expected type %s, got %s", MessageTypeEvent, msg.Type)
	}

	data, ok := msg.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Data should be a map")
	}
	if data["event_id"] != "evt1" {
		t.Errorf("Expected event_id 'evt1', got %v", data["event_id"])
	}
	if data["camera_id"] != "cam1" {
		t.Errorf("Expected camera_id 'cam1', got %v", data["camera_id"])
	}
	if data["event_type"] != "motion" {
		t.Errorf("Expected event_type 'motion', got %v", data["event_type"])
	}
	if data["label"] != "person" {
		t.Errorf("Expected label 'person', got %v", data["label"])
	}
	if data["confidence"] != 0.95 {
		t.Errorf("Expected confidence 0.95, got %v", data["confidence"])
	}
}

func TestCameraStateMessage(t *testing.T) {
	msg := CameraStateMessage("cam1", "recording", 30.0, 5000)
	if msg.Type != MessageTypeCameraState {
		t.Errorf("Expected type %s, got %s", MessageTypeCameraState, msg.Type)
	}

	data, ok := msg.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Data should be a map")
	}
	if data["camera_id"] != "cam1" {
		t.Errorf("Expected camera_id 'cam1', got %v", data["camera_id"])
	}
	if data["status"] != "recording" {
		t.Errorf("Expected status 'recording', got %v", data["status"])
	}
	if data["fps"] != 30.0 {
		t.Errorf("Expected fps 30.0, got %v", data["fps"])
	}
	if data["bitrate"] != 5000 {
		t.Errorf("Expected bitrate 5000, got %v", data["bitrate"])
	}
}

func TestDetectionMessage(t *testing.T) {
	detections := []map[string]interface{}{
		{"class": "person", "confidence": 0.9},
		{"class": "car", "confidence": 0.85},
	}
	msg := DetectionMessage("cam1", detections)
	if msg.Type != MessageTypeDetection {
		t.Errorf("Expected type %s, got %s", MessageTypeDetection, msg.Type)
	}

	data, ok := msg.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Data should be a map")
	}
	if data["camera_id"] != "cam1" {
		t.Errorf("Expected camera_id 'cam1', got %v", data["camera_id"])
	}
	dets, ok := data["detections"].([]map[string]interface{})
	if !ok {
		t.Fatal("detections should be a slice of maps")
	}
	if len(dets) != 2 {
		t.Errorf("Expected 2 detections, got %d", len(dets))
	}
}

func TestDoorbellRingMessage(t *testing.T) {
	msg := DoorbellRingMessage("cam1", "Front Door", "evt1", "/snapshot/1.jpg")
	if msg.Type != MessageTypeDoorbell {
		t.Errorf("Expected type %s, got %s", MessageTypeDoorbell, msg.Type)
	}

	data, ok := msg.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Data should be a map")
	}
	if data["action"] != "ring" {
		t.Errorf("Expected action 'ring', got %v", data["action"])
	}
	if data["camera_id"] != "cam1" {
		t.Errorf("Expected camera_id 'cam1', got %v", data["camera_id"])
	}
	if data["camera_name"] != "Front Door" {
		t.Errorf("Expected camera_name 'Front Door', got %v", data["camera_name"])
	}
}

func TestDoorbellAnsweredMessage(t *testing.T) {
	msg := DoorbellAnsweredMessage("cam1", "sess1", "user1")
	if msg.Type != MessageTypeDoorbell {
		t.Errorf("Expected type %s, got %s", MessageTypeDoorbell, msg.Type)
	}

	data, ok := msg.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Data should be a map")
	}
	if data["action"] != "answered" {
		t.Errorf("Expected action 'answered', got %v", data["action"])
	}
	if data["session_id"] != "sess1" {
		t.Errorf("Expected session_id 'sess1', got %v", data["session_id"])
	}
}

func TestAudioSessionMessage(t *testing.T) {
	msg := AudioSessionMessage("cam1", "sess1", "started", true)
	if msg.Type != MessageTypeAudioState {
		t.Errorf("Expected type %s, got %s", MessageTypeAudioState, msg.Type)
	}

	data, ok := msg.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Data should be a map")
	}
	if data["camera_id"] != "cam1" {
		t.Errorf("Expected camera_id 'cam1', got %v", data["camera_id"])
	}
	if data["session_id"] != "sess1" {
		t.Errorf("Expected session_id 'sess1', got %v", data["session_id"])
	}
	if data["action"] != "started" {
		t.Errorf("Expected action 'started', got %v", data["action"])
	}
	if data["active"] != true {
		t.Errorf("Expected active true, got %v", data["active"])
	}
}

func TestHub_Run_RegisterUnregister(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	// Create a mock client
	client := &Client{
		hub:           hub,
		send:          make(chan []byte, 256),
		subscriptions: map[string]bool{"*": true},
	}

	// Register client
	hub.register <- client
	time.Sleep(10 * time.Millisecond)
	if hub.ClientCount() != 1 {
		t.Errorf("Expected 1 client, got %d", hub.ClientCount())
	}

	// Unregister client
	hub.unregister <- client
	time.Sleep(10 * time.Millisecond)
	if hub.ClientCount() != 0 {
		t.Errorf("Expected 0 clients, got %d", hub.ClientCount())
	}
}

func TestHub_Broadcast(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	client := &Client{
		hub:           hub,
		send:          make(chan []byte, 256),
		subscriptions: map[string]bool{"*": true},
	}

	hub.register <- client
	time.Sleep(10 * time.Millisecond)

	msg := Message{Type: MessageTypeStats, Data: "test"}
	hub.Broadcast(msg)
	time.Sleep(10 * time.Millisecond)

	select {
	case data := <-client.send:
		var received Message
		if err := json.Unmarshal(data, &received); err != nil {
			t.Fatalf("Failed to unmarshal message: %v", err)
		}
		if received.Type != MessageTypeStats {
			t.Errorf("Expected type %s, got %s", MessageTypeStats, received.Type)
		}
	default:
		t.Error("Expected message on client.send channel")
	}
}

func TestHub_BroadcastToCamera(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	// Client subscribed to specific camera
	client1 := &Client{
		hub:           hub,
		send:          make(chan []byte, 256),
		subscriptions: map[string]bool{"cam1": true},
	}
	// Client subscribed to all cameras
	client2 := &Client{
		hub:           hub,
		send:          make(chan []byte, 256),
		subscriptions: map[string]bool{"*": true},
	}
	// Client subscribed to different camera
	client3 := &Client{
		hub:           hub,
		send:          make(chan []byte, 256),
		subscriptions: map[string]bool{"cam2": true},
	}

	hub.register <- client1
	hub.register <- client2
	hub.register <- client3
	time.Sleep(10 * time.Millisecond)

	msg := Message{Type: MessageTypeEvent, Data: "test for cam1"}
	hub.BroadcastToCamera("cam1", msg)
	time.Sleep(10 * time.Millisecond)

	// client1 and client2 should receive
	select {
	case <-client1.send:
	default:
		t.Error("client1 should receive message")
	}
	select {
	case <-client2.send:
	default:
		t.Error("client2 should receive message")
	}

	// client3 should not receive
	select {
	case <-client3.send:
		t.Error("client3 should not receive message")
	default:
		// Expected
	}
}

func TestHub_BroadcastRaw(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	client := &Client{
		hub:           hub,
		send:          make(chan []byte, 256),
		subscriptions: map[string]bool{"*": true},
	}

	hub.register <- client
	time.Sleep(10 * time.Millisecond)

	rawData := map[string]string{"key": "value"}
	hub.BroadcastRaw(rawData)
	time.Sleep(10 * time.Millisecond)

	select {
	case data := <-client.send:
		var received map[string]string
		if err := json.Unmarshal(data, &received); err != nil {
			t.Fatalf("Failed to unmarshal message: %v", err)
		}
		if received["key"] != "value" {
			t.Errorf("Expected key 'value', got %v", received["key"])
		}
	default:
		t.Error("Expected message on client.send channel")
	}
}

func TestHub_BroadcastRawToCamera(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	client := &Client{
		hub:           hub,
		send:          make(chan []byte, 256),
		subscriptions: map[string]bool{"cam1": true},
	}

	hub.register <- client
	time.Sleep(10 * time.Millisecond)

	rawData := map[string]string{"key": "value"}
	hub.BroadcastRawToCamera("cam1", rawData)
	time.Sleep(10 * time.Millisecond)

	select {
	case <-client.send:
		// Success
	default:
		t.Error("Expected message on client.send channel")
	}
}

func TestHubBroadcaster_Broadcast(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	broadcaster := NewHubBroadcaster(hub)

	client := &Client{
		hub:           hub,
		send:          make(chan []byte, 256),
		subscriptions: map[string]bool{"*": true},
	}

	hub.register <- client
	time.Sleep(10 * time.Millisecond)

	broadcaster.Broadcast(map[string]string{"test": "data"})
	time.Sleep(10 * time.Millisecond)

	select {
	case <-client.send:
		// Success
	default:
		t.Error("Expected message on client.send channel")
	}
}

func TestHubBroadcaster_BroadcastToCamera(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	broadcaster := NewHubBroadcaster(hub)

	client := &Client{
		hub:           hub,
		send:          make(chan []byte, 256),
		subscriptions: map[string]bool{"cam1": true},
	}

	hub.register <- client
	time.Sleep(10 * time.Millisecond)

	broadcaster.BroadcastToCamera("cam1", map[string]string{"test": "data"})
	time.Sleep(10 * time.Millisecond)

	select {
	case <-client.send:
		// Success
	default:
		t.Error("Expected message on client.send channel")
	}
}

func TestHub_HandleWebSocket(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	server := httptest.NewServer(http.HandlerFunc(hub.HandleWebSocket))
	defer server.Close()

	// Convert http URL to ws URL
	url := "ws" + strings.TrimPrefix(server.URL, "http")

	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer ws.Close()

	// Give time for registration
	time.Sleep(50 * time.Millisecond)

	if hub.ClientCount() != 1 {
		t.Errorf("Expected 1 client, got %d", hub.ClientCount())
	}

	// Send a ping message
	pingMsg := Message{Type: MessageTypePing}
	if err := ws.WriteJSON(pingMsg); err != nil {
		t.Fatalf("Failed to send ping: %v", err)
	}

	// Read pong response
	ws.SetReadDeadline(time.Now().Add(time.Second))
	var response Message
	if err := ws.ReadJSON(&response); err != nil {
		t.Fatalf("Failed to read pong: %v", err)
	}

	if response.Type != MessageTypePong {
		t.Errorf("Expected pong message, got %s", response.Type)
	}
}

func TestClient_HandleMessage_Subscribe(t *testing.T) {
	hub := NewHub()
	client := &Client{
		hub:           hub,
		send:          make(chan []byte, 256),
		subscriptions: make(map[string]bool),
	}

	// Test subscribe message
	msg := Message{
		Type: MessageTypeSubscribe,
		Data: []interface{}{"cam1", "cam2"},
	}
	data, _ := json.Marshal(msg)
	client.handleMessage(data)

	if !client.subscriptions["cam1"] {
		t.Error("Expected subscription to cam1")
	}
	if !client.subscriptions["cam2"] {
		t.Error("Expected subscription to cam2")
	}
}

func TestClient_HandleMessage_Unsubscribe(t *testing.T) {
	hub := NewHub()
	client := &Client{
		hub:           hub,
		send:          make(chan []byte, 256),
		subscriptions: map[string]bool{"cam1": true, "cam2": true},
	}

	// Test unsubscribe message
	msg := Message{
		Type: MessageTypeUnsubscribe,
		Data: []interface{}{"cam1"},
	}
	data, _ := json.Marshal(msg)
	client.handleMessage(data)

	if client.subscriptions["cam1"] {
		t.Error("Expected cam1 to be unsubscribed")
	}
	if !client.subscriptions["cam2"] {
		t.Error("Expected cam2 to still be subscribed")
	}
}

func TestClient_HandleMessage_InvalidJSON(t *testing.T) {
	hub := NewHub()
	client := &Client{
		hub:           hub,
		send:          make(chan []byte, 256),
		subscriptions: make(map[string]bool),
	}

	// Should not panic on invalid JSON
	client.handleMessage([]byte("invalid json"))
}

func TestUpgrader_CheckOrigin(t *testing.T) {
	// Test with empty origin
	req := httptest.NewRequest("GET", "/ws", nil)
	if !upgrader.CheckOrigin(req) {
		t.Error("Empty origin should be allowed")
	}

	// Test with origin
	req.Header.Set("Origin", "http://localhost:3000")
	if !upgrader.CheckOrigin(req) {
		t.Error("Origin should be allowed")
	}
}
