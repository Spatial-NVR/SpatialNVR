package wyze

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Spatial-NVR/SpatialNVR/internal/plugin"
)

const (
	PluginID      = "wyze"
	PluginName    = "Wyze"
	PluginVersion = "1.0.0"
)

// Plugin implements the Wyze camera provider
type Plugin struct {
	cameras   map[string]*WyzeCamera
	client    *Client
	bridgeURL string // wyze-bridge URL for streams

	logger *slog.Logger

	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a new Wyze plugin instance
func New() *Plugin {
	return &Plugin{
		cameras: make(map[string]*WyzeCamera),
		logger:  slog.Default().With("plugin", PluginID),
	}
}

// Manifest returns plugin metadata
func (p *Plugin) Manifest() plugin.PluginManifest {
	return plugin.PluginManifest{
		ID:          PluginID,
		Name:        PluginName,
		Version:     PluginVersion,
		Description: "Wyze camera integration via cloud API and wyze-bridge",
		Author:      "NVR System",
		Runtime: plugin.PluginRuntime{
			Type: "builtin",
		},
		Capabilities: []plugin.Capability{
			plugin.CapabilityDiscovery,
			plugin.CapabilityVideo,
			plugin.CapabilityPTZ,
			plugin.CapabilityMotion,
			plugin.CapabilityDoorbell,
		},
		ConfigSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"email": map[string]interface{}{
					"type":        "string",
					"description": "Wyze account email",
				},
				"password": map[string]interface{}{
					"type":        "string",
					"description": "Wyze account password",
				},
				"api_key": map[string]interface{}{
					"type":        "string",
					"description": "Wyze API key from developer console",
				},
				"key_id": map[string]interface{}{
					"type":        "string",
					"description": "Wyze API key ID",
				},
				"bridge_url": map[string]interface{}{
					"type":        "string",
					"description": "wyze-bridge RTSP URL (e.g., rtsp://localhost:8554)",
				},
			},
			"required": []string{"email", "password", "api_key", "key_id"},
		},
	}
}

// Initialize prepares the plugin with configuration
func (p *Plugin) Initialize(ctx context.Context, cfg map[string]interface{}) error {
	if cfg == nil {
		return nil
	}

	email, _ := cfg["email"].(string)
	password, _ := cfg["password"].(string)
	apiKey, _ := cfg["api_key"].(string)
	keyID, _ := cfg["key_id"].(string)
	bridgeURL, _ := cfg["bridge_url"].(string)

	if email == "" || password == "" {
		return fmt.Errorf("email and password are required")
	}
	if apiKey == "" || keyID == "" {
		return fmt.Errorf("api_key and key_id are required (get from https://developer-api-console.wyze.com)")
	}

	p.client = NewClient(email, password, apiKey, keyID)
	p.bridgeURL = bridgeURL
	if p.bridgeURL == "" {
		p.bridgeURL = "rtsp://localhost:8554" // Default wyze-bridge URL
	}

	p.logger.Info("Plugin initialized", "email", email, "bridge_url", p.bridgeURL)
	return nil
}

// Start begins plugin operation
func (p *Plugin) Start(ctx context.Context) error {
	p.ctx, p.cancel = context.WithCancel(ctx)

	if p.client == nil {
		p.logger.Warn("No credentials configured, plugin inactive")
		return nil
	}

	// Login to Wyze
	loginCtx, cancel := context.WithTimeout(p.ctx, 30*time.Second)
	defer cancel()

	if err := p.client.Login(loginCtx); err != nil {
		return fmt.Errorf("failed to login to Wyze: %w", err)
	}

	// Discover cameras
	devices, err := p.client.GetDeviceList(loginCtx)
	if err != nil {
		return fmt.Errorf("failed to get device list: %w", err)
	}

	p.mu.Lock()
	for _, device := range devices {
		cam := NewWyzeCamera(device, p.bridgeURL, p.client)
		p.cameras[device.MAC] = cam
		p.logger.Info("Added camera", "id", device.MAC, "name", device.Name, "model", device.Model)
	}
	p.mu.Unlock()

	p.logger.Info("Plugin started", "cameras", len(p.cameras))
	return nil
}

// Stop shuts down the plugin
func (p *Plugin) Stop() error {
	if p.cancel != nil {
		p.cancel()
	}

	p.logger.Info("Plugin stopped")
	return nil
}

// Health returns the plugin health status
func (p *Plugin) Health() plugin.HealthStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	online := 0
	total := len(p.cameras)

	for _, cam := range p.cameras {
		if cam.IsOnline() {
			online++
		}
	}

	state := plugin.HealthStateHealthy
	msg := fmt.Sprintf("%d/%d cameras online", online, total)

	if p.client == nil {
		state = plugin.HealthStateUnknown
		msg = "No credentials configured"
	} else if total == 0 {
		state = plugin.HealthStateUnknown
		msg = "No cameras found"
	} else if online == 0 {
		state = plugin.HealthStateUnhealthy
	} else if online < total {
		state = plugin.HealthStateDegraded
	}

	return plugin.HealthStatus{
		State:     state,
		Message:   msg,
		LastCheck: time.Now(),
		Details: map[string]interface{}{
			"cameras_online": online,
			"cameras_total":  total,
			"bridge_url":     p.bridgeURL,
		},
	}
}

