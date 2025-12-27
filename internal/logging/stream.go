package logging

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"time"
)

// LogEntry represents a structured log entry
type LogEntry struct {
	Time      time.Time              `json:"time"`
	Level     string                 `json:"level"`
	Message   string                 `json:"msg"`
	Component string                 `json:"component,omitempty"`
	Attrs     map[string]interface{} `json:"attrs,omitempty"`
}

// RingBuffer stores the most recent log entries
type RingBuffer struct {
	entries []LogEntry
	size    int
	head    int
	count   int
	mu      sync.RWMutex

	// Subscribers for live streaming
	subscribers map[chan LogEntry]bool
	subMu       sync.RWMutex
}

// NewRingBuffer creates a new ring buffer with the specified size
func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		entries:     make([]LogEntry, size),
		size:        size,
		subscribers: make(map[chan LogEntry]bool),
	}
}

// Add adds a log entry to the ring buffer
func (rb *RingBuffer) Add(entry LogEntry) {
	rb.mu.Lock()
	rb.entries[rb.head] = entry
	rb.head = (rb.head + 1) % rb.size
	if rb.count < rb.size {
		rb.count++
	}
	rb.mu.Unlock()

	// Notify subscribers
	rb.subMu.RLock()
	for ch := range rb.subscribers {
		select {
		case ch <- entry:
		default:
			// Skip if subscriber can't keep up
		}
	}
	rb.subMu.RUnlock()
}

// GetRecent returns the most recent n entries
func (rb *RingBuffer) GetRecent(n int) []LogEntry {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if n > rb.count {
		n = rb.count
	}

	result := make([]LogEntry, n)
	start := (rb.head - n + rb.size) % rb.size
	for i := 0; i < n; i++ {
		result[i] = rb.entries[(start+i)%rb.size]
	}
	return result
}

// Subscribe creates a channel that receives new log entries
func (rb *RingBuffer) Subscribe() chan LogEntry {
	ch := make(chan LogEntry, 100)
	rb.subMu.Lock()
	rb.subscribers[ch] = true
	rb.subMu.Unlock()
	return ch
}

// Unsubscribe removes a subscription
func (rb *RingBuffer) Unsubscribe(ch chan LogEntry) {
	rb.subMu.Lock()
	delete(rb.subscribers, ch)
	rb.subMu.Unlock()
	close(ch)
}

// StreamHandler is a slog handler that captures logs to a ring buffer
type StreamHandler struct {
	buffer   *RingBuffer
	fallback slog.Handler
	level    slog.Level
	attrs    []slog.Attr
	groups   []string
}

// NewStreamHandler creates a handler that captures logs to the ring buffer
func NewStreamHandler(buffer *RingBuffer, fallback io.Writer, level slog.Level) *StreamHandler {
	return &StreamHandler{
		buffer:   buffer,
		fallback: slog.NewJSONHandler(fallback, &slog.HandlerOptions{Level: level}),
		level:    level,
	}
}

// Enabled implements slog.Handler
func (h *StreamHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle implements slog.Handler
func (h *StreamHandler) Handle(ctx context.Context, r slog.Record) error {
	// Extract attributes
	attrs := make(map[string]interface{})
	var component string

	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "component" {
			component = a.Value.String()
		} else {
			attrs[a.Key] = a.Value.Any()
		}
		return true
	})

	// Add handler-level attrs
	for _, a := range h.attrs {
		if a.Key == "component" {
			component = a.Value.String()
		} else {
			attrs[a.Key] = a.Value.Any()
		}
	}

	entry := LogEntry{
		Time:      r.Time,
		Level:     r.Level.String(),
		Message:   r.Message,
		Component: component,
		Attrs:     attrs,
	}

	h.buffer.Add(entry)

	// Also write to fallback
	return h.fallback.Handle(ctx, r)
}

// WithAttrs implements slog.Handler
func (h *StreamHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &StreamHandler{
		buffer:   h.buffer,
		fallback: h.fallback.WithAttrs(attrs),
		level:    h.level,
		attrs:    append(h.attrs, attrs...),
		groups:   h.groups,
	}
}

// WithGroup implements slog.Handler
func (h *StreamHandler) WithGroup(name string) slog.Handler {
	return &StreamHandler{
		buffer:   h.buffer,
		fallback: h.fallback.WithGroup(name),
		level:    h.level,
		attrs:    h.attrs,
		groups:   append(h.groups, name),
	}
}

// Global log buffer
var globalBuffer = NewRingBuffer(1000)

// GetLogBuffer returns the global log buffer
func GetLogBuffer() *RingBuffer {
	return globalBuffer
}

// LogEntryToJSON converts a log entry to JSON string
func LogEntryToJSON(entry LogEntry) string {
	data, _ := json.Marshal(entry)
	return string(data)
}
