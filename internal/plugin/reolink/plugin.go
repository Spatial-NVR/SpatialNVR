package reolink

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Spatial-NVR/SpatialNVR/internal/plugin"
)

const (
	PluginID      = "reolink"
	PluginName    = "Reolink"
	PluginVersion = "1.0.0"
)

// Plugin implements the Reolink camera provider
type Plugin struct {
	cameras map[string]*ReolinkCamera
	devices []DeviceConfig

	logger *slog.Logger

	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

// DeviceConfig holds configuration for a Reolink device
type DeviceConfig struct {
	Host     string `yaml:"host" json:"host"`
	Port     int    `yaml:"port,omitempty" json:"port,omitempty"`
	Username string `yaml:"username" json:"username"`
	Password string `yaml:"password" json:"password"`
	Channels []int  `yaml:"channels,omitempty" json:"channels,omitempty"` // For NVRs
	Name     string `yaml:"name,omitempty" json:"name,omitempty"`         // Custom name
}

// New creates a new Reolink plugin instance
func New() *Plugin {
	return &Plugin{
		cameras: make(map[string]*ReolinkCamera),
		logger:  slog.Default().With("plugin", PluginID),
	}
}

// Manifest returns plugin metadata
func (p *Plugin) Manifest() plugin.PluginManifest {
	return plugin.PluginManifest{
		ID:          PluginID,
		Name:        PluginName,
		Version:     PluginVersion,
		Description: "Reolink camera and NVR integration via HTTP API",
		Author:      "NVR System",
		Runtime: plugin.PluginRuntime{
			Type: "builtin",
		},
		Capabilities: []plugin.Capability{
			plugin.CapabilityDiscovery,
			plugin.CapabilityNVR,
			plugin.CapabilityVideo,
			plugin.CapabilityPTZ,
			plugin.CapabilityMotion,
			plugin.CapabilityAIDetection,
		},
	}
}

// Initialize prepares the plugin with configuration
func (p *Plugin) Initialize(ctx context.Context, cfg map[string]interface{}) error {
	// Parse device configurations
	if err := p.parseConfig(cfg); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	p.logger.Info("Plugin initialized", "devices", len(p.devices))
	return nil
}

// parseConfig extracts device configurations from plugin config
func (p *Plugin) parseConfig(cfg map[string]interface{}) error {
	p.devices = nil

	if cfg == nil {
		return nil
	}

	// Look for "devices" array in config
	devicesRaw, ok := cfg["devices"]
	if !ok {
		// Try single device config
		if host, ok := cfg["host"].(string); ok {
			device := DeviceConfig{Host: host}
			if port, ok := cfg["port"].(float64); ok {
				device.Port = int(port)
			}
			if user, ok := cfg["username"].(string); ok {
				device.Username = user
			}
			if pass, ok := cfg["password"].(string); ok {
				device.Password = pass
			}
			if name, ok := cfg["name"].(string); ok {
				device.Name = name
			}
			p.devices = append(p.devices, device)
		}
		return nil
	}

	// Parse devices array
	devicesList, ok := devicesRaw.([]interface{})
	if !ok {
		return fmt.Errorf("devices must be an array")
	}

	for _, d := range devicesList {
		deviceMap, ok := d.(map[string]interface{})
		if !ok {
			continue
		}

		device := DeviceConfig{}
		if host, ok := deviceMap["host"].(string); ok {
			device.Host = host
		}
		if port, ok := deviceMap["port"].(float64); ok {
			device.Port = int(port)
		}
		if user, ok := deviceMap["username"].(string); ok {
			device.Username = user
		}
		if pass, ok := deviceMap["password"].(string); ok {
			device.Password = pass
		}
		if name, ok := deviceMap["name"].(string); ok {
			device.Name = name
		}
		if channels, ok := deviceMap["channels"].([]interface{}); ok {
			for _, ch := range channels {
				if chNum, ok := ch.(float64); ok {
					device.Channels = append(device.Channels, int(chNum))
				}
			}
		}

		if device.Host != "" {
			p.devices = append(p.devices, device)
		}
	}

	return nil
}

// Start begins plugin operation
func (p *Plugin) Start(ctx context.Context) error {
	p.ctx, p.cancel = context.WithCancel(ctx)

	// Connect to configured devices
	for _, device := range p.devices {
		if err := p.connectDevice(device); err != nil {
			p.logger.Error("Failed to connect to device", "host", device.Host, "error", err)
		}
	}

	// Start event polling for all cameras
	p.mu.RLock()
	for _, cam := range p.cameras {
		if err := cam.StartEventPolling(p.ctx); err != nil {
			p.logger.Warn("Failed to start event polling", "camera", cam.ID(), "error", err)
		}
	}
	p.mu.RUnlock()

	p.logger.Info("Plugin started", "cameras", len(p.cameras))
	return nil
}

// connectDevice connects to a Reolink device and discovers its cameras
func (p *Plugin) connectDevice(device DeviceConfig) error {
	client := NewClient(device.Host, device.Port, device.Username, device.Password)

	// Login
	ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
	defer cancel()

	if err := client.Login(ctx); err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	// Get device info
	info, err := client.GetDeviceInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to get device info: %w", err)
	}

	p.logger.Info("Connected to device",
		"host", device.Host,
		"model", info.Model,
		"name", info.Name,
		"channels", info.ChannelCount)

	// Get abilities
	ability, _ := client.GetAbility(ctx, 0)

	// Determine which channels to use
	channels := device.Channels
	if len(channels) == 0 {
		// Use all available channels
		for i := 0; i < info.ChannelCount; i++ {
			channels = append(channels, i)
		}
	}

	// Create camera for each channel
	for _, ch := range channels {
		cameraID := fmt.Sprintf("%s_ch%d", device.Host, ch)
		cameraName := info.Name
		if device.Name != "" {
			cameraName = device.Name
		}
		if info.ChannelCount > 1 {
			cameraName = fmt.Sprintf("%s Ch%d", cameraName, ch+1)
		}

		cam := NewReolinkCamera(cameraID, cameraName, info.Model, device.Host, ch, client)
		if ability != nil {
			cam.SetAbility(ability)
		}

		// Get encoder config
		if encCfg, err := client.GetEncoderConfig(ctx, ch); err == nil {
			cam.SetEncoderConfig(encCfg)
		}

		p.mu.Lock()
		p.cameras[cameraID] = cam
		p.mu.Unlock()

		p.logger.Info("Added camera", "id", cameraID, "name", cameraName)
	}

	return nil
}

