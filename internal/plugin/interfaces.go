// Package plugin provides the plugin framework for the NVR system.
// Plugins are external processes that communicate with the NVR via JSON-RPC over stdio.
// This allows plugins to be written in any language and distributed independently.
package plugin

import (
	"context"
	"time"
)

// PluginManifest describes a plugin package available for installation
type PluginManifest struct {
	// ID is the unique identifier (e.g., "reolink", "wyze")
	ID string `json:"id" yaml:"id"`

	// Name is the human-readable name
	Name string `json:"name" yaml:"name"`

	// Version is the plugin version (semver)
	Version string `json:"version" yaml:"version"`

	// Description explains what the plugin does
	Description string `json:"description" yaml:"description"`

	// Author is the plugin author/maintainer
	Author string `json:"author" yaml:"author"`

	// Homepage is the plugin's website or repository
	Homepage string `json:"homepage,omitempty" yaml:"homepage,omitempty"`

	// License is the plugin license
	License string `json:"license,omitempty" yaml:"license,omitempty"`

	// Runtime specifies how to run the plugin
	Runtime PluginRuntime `json:"runtime" yaml:"runtime"`

	// Capabilities lists what this plugin can do
	Capabilities []Capability `json:"capabilities" yaml:"capabilities"`

	// Dependencies lists required external binaries or libraries
	Dependencies []PluginDependency `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`

	// ConfigSchema describes the plugin's configuration options (JSON Schema)
	ConfigSchema map[string]interface{} `json:"config_schema,omitempty" yaml:"config_schema,omitempty"`
}

// PluginRuntime specifies how a plugin is executed
type PluginRuntime struct {
	// Type is the runtime type: "binary", "python", "node"
	Type string `json:"type" yaml:"type"`

	// Binary is the path to the executable (relative to plugin dir)
	Binary string `json:"binary,omitempty" yaml:"binary,omitempty"`

	// EntryPoint is the main script for interpreted languages
	EntryPoint string `json:"entry_point,omitempty" yaml:"entry_point,omitempty"`

	// Script is an alias for EntryPoint (used by some plugin manifests)
	Script string `json:"script,omitempty" yaml:"script,omitempty"`

	// Setup is a setup script to run before the plugin (e.g., install dependencies)
	Setup string `json:"setup,omitempty" yaml:"setup,omitempty"`

	// Args are additional arguments to pass to the runtime
	Args []string `json:"args,omitempty" yaml:"args,omitempty"`

	// Env specifies environment variables to set when running the plugin
	// Format: KEY=value
	Env []string `json:"env,omitempty" yaml:"env,omitempty"`
}

// GetEntryPoint returns the entry point script, checking both EntryPoint and Script fields
func (r PluginRuntime) GetEntryPoint() string {
	if r.EntryPoint != "" {
		return r.EntryPoint
	}
	return r.Script
}

// PluginDependency describes an external dependency
type PluginDependency struct {
	// Name of the dependency
	Name string `json:"name" yaml:"name"`

	// Type: "binary", "library", "python-package"
	Type string `json:"type" yaml:"type"`

	// URL to download from (supports platform placeholders: {os}, {arch})
	URL string `json:"url,omitempty" yaml:"url,omitempty"`

	// Version constraint
	Version string `json:"version,omitempty" yaml:"version,omitempty"`

	// Platforms this dependency applies to
	Platforms []string `json:"platforms,omitempty" yaml:"platforms,omitempty"`
}

// Capability represents a feature that a plugin or camera supports
type Capability string

const (
	// Camera capabilities
	CapabilityVideo       Capability = "video"         // Can provide video streams
	CapabilityAudio       Capability = "audio"         // Has audio support
	CapabilityTwoWayAudio Capability = "two_way_audio" // Supports two-way audio
	CapabilityPTZ         Capability = "ptz"           // Pan-Tilt-Zoom control
	CapabilityPresets     Capability = "presets"       // PTZ preset positions
	CapabilityMotion      Capability = "motion"        // Motion detection
	CapabilityAIDetection Capability = "ai_detection"  // AI-based object detection
	CapabilityDoorbell    Capability = "doorbell"      // Doorbell functionality
	CapabilitySiren       Capability = "siren"         // Has siren/alarm
	CapabilityFloodlight  Capability = "floodlight"    // Has controllable light
	CapabilityBattery     Capability = "battery"       // Battery powered
	CapabilityNightVision Capability = "night_vision"  // IR night vision
	CapabilityColorNight  Capability = "color_night"   // Color night vision
	CapabilitySnapshot    Capability = "snapshot"      // Can capture snapshots

	// Plugin-level capabilities
	CapabilityDiscovery Capability = "discovery" // Can discover devices on network
	CapabilityNVR       Capability = "nvr"       // Supports NVR devices with multiple channels
)

// PluginState represents the current state of a plugin
type PluginState string

const (
	PluginStateNotInstalled PluginState = "not_installed"
	PluginStateInstalled    PluginState = "installed"
	PluginStateStarting     PluginState = "starting"
	PluginStateRunning      PluginState = "running"
	PluginStateStopping     PluginState = "stopping"
	PluginStateStopped      PluginState = "stopped"
	PluginStateError        PluginState = "error"
)

