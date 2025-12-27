package plugin

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

// mockWriteCloser implements io.WriteCloser for testing
type mockWriteCloser struct {
	io.Writer
	closed bool
}

func (m *mockWriteCloser) Close() error {
	m.closed = true
	return nil
}

// mockReadCloser implements io.ReadCloser for testing
type mockReadCloser struct {
	io.Reader
	closed bool
}

func (m *mockReadCloser) Close() error {
	m.closed = true
	return nil
}

func TestJSONRPCRequest_Fields(t *testing.T) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "test_method",
		Params:  json.RawMessage(`{"key":"value"}`),
	}

	if req.JSONRPC != "2.0" {
		t.Errorf("Expected JSONRPC '2.0', got %s", req.JSONRPC)
	}
	if req.ID != 1 {
		t.Errorf("Expected ID 1, got %v", req.ID)
	}
	if req.Method != "test_method" {
		t.Errorf("Expected Method 'test_method', got %s", req.Method)
	}

	// Test JSON marshaling
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}
	if !strings.Contains(string(data), "test_method") {
		t.Error("Marshaled JSON should contain method")
	}
}

func TestJSONRPCResponse_Fields(t *testing.T) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      1,
		Result:  json.RawMessage(`{"result":"success"}`),
	}

	if resp.JSONRPC != "2.0" {
		t.Errorf("Expected JSONRPC '2.0', got %s", resp.JSONRPC)
	}
	if resp.ID != 1 {
		t.Errorf("Expected ID 1, got %v", resp.ID)
	}
	if resp.Error != nil {
		t.Error("Expected Error to be nil")
	}

	// Test with error
	errResp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      2,
		Error: &JSONRPCError{
			Code:    RPCMethodNotFound,
			Message: "method not found",
		},
	}

	if errResp.Error == nil {
		t.Error("Expected Error to be set")
	}
	if errResp.Error.Code != RPCMethodNotFound {
		t.Errorf("Expected error code %d, got %d", RPCMethodNotFound, errResp.Error.Code)
	}
}

func TestJSONRPCError_Fields(t *testing.T) {
	tests := []struct {
		code    int
		message string
	}{
		{RPCParseError, "parse error"},
		{RPCInvalidRequest, "invalid request"},
		{RPCMethodNotFound, "method not found"},
		{RPCInvalidParams, "invalid params"},
		{RPCInternalError, "internal error"},
	}

	for _, tt := range tests {
		rpcErr := &JSONRPCError{
			Code:    tt.code,
			Message: tt.message,
			Data:    map[string]string{"detail": "test"},
		}

		if rpcErr.Code != tt.code {
			t.Errorf("Expected code %d, got %d", tt.code, rpcErr.Code)
		}
		if rpcErr.Message != tt.message {
			t.Errorf("Expected message %s, got %s", tt.message, rpcErr.Message)
		}
	}
}

func TestRPCErrorCodes(t *testing.T) {
	if RPCParseError != -32700 {
		t.Errorf("Expected RPCParseError -32700, got %d", RPCParseError)
	}
	if RPCInvalidRequest != -32600 {
		t.Errorf("Expected RPCInvalidRequest -32600, got %d", RPCInvalidRequest)
	}
	if RPCMethodNotFound != -32601 {
		t.Errorf("Expected RPCMethodNotFound -32601, got %d", RPCMethodNotFound)
	}
	if RPCInvalidParams != -32602 {
		t.Errorf("Expected RPCInvalidParams -32602, got %d", RPCInvalidParams)
	}
	if RPCInternalError != -32603 {
		t.Errorf("Expected RPCInternalError -32603, got %d", RPCInternalError)
	}
}

