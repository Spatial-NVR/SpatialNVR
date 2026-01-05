// Package sdk provides the Plugin SDK for developing NVR plugins.
// All plugins (core and third-party) use this SDK to integrate with the NVR system.
package sdk

import (
	"context"
	"net/http"
	"time"
)

// PluginManifest describes a plugin's metadata and capabilities
type PluginManifest struct {
	// Required fields
	ID          string `json:"id" yaml:"id"`
	Name        string `json:"name" yaml:"name"`
	Version     string `json:"version" yaml:"version"`
	Description string `json:"description" yaml:"description"`

	// Plugin classification
	Category string `json:"category" yaml:"category"` // "core", "camera", "integration", "detection"
	Critical bool   `json:"critical" yaml:"critical"` // If true, NVR won't start without this plugin

	// Runtime configuration
	Runtime RuntimeConfig `json:"runtime" yaml:"runtime"`

	// Dependencies on other plugins
	Dependencies []string `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`

	// Capabilities this plugin provides
	Capabilities []string `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`

	// Configuration schema (JSON Schema)
	ConfigSchema map[string]interface{} `json:"config_schema,omitempty" yaml:"config_schema,omitempty"`

	// Author information
	Author    string `json:"author,omitempty" yaml:"author,omitempty"`
	Homepage  string `json:"homepage,omitempty" yaml:"homepage,omitempty"`
	Repository string `json:"repository,omitempty" yaml:"repository,omitempty"`
}

// RuntimeConfig specifies how to run the plugin
type RuntimeConfig struct {
	Type       string   `json:"type" yaml:"type"`                         // "go", "python", "node", "binary"
	EntryPoint string   `json:"entry_point" yaml:"entry_point"`           // Main file or binary (legacy)
	Script     string   `json:"script,omitempty" yaml:"script,omitempty"` // Python/Node script file
	Binary     string   `json:"binary,omitempty" yaml:"binary,omitempty"` // Binary file name
	Setup      string   `json:"setup,omitempty" yaml:"setup,omitempty"`   // Setup script (e.g., setup.sh)
	Args       []string `json:"args,omitempty" yaml:"args,omitempty"`
}

// Plugin is the base interface all plugins must implement
type Plugin interface {
	// Manifest returns the plugin's metadata
	Manifest() PluginManifest

	// Initialize prepares the plugin with its runtime environment
	Initialize(ctx context.Context, runtime *PluginRuntime) error

	// Start begins plugin operation
	Start(ctx context.Context) error

	// Stop gracefully shuts down the plugin
	Stop(ctx context.Context) error

	// Health returns the plugin's current health status
	Health() HealthStatus

	// Routes returns HTTP routes this plugin provides
	// Routes are mounted at /api/v1/plugins/{plugin-id}/
	Routes() http.Handler
}

// ServicePlugin is for plugins that provide NVR services
type ServicePlugin interface {
	Plugin

	// EventSubscriptions returns event types this plugin wants to receive
	EventSubscriptions() []string

	// HandleEvent processes an event from the event bus
	HandleEvent(ctx context.Context, event *Event) error

	// OnConfigChange is called when plugin configuration changes
	OnConfigChange(config map[string]interface{})
}

// CameraPlugin is for plugins that provide camera integrations
type CameraPlugin interface {
	Plugin

	// DiscoverCameras searches for available cameras
	DiscoverCameras(ctx context.Context) ([]DiscoveredCamera, error)

	// ListCameras returns all cameras managed by this plugin
	ListCameras() []Camera

	// GetCamera returns a specific camera
	GetCamera(id string) *Camera

	// AddCamera adds a camera with the given configuration
	AddCamera(ctx context.Context, config CameraConfig) (*Camera, error)

	// RemoveCamera removes a camera
	RemoveCamera(ctx context.Context, id string) error

	// PTZControl sends a PTZ command to a camera
	PTZControl(ctx context.Context, cameraID string, cmd PTZCommand) error
}

// HealthStatus represents plugin health
type HealthStatus struct {
	State       HealthState       `json:"state"`
	Message     string            `json:"message,omitempty"`
	LastChecked time.Time         `json:"last_checked"`
	Details     map[string]string `json:"details,omitempty"`
}

// HealthState represents health states
type HealthState string

const (
	HealthStateHealthy   HealthState = "healthy"
	HealthStateDegraded  HealthState = "degraded"
	HealthStateUnhealthy HealthState = "unhealthy"
	HealthStateUnknown   HealthState = "unknown"
)

// Event represents an NVR event
type Event struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	CameraID    string                 `json:"camera_id,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
	Data        map[string]interface{} `json:"data,omitempty"`
	ObjectType  string                 `json:"object_type,omitempty"`
	Confidence  float64                `json:"confidence,omitempty"`
	BoundingBox *BoundingBox           `json:"bounding_box,omitempty"`
	TrackID     string                 `json:"track_id,omitempty"`
	Duration    time.Duration          `json:"duration,omitempty"`
}

// BoundingBox represents a detection bounding box
type BoundingBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Camera represents a camera managed by a plugin
type Camera struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	PluginID    string            `json:"plugin_id"`
	Vendor      string            `json:"vendor,omitempty"`
	Model       string            `json:"model,omitempty"`
	MainStream  string            `json:"main_stream"`
	SubStream   string            `json:"sub_stream,omitempty"`
	SnapshotURL string            `json:"snapshot_url,omitempty"`
	Status      string            `json:"status"`
	HasPTZ      bool              `json:"has_ptz"`
	HasAudio    bool              `json:"has_audio"`
	Channels    int               `json:"channels"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// CameraConfig is used to add a camera
