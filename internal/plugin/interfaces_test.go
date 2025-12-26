package plugin

import (
	"testing"
	"time"
)

func TestCapability_Constants(t *testing.T) {
	tests := []struct {
		capability Capability
		expected   string
	}{
		{CapabilityVideo, "video"},
		{CapabilityAudio, "audio"},
		{CapabilityTwoWayAudio, "two_way_audio"},
		{CapabilityPTZ, "ptz"},
		{CapabilityPresets, "presets"},
		{CapabilityMotion, "motion"},
		{CapabilityAIDetection, "ai_detection"},
		{CapabilityDoorbell, "doorbell"},
		{CapabilitySiren, "siren"},
		{CapabilityFloodlight, "floodlight"},
		{CapabilityBattery, "battery"},
		{CapabilityNightVision, "night_vision"},
		{CapabilityColorNight, "color_night"},
		{CapabilitySnapshot, "snapshot"},
		{CapabilityDiscovery, "discovery"},
		{CapabilityNVR, "nvr"},
	}

	for _, tt := range tests {
		if string(tt.capability) != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, string(tt.capability))
		}
	}
}

func TestPluginState_Constants(t *testing.T) {
	tests := []struct {
		state    PluginState
		expected string
	}{
		{PluginStateNotInstalled, "not_installed"},
		{PluginStateInstalled, "installed"},
		{PluginStateStarting, "starting"},
		{PluginStateRunning, "running"},
		{PluginStateStopping, "stopping"},
		{PluginStateStopped, "stopped"},
		{PluginStateError, "error"},
	}

	for _, tt := range tests {
		if string(tt.state) != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, string(tt.state))
		}
	}
}

func TestHealthState_Constants(t *testing.T) {
	tests := []struct {
		state    HealthState
		expected string
	}{
		{HealthStateHealthy, "healthy"},
		{HealthStateUnhealthy, "unhealthy"},
		{HealthStateDegraded, "degraded"},
		{HealthStateUnknown, "unknown"},
	}

	for _, tt := range tests {
		if string(tt.state) != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, string(tt.state))
		}
	}
}

func TestStreamQuality_Constants(t *testing.T) {
	tests := []struct {
		quality  StreamQuality
		expected string
	}{
		{StreamQualityMain, "main"},
		{StreamQualitySub, "sub"},
	}

	for _, tt := range tests {
		if string(tt.quality) != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, string(tt.quality))
		}
	}
}

func TestCameraEventType_Constants(t *testing.T) {
	tests := []struct {
		eventType CameraEventType
		expected  string
	}{
		{EventTypeMotion, "motion"},
		{EventTypePerson, "person"},
		{EventTypeVehicle, "vehicle"},
		{EventTypeAnimal, "animal"},
		{EventTypePackage, "package"},
		{EventTypeFace, "face"},
		{EventTypeRing, "ring"},
		{EventTypeAudioAlert, "audio_alert"},
		{EventTypeLineCross, "line_cross"},
		{EventTypeIntrusion, "intrusion"},
	}

	for _, tt := range tests {
		if string(tt.eventType) != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, string(tt.eventType))
		}
	}
}

func TestPluginManifest_Fields(t *testing.T) {
	manifest := PluginManifest{
		ID:          "test-plugin",
		Name:        "Test Plugin",
		Version:     "1.0.0",
		Description: "A test plugin",
		Author:      "Test Author",
		Homepage:    "https://example.com",
		License:     "MIT",
		Runtime: PluginRuntime{
			Type:       "binary",
			Binary:     "plugin",
			EntryPoint: "",
			Args:       []string{"--debug"},
		},
		Capabilities: []Capability{CapabilityVideo, CapabilityPTZ},
		Dependencies: []PluginDependency{
			{
				Name:    "ffmpeg",
				Type:    "binary",
				Version: "4.0+",
			},
		},
		ConfigSchema: map[string]interface{}{
			"type": "object",
		},
	}

	if manifest.ID != "test-plugin" {
		t.Errorf("Expected ID 'test-plugin', got %s", manifest.ID)
	}
	if manifest.Name != "Test Plugin" {
		t.Errorf("Expected Name 'Test Plugin', got %s", manifest.Name)
	}
	if manifest.Version != "1.0.0" {
		t.Errorf("Expected Version '1.0.0', got %s", manifest.Version)
	}
	if len(manifest.Capabilities) != 2 {
		t.Errorf("Expected 2 capabilities, got %d", len(manifest.Capabilities))
	}
	if manifest.Runtime.Type != "binary" {
		t.Errorf("Expected Runtime.Type 'binary', got %s", manifest.Runtime.Type)
	}
	if len(manifest.Dependencies) != 1 {
		t.Errorf("Expected 1 dependency, got %d", len(manifest.Dependencies))
	}
}