func TestNewPluginClient(t *testing.T) {
	stdin := &mockWriteCloser{Writer: &strings.Builder{}}
	stdout := &mockReadCloser{Reader: strings.NewReader("")}
	stderr := &mockReadCloser{Reader: strings.NewReader("")}

	client := NewPluginClient(stdin, stdout, stderr)
	if client == nil {
		t.Fatal("NewPluginClient returned nil")
	}

	// Close should not panic
	err := client.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestPluginClient_Close(t *testing.T) {
	stdin := &mockWriteCloser{Writer: &strings.Builder{}}
	stdout := &mockReadCloser{Reader: strings.NewReader("")}
	stderr := &mockReadCloser{Reader: strings.NewReader("")}

	client := NewPluginClient(stdin, stdout, stderr)

	err := client.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	if !stdin.closed {
		t.Error("stdin should be closed")
	}
	if !stdout.closed {
		t.Error("stdout should be closed")
	}
	if !stderr.closed {
		t.Error("stderr should be closed")
	}
}

func TestPluginClient_Events(t *testing.T) {
	stdin := &mockWriteCloser{Writer: &strings.Builder{}}
	stdout := &mockReadCloser{Reader: strings.NewReader("")}
	stderr := &mockReadCloser{Reader: strings.NewReader("")}

	client := NewPluginClient(stdin, stdout, stderr)
	defer func() { _ = client.Close() }()

	events := client.Events()
	if events == nil {
		t.Fatal("Events() returned nil channel")
	}
}

func TestPluginClient_Call_ContextCancelled(t *testing.T) {
	// Create a response that will block
	stdin := &mockWriteCloser{Writer: &strings.Builder{}}
	stdout := &mockReadCloser{Reader: strings.NewReader("")}
	stderr := &mockReadCloser{Reader: strings.NewReader("")}

	client := NewPluginClient(stdin, stdout, stderr)
	defer func() { _ = client.Close() }()

	// Cancel context immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Call(ctx, "test", nil)
	if err == nil {
		t.Error("Expected error for cancelled context")
	}
}

func TestPluginClient_Call_Closed(t *testing.T) {
	stdin := &mockWriteCloser{Writer: &strings.Builder{}}
	stdout := &mockReadCloser{Reader: strings.NewReader("")}
	stderr := &mockReadCloser{Reader: strings.NewReader("")}

	client := NewPluginClient(stdin, stdout, stderr)
	_ = client.Close()

	_, err := client.Call(context.Background(), "test", nil)
	if err == nil {
		t.Error("Expected error for closed client")
	}
}

func TestNewPluginProxy(t *testing.T) {
	stdin := &mockWriteCloser{Writer: &strings.Builder{}}
	stdout := &mockReadCloser{Reader: strings.NewReader("")}
	stderr := &mockReadCloser{Reader: strings.NewReader("")}

	client := NewPluginClient(stdin, stdout, stderr)
	proxy := NewPluginProxy(client)

	if proxy == nil {
		t.Fatal("NewPluginProxy returned nil")
	}
	if proxy.client != client {
		t.Error("Proxy client not set correctly")
	}

	err := proxy.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestPluginProxy_Events(t *testing.T) {
	stdin := &mockWriteCloser{Writer: &strings.Builder{}}
	stdout := &mockReadCloser{Reader: strings.NewReader("")}
	stderr := &mockReadCloser{Reader: strings.NewReader("")}

	client := NewPluginClient(stdin, stdout, stderr)
	proxy := NewPluginProxy(client)
	defer func() { _ = proxy.Close() }()

	events := proxy.Events()
	if events == nil {
		t.Fatal("Events() returned nil channel")
	}
}

func TestPluginProxy_Methods_ClosedClient(t *testing.T) {
	stdin := &mockWriteCloser{Writer: &strings.Builder{}}
	stdout := &mockReadCloser{Reader: strings.NewReader("")}
	stderr := &mockReadCloser{Reader: strings.NewReader("")}

	client := NewPluginClient(stdin, stdout, stderr)
	proxy := NewPluginProxy(client)
	_ = client.Close()

	ctx := context.Background()

	// All methods should error on closed client
	err := proxy.Initialize(ctx, nil)
	if err == nil {
		t.Error("Initialize should fail on closed client")
	}

	err = proxy.Shutdown(ctx)
	if err == nil {
		t.Error("Shutdown should fail on closed client")
	}

	_, err = proxy.Health(ctx)
	if err == nil {
		t.Error("Health should fail on closed client")
	}

	_, err = proxy.DiscoverCameras(ctx)
	if err == nil {
		t.Error("DiscoverCameras should fail on closed client")
	}

	_, err = proxy.ListCameras(ctx)
	if err == nil {
		t.Error("ListCameras should fail on closed client")
	}

	_, err = proxy.GetCamera(ctx, "cam1")
	if err == nil {
		t.Error("GetCamera should fail on closed client")
	}

	_, err = proxy.AddCamera(ctx, CameraConfig{})
	if err == nil {
		t.Error("AddCamera should fail on closed client")
	}

	err = proxy.RemoveCamera(ctx, "cam1")
	if err == nil {
		t.Error("RemoveCamera should fail on closed client")
	}

	err = proxy.PTZControl(ctx, "cam1", PTZCommand{})
	if err == nil {
		t.Error("PTZControl should fail on closed client")
	}

	_, err = proxy.GetSnapshot(ctx, "cam1")
	if err == nil {
		t.Error("GetSnapshot should fail on closed client")
	}
}

func TestPluginProxy_ContextTimeout(t *testing.T) {
	stdin := &mockWriteCloser{Writer: &strings.Builder{}}
	stdout := &mockReadCloser{Reader: strings.NewReader("")}
	stderr := &mockReadCloser{Reader: strings.NewReader("")}

	client := NewPluginClient(stdin, stdout, stderr)
	proxy := NewPluginProxy(client)
	defer func() { _ = proxy.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Wait for timeout
	time.Sleep(5 * time.Millisecond)

	err := proxy.Initialize(ctx, nil)
	if err == nil {
		t.Error("Expected timeout error")
	}
}

func TestJSONRPCRequest_Marshaling(t *testing.T) {
	tests := []struct {
		name   string
		req    JSONRPCRequest
		expect string
	}{
		{
			name: "basic request",
			req: JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      1,
				Method:  "test",
			},
			expect: `"jsonrpc":"2.0"`,
		},
		{
			name: "with params",
			req: JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      1,
				Method:  "test",
				Params:  json.RawMessage(`{"key":"value"}`),
			},
			expect: `"params":{"key":"value"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.req)
			if err != nil {
				t.Fatalf("Failed to marshal: %v", err)
			}
			if !strings.Contains(string(data), tt.expect) {
				t.Errorf("Expected %s in %s", tt.expect, string(data))
			}
		})
	}
}

func TestJSONRPCResponse_Marshaling(t *testing.T) {
	tests := []struct {
		name   string
		resp   JSONRPCResponse
		expect string
	}{
		{
			name: "success response",
			resp: JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      1,
				Result:  json.RawMessage(`{"success":true}`),
			},
			expect: `"result":{"success":true}`,
		},
		{
			name: "error response",
			resp: JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      1,
				Error: &JSONRPCError{
					Code:    -32601,
					Message: "method not found",
				},
			},
			expect: `"error":{"code":-32601`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.resp)
			if err != nil {
				t.Fatalf("Failed to marshal: %v", err)
			}
			if !strings.Contains(string(data), tt.expect) {
				t.Errorf("Expected %s in %s", tt.expect, string(data))
			}
		})
	}
}
