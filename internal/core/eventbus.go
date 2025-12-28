// Package core provides the minimal NVR core infrastructure.
// This includes the plugin loader, event bus, and API gateway.
package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// EventBus provides pub/sub messaging between plugins using embedded NATS
type EventBus struct {
	server *server.Server
	conn   *nats.Conn
	logger *slog.Logger

	// Subscription tracking
	subs   map[string][]*nats.Subscription
	subsMu sync.RWMutex
}

// EventBusConfig configures the event bus
type EventBusConfig struct {
	// Host for the NATS server (default: 127.0.0.1)
	Host string
	// Port for the NATS server (default: 12001)
	Port int
	// StoreDir for JetStream persistence (optional)
	StoreDir string
	// EnableJetStream enables JetStream for persistent messaging
	EnableJetStream bool
	// PortManager for dynamic port allocation
	PortManager *PortManager
}

// DefaultEventBusConfig returns default configuration
func DefaultEventBusConfig() EventBusConfig {
	return EventBusConfig{
		Host:            "127.0.0.1",
		Port:            DefaultNATSPort, // 12001
		EnableJetStream: true,
		PortManager:     GetPortManager(),
	}
}

// NewEventBus creates an embedded NATS server for plugin communication
func NewEventBus(cfg EventBusConfig, logger *slog.Logger) (*EventBus, error) {
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Port == 0 {
		cfg.Port = DefaultNATSPort
	}

	// Use port manager to find an available port
	pm := cfg.PortManager
	if pm == nil {
		pm = GetPortManager()
	}

	// Try to reserve the preferred port, or find an available one
	actualPort, err := pm.ReserveOrFind(cfg.Port, "nats")
	if err != nil {
		return nil, fmt.Errorf("failed to allocate NATS port: %w", err)
	}

	if actualPort != cfg.Port {
		logger.Info("NATS port conflict detected, using alternative",
			"preferred", cfg.Port, "actual", actualPort)
	}

	opts := &server.Options{
		Host:   cfg.Host,
		Port:   actualPort,
		NoSigs: true,
		NoLog:  true, // We'll use our own logger
	}

	if cfg.EnableJetStream {
		opts.JetStream = true
		if cfg.StoreDir != "" {
			opts.StoreDir = cfg.StoreDir
		}
	}

	ns, err := server.NewServer(opts)
	if err != nil {
		pm.Release(actualPort)
		return nil, fmt.Errorf("failed to create NATS server: %w", err)
	}

	// Start the server
	go ns.Start()

	// Wait for server to be ready - NATS embedded server is typically ready in <100ms
	if !ns.ReadyForConnections(2 * time.Second) {
		ns.Shutdown()
		pm.Release(actualPort)
		return nil, fmt.Errorf("NATS server not ready after 2 seconds (port %d)", actualPort)
	}

	// Connect to the embedded server
	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		ns.Shutdown()
		return nil, fmt.Errorf("failed to connect to embedded NATS: %w", err)
	}

	eb := &EventBus{
		server: ns,
		conn:   nc,
		logger: logger.With("component", "eventbus"),
		subs:   make(map[string][]*nats.Subscription),
	}

	logger.Info("Event bus started", "url", ns.ClientURL(), "jetstream", cfg.EnableJetStream)

	return eb, nil
}

// Conn returns the NATS connection for direct use
func (eb *EventBus) Conn() *nats.Conn {
	return eb.conn
}

// ClientURL returns the NATS client URL
func (eb *EventBus) ClientURL() string {
	return eb.server.ClientURL()
}

// Publish publishes a message to a subject
func (eb *EventBus) Publish(subject string, data interface{}) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}
	return eb.conn.Publish(subject, payload)
}

// PublishRaw publishes raw bytes to a subject
func (eb *EventBus) PublishRaw(subject string, data []byte) error {
	return eb.conn.Publish(subject, data)
}

// Subscribe subscribes to a subject
func (eb *EventBus) Subscribe(subject string, handler func(*nats.Msg)) (*nats.Subscription, error) {
	sub, err := eb.conn.Subscribe(subject, handler)
	if err != nil {
		return nil, err
	}

	eb.subsMu.Lock()
	eb.subs[subject] = append(eb.subs[subject], sub)
	eb.subsMu.Unlock()

	return sub, nil
}

