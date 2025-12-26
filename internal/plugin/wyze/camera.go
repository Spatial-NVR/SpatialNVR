package wyze

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Spatial-NVR/SpatialNVR/internal/plugin"
)

// WyzeCamera represents a Wyze camera
type WyzeCamera struct {
	mac          string
	name         string
	model        string
	productType  string
	p2pID        string
	capabilities []plugin.Capability
	bridgeURL    string // wyze-bridge base URL
	client       *Client

	online    bool
	lastSeen  time.Time
	mu        sync.RWMutex

	// Event handling
	eventHandlers []plugin.EventHandler
	eventMu       sync.RWMutex
}

// NewWyzeCamera creates a new Wyze camera instance
func NewWyzeCamera(device WyzeDevice, bridgeURL string, client *Client) *WyzeCamera {
	// Convert string capabilities to plugin.Capability
	caps := make([]plugin.Capability, 0)
	for _, c := range device.Capabilities {
		switch c {
		case "video":
			caps = append(caps, plugin.CapabilityVideo)
		case "snapshot":
			caps = append(caps, plugin.CapabilitySnapshot)
		case "motion":
			caps = append(caps, plugin.CapabilityMotion)
		case "ptz":
			caps = append(caps, plugin.CapabilityPTZ)
		case "two_way_audio":
			caps = append(caps, plugin.CapabilityTwoWayAudio)
		case "doorbell":
			caps = append(caps, plugin.CapabilityDoorbell)
		case "battery":
			caps = append(caps, plugin.CapabilityBattery)
		case "floodlight":
			caps = append(caps, plugin.CapabilityFloodlight)
		case "siren":
			caps = append(caps, plugin.CapabilitySiren)
		case "night_vision":
			caps = append(caps, plugin.CapabilityNightVision)
		}
	}

	return &WyzeCamera{
		mac:          device.MAC,
		name:         device.Name,
		model:        device.Model,
		productType:  device.ProductType,
		p2pID:        device.P2PID,
		capabilities: caps,
		bridgeURL:    bridgeURL,
		client:       client,
		online:       device.IsOnline,
		lastSeen:     time.Now(),
	}
}

// ID returns the camera ID (MAC address)
func (c *WyzeCamera) ID() string {
	return c.mac
}

// Name returns the camera name
func (c *WyzeCamera) Name() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.name
}

// Model returns the camera model
func (c *WyzeCamera) Model() string {
	return c.model
}

// Host returns the bridge URL for this camera
func (c *WyzeCamera) Host() string {
	return c.bridgeURL
}

// StreamURL returns the stream URL for the given quality
// Uses wyze-bridge for streaming
func (c *WyzeCamera) StreamURL(quality plugin.StreamQuality) string {
	if c.bridgeURL == "" {
		return ""
	}

	// wyze-bridge stream format: rtsp://bridge:8554/camera-name
	// The camera name in wyze-bridge is typically the nickname with spaces replaced
	streamName := sanitizeName(c.name)

	// Parse bridge URL and construct RTSP URL
	bridgeHost := c.bridgeURL
	if strings.HasPrefix(bridgeHost, "rtsp://") {
		bridgeHost = strings.TrimPrefix(bridgeHost, "rtsp://")
	}
	if strings.HasPrefix(bridgeHost, "http://") {
		bridgeHost = strings.TrimPrefix(bridgeHost, "http://")
	}

	// Default port for wyze-bridge RTSP
	if !strings.Contains(bridgeHost, ":") {
		bridgeHost = bridgeHost + ":8554"
	}

	return fmt.Sprintf("rtsp://%s/%s", bridgeHost, streamName)
}

// sanitizeName converts a camera name to wyze-bridge format
func sanitizeName(name string) string {
	// wyze-bridge typically uses lowercase with hyphens
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "_", "-")
	// URL encode for safety
	return url.PathEscape(name)
}

