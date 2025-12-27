// Package nvrcoreevents provides the NVR Core Events Plugin
// This plugin handles event storage, retrieval, and real-time streaming
package nvrcoreevents

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Spatial-NVR/SpatialNVR/internal/database"
	"github.com/Spatial-NVR/SpatialNVR/internal/events"
	"github.com/Spatial-NVR/SpatialNVR/sdk"
)

// EventsPlugin implements the events service as a plugin
type EventsPlugin struct {
	sdk.BaseServicePlugin

	eventService *events.Service
	db           *database.DB

	maxEvents     int
	retentionDays int

	// SSE subscribers
	sseClients   map[chan *events.Event]struct{}
	sseClientsMu sync.RWMutex

	mu      sync.RWMutex
	started bool
}

// New creates a new EventsPlugin instance
func New() *EventsPlugin {
	p := &EventsPlugin{
		sseClients: make(map[chan *events.Event]struct{}),
	}
	p.SetManifest(sdk.PluginManifest{
		ID:           "nvr-core-events",
		Name:         "Events Service",
		Version:      "1.0.0",
		Description:  "Event management, storage, and streaming",
		Category:     "core",
		Critical:     true,
		Dependencies: []string{},
		Capabilities: []string{
			sdk.CapabilityEvents,
		},
	})
	return p
}

// Initialize sets up the plugin
func (p *EventsPlugin) Initialize(ctx context.Context, runtime *sdk.PluginRuntime) error {
	if err := p.BaseServicePlugin.Initialize(ctx, runtime); err != nil {
		return err
	}

	// Get configuration
	p.maxEvents = runtime.ConfigInt("max_events", 10000)
	p.retentionDays = runtime.ConfigInt("retention_days", 30)

	// Get database from runtime
	db := runtime.DB()
	if db != nil {
		p.db = &database.DB{DB: db}
	}

	return nil
}

// Start starts the events service
func (p *EventsPlugin) Start(ctx context.Context) error {
	runtime := p.Runtime()
	if runtime == nil {
		return nil
	}

	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	// Create event service
	if p.db != nil {
		p.eventService = events.NewService(p.db)
	}

	// Subscribe to all detection and motion events
	if err := p.subscribeToEvents(); err != nil {
		runtime.Logger().Warn("Failed to subscribe to events", "error", err)
	}

	// Start cleanup routine
	go p.cleanupRoutine(ctx)

	p.mu.Lock()
	p.started = true
	p.mu.Unlock()

	p.SetHealthy("Events service running")
	runtime.Logger().Info("Events plugin started")

	return nil
}

// Stop stops the events service
func (p *EventsPlugin) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Close all SSE connections
	p.sseClientsMu.Lock()
	for ch := range p.sseClients {
		close(ch)
		delete(p.sseClients, ch)
	}
	p.sseClientsMu.Unlock()

	p.started = false
	p.SetHealth(sdk.HealthStateUnknown, "Events service stopped")

	return nil
}

// Health returns the plugin's health status
func (p *EventsPlugin) Health() sdk.HealthStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.started {
		return sdk.HealthStatus{
			State:       sdk.HealthStateUnknown,
			Message:     "Not started",
			LastChecked: time.Now(),
		}
	}

	return sdk.HealthStatus{
		State:       sdk.HealthStateHealthy,
		Message:     "Events service operational",
		LastChecked: time.Now(),
	}
}

// Routes returns the HTTP routes for this plugin
func (p *EventsPlugin) Routes() http.Handler {
	r := chi.NewRouter()

	// Event CRUD
	r.Get("/", p.handleListEvents)
	r.Post("/", p.handleCreateEvent)
	r.Get("/{id}", p.handleGetEvent)
	r.Put("/{id}", p.handleUpdateEvent)
	r.Delete("/{id}", p.handleDeleteEvent)

	// Event actions
	r.Post("/{id}/acknowledge", p.handleAcknowledge)
	r.Post("/{id}/unacknowledge", p.handleUnacknowledge)

	// Streaming
	r.Get("/stream", p.handleSSE)
	r.Get("/ws", p.handleWebSocket)

	// Stats
	r.Get("/stats", p.handleGetStats)
	r.Get("/cameras/{cameraId}", p.handleGetCameraEvents)

	return r
}

