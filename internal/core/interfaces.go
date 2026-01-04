package core

import (
	"context"
	"encoding/json"
)

// PluginRPCCaller is the interface for calling external plugins via RPC
type PluginRPCCaller interface {
	// Call sends a JSON-RPC request to the plugin and returns the result
	Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error)
}

// PluginRPCProvider provides access to plugin RPC callers
type PluginRPCProvider interface {
	// GetPluginCaller returns a PluginRPCCaller for the specified plugin ID
	// Returns nil, false if the plugin is not found or doesn't support RPC
	GetPluginCaller(id string) (PluginRPCCaller, bool)
}