// OnConfigChange handles configuration changes
func (p *Plugin) OnConfigChange(cfg map[string]interface{}) {
	// For now, just log the change
	// Full hot-reload would require re-login
	p.logger.Info("Configuration updated")
}

// DiscoverCameras searches for Wyze cameras
func (p *Plugin) DiscoverCameras(ctx context.Context) ([]plugin.DiscoveredCamera, error) {
	if p.client == nil {
		return nil, fmt.Errorf("not logged in")
	}

	devices, err := p.client.GetDeviceList(ctx)
	if err != nil {
		return nil, err
	}

	var discovered []plugin.DiscoveredCamera
	for _, device := range devices {
		caps := make([]plugin.Capability, 0)
		for _, c := range device.Capabilities {
			switch c {
			case "video":
				caps = append(caps, plugin.CapabilityVideo)
			case "ptz":
				caps = append(caps, plugin.CapabilityPTZ)
			case "doorbell":
				caps = append(caps, plugin.CapabilityDoorbell)
			case "battery":
				caps = append(caps, plugin.CapabilityBattery)
			}
		}

		discovered = append(discovered, plugin.DiscoveredCamera{
			ID:              device.MAC,
			Name:            device.Name,
			Model:           device.Model,
			Manufacturer:    "Wyze",
			FirmwareVersion: device.FirmwareVer,
			Capabilities:    caps,
		})
	}

	return discovered, nil
}

// ListCameras returns all cameras
func (p *Plugin) ListCameras() []plugin.PluginCamera {
	p.mu.RLock()
	defer p.mu.RUnlock()

	cameras := make([]plugin.PluginCamera, 0, len(p.cameras))
	for _, cam := range p.cameras {
		cameras = append(cameras, plugin.PluginCamera{
			ID:           cam.ID(),
			PluginID:     PluginID,
			Name:         cam.Name(),
			Model:        cam.Model(),
			Host:         cam.Host(),
			MainStream:   cam.StreamURL(plugin.StreamQualityMain),
			SubStream:    cam.StreamURL(plugin.StreamQualitySub),
			SnapshotURL:  cam.SnapshotURL(),
			Capabilities: cam.Capabilities(),
			Online:       cam.IsOnline(),
			LastSeen:     cam.LastSeen(),
		})
	}
	return cameras
}

// GetCamera returns a camera by ID
func (p *Plugin) GetCamera(id string) *plugin.PluginCamera {
	p.mu.RLock()
	defer p.mu.RUnlock()

	cam, ok := p.cameras[id]
	if !ok {
		return nil
	}

	return &plugin.PluginCamera{
		ID:           cam.ID(),
		PluginID:     PluginID,
		Name:         cam.Name(),
		Model:        cam.Model(),
		Host:         cam.Host(),
		MainStream:   cam.StreamURL(plugin.StreamQualityMain),
		SubStream:    cam.StreamURL(plugin.StreamQualitySub),
		SnapshotURL:  cam.SnapshotURL(),
		Capabilities: cam.Capabilities(),
		Online:       cam.IsOnline(),
		LastSeen:     cam.LastSeen(),
	}
}

// AddCamera adds a new camera (not supported for Wyze - cameras discovered from cloud)
func (p *Plugin) AddCamera(ctx context.Context, cfg plugin.CameraConfig) (*plugin.PluginCamera, error) {
	return nil, fmt.Errorf("Wyze cameras are discovered automatically from your account")
}

// RemoveCamera removes a camera (not supported for Wyze)
func (p *Plugin) RemoveCamera(ctx context.Context, id string) error {
	return fmt.Errorf("Wyze cameras cannot be removed - they are synced from your account")
}

// PTZControl sends a PTZ command to a camera
func (p *Plugin) PTZControl(ctx context.Context, cameraID string, cmd plugin.PTZCommand) error {
	p.mu.RLock()
	cam, ok := p.cameras[cameraID]
	p.mu.RUnlock()

	if !ok {
		return fmt.Errorf("camera not found: %s", cameraID)
	}

	switch cmd.Action {
	case "pan":
		return cam.Pan(ctx, cmd.Direction, cmd.Speed)
	case "tilt":
		return cam.Tilt(ctx, cmd.Direction, cmd.Speed)
	case "zoom":
		return cam.Zoom(ctx, cmd.Direction, cmd.Speed)
	case "stop":
		return cam.Stop(ctx)
	case "preset":
		return cam.GoToPreset(ctx, cmd.Preset)
	default:
		return fmt.Errorf("unknown PTZ action: %s", cmd.Action)
	}
}