func TestPluginStatus_Fields(t *testing.T) {
	now := time.Now()
	status := PluginStatus{
		Manifest: PluginManifest{
			ID:   "test-plugin",
			Name: "Test Plugin",
		},
		State:       PluginStateRunning,
		Enabled:     true,
		Health:      HealthStatus{State: HealthStateHealthy},
		CameraCount: 5,
		LastError:   "",
		StartedAt:   &now,
	}

	if status.Manifest.ID != "test-plugin" {
		t.Errorf("Expected Manifest.ID 'test-plugin', got %s", status.Manifest.ID)
	}
	if status.State != PluginStateRunning {
		t.Errorf("Expected State 'running', got %s", status.State)
	}
	if !status.Enabled {
		t.Error("Expected Enabled true")
	}
	if status.CameraCount != 5 {
		t.Errorf("Expected CameraCount 5, got %d", status.CameraCount)
	}
	if status.StartedAt == nil {
		t.Error("Expected StartedAt to be set")
	}
}

func TestHealthStatus_Fields(t *testing.T) {
	now := time.Now()
	health := HealthStatus{
		State:     HealthStateDegraded,
		Message:   "High CPU usage",
		LastCheck: now,
		Details: map[string]interface{}{
			"cpu_percent": 85.5,
			"memory_mb":   512,
		},
	}

	if health.State != HealthStateDegraded {
		t.Errorf("Expected State 'degraded', got %s", health.State)
	}
	if health.Message != "High CPU usage" {
		t.Errorf("Expected Message 'High CPU usage', got %s", health.Message)
	}
	if health.LastCheck.IsZero() {
		t.Error("Expected LastCheck to be set")
	}
	if health.Details["cpu_percent"] != 85.5 {
		t.Errorf("Expected cpu_percent 85.5, got %v", health.Details["cpu_percent"])
	}
}

func TestDiscoveredCamera_Fields(t *testing.T) {
	cam := DiscoveredCamera{
		ID:              "cam-1",
		Name:            "Front Door",
		Model:           "RLC-811A",
		Manufacturer:    "Reolink",
		Host:            "192.168.1.100",
		Port:            554,
		Channels:        1,
		Capabilities:    []Capability{CapabilityVideo, CapabilityPTZ},
		FirmwareVersion: "3.0.0",
		Serial:          "ABC123",
	}

	if cam.ID != "cam-1" {
		t.Errorf("Expected ID 'cam-1', got %s", cam.ID)
	}
	if cam.Host != "192.168.1.100" {
		t.Errorf("Expected Host '192.168.1.100', got %s", cam.Host)
	}
	if cam.Port != 554 {
		t.Errorf("Expected Port 554, got %d", cam.Port)
	}
	if len(cam.Capabilities) != 2 {
		t.Errorf("Expected 2 capabilities, got %d", len(cam.Capabilities))
	}
}

func TestPluginCamera_Fields(t *testing.T) {
	now := time.Now()
	cam := PluginCamera{
		ID:           "cam-1",
		PluginID:     "reolink",
		Name:         "Front Door",
		Model:        "RLC-811A",
		Host:         "192.168.1.100",
		MainStream:   "rtsp://192.168.1.100/h264Preview_01_main",
		SubStream:    "rtsp://192.168.1.100/h264Preview_01_sub",
		SnapshotURL:  "http://192.168.1.100/cgi-bin/api.cgi?cmd=Snap",
		Capabilities: []Capability{CapabilityVideo},
		Online:       true,
		LastSeen:     now,
		Config: CameraConfig{
			Host:     "192.168.1.100",
			Port:     554,
			Username: "admin",
			Password: "password",
		},
	}

	if cam.ID != "cam-1" {
		t.Errorf("Expected ID 'cam-1', got %s", cam.ID)
	}
	if cam.PluginID != "reolink" {
		t.Errorf("Expected PluginID 'reolink', got %s", cam.PluginID)
	}
	if cam.MainStream != "rtsp://192.168.1.100/h264Preview_01_main" {
		t.Errorf("Unexpected MainStream: %s", cam.MainStream)
	}
	if !cam.Online {
		t.Error("Expected Online true")
	}
	if cam.Config.Username != "admin" {
		t.Errorf("Expected Config.Username 'admin', got %s", cam.Config.Username)
	}
}

