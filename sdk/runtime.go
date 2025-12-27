package sdk

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

// LogEntry represents a captured log entry
type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// PluginRuntime provides the runtime environment for plugins
type PluginRuntime struct {
	pluginID string
	nats     *nats.Conn
	db       *sql.DB
	config   map[string]interface{}
	logger   *slog.Logger

	// Event subscriptions
	subs   []*nats.Subscription
	subsMu sync.Mutex

	// Log buffer
	logBuffer []LogEntry
	logMu     sync.RWMutex
	maxLogs   int

	// Shutdown channel
	stopCh chan struct{}
}

// NewPluginRuntime creates a new runtime for a plugin
func NewPluginRuntime(pluginID string, nc *nats.Conn, db *sql.DB, config map[string]interface{}, logger *slog.Logger) *PluginRuntime {
	r := &PluginRuntime{
		pluginID:  pluginID,
		nats:      nc,
		db:        db,
		config:    config,
		stopCh:    make(chan struct{}),
		logBuffer: make([]LogEntry, 0, 1000),
		maxLogs:   1000,
	}

	// Create a wrapped logger that captures log entries
	r.logger = slog.New(&logCaptureHandler{
		runtime: r,
		inner:   logger.With("plugin", pluginID).Handler(),
	})

	return r
}

// logCaptureHandler wraps slog.Handler to capture log entries
type logCaptureHandler struct {
	runtime *PluginRuntime
	inner   slog.Handler
}

func (h *logCaptureHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *logCaptureHandler) Handle(ctx context.Context, record slog.Record) error {
	// Capture to buffer
	entry := LogEntry{
		Timestamp: record.Time,
		Level:     record.Level.String(),
		Message:   record.Message,
		Metadata:  make(map[string]interface{}),
	}

	record.Attrs(func(a slog.Attr) bool {
		entry.Metadata[a.Key] = a.Value.Any()
		return true
	})

	h.runtime.addLog(entry)

	// Pass through to original handler
	return h.inner.Handle(ctx, record)
}

func (h *logCaptureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &logCaptureHandler{
		runtime: h.runtime,
		inner:   h.inner.WithAttrs(attrs),
	}
}

func (h *logCaptureHandler) WithGroup(name string) slog.Handler {
	return &logCaptureHandler{
		runtime: h.runtime,
		inner:   h.inner.WithGroup(name),
	}
}

// addLog adds a log entry to the buffer
func (r *PluginRuntime) addLog(entry LogEntry) {
	r.logMu.Lock()
	defer r.logMu.Unlock()

	r.logBuffer = append(r.logBuffer, entry)

	// Trim if exceeds max
	if len(r.logBuffer) > r.maxLogs {
		r.logBuffer = r.logBuffer[len(r.logBuffer)-r.maxLogs:]
	}
}

// AddLog adds a log entry to the buffer (public method for external plugins)
func (r *PluginRuntime) AddLog(level, message string, metadata map[string]interface{}) {
	r.addLog(LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
		Metadata:  metadata,
	})
}

// GetLogs returns the last n log entries
func (r *PluginRuntime) GetLogs(n int) []LogEntry {
	r.logMu.RLock()
	defer r.logMu.RUnlock()

	if n <= 0 || n > len(r.logBuffer) {
		n = len(r.logBuffer)
	}

	start := len(r.logBuffer) - n
	if start < 0 {
		start = 0
	}

	result := make([]LogEntry, n)
	copy(result, r.logBuffer[start:])
	return result
}

// PluginID returns the plugin's ID
func (r *PluginRuntime) PluginID() string {
	return r.pluginID
}

// Logger returns a logger for the plugin
func (r *PluginRuntime) Logger() *slog.Logger {
	return r.logger
}

// Config returns the plugin's configuration
func (r *PluginRuntime) Config() map[string]interface{} {
	return r.config
}

// ConfigValue returns a specific config value
func (r *PluginRuntime) ConfigValue(key string) interface{} {
	return r.config[key]
}

// ConfigString returns a string config value
func (r *PluginRuntime) ConfigString(key string, defaultVal string) string {
	if v, ok := r.config[key].(string); ok {
		return v
	}
	return defaultVal
}

