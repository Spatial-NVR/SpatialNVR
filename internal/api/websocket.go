// Package api provides HTTP API handlers and WebSocket support
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow connections from localhost in development
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		// In production, validate against allowed origins
		return true
	},
}

// MessageType represents the type of WebSocket message
type MessageType string

const (
	MessageTypeEvent       MessageType = "event"
	MessageTypeCameraState MessageType = "camera_state"
	MessageTypeDetection   MessageType = "detection"
	MessageTypeStats       MessageType = "stats"
	MessageTypePing        MessageType = "ping"
	MessageTypePong        MessageType = "pong"
	MessageTypeSubscribe   MessageType = "subscribe"
	MessageTypeUnsubscribe MessageType = "unsubscribe"
	MessageTypeDoorbell    MessageType = "doorbell"
	MessageTypeAudioState  MessageType = "audio_state"
)

// Message represents a WebSocket message
type Message struct {
	Type      MessageType `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data,omitempty"`
}

// Client represents a WebSocket client
type Client struct {
	hub           *Hub
	conn          *websocket.Conn
	send          chan []byte
	subscriptions map[string]bool // camera IDs to subscribe to, "*" for all
}

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
	logger     *slog.Logger
}

// NewHub creates a new WebSocket hub
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		logger:     slog.Default().With("component", "websocket-hub"),
	}
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			h.logger.Debug("Client connected", "total_clients", len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			h.logger.Debug("Client disconnected", "total_clients", len(h.clients))

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// Client buffer full, skip
					h.logger.Warn("Client buffer full, dropping message")
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends a message to all connected clients
func (h *Hub) Broadcast(msg Message) {
	msg.Timestamp = time.Now()
	data, err := json.Marshal(msg)
	if err != nil {
		h.logger.Error("Failed to marshal broadcast message", "error", err)
		return
	}

	select {
	case h.broadcast <- data:
	default:
		h.logger.Warn("Broadcast channel full, dropping message")
	}
}

// BroadcastToCamera sends a message to clients subscribed to a specific camera
func (h *Hub) BroadcastToCamera(cameraID string, msg Message) {
	msg.Timestamp = time.Now()
	data, err := json.Marshal(msg)
	if err != nil {
		h.logger.Error("Failed to marshal camera message", "error", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		// Send if subscribed to this camera or to all cameras
		if client.subscriptions["*"] || client.subscriptions[cameraID] {
			select {
			case client.send <- data:
			default:
				// Client buffer full, skip
			}
		}
	}
}

// BroadcastRaw sends a raw message to all clients (for generic interface{} messages)
func (h *Hub) BroadcastRaw(msg interface{}) {
	data, err := json.Marshal(msg)
	if err != nil {
		h.logger.Error("Failed to marshal raw message", "error", err)
		return
	}

	select {
	case h.broadcast <- data:
	default:
		h.logger.Warn("Broadcast channel full, dropping message")
	}
}

// BroadcastRawToCamera sends a raw message to clients subscribed to a specific camera
func (h *Hub) BroadcastRawToCamera(cameraID string, msg interface{}) {
	data, err := json.Marshal(msg)
	if err != nil {
		h.logger.Error("Failed to marshal raw camera message", "error", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		if client.subscriptions["*"] || client.subscriptions[cameraID] {
			select {
			case client.send <- data:
			default:
			}
		}
	}
}

// ClientCount returns the number of connected clients
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// HubBroadcaster wraps Hub to satisfy the audio.Broadcaster interface
type HubBroadcaster struct {
	hub *Hub
}

// NewHubBroadcaster creates a new HubBroadcaster
func NewHubBroadcaster(hub *Hub) *HubBroadcaster {
	return &HubBroadcaster{hub: hub}
}

// Broadcast sends a message to all clients
func (b *HubBroadcaster) Broadcast(msg interface{}) {
	b.hub.BroadcastRaw(msg)
}

// BroadcastToCamera sends a message to clients subscribed to a specific camera
func (b *HubBroadcaster) BroadcastToCamera(cameraID string, msg interface{}) {
	b.hub.BroadcastRawToCamera(cameraID, msg)
}

// HandleWebSocket handles WebSocket connections
func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("Failed to upgrade connection", "error", err)
		return
	}

	client := &Client{
		hub:           h,
		conn:          conn,
		send:          make(chan []byte, 256),
		subscriptions: map[string]bool{"*": true}, // Subscribe to all by default
	}

	h.register <- client

	// Start goroutines for reading and writing
	go client.writePump()
	go client.readPump()
}

// readPump pumps messages from the WebSocket connection to the hub
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		_ = c.conn.Close()
	}()

	c.conn.SetReadLimit(4096)
	_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.hub.logger.Error("WebSocket read error", "error", err)
			}
			break
		}

		// Handle client messages (subscriptions, pings)
		c.handleMessage(message)
	}
}