// PluginStatus represents the runtime status of an installed plugin
type PluginStatus struct {
	Manifest   PluginManifest `json:"manifest"`
	State      PluginState    `json:"state"`
	Enabled    bool           `json:"enabled"`
	Health     HealthStatus   `json:"health"`
	CameraCount int           `json:"camera_count"`
	LastError  string         `json:"last_error,omitempty"`
	StartedAt  *time.Time     `json:"started_at,omitempty"`
}

// HealthStatus represents plugin health
type HealthStatus struct {
	State     HealthState            `json:"state"`
	Message   string                 `json:"message,omitempty"`
	LastCheck time.Time              `json:"last_check"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// HealthState represents the health state enum
type HealthState string

const (
	HealthStateHealthy   HealthState = "healthy"
	HealthStateUnhealthy HealthState = "unhealthy"
	HealthStateDegraded  HealthState = "degraded"
	HealthStateUnknown   HealthState = "unknown"
)

// StreamQuality represents video stream quality levels
type StreamQuality string

const (
	StreamQualityMain StreamQuality = "main" // Full resolution
	StreamQualitySub  StreamQuality = "sub"  // Lower resolution
)

// DiscoveredCamera represents a camera found during discovery
type DiscoveredCamera struct {
	ID              string       `json:"id"`
	Name            string       `json:"name"`
	Model           string       `json:"model"`
	Manufacturer    string       `json:"manufacturer"`
	Host            string       `json:"host"`
	Port            int          `json:"port"`
	Channels        int          `json:"channels"`
	Capabilities    []Capability `json:"capabilities"`
	FirmwareVersion string       `json:"firmware_version,omitempty"`
	Serial          string       `json:"serial,omitempty"`
}

// PluginCamera represents an active camera managed by a plugin
type PluginCamera struct {
	ID           string        `json:"id"`
	PluginID     string        `json:"plugin_id"`
	Name         string        `json:"name"`
	Model        string        `json:"model"`
	Host         string        `json:"host"`
	MainStream   string        `json:"main_stream"`   // RTSP/RTMP URL
	SubStream    string        `json:"sub_stream"`    // Lower quality stream URL
	SnapshotURL  string        `json:"snapshot_url"`
	Capabilities []Capability  `json:"capabilities"`
	Online       bool          `json:"online"`
	LastSeen     time.Time     `json:"last_seen"`
	Config       CameraConfig  `json:"config"`
}

// CameraConfig holds configuration for a plugin camera
type CameraConfig struct {
	Host     string                 `json:"host" yaml:"host"`
	Port     int                    `json:"port,omitempty" yaml:"port,omitempty"`
	Username string                 `json:"username" yaml:"username"`
	Password string                 `json:"password" yaml:"password"`
	Channel  int                    `json:"channel,omitempty" yaml:"channel,omitempty"`
	Name     string                 `json:"name,omitempty" yaml:"name,omitempty"`
	Extra    map[string]interface{} `json:"extra,omitempty" yaml:"extra,omitempty"`
}

// CameraEvent represents an event from a camera
type CameraEvent struct {
	Type      CameraEventType        `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	CameraID  string                 `json:"camera_id"`
	PluginID  string                 `json:"plugin_id"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// CameraEventType represents types of camera events
type CameraEventType string

const (
	EventTypeMotion     CameraEventType = "motion"
	EventTypePerson     CameraEventType = "person"
	EventTypeVehicle    CameraEventType = "vehicle"
	EventTypeAnimal     CameraEventType = "animal"
	EventTypePackage    CameraEventType = "package"
	EventTypeFace       CameraEventType = "face"
	EventTypeRing       CameraEventType = "ring"
	EventTypeAudioAlert CameraEventType = "audio_alert"
	EventTypeLineCross  CameraEventType = "line_cross"
	EventTypeIntrusion  CameraEventType = "intrusion"
)

// PTZCommand represents a PTZ control command
type PTZCommand struct {
	Action    string  `json:"action"` // pan, tilt, zoom, stop, preset
	Direction float64 `json:"direction,omitempty"` // -1.0 to 1.0
	Speed     float64 `json:"speed,omitempty"` // 0.0 to 1.0
	Preset    string  `json:"preset,omitempty"` // preset name for preset action
}

// EventHandler is a callback for camera events
type EventHandler func(event CameraEvent)

// PluginRPCInterface defines the JSON-RPC methods that plugins must implement
// Plugins communicate via JSON-RPC 2.0 over stdin/stdout
type PluginRPCInterface interface {
	// Lifecycle
	Initialize(ctx context.Context, config map[string]interface{}) error
	Shutdown(ctx context.Context) error
	Health(ctx context.Context) (*HealthStatus, error)

	// Camera management
	DiscoverCameras(ctx context.Context) ([]DiscoveredCamera, error)
	AddCamera(ctx context.Context, config CameraConfig) (*PluginCamera, error)
	RemoveCamera(ctx context.Context, cameraID string) error
	ListCameras(ctx context.Context) ([]PluginCamera, error)
	GetCamera(ctx context.Context, cameraID string) (*PluginCamera, error)

	// Camera control (optional - based on capabilities)
	PTZControl(ctx context.Context, cameraID string, cmd PTZCommand) error
	GetSnapshot(ctx context.Context, cameraID string) ([]byte, error)
}