// SnapshotURL returns a snapshot URL
func (c *WyzeCamera) SnapshotURL() string {
	if c.bridgeURL == "" {
		return ""
	}

	// wyze-bridge provides snapshots at /img/camera-name.jpg
	bridgeHost := c.bridgeURL
	if strings.HasPrefix(bridgeHost, "rtsp://") {
		// Convert to HTTP for snapshot
		bridgeHost = strings.TrimPrefix(bridgeHost, "rtsp://")
	}
	if !strings.HasPrefix(bridgeHost, "http://") && !strings.HasPrefix(bridgeHost, "https://") {
		bridgeHost = "http://" + bridgeHost
	}

	// Remove port if present and use HTTP port 5000 (default wyze-bridge web UI)
	if idx := strings.LastIndex(bridgeHost, ":"); idx > 0 {
		bridgeHost = bridgeHost[:idx]
	}

	streamName := sanitizeName(c.name)
	return fmt.Sprintf("%s:5000/img/%s.jpg", bridgeHost, streamName)
}

// Capabilities returns camera capabilities
func (c *WyzeCamera) Capabilities() []plugin.Capability {
	c.mu.RLock()
	defer c.mu.RUnlock()
	caps := make([]plugin.Capability, len(c.capabilities))
	copy(caps, c.capabilities)
	return caps
}

// IsOnline returns whether the camera is online
func (c *WyzeCamera) IsOnline() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.online
}

// LastSeen returns when the camera was last seen online
func (c *WyzeCamera) LastSeen() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastSeen
}

// SetOnline updates the online status
func (c *WyzeCamera) SetOnline(online bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.online = online
	if online {
		c.lastSeen = time.Now()
	}
}

// HasCapability checks if the camera has a specific capability
func (c *WyzeCamera) HasCapability(cap plugin.Capability) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, cc := range c.capabilities {
		if cc == cap {
			return true
		}
	}
	return false
}

// PTZ Methods (for PTZ-capable cameras like Cam Pan)

// Pan pans the camera horizontally
func (c *WyzeCamera) Pan(ctx context.Context, direction float64, speed float64) error {
	if !c.HasCapability(plugin.CapabilityPTZ) {
		return fmt.Errorf("camera does not support PTZ")
	}

	// Wyze PTZ uses action commands
	actionKey := "P1001" // Rotate action
	var action string
	if direction < -0.1 {
		action = "left"
	} else if direction > 0.1 {
		action = "right"
	} else {
		return nil // No movement
	}

	return c.client.RunAction(ctx, c.mac, c.model, actionKey, map[string]interface{}{
		"direction": action,
		"speed":     int(speed * 100),
	})
}

// Tilt tilts the camera vertically
func (c *WyzeCamera) Tilt(ctx context.Context, direction float64, speed float64) error {
	if !c.HasCapability(plugin.CapabilityPTZ) {
		return fmt.Errorf("camera does not support PTZ")
	}

	actionKey := "P1001"
	var action string
	if direction < -0.1 {
		action = "down"
	} else if direction > 0.1 {
		action = "up"
	} else {
		return nil
	}

	return c.client.RunAction(ctx, c.mac, c.model, actionKey, map[string]interface{}{
		"direction": action,
		"speed":     int(speed * 100),
	})
}

// Zoom - Wyze cameras don't have optical zoom
func (c *WyzeCamera) Zoom(ctx context.Context, direction float64, speed float64) error {
	return fmt.Errorf("zoom not supported on Wyze cameras")
}

// Stop stops PTZ movement
func (c *WyzeCamera) Stop(ctx context.Context) error {
	if !c.HasCapability(plugin.CapabilityPTZ) {
		return nil
	}

	return c.client.RunAction(ctx, c.mac, c.model, "P1001", map[string]interface{}{
		"direction": "stop",
	})
}

// GoToPreset moves to a waypoint
func (c *WyzeCamera) GoToPreset(ctx context.Context, preset string) error {
	if !c.HasCapability(plugin.CapabilityPTZ) {
		return fmt.Errorf("camera does not support PTZ")
	}

	return c.client.RunAction(ctx, c.mac, c.model, "P1009", map[string]interface{}{
		"waypoint": preset,
	})
}