// writePump pumps messages from the hub to the WebSocket connection
func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				// Hub closed the channel
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			_, _ = w.Write(message)

			// Batch pending messages
			n := len(c.send)
			for i := 0; i < n; i++ {
				_, _ = w.Write([]byte{'\n'})
				_, _ = w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage handles incoming messages from the client
func (c *Client) handleMessage(data []byte) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	switch msg.Type {
	case MessageTypePing:
		// Respond with pong
		response := Message{Type: MessageTypePong, Timestamp: time.Now()}
		if data, err := json.Marshal(response); err == nil {
			select {
			case c.send <- data:
			default:
			}
		}

	case MessageTypeSubscribe:
		// Subscribe to camera(s)
		if cameras, ok := msg.Data.([]interface{}); ok {
			for _, cam := range cameras {
				if cameraID, ok := cam.(string); ok {
					c.subscriptions[cameraID] = true
				}
			}
		}

	case MessageTypeUnsubscribe:
		// Unsubscribe from camera(s)
		if cameras, ok := msg.Data.([]interface{}); ok {
			for _, cam := range cameras {
				if cameraID, ok := cam.(string); ok {
					delete(c.subscriptions, cameraID)
				}
			}
		}
	}
}

// EventMessage creates an event message
func EventMessage(eventID, cameraID, eventType, label string, confidence float64) Message {
	return Message{
		Type: MessageTypeEvent,
		Data: map[string]interface{}{
			"event_id":   eventID,
			"camera_id":  cameraID,
			"event_type": eventType,
			"label":      label,
			"confidence": confidence,
		},
	}
}

// CameraStateMessage creates a camera state message
func CameraStateMessage(cameraID, status string, fps float64, bitrate int) Message {
	return Message{
		Type: MessageTypeCameraState,
		Data: map[string]interface{}{
			"camera_id": cameraID,
			"status":    status,
			"fps":       fps,
			"bitrate":   bitrate,
		},
	}
}

// DetectionMessage creates a detection message for live view overlay
func DetectionMessage(cameraID string, detections []map[string]interface{}) Message {
	return Message{
		Type: MessageTypeDetection,
		Data: map[string]interface{}{
			"camera_id":  cameraID,
			"detections": detections,
		},
	}
}

// DoorbellRingMessage creates a doorbell ring notification message
func DoorbellRingMessage(cameraID, cameraName, eventID string, snapshotURL string) Message {
	return Message{
		Type: MessageTypeDoorbell,
		Data: map[string]interface{}{
			"camera_id":    cameraID,
			"camera_name":  cameraName,
			"event_id":     eventID,
			"action":       "ring",
			"snapshot_url": snapshotURL,
		},
	}
}

// DoorbellAnsweredMessage creates a doorbell answered notification message
func DoorbellAnsweredMessage(cameraID, sessionID, userID string) Message {
	return Message{
		Type: MessageTypeDoorbell,
		Data: map[string]interface{}{
			"camera_id":  cameraID,
			"session_id": sessionID,
			"user_id":    userID,
			"action":     "answered",
		},
	}
}

// AudioSessionMessage creates an audio session state message
func AudioSessionMessage(cameraID, sessionID, action string, active bool) Message {
	return Message{
		Type: MessageTypeAudioState,
		Data: map[string]interface{}{
			"camera_id":  cameraID,
			"session_id": sessionID,
			"action":     action,
			"active":     active,
		},
	}
}
