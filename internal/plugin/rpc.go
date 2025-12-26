package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

// JSONRPCRequest represents a JSON-RPC 2.0 request
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError represents a JSON-RPC 2.0 error
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Standard JSON-RPC error codes
const (
	RPCParseError     = -32700
	RPCInvalidRequest = -32600
	RPCMethodNotFound = -32601
	RPCInvalidParams  = -32602
	RPCInternalError  = -32603
)

// PluginClient communicates with a plugin process via JSON-RPC over stdio
type PluginClient struct {
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	stderr  io.ReadCloser

	scanner  *bufio.Scanner
	encoder  *json.Encoder

	pending  map[interface{}]chan *JSONRPCResponse
	events   chan *JSONRPCRequest // Notifications from plugin

	nextID   int64
	mu       sync.Mutex
	closed   bool
}

// NewPluginClient creates a client for communicating with a plugin process
func NewPluginClient(stdin io.WriteCloser, stdout, stderr io.ReadCloser) *PluginClient {
	c := &PluginClient{
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
		scanner: bufio.NewScanner(stdout),
		encoder: json.NewEncoder(stdin),
		pending: make(map[interface{}]chan *JSONRPCResponse),
		events:  make(chan *JSONRPCRequest, 100),
	}

	// Set a large buffer for the scanner (plugins may send large responses)
	c.scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024) // 10MB max

	go c.readLoop()

	return c
}

// readLoop reads responses and notifications from the plugin
func (c *PluginClient) readLoop() {
	for c.scanner.Scan() {
		line := c.scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// Try to parse as a response first
		var resp JSONRPCResponse
		if err := json.Unmarshal(line, &resp); err == nil && resp.ID != nil {
			c.mu.Lock()
			if ch, ok := c.pending[resp.ID]; ok {
				ch <- &resp
				delete(c.pending, resp.ID)
			}
			c.mu.Unlock()
			continue
		}

		// Try to parse as a notification (no ID)
		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err == nil && req.Method != "" {
			select {
			case c.events <- &req:
			default:
				// Event channel full, drop
			}
		}
	}

	// Scanner finished, close pending requests
	c.mu.Lock()
	c.closed = true
	for _, ch := range c.pending {
		close(ch)
	}
	c.pending = make(map[interface{}]chan *JSONRPCResponse)
	c.mu.Unlock()
}

// Call makes a JSON-RPC call and waits for a response
func (c *PluginClient) Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	id := atomic.AddInt64(&c.nextID, 1)

	var paramsJSON json.RawMessage
	if params != nil {
		var err error
		paramsJSON, err = json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
	}

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  paramsJSON,
	}

	// Register response channel
	respCh := make(chan *JSONRPCResponse, 1)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("plugin client closed")
	}
	c.pending[id] = respCh
	c.mu.Unlock()

	// Send request
	c.mu.Lock()
	err := c.encoder.Encode(req)
	c.mu.Unlock()
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Wait for response
	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	case resp, ok := <-respCh:
		if !ok {
			return nil, fmt.Errorf("plugin client closed")
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("RPC error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	}
}

// Events returns a channel of notifications from the plugin
func (c *PluginClient) Events() <-chan *JSONRPCRequest {
	return c.events
}

// Close closes the client and releases resources
func (c *PluginClient) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()

	c.stdin.Close()
	c.stdout.Close()
	if c.stderr != nil {
		c.stderr.Close()
	}
	return nil
}

// PluginProxy wraps PluginClient to provide typed methods
type PluginProxy struct {
	client *PluginClient
}

// NewPluginProxy creates a typed proxy for a plugin client
func NewPluginProxy(client *PluginClient) *PluginProxy {
	return &PluginProxy{client: client}
}

// Initialize calls the plugin's initialize method
func (p *PluginProxy) Initialize(ctx context.Context, config map[string]interface{}) error {
	_, err := p.client.Call(ctx, "initialize", config)
	return err
}