type CameraConfig struct {
	Host     string                 `json:"host"`
	Port     int                    `json:"port"`
	Username string                 `json:"username"`
	Password string                 `json:"password"`
	Name     string                 `json:"name,omitempty"`
	Channel  int                    `json:"channel,omitempty"`
	Options  map[string]interface{} `json:"options,omitempty"`
}

// DiscoveredCamera represents a camera found during discovery
type DiscoveredCamera struct {
	Host       string            `json:"host"`
	Port       int               `json:"port"`
	Vendor     string            `json:"vendor"`
	Model      string            `json:"model"`
	Name       string            `json:"name"`
	Channels   int               `json:"channels"`
	HasPTZ     bool              `json:"has_ptz"`
	HasAudio   bool              `json:"has_audio"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// PTZCommand represents a PTZ control command
type PTZCommand struct {
	Action   string  `json:"action"` // "move", "stop", "preset", "zoom"
	Pan      float64 `json:"pan,omitempty"`
	Tilt     float64 `json:"tilt,omitempty"`
	Zoom     float64 `json:"zoom,omitempty"`
	Speed    float64 `json:"speed,omitempty"`
	PresetID int     `json:"preset_id,omitempty"`
}

// Zone represents a detection zone
type Zone struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Points   []Point    `json:"points"`
	Types    []string   `json:"types,omitempty"` // Object types to detect in this zone
	MinArea  float64    `json:"min_area,omitempty"`
}

// Point represents a 2D point
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Standard event types
const (
	EventTypePluginStarted   = "plugin.started"
	EventTypePluginStopped   = "plugin.stopped"
	EventTypePluginError     = "plugin.error"
	EventTypeCameraAdded     = "camera.added"
	EventTypeCameraRemoved   = "camera.removed"
	EventTypeCameraUpdated   = "camera.updated"
	EventTypeCameraOnline    = "camera.online"
	EventTypeCameraOffline   = "camera.offline"
	EventTypeDetection       = "detection.detected"
	EventTypeDetectionEnded  = "detection.ended"
	EventTypeRecordingStart  = "recording.started"
	EventTypeRecordingStop   = "recording.stopped"
	EventTypeConfigChanged   = "config.changed"
	EventTypeMotion          = "motion.detected"
	EventTypeMotionEnded     = "motion.ended"
)

// Standard capabilities
const (
	CapabilityCamera          = "camera"
	CapabilityConfig          = "config"
	CapabilityStreaming       = "streaming"
	CapabilityRecording       = "recording"
	CapabilityPlayback        = "playback"
	CapabilityDetection       = "detection"
	CapabilityFaceRecognition = "face_recognition"
	CapabilityLPR             = "lpr"
	CapabilityPTZ             = "ptz"
	CapabilityAudio           = "audio"
	CapabilityTwoWayAudio     = "two_way_audio"
	CapabilityMotion          = "motion"
	CapabilityEvents          = "events"
	CapabilityTimeline        = "timeline"
	CapabilityExport          = "export"
	CapabilitySettings        = "settings" // Plugin provides declarative settings UI
)

// SettingType defines the type of a setting input
type SettingType string

const (
	SettingTypeString    SettingType = "string"
	SettingTypeNumber    SettingType = "number"
	SettingTypeInteger   SettingType = "integer"
	SettingTypeBoolean   SettingType = "boolean"
	SettingTypePassword  SettingType = "password"
	SettingTypeTextarea  SettingType = "textarea"
	SettingTypeButton    SettingType = "button"
	SettingTypeDevice    SettingType = "device"    // Device/camera picker
	SettingTypeInterface SettingType = "interface" // Interface picker
	SettingTypeClippath  SettingType = "clippath"  // Zone/polygon editor
	SettingTypeTime      SettingType = "time"
	SettingTypeDate      SettingType = "date"
	SettingTypeDatetime  SettingType = "datetime"
)

// Setting represents a single configuration setting that plugins expose
// The NVR renders these declaratively - plugins describe their UI, they don't provide it
type Setting struct {
	// Key is the unique identifier for this setting
	Key string `json:"key"`

	// Title is the display name shown to users
	Title string `json:"title"`

	// Description provides additional context
	Description string `json:"description,omitempty"`

	// Type determines the input component rendered
	Type SettingType `json:"type"`

	// Group organizes settings into sections
	Group string `json:"group,omitempty"`

	// Subgroup for further organization within a group
	Subgroup string `json:"subgroup,omitempty"`

	// Value is the current setting value
	Value interface{} `json:"value,omitempty"`

	// Placeholder text for input fields
	Placeholder string `json:"placeholder,omitempty"`

	// Choices for dropdown/select inputs
	Choices []SettingChoice `json:"choices,omitempty"`

	// Multiple allows selecting multiple choices
	Multiple bool `json:"multiple,omitempty"`

	// Readonly prevents user modification
	Readonly bool `json:"readonly,omitempty"`

	// Range for number inputs [min, max]
	Range []float64 `json:"range,omitempty"`

	// DeviceFilter filters device picker by interface/capability
	DeviceFilter string `json:"device_filter,omitempty"`

	// Immediate causes value to be applied without save button
	Immediate bool `json:"immediate,omitempty"`

	// Combobox allows typing custom values in addition to choices
	Combobox bool `json:"combobox,omitempty"`
}

// SettingChoice represents an option in a dropdown/select
type SettingChoice struct {
	Title string      `json:"title"`
	Value interface{} `json:"value"`
}

// SettingsProvider is implemented by plugins that expose declarative settings
// This enables a Scrypted-style architecture where plugins describe their UI
// and the NVR renders it generically - no plugin-specific UI code needed
type SettingsProvider interface {
	// GetSettings returns the current settings configuration
	GetSettings(ctx context.Context) ([]Setting, error)

	// PutSetting updates a single setting value
	// For button types, this triggers the action
	PutSetting(ctx context.Context, key string, value interface{}) error
}