// EventSubscriptions returns events this plugin subscribes to
func (p *EventsPlugin) EventSubscriptions() []string {
	return []string{
		sdk.EventTypeDetection,
		sdk.EventTypeMotion,
		sdk.EventTypeMotionEnded,
		sdk.EventTypeCameraAdded,
		sdk.EventTypeCameraRemoved,
	}
}

// HandleEvent processes incoming events
func (p *EventsPlugin) HandleEvent(ctx context.Context, event *sdk.Event) error {
	if p.eventService == nil {
		return nil
	}

	// Get camera ID safely
	cameraID := ""
	if id, ok := event.Data["camera_id"].(string); ok {
		cameraID = id
	}

	// Map SDK event type to events.EventType
	var eventType events.EventType
	switch event.Type {
	case sdk.EventTypeDetection:
		eventType = events.EventPerson // Default to person, could be refined
	case sdk.EventTypeMotion:
		eventType = events.EventMotion
	default:
		eventType = events.EventStateChange
	}

	// Convert SDK event to events.Event and store
	e := &events.Event{
		CameraID:  cameraID,
		EventType: eventType,
		Timestamp: event.Timestamp,
	}

	if label, ok := event.Data["label"].(string); ok {
		e.Label = label
	}
	if conf, ok := event.Data["confidence"].(float64); ok {
		e.Confidence = conf
	}

	if err := p.eventService.Create(ctx, e); err != nil {
		p.Runtime().Logger().Error("Failed to store event", "error", err)
		return err
	}

	// Broadcast to SSE clients
	p.broadcastToSSE(e)

	return nil
}

// Private methods

func (p *EventsPlugin) subscribeToEvents() error {
	runtime := p.Runtime()
	if runtime == nil {
		return nil
	}

	return runtime.SubscribeEvents(func(event *sdk.Event) {
		ctx := context.Background()
		_ = p.HandleEvent(ctx, event)
	}, p.EventSubscriptions()...)
}

func (p *EventsPlugin) cleanupRoutine(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.cleanupOldEvents(ctx)
		}
	}
}

func (p *EventsPlugin) cleanupOldEvents(ctx context.Context) {
	// Cleanup routine - delete events older than retention period
	// Note: DeleteBefore method would need to be added to events.Service
	// For now, log that cleanup would happen
	p.Runtime().Logger().Info("Event cleanup routine executed", "retention_days", p.retentionDays)
}

func (p *EventsPlugin) broadcastToSSE(event *events.Event) {
	p.sseClientsMu.RLock()
	defer p.sseClientsMu.RUnlock()

	for ch := range p.sseClients {
		select {
		case ch <- event:
		default:
			// Channel full, skip
		}
	}
}

// HTTP Handlers

func (p *EventsPlugin) handleListEvents(w http.ResponseWriter, r *http.Request) {
	if p.eventService == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Event service not available")
		return
	}

	opts := events.ListOptions{
		Limit:  100,
		Offset: 0,
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			opts.Limit = limit
		}
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset >= 0 {
			opts.Offset = offset
		}
	}
	if cameraID := r.URL.Query().Get("camera_id"); cameraID != "" {
		opts.CameraID = cameraID
	}
	if eventType := r.URL.Query().Get("type"); eventType != "" {
		opts.EventType = events.EventType(eventType)
	}

	eventsList, total, err := p.eventService.List(r.Context(), opts)
	if err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	p.respondJSON(w, map[string]interface{}{
		"events": eventsList,
		"total":  total,
		"limit":  opts.Limit,
		"offset": opts.Offset,
	})
}

