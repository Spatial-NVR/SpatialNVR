package reolink

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Spatial-NVR/SpatialNVR/internal/plugin"
)

// ReolinkCamera represents a Reolink camera
type ReolinkCamera struct {
	id           string
	name         string
	model        string
	host         string
	channel      int
	client       *Client
	capabilities []plugin.Capability
	ability      *Ability
	encoderCfg   *EncoderConfig

	online    bool
	lastSeen  time.Time
	mu        sync.RWMutex

	// Event handling
	eventHandlers []plugin.EventHandler
	eventMu       sync.RWMutex
	eventCancel   context.CancelFunc
	eventWg       sync.WaitGroup

	// Last event states for change detection
	lastMotion   bool
	lastAIState  *AIState
}

// NewReolinkCamera creates a new Reolink camera instance
func NewReolinkCamera(id, name, model, host string, channel int, client *Client) *ReolinkCamera {
	return &ReolinkCamera{
		id:           id,
		name:         name,
		model:        model,
		host:         host,
		channel:      channel,
		client:       client,
		capabilities: []plugin.Capability{plugin.CapabilityVideo},
		online:       true,
		lastSeen:     time.Now(),
	}
}

// SetAbility updates camera capabilities based on device abilities
func (c *ReolinkCamera) SetAbility(ability *Ability) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ability = ability
	c.capabilities = []plugin.Capability{plugin.CapabilityVideo, plugin.CapabilitySnapshot}

	if ability.PTZ || ability.PanTilt {
		c.capabilities = append(c.capabilities, plugin.CapabilityPTZ)
	}
	if ability.TwoWayAudio {
		c.capabilities = append(c.capabilities, plugin.CapabilityTwoWayAudio)
	}
	if ability.AudioAlarm {
		c.capabilities = append(c.capabilities, plugin.CapabilitySiren)
	}

	// Assume all Reolink cameras have motion detection
	c.capabilities = append(c.capabilities, plugin.CapabilityMotion)
	c.capabilities = append(c.capabilities, plugin.CapabilityAIDetection)
}

// SetEncoderConfig stores the encoder configuration
func (c *ReolinkCamera) SetEncoderConfig(cfg *EncoderConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.encoderCfg = cfg
}

// ID returns the camera ID
func (c *ReolinkCamera) ID() string {
	return c.id
}

// Name returns the camera name
func (c *ReolinkCamera) Name() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.name
}

// Model returns the camera model
func (c *ReolinkCamera) Model() string {
	return c.model
}

// Host returns the camera host
func (c *ReolinkCamera) Host() string {
	return c.host
}

// StreamURL returns the stream URL for the given quality
func (c *ReolinkCamera) StreamURL(quality plugin.StreamQuality) string {
	stream := "main"
	if quality == plugin.StreamQualitySub {
		stream = "sub"
	}

	// Prefer RTMP for better compatibility
	return c.client.RTMPStreamURL(c.channel, stream)
}

// SnapshotURL returns the snapshot URL
func (c *ReolinkCamera) SnapshotURL() string {
	return fmt.Sprintf("http://%s/cgi-bin/api.cgi?cmd=Snap&channel=%d", c.host, c.channel)
}

// Capabilities returns camera capabilities
func (c *ReolinkCamera) Capabilities() []plugin.Capability {
	c.mu.RLock()
	defer c.mu.RUnlock()
	caps := make([]plugin.Capability, len(c.capabilities))
	copy(caps, c.capabilities)
	return caps
}

// IsOnline returns whether the camera is online
func (c *ReolinkCamera) IsOnline() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.online
}

// LastSeen returns when the camera was last seen
func (c *ReolinkCamera) LastSeen() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastSeen
}

// SetOnline updates the online status
func (c *ReolinkCamera) SetOnline(online bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.online = online
	if online {
		c.lastSeen = time.Now()
	}
}

// PTZ Methods - implements plugin.PTZCapable

// Pan pans the camera horizontally
func (c *ReolinkCamera) Pan(ctx context.Context, direction float64, speed float64) error {
	op := PTZOpStop
	if direction < -0.1 {
		op = PTZOpLeft
	} else if direction > 0.1 {
		op = PTZOpRight
	}

	return c.client.PTZControl(ctx, c.channel, PTZCommand{
		Operation: op,
		Speed:     int(speed * 64),
	})
}

// Tilt tilts the camera vertically
func (c *ReolinkCamera) Tilt(ctx context.Context, direction float64, speed float64) error {
	op := PTZOpStop
	if direction < -0.1 {
		op = PTZOpDown
	} else if direction > 0.1 {
		op = PTZOpUp
	}

	return c.client.PTZControl(ctx, c.channel, PTZCommand{
		Operation: op,
		Speed:     int(speed * 64),
	})
}