func TestCameraConfig_Fields(t *testing.T) {
	config := CameraConfig{
		Host:     "192.168.1.100",
		Port:     554,
		Username: "admin",
		Password: "secure123",
		Channel:  1,
		Name:     "Front Door",
		Extra: map[string]interface{}{
			"use_https": true,
			"timeout":   30,
		},
	}

	if config.Host != "192.168.1.100" {
		t.Errorf("Expected Host '192.168.1.100', got %s", config.Host)
	}
	if config.Port != 554 {
		t.Errorf("Expected Port 554, got %d", config.Port)
	}
	if config.Channel != 1 {
		t.Errorf("Expected Channel 1, got %d", config.Channel)
	}
	if config.Extra["use_https"] != true {
		t.Error("Expected Extra['use_https'] true")
	}
}

func TestCameraEvent_Fields(t *testing.T) {
	now := time.Now()
	event := CameraEvent{
		Type:      EventTypePerson,
		Timestamp: now,
		CameraID:  "cam-1",
		PluginID:  "reolink",
		Details: map[string]interface{}{
			"confidence": 0.95,
			"zone":       "front",
		},
	}

	if event.Type != EventTypePerson {
		t.Errorf("Expected Type 'person', got %s", event.Type)
	}
	if event.CameraID != "cam-1" {
		t.Errorf("Expected CameraID 'cam-1', got %s", event.CameraID)
	}
	if event.Details["confidence"] != 0.95 {
		t.Errorf("Expected confidence 0.95, got %v", event.Details["confidence"])
	}
}

func TestPTZCommand_Fields(t *testing.T) {
	cmd := PTZCommand{
		Action:    "pan",
		Direction: -0.5,
		Speed:     0.8,
		Preset:    "",
	}

	if cmd.Action != "pan" {
		t.Errorf("Expected Action 'pan', got %s", cmd.Action)
	}
	if cmd.Direction != -0.5 {
		t.Errorf("Expected Direction -0.5, got %f", cmd.Direction)
	}
	if cmd.Speed != 0.8 {
		t.Errorf("Expected Speed 0.8, got %f", cmd.Speed)
	}

	// Test preset command
	presetCmd := PTZCommand{
		Action: "preset",
		Preset: "home",
	}

	if presetCmd.Preset != "home" {
		t.Errorf("Expected Preset 'home', got %s", presetCmd.Preset)
	}
}

func TestPluginDependency_Fields(t *testing.T) {
	dep := PluginDependency{
		Name:      "ffmpeg",
		Type:      "binary",
		URL:       "https://example.com/ffmpeg-{os}-{arch}.tar.gz",
		Version:   "4.0+",
		Platforms: []string{"linux-amd64", "darwin-arm64"},
	}

	if dep.Name != "ffmpeg" {
		t.Errorf("Expected Name 'ffmpeg', got %s", dep.Name)
	}
	if dep.Type != "binary" {
		t.Errorf("Expected Type 'binary', got %s", dep.Type)
	}
	if len(dep.Platforms) != 2 {
		t.Errorf("Expected 2 platforms, got %d", len(dep.Platforms))
	}
}

func TestPluginRuntime_Fields(t *testing.T) {
	tests := []struct {
		name    string
		runtime PluginRuntime
	}{
		{
			name: "binary runtime",
			runtime: PluginRuntime{
				Type:   "binary",
				Binary: "plugin",
				Args:   []string{"--config", "/etc/plugin.yaml"},
			},
		},
		{
			name: "python runtime",
			runtime: PluginRuntime{
				Type:       "python",
				EntryPoint: "main.py",
				Args:       []string{"--debug"},
			},
		},
		{
			name: "node runtime",
			runtime: PluginRuntime{
				Type:       "node",
				EntryPoint: "index.js",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.runtime.Type == "" {
				t.Error("Expected Type to be set")
			}
		})
	}
}