func (p *EventsPlugin) handleCreateEvent(w http.ResponseWriter, r *http.Request) {
	if p.eventService == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Event service not available")
		return
	}

	var event events.Event
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		p.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := p.eventService.Create(r.Context(), &event); err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusCreated)
	p.respondJSON(w, event)
}

func (p *EventsPlugin) handleGetEvent(w http.ResponseWriter, r *http.Request) {
	if p.eventService == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Event service not available")
		return
	}

	id := chi.URLParam(r, "id")
	event, err := p.eventService.Get(r.Context(), id)
	if err != nil {
		p.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	p.respondJSON(w, event)
}

func (p *EventsPlugin) handleUpdateEvent(w http.ResponseWriter, r *http.Request) {
	// Update not currently supported - would require adding Update method to events.Service
	p.respondError(w, http.StatusNotImplemented, "Event update not implemented")
}

func (p *EventsPlugin) handleDeleteEvent(w http.ResponseWriter, r *http.Request) {
	if p.eventService == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Event service not available")
		return
	}

	id := chi.URLParam(r, "id")

	if err := p.eventService.Delete(r.Context(), id); err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (p *EventsPlugin) handleAcknowledge(w http.ResponseWriter, r *http.Request) {
	if p.eventService == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Event service not available")
		return
	}

	id := chi.URLParam(r, "id")

	if err := p.eventService.Acknowledge(r.Context(), id); err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	p.respondJSON(w, map[string]string{"status": "acknowledged"})
}

func (p *EventsPlugin) handleUnacknowledge(w http.ResponseWriter, r *http.Request) {
	// Unacknowledge not currently supported
	p.respondError(w, http.StatusNotImplemented, "Event unacknowledge not implemented")
}

func (p *EventsPlugin) handleSSE(w http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Create channel for this client
	ch := make(chan *events.Event, 100)

	p.sseClientsMu.Lock()
	p.sseClients[ch] = struct{}{}
	p.sseClientsMu.Unlock()

	defer func() {
		p.sseClientsMu.Lock()
		delete(p.sseClients, ch)
		p.sseClientsMu.Unlock()
	}()

	flusher, ok := w.(http.Flusher)
	if !ok {
		p.respondError(w, http.StatusInternalServerError, "Streaming not supported")
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}

			data, _ := json.Marshal(event)
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(data)
			_, _ = w.Write([]byte("\n\n"))
			flusher.Flush()
		}
	}
}

func (p *EventsPlugin) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// WebSocket implementation would go here
	// For now, redirect to SSE
	p.handleSSE(w, r)
}

func (p *EventsPlugin) handleGetStats(w http.ResponseWriter, r *http.Request) {
	if p.eventService == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Event service not available")
		return
	}

	// GetStats requires a cameraID, use empty for all cameras
	cameraID := r.URL.Query().Get("camera_id")
	stats, err := p.eventService.GetStats(r.Context(), cameraID)
	if err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	p.respondJSON(w, stats)
}

func (p *EventsPlugin) handleGetCameraEvents(w http.ResponseWriter, r *http.Request) {
	if p.eventService == nil {
		p.respondError(w, http.StatusServiceUnavailable, "Event service not available")
		return
	}

	cameraID := chi.URLParam(r, "cameraId")

	opts := events.ListOptions{
		CameraID: cameraID,
		Limit:    100,
	}

	eventsList, total, err := p.eventService.List(r.Context(), opts)
	if err != nil {
		p.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	p.respondJSON(w, map[string]interface{}{
		"events":    eventsList,
		"total":     total,
		"camera_id": cameraID,
	})
}

// Helper methods

func (p *EventsPlugin) respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (p *EventsPlugin) respondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

// Ensure EventsPlugin implements the sdk.Plugin interface
var _ sdk.Plugin = (*EventsPlugin)(nil)
var _ sdk.ServicePlugin = (*EventsPlugin)(nil)

// Prevent unused function warning
var _ = New