// Stop shuts down the plugin
func (p *Plugin) Stop() error {
	if p.cancel != nil {
		p.cancel()
	}

	// Stop all camera event polling
	p.mu.RLock()
	for _, cam := range p.cameras {
		_ = cam.StopEventPolling()
	}
	p.mu.RUnlock()

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

	if total == 0 {
		state = plugin.HealthStateUnknown
		msg = "No cameras configured"
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
		},
	}
}

// OnConfigChange handles configuration changes
func (p *Plugin) OnConfigChange(cfg map[string]interface{}) {
	if err := p.parseConfig(cfg); err != nil {
		p.logger.Error("Failed to parse new config", "error", err)
		return
	}

	// TODO: Implement hot-reload of cameras
	p.logger.Info("Configuration updated", "devices", len(p.devices))
}

// DiscoverCameras searches for Reolink cameras
func (p *Plugin) DiscoverCameras(ctx context.Context) ([]plugin.DiscoveredCamera, error) {
	// Return already connected cameras as "discovered"
	p.mu.RLock()
	defer p.mu.RUnlock()

	var discovered []plugin.DiscoveredCamera
	for _, cam := range p.cameras {
		discovered = append(discovered, plugin.DiscoveredCamera{
			ID:           cam.ID(),
			Name:         cam.Name(),
			Model:        cam.Model(),
			Manufacturer: "Reolink",
			Host:         cam.Host(),
			Capabilities: cam.Capabilities(),
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

// AddCamera adds a new camera
func (p *Plugin) AddCamera(ctx context.Context, cfg plugin.CameraConfig) (*plugin.PluginCamera, error) {
	device := DeviceConfig{
		Host:     cfg.Host,
		Port:     cfg.Port,
		Username: cfg.Username,
		Password: cfg.Password,
		Name:     cfg.Name,
	}

	if cfg.Channel > 0 {
		device.Channels = []int{cfg.Channel}
	}

	if err := p.connectDevice(device); err != nil {
		return nil, err
	}

	// Return the first camera that was added
	cameraID := fmt.Sprintf("%s_ch%d", cfg.Host, cfg.Channel)
	return p.GetCamera(cameraID), nil
}

// RemoveCamera removes a camera
func (p *Plugin) RemoveCamera(ctx context.Context, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	cam, ok := p.cameras[id]
	if !ok {
		return fmt.Errorf("camera not found: %s", id)
	}

	_ = cam.StopEventPolling()
	delete(p.cameras, id)

	p.logger.Info("Removed camera", "id", id)
	return nil
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