// ConfigInt returns an int config value
func (r *PluginRuntime) ConfigInt(key string, defaultVal int) int {
	switch v := r.config[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return defaultVal
}

// ConfigFloat returns a float config value
func (r *PluginRuntime) ConfigFloat(key string, defaultVal float64) float64 {
	switch v := r.config[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	}
	return defaultVal
}

// ConfigBool returns a bool config value
func (r *PluginRuntime) ConfigBool(key string, defaultVal bool) bool {
	if v, ok := r.config[key].(bool); ok {
		return v
	}
	return defaultVal
}

// DB returns the shared database connection
func (r *PluginRuntime) DB() *sql.DB {
	return r.db
}

// PublishEvent publishes an event to the event bus
func (r *PluginRuntime) PublishEvent(eventType string, data interface{}) error {
	event := &Event{
		ID:        fmt.Sprintf("%s-%d", r.pluginID, time.Now().UnixNano()),
		Type:      eventType,
		Timestamp: time.Now(),
	}

	// If data is already an Event, use it directly
	if e, ok := data.(*Event); ok {
		event = e
		event.Type = eventType
	} else if dataMap, ok := data.(map[string]interface{}); ok {
		event.Data = dataMap
	} else {
		// Convert to map
		jsonData, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("failed to marshal event data: %w", err)
		}
		var dataMap map[string]interface{}
		if err := json.Unmarshal(jsonData, &dataMap); err != nil {
			event.Data = map[string]interface{}{"value": data}
		} else {
			event.Data = dataMap
		}
	}

	jsonBytes, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	subject := fmt.Sprintf("events.%s", eventType)
	return r.nats.Publish(subject, jsonBytes)
}

// SubscribeEvents subscribes to multiple event types
func (r *PluginRuntime) SubscribeEvents(handler func(*Event), eventTypes ...string) error {
	r.subsMu.Lock()
	defer r.subsMu.Unlock()

	for _, eventType := range eventTypes {
		subject := fmt.Sprintf("events.%s", eventType)
		sub, err := r.nats.Subscribe(subject, func(msg *nats.Msg) {
			var event Event
			if err := json.Unmarshal(msg.Data, &event); err != nil {
				r.logger.Error("Failed to unmarshal event", "error", err)
				return
			}
			handler(&event)
		})
		if err != nil {
			return fmt.Errorf("failed to subscribe to %s: %w", eventType, err)
		}
		r.subs = append(r.subs, sub)
	}

	return nil
}

// Request sends a request to another plugin and waits for response
func (r *PluginRuntime) Request(pluginID, method string, data interface{}, timeout time.Duration) ([]byte, error) {
	subject := fmt.Sprintf("plugins.%s.%s", pluginID, method)

	payload, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	msg, err := r.nats.Request(subject, payload, timeout)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return msg.Data, nil
}

// HandleRequests registers a handler for incoming plugin requests
func (r *PluginRuntime) HandleRequests(method string, handler func([]byte) ([]byte, error)) error {
	subject := fmt.Sprintf("plugins.%s.%s", r.pluginID, method)

	_, err := r.nats.Subscribe(subject, func(msg *nats.Msg) {
		response, err := handler(msg.Data)
		if err != nil {
			r.logger.Error("Request handler failed", "method", method, "error", err)
			errResp, _ := json.Marshal(map[string]string{"error": err.Error()})
			_ = msg.Respond(errResp)
			return
		}
		_ = msg.Respond(response)
	})

	return err
}

// Broadcast sends a message to all subscribers of a subject
func (r *PluginRuntime) Broadcast(subject string, data interface{}) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal broadcast data: %w", err)
	}
	return r.nats.Publish(subject, payload)
}

// Stop cleans up the runtime
func (r *PluginRuntime) Stop() {
	close(r.stopCh)

	r.subsMu.Lock()
	defer r.subsMu.Unlock()

	for _, sub := range r.subs {
		_ = sub.Unsubscribe()
	}
	r.subs = nil
}

// Done returns a channel that's closed when the runtime is stopped
func (r *PluginRuntime) Done() <-chan struct{} {
	return r.stopCh
}