// Zoom adjusts the camera zoom
func (c *ReolinkCamera) Zoom(ctx context.Context, direction float64, speed float64) error {
	op := PTZOpStop
	if direction < -0.1 {
		op = PTZOpZoomOut
	} else if direction > 0.1 {
		op = PTZOpZoomIn
	}

	return c.client.PTZControl(ctx, c.channel, PTZCommand{
		Operation: op,
		Speed:     int(speed * 64),
	})
}

// Stop stops PTZ movement
func (c *ReolinkCamera) Stop(ctx context.Context) error {
	return c.client.PTZControl(ctx, c.channel, PTZCommand{
		Operation: PTZOpStop,
	})
}

// GoToPreset moves to a preset position
func (c *ReolinkCamera) GoToPreset(ctx context.Context, preset string) error {
	return c.client.PTZControl(ctx, c.channel, PTZCommand{
		Operation: PTZOpToPos,
		Preset:    preset,
	})
}

// SavePreset saves current position as a preset
func (c *ReolinkCamera) SavePreset(ctx context.Context, preset string) error {
	// Reolink uses SetPtzPreset command
	// For now, just return an error as this requires additional API work
	return fmt.Errorf("save preset not yet implemented")
}

// ListPresets returns available presets
func (c *ReolinkCamera) ListPresets(ctx context.Context) ([]string, error) {
	presets, err := c.client.GetPresets(ctx, c.channel)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(presets))
	for _, p := range presets {
		names = append(names, p.Name)
	}
	return names, nil
}

// Event Methods - implements plugin.EventEmitter

// Subscribe registers an event handler
func (c *ReolinkCamera) Subscribe(handler plugin.EventHandler) func() {
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

// StartEventPolling begins polling for events
func (c *ReolinkCamera) StartEventPolling(ctx context.Context) error {
	ctx, c.eventCancel = context.WithCancel(ctx)

	c.eventWg.Add(1)
	go func() {
		defer c.eventWg.Done()
		c.pollEvents(ctx)
	}()

	return nil
}

// StopEventPolling stops event polling
func (c *ReolinkCamera) StopEventPolling() error {
	if c.eventCancel != nil {
		c.eventCancel()
	}
	c.eventWg.Wait()
	return nil
}

// pollEvents polls for motion and AI events
func (c *ReolinkCamera) pollEvents(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.checkEvents(ctx)
		}
	}
}

// checkEvents checks for new events
func (c *ReolinkCamera) checkEvents(ctx context.Context) {
	// Check motion state
	motion, err := c.client.GetMotionState(ctx, c.channel)
	if err == nil {
		c.mu.Lock()
		wasMotion := c.lastMotion
		c.lastMotion = motion
		c.mu.Unlock()

		if motion && !wasMotion {
			c.emitEvent(plugin.CameraEvent{
				Type:      plugin.EventTypeMotion,
				Timestamp: time.Now(),
				CameraID:  c.id,
			})
		}
	}

	// Check AI state
	aiState, err := c.client.GetAIState(ctx, c.channel)
	if err == nil {
		c.mu.Lock()
		lastAI := c.lastAIState
		c.lastAIState = aiState
		c.mu.Unlock()

		// Emit events for newly detected objects
		if aiState.Person && (lastAI == nil || !lastAI.Person) {
			c.emitEvent(plugin.CameraEvent{
				Type:      plugin.EventTypePerson,
				Timestamp: time.Now(),
				CameraID:  c.id,
			})
		}
		if aiState.Vehicle && (lastAI == nil || !lastAI.Vehicle) {
			c.emitEvent(plugin.CameraEvent{
				Type:      plugin.EventTypeVehicle,
				Timestamp: time.Now(),
				CameraID:  c.id,
			})
		}
		if aiState.Animal && (lastAI == nil || !lastAI.Animal) {
			c.emitEvent(plugin.CameraEvent{
				Type:      plugin.EventTypeAnimal,
				Timestamp: time.Now(),
				CameraID:  c.id,
			})
		}
		if aiState.Package && (lastAI == nil || !lastAI.Package) {
			c.emitEvent(plugin.CameraEvent{
				Type:      plugin.EventTypePackage,
				Timestamp: time.Now(),
				CameraID:  c.id,
			})
		}
		if aiState.Face && (lastAI == nil || !lastAI.Face) {
			c.emitEvent(plugin.CameraEvent{
				Type:      plugin.EventTypeFace,
				Timestamp: time.Now(),
				CameraID:  c.id,
			})
		}
	}

	// Update online status
	c.SetOnline(err == nil)
}

// emitEvent sends an event to all handlers
func (c *ReolinkCamera) emitEvent(event plugin.CameraEvent) {
	c.eventMu.RLock()
	handlers := make([]plugin.EventHandler, len(c.eventHandlers))
	copy(handlers, c.eventHandlers)
	c.eventMu.RUnlock()

	for _, handler := range handlers {
		handler(event)
	}
}