// SavePreset saves a waypoint
func (c *WyzeCamera) SavePreset(ctx context.Context, preset string) error {
	return fmt.Errorf("save preset not implemented for Wyze cameras")
}

// ListPresets returns preset names
func (c *WyzeCamera) ListPresets(ctx context.Context) ([]string, error) {
	// Wyze cameras have 4 preset waypoints
	return []string{"1", "2", "3", "4"}, nil
}

// Event Methods

// Subscribe registers an event handler
func (c *WyzeCamera) Subscribe(handler plugin.EventHandler) func() {
	c.eventMu.Lock()
	c.eventHandlers = append(c.eventHandlers, handler)
	index := len(c.eventHandlers) - 1
	c.eventMu.Unlock()

	return func() {
		c.eventMu.Lock()
		defer c.eventMu.Unlock()
		if index < len(c.eventHandlers) {
			c.eventHandlers = append(c.eventHandlers[:index], c.eventHandlers[index+1:]...)
		}
	}
}

// StartEventPolling starts polling for events
// Note: For Wyze, events come from the cloud, not direct polling
func (c *WyzeCamera) StartEventPolling(ctx context.Context) error {
	// Wyze event polling would require webhook integration or cloud API polling
	// For now, this is a no-op - events would need to come from wyze-bridge
	return nil
}

// StopEventPolling stops event polling
func (c *WyzeCamera) StopEventPolling() error {
	return nil
}

// emitEvent sends an event to all handlers
func (c *WyzeCamera) emitEvent(event plugin.CameraEvent) {
	c.eventMu.RLock()
	handlers := make([]plugin.EventHandler, len(c.eventHandlers))
	copy(handlers, c.eventHandlers)
	c.eventMu.RUnlock()

	for _, handler := range handlers {
		handler(event)
	}
}

// Siren Methods (for Floodlight cameras)

// TriggerSiren activates the siren
func (c *WyzeCamera) TriggerSiren(ctx context.Context, duration time.Duration) error {
	if !c.HasCapability(plugin.CapabilitySiren) {
		return fmt.Errorf("camera does not have a siren")
	}

	return c.client.RunAction(ctx, c.mac, c.model, "P1003", map[string]interface{}{
		"siren":    1,
		"duration": int(duration.Seconds()),
	})
}

// StopSiren stops the siren
func (c *WyzeCamera) StopSiren(ctx context.Context) error {
	if !c.HasCapability(plugin.CapabilitySiren) {
		return nil
	}

	return c.client.RunAction(ctx, c.mac, c.model, "P1003", map[string]interface{}{
		"siren": 0,
	})
}

// IsSirenActive returns whether siren is active
func (c *WyzeCamera) IsSirenActive() bool {
	// Would need to query device state
	return false
}

// Floodlight Methods

// SetLight turns the floodlight on or off
func (c *WyzeCamera) SetLight(ctx context.Context, on bool) error {
	if !c.HasCapability(plugin.CapabilityFloodlight) {
		return fmt.Errorf("camera does not have a floodlight")
	}

	value := 0
	if on {
		value = 1
	}

	return c.client.RunAction(ctx, c.mac, c.model, "P1004", map[string]interface{}{
		"floodlight": value,
	})
}

// SetBrightness sets floodlight brightness
func (c *WyzeCamera) SetBrightness(ctx context.Context, percent int) error {
	if !c.HasCapability(plugin.CapabilityFloodlight) {
		return fmt.Errorf("camera does not have a floodlight")
	}

	return c.client.RunAction(ctx, c.mac, c.model, "P1004", map[string]interface{}{
		"brightness": percent,
	})
}

// IsLightOn returns whether floodlight is on
func (c *WyzeCamera) IsLightOn() bool {
	return false
}

// GetBrightness returns current brightness
func (c *WyzeCamera) GetBrightness() int {
	return 0
}
