package sdk

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// BasePlugin provides common functionality for all plugins
// Embed this in your plugin struct to get default implementations
type BasePlugin struct {
	runtime  *PluginRuntime
	manifest PluginManifest
	health   HealthStatus
	healthMu sync.RWMutex
}

// SetManifest sets the plugin manifest (call this in your plugin's init)
func (p *BasePlugin) SetManifest(m PluginManifest) {
	p.manifest = m
}

// Manifest returns the plugin's manifest
func (p *BasePlugin) Manifest() PluginManifest {
	return p.manifest
}

// Initialize stores the runtime
func (p *BasePlugin) Initialize(ctx context.Context, runtime *PluginRuntime) error {
	p.runtime = runtime
	p.SetHealthy("Plugin initialized")
	return nil
}

// Start is a no-op (override in your plugin)
func (p *BasePlugin) Start(ctx context.Context) error {
	p.SetHealthy("Plugin started")
	return nil
}

// Stop is a no-op (override in your plugin)
func (p *BasePlugin) Stop(ctx context.Context) error {
	p.SetHealth(HealthStateUnknown, "Plugin stopped")
	return nil
}

// Health returns the current health status
func (p *BasePlugin) Health() HealthStatus {
	p.healthMu.RLock()
	defer p.healthMu.RUnlock()
	return p.health
}

// SetHealth updates the health status
func (p *BasePlugin) SetHealth(state HealthState, message string) {
	p.healthMu.Lock()
	defer p.healthMu.Unlock()
	p.health = HealthStatus{
		State:       state,
		Message:     message,
		LastChecked: time.Now(),
	}
}

// SetHealthy sets healthy state with a message
func (p *BasePlugin) SetHealthy(message string) {
	p.SetHealth(HealthStateHealthy, message)
}

// SetUnhealthy sets unhealthy state with a message
func (p *BasePlugin) SetUnhealthy(message string) {
	p.SetHealth(HealthStateUnhealthy, message)
}

// SetDegraded sets degraded state with a message
func (p *BasePlugin) SetDegraded(message string) {
	p.SetHealth(HealthStateDegraded, message)
}

// Routes returns nil (override in your plugin to provide routes)
func (p *BasePlugin) Routes() http.Handler {
	return nil
}

// Runtime returns the plugin runtime
func (p *BasePlugin) Runtime() *PluginRuntime {
	return p.runtime
}

// Logger returns the plugin's logger
func (p *BasePlugin) Logger() interface{} {
	if p.runtime != nil {
		return p.runtime.Logger()
	}
	return nil
}

// PublishEvent publishes an event
func (p *BasePlugin) PublishEvent(eventType string, data interface{}) error {
	if p.runtime == nil {
		return nil
	}
	return p.runtime.PublishEvent(eventType, data)
}

// SubscribeEvents subscribes to events
func (p *BasePlugin) SubscribeEvents(handler func(*Event), eventTypes ...string) error {
	if p.runtime == nil {
		return nil
	}
	return p.runtime.SubscribeEvents(handler, eventTypes...)
}

// Config returns the plugin's configuration
func (p *BasePlugin) Config() map[string]interface{} {
	if p.runtime == nil {
		return nil
	}
	return p.runtime.Config()
}

// ConfigString returns a string config value
func (p *BasePlugin) ConfigString(key string, defaultVal string) string {
	if p.runtime == nil {
		return defaultVal
	}
	return p.runtime.ConfigString(key, defaultVal)
}

// ConfigInt returns an int config value
func (p *BasePlugin) ConfigInt(key string, defaultVal int) int {
	if p.runtime == nil {
		return defaultVal
	}
	return p.runtime.ConfigInt(key, defaultVal)
}

// ConfigFloat returns a float config value
func (p *BasePlugin) ConfigFloat(key string, defaultVal float64) float64 {
	if p.runtime == nil {
		return defaultVal
	}
	return p.runtime.ConfigFloat(key, defaultVal)
}

// ConfigBool returns a bool config value
func (p *BasePlugin) ConfigBool(key string, defaultVal bool) bool {
	if p.runtime == nil {
		return defaultVal
	}
	return p.runtime.ConfigBool(key, defaultVal)
}

// BaseServicePlugin provides additional functionality for service plugins
type BaseServicePlugin struct {
	BasePlugin
	eventHandler func(*Event)
}

// EventSubscriptions returns empty by default
func (p *BaseServicePlugin) EventSubscriptions() []string {
	return nil
}

// HandleEvent processes an event (override in your plugin)
func (p *BaseServicePlugin) HandleEvent(ctx context.Context, event *Event) error {
	if p.eventHandler != nil {
		p.eventHandler(event)
	}
	return nil
}

// SetEventHandler sets the event handler function
func (p *BaseServicePlugin) SetEventHandler(handler func(*Event)) {
	p.eventHandler = handler
}

// OnConfigChange handles config changes (override in your plugin)
func (p *BaseServicePlugin) OnConfigChange(config map[string]interface{}) {
	// Override in your plugin to handle config changes
}

// BaseCameraPlugin provides additional functionality for camera plugins
type BaseCameraPlugin struct {
	BasePlugin
	cameras   map[string]*Camera
	camerasMu sync.RWMutex
}

// InitCameras initializes the cameras map
func (p *BaseCameraPlugin) InitCameras() {
	p.cameras = make(map[string]*Camera)
}

// DiscoverCameras returns empty by default (override in your plugin)
func (p *BaseCameraPlugin) DiscoverCameras(ctx context.Context) ([]DiscoveredCamera, error) {
	return nil, nil
}

// ListCameras returns all cameras
func (p *BaseCameraPlugin) ListCameras() []Camera {
	p.camerasMu.RLock()
	defer p.camerasMu.RUnlock()

	result := make([]Camera, 0, len(p.cameras))
	for _, cam := range p.cameras {
		result = append(result, *cam)
	}
	return result
}

// GetCamera returns a specific camera
func (p *BaseCameraPlugin) GetCamera(id string) *Camera {
	p.camerasMu.RLock()
	defer p.camerasMu.RUnlock()
	return p.cameras[id]
}

// AddCameraToRegistry adds a camera to the registry
func (p *BaseCameraPlugin) AddCameraToRegistry(cam *Camera) {
	p.camerasMu.Lock()
	defer p.camerasMu.Unlock()
	p.cameras[cam.ID] = cam
}

// RemoveCameraFromRegistry removes a camera from the registry
func (p *BaseCameraPlugin) RemoveCameraFromRegistry(id string) {
	p.camerasMu.Lock()
	defer p.camerasMu.Unlock()
	delete(p.cameras, id)
}

// AddCamera adds a camera (override in your plugin)
func (p *BaseCameraPlugin) AddCamera(ctx context.Context, config CameraConfig) (*Camera, error) {
	return nil, nil
}

// RemoveCamera removes a camera (override in your plugin)
func (p *BaseCameraPlugin) RemoveCamera(ctx context.Context, id string) error {
	return nil
}

// PTZControl sends a PTZ command (override in your plugin)
func (p *BaseCameraPlugin) PTZControl(ctx context.Context, cameraID string, cmd PTZCommand) error {
	return nil
}