// Shutdown calls the plugin's shutdown method
func (p *PluginProxy) Shutdown(ctx context.Context) error {
	_, err := p.client.Call(ctx, "shutdown", nil)
	return err
}

// Health calls the plugin's health method
func (p *PluginProxy) Health(ctx context.Context) (*HealthStatus, error) {
	result, err := p.client.Call(ctx, "health", nil)
	if err != nil {
		return nil, err
	}
	var health HealthStatus
	if err := json.Unmarshal(result, &health); err != nil {
		return nil, fmt.Errorf("failed to unmarshal health: %w", err)
	}
	return &health, nil
}

// DiscoverCameras calls the plugin's discover method
func (p *PluginProxy) DiscoverCameras(ctx context.Context) ([]DiscoveredCamera, error) {
	result, err := p.client.Call(ctx, "discover_cameras", nil)
	if err != nil {
		return nil, err
	}
	var cameras []DiscoveredCamera
	if err := json.Unmarshal(result, &cameras); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cameras: %w", err)
	}
	return cameras, nil
}

// AddCamera adds a camera to the plugin
func (p *PluginProxy) AddCamera(ctx context.Context, config CameraConfig) (*PluginCamera, error) {
	result, err := p.client.Call(ctx, "add_camera", config)
	if err != nil {
		return nil, err
	}
	var camera PluginCamera
	if err := json.Unmarshal(result, &camera); err != nil {
		return nil, fmt.Errorf("failed to unmarshal camera: %w", err)
	}
	return &camera, nil
}

// RemoveCamera removes a camera from the plugin
func (p *PluginProxy) RemoveCamera(ctx context.Context, cameraID string) error {
	_, err := p.client.Call(ctx, "remove_camera", map[string]string{"camera_id": cameraID})
	return err
}

// ListCameras lists all cameras managed by the plugin
func (p *PluginProxy) ListCameras(ctx context.Context) ([]PluginCamera, error) {
	result, err := p.client.Call(ctx, "list_cameras", nil)
	if err != nil {
		return nil, err
	}
	var cameras []PluginCamera
	if err := json.Unmarshal(result, &cameras); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cameras: %w", err)
	}
	return cameras, nil
}

// GetCamera gets a specific camera
func (p *PluginProxy) GetCamera(ctx context.Context, cameraID string) (*PluginCamera, error) {
	result, err := p.client.Call(ctx, "get_camera", map[string]string{"camera_id": cameraID})
	if err != nil {
		return nil, err
	}
	var camera PluginCamera
	if err := json.Unmarshal(result, &camera); err != nil {
		return nil, fmt.Errorf("failed to unmarshal camera: %w", err)
	}
	return &camera, nil
}

// PTZControl sends a PTZ command to a camera
func (p *PluginProxy) PTZControl(ctx context.Context, cameraID string, cmd PTZCommand) error {
	params := map[string]interface{}{
		"camera_id": cameraID,
		"command":   cmd,
	}
	_, err := p.client.Call(ctx, "ptz_control", params)
	return err
}

// GetSnapshot captures a snapshot from a camera
func (p *PluginProxy) GetSnapshot(ctx context.Context, cameraID string) ([]byte, error) {
	result, err := p.client.Call(ctx, "get_snapshot", map[string]string{"camera_id": cameraID})
	if err != nil {
		return nil, err
	}
	// Result is base64 encoded
	var encoded string
	if err := json.Unmarshal(result, &encoded); err != nil {
		return nil, fmt.Errorf("failed to unmarshal snapshot: %w", err)
	}
	// Decode base64
	// Note: caller should use encoding/base64 to decode
	return result, nil
}

// Events returns the event channel from the underlying client
func (p *PluginProxy) Events() <-chan *JSONRPCRequest {
	return p.client.Events()
}

// Close closes the proxy and underlying client
func (p *PluginProxy) Close() error {
	return p.client.Close()
}