// SubscribeJSON subscribes to a subject and unmarshals JSON messages
func (eb *EventBus) SubscribeJSON(subject string, handler func(interface{})) (*nats.Subscription, error) {
	return eb.Subscribe(subject, func(msg *nats.Msg) {
		var data interface{}
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			eb.logger.Error("Failed to unmarshal message", "subject", subject, "error", err)
			return
		}
		handler(data)
	})
}

// QueueSubscribe subscribes to a subject with a queue group for load balancing
func (eb *EventBus) QueueSubscribe(subject, queue string, handler func(*nats.Msg)) (*nats.Subscription, error) {
	sub, err := eb.conn.QueueSubscribe(subject, queue, handler)
	if err != nil {
		return nil, err
	}

	eb.subsMu.Lock()
	eb.subs[subject] = append(eb.subs[subject], sub)
	eb.subsMu.Unlock()

	return sub, nil
}

// Request sends a request and waits for a response
func (eb *EventBus) Request(subject string, data interface{}, timeout time.Duration) (*nats.Msg, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}
	return eb.conn.Request(subject, payload, timeout)
}

// RequestRaw sends a raw request and waits for a response
func (eb *EventBus) RequestRaw(subject string, data []byte, timeout time.Duration) (*nats.Msg, error) {
	return eb.conn.Request(subject, data, timeout)
}

// Unsubscribe removes all subscriptions for a subject
func (eb *EventBus) Unsubscribe(subject string) {
	eb.subsMu.Lock()
	defer eb.subsMu.Unlock()

	if subs, ok := eb.subs[subject]; ok {
		for _, sub := range subs {
			_ = sub.Unsubscribe()
		}
		delete(eb.subs, subject)
	}
}

// Stop shuts down the event bus
func (eb *EventBus) Stop() {
	// Drain connection
	_ = eb.conn.Drain()

	// Shutdown NATS server
	eb.server.Shutdown()

	eb.logger.Info("Event bus stopped")
}

// WaitForShutdown blocks until the server shuts down
func (eb *EventBus) WaitForShutdown() {
	eb.server.WaitForShutdown()
}

// Event types for plugin lifecycle
const (
	SubjectPluginStarted  = "plugins.lifecycle.started"
	SubjectPluginStopped  = "plugins.lifecycle.stopped"
	SubjectPluginError    = "plugins.lifecycle.error"
	SubjectPluginHealth   = "plugins.health"
	SubjectConfigChanged  = "config.changed"
	SubjectSystemShutdown = "system.shutdown"
)

// PluginLifecycleEvent represents a plugin lifecycle event
type PluginLifecycleEvent struct {
	PluginID  string    `json:"plugin_id"`
	Event     string    `json:"event"` // "started", "stopped", "error"
	Timestamp time.Time `json:"timestamp"`
	Error     string    `json:"error,omitempty"`
	Version   string    `json:"version,omitempty"`
}

// PublishPluginStarted publishes a plugin started event
func (eb *EventBus) PublishPluginStarted(pluginID, version string) error {
	return eb.Publish(SubjectPluginStarted, PluginLifecycleEvent{
		PluginID:  pluginID,
		Event:     "started",
		Timestamp: time.Now(),
		Version:   version,
	})
}

// PublishPluginStopped publishes a plugin stopped event
func (eb *EventBus) PublishPluginStopped(pluginID string) error {
	return eb.Publish(SubjectPluginStopped, PluginLifecycleEvent{
		PluginID:  pluginID,
		Event:     "stopped",
		Timestamp: time.Now(),
	})
}

// PublishPluginError publishes a plugin error event
func (eb *EventBus) PublishPluginError(pluginID string, err error) error {
	return eb.Publish(SubjectPluginError, PluginLifecycleEvent{
		PluginID:  pluginID,
		Event:     "error",
		Timestamp: time.Now(),
		Error:     err.Error(),
	})
}

// HealthCheck performs a health check on the event bus
func (eb *EventBus) HealthCheck(ctx context.Context) error {
	// Simple ping to verify connectivity
	if !eb.conn.IsConnected() {
		return fmt.Errorf("NATS connection not active")
	}

	// Try a quick request-response
	_, err := eb.conn.Request("_health", []byte("ping"), 2*time.Second)
	if err == nats.ErrNoResponders {
		// No responders is OK, just means no one is listening
		return nil
	}
	return err
}
