package core

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Spatial-NVR/SpatialNVR/sdk"
)

// ExternalPlugin wraps an external plugin process that communicates via JSON-RPC
type ExternalPlugin struct {
	manifest   sdk.PluginManifest
	binaryPath string
	pluginDir  string // Directory where the plugin is installed
	config     map[string]interface{}
	runtime    *sdk.PluginRuntime // Plugin runtime for logging

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	requestID   uint64
	pending     map[uint64]chan *JSONRPCResponse
	pendingLock sync.Mutex

	running   bool
	runningMu sync.RWMutex

	stopCh   chan struct{}
	stopOnce sync.Once
}

// JSONRPCRequest is a JSON-RPC 2.0 request
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      uint64      `json:"id,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// JSONRPCResponse is a JSON-RPC 2.0 response
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError is a JSON-RPC 2.0 error
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// NewExternalPlugin creates a wrapper for an external plugin binary
func NewExternalPlugin(manifest sdk.PluginManifest, binaryPath string, pluginDir string) *ExternalPlugin {
	return &ExternalPlugin{
		manifest:   manifest,
		binaryPath: binaryPath,
		pluginDir:  pluginDir,
		pending:    make(map[uint64]chan *JSONRPCResponse),
		stopCh:     make(chan struct{}),
	}
}

// Manifest returns the plugin's manifest
func (p *ExternalPlugin) Manifest() sdk.PluginManifest {
	return p.manifest
}

// Initialize starts the external process and sends the initialize command
func (p *ExternalPlugin) Initialize(ctx context.Context, runtime *sdk.PluginRuntime) error {
	p.runtime = runtime
	if runtime != nil {
		p.config = runtime.Config()
	}

	// Create a new stop channel and reset stopOnce for this run (important for restarts)
	p.stopCh = make(chan struct{})
	p.stopOnce = sync.Once{}

	// Start the process - use background context so process survives after init
	// The ctx is only for the initialization call timeout, not the process lifecycle
	p.cmd = exec.Command(p.binaryPath)

	// Set working directory to plugin directory so it can find its resources
	if p.pluginDir != "" {
		p.cmd.Dir = p.pluginDir
	}

	// Pass environment with PLUGIN_PATH set
	env := os.Environ()
	if p.pluginDir != "" {
		env = append(env, fmt.Sprintf("PLUGIN_PATH=%s", p.pluginDir))
	}
	p.cmd.Env = env

	var err error
	p.stdin, err = p.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	p.stdout, err = p.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	p.stderr, err = p.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start plugin process: %w", err)
	}

	p.runningMu.Lock()
	p.running = true
	p.runningMu.Unlock()

	// Start response reader
	go p.readResponses()

	// Start stderr logger
	go p.readStderr()

	// Send initialize command
	_, err = p.Call(ctx, "initialize", p.config)
	if err != nil {
		p.Stop(ctx)
		return fmt.Errorf("failed to initialize plugin: %w", err)
	}

	return nil
}

// Start is a no-op for external plugins (they start in Initialize)
func (p *ExternalPlugin) Start(ctx context.Context) error {
	return nil
}

// Stop shuts down the external plugin process
func (p *ExternalPlugin) Stop(ctx context.Context) error {
	p.runningMu.Lock()
	if !p.running {
		p.runningMu.Unlock()
		return nil
	}
	p.running = false
	p.runningMu.Unlock()

	// Use sync.Once to ensure channel is only closed once
	p.stopOnce.Do(func() {
		close(p.stopCh)
	})

	// Send shutdown command
	p.Call(ctx, "shutdown", nil)

	// Close stdin to signal EOF
	if p.stdin != nil {
		p.stdin.Close()
	}

	// Wait for process with timeout
	done := make(chan error, 1)
	go func() {
		done <- p.cmd.Wait()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		p.cmd.Process.Kill()
	}

	return nil
}

// Health returns the plugin's health status
func (p *ExternalPlugin) Health() sdk.HealthStatus {
	p.runningMu.RLock()
	running := p.running
	p.runningMu.RUnlock()

	if !running {
		return sdk.HealthStatus{
			State:       sdk.HealthStateUnhealthy,
			Message:     "Plugin process not running",
			LastChecked: time.Now(),
		}
	}

	// Try to get health from plugin
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := p.Call(ctx, "health", nil)
	if err != nil {
		return sdk.HealthStatus{
			State:       sdk.HealthStateUnhealthy,
			Message:     fmt.Sprintf("Health check failed: %v", err),
			LastChecked: time.Now(),
		}
	}

	var health struct {
		State   string `json:"state"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(resp, &health); err != nil {
		return sdk.HealthStatus{
			State:       sdk.HealthStateHealthy,
			Message:     "Plugin running",
			LastChecked: time.Now(),
		}
	}

	state := sdk.HealthStateHealthy
	switch health.State {
	case "degraded":
		state = sdk.HealthStateDegraded
	case "unhealthy":
		state = sdk.HealthStateUnhealthy
	}

	return sdk.HealthStatus{
		State:       state,
		Message:     health.Message,
		LastChecked: time.Now(),
	}
}

// Routes returns any HTTP routes the plugin provides
func (p *ExternalPlugin) Routes() http.Handler {
	return nil
}

// Call sends a JSON-RPC request and waits for the response
func (p *ExternalPlugin) Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	p.runningMu.RLock()
	running := p.running
	p.runningMu.RUnlock()

	if !running && method != "shutdown" {
		return nil, fmt.Errorf("plugin not running")
	}

	id := atomic.AddUint64(&p.requestID, 1)

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create response channel
	respCh := make(chan *JSONRPCResponse, 1)
	p.pendingLock.Lock()
	p.pending[id] = respCh
	p.pendingLock.Unlock()

	defer func() {
		p.pendingLock.Lock()
		delete(p.pending, id)
		p.pendingLock.Unlock()
	}()

	// Send request
	_, err = p.stdin.Write(append(reqBytes, '\n'))
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Wait for response
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-respCh:
		if resp.Error != nil {
			return nil, fmt.Errorf("RPC error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	}
}

// readResponses reads JSON-RPC responses from stdout
func (p *ExternalPlugin) readResponses() {
	scanner := bufio.NewScanner(p.stdout)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var resp JSONRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			continue
		}

		p.pendingLock.Lock()
		if ch, ok := p.pending[resp.ID]; ok {
			ch <- &resp
		}
		p.pendingLock.Unlock()
	}
}

// readStderr reads and logs stderr from the plugin
func (p *ExternalPlugin) readStderr() {
	scanner := bufio.NewScanner(p.stderr)
	for scanner.Scan() {
		line := scanner.Text()
		// Print to stdout for debugging
		fmt.Printf("[%s] %s\n", p.manifest.ID, line)

		// Store in runtime's log buffer for API access
		if p.runtime != nil {
			p.runtime.AddLog("info", line, nil)
		}
	}
}
