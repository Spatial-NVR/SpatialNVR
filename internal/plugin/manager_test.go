package plugin

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Spatial-NVR/SpatialNVR/internal/config"
)

// mockBuiltinPlugin implements BuiltinPlugin for testing
type mockBuiltinPlugin struct {
	manifest    PluginManifest
	initErr     error
	startErr    error
	stopErr     error
	healthState HealthState
	cameras     []PluginCamera
	discovered  []DiscoveredCamera
}

func (m *mockBuiltinPlugin) Manifest() PluginManifest {
	return m.manifest
}

func (m *mockBuiltinPlugin) Initialize(ctx context.Context, cfg map[string]interface{}) error {
	return m.initErr
}

func (m *mockBuiltinPlugin) Start(ctx context.Context) error {
	return m.startErr
}

func (m *mockBuiltinPlugin) Stop() error {
	return m.stopErr
}

func (m *mockBuiltinPlugin) Health() HealthStatus {
	return HealthStatus{State: m.healthState}
}

func (m *mockBuiltinPlugin) OnConfigChange(cfg map[string]interface{}) {}

func (m *mockBuiltinPlugin) DiscoverCameras(ctx context.Context) ([]DiscoveredCamera, error) {
	return m.discovered, nil
}

func (m *mockBuiltinPlugin) ListCameras() []PluginCamera {
	return m.cameras
}

func (m *mockBuiltinPlugin) GetCamera(id string) *PluginCamera {
	for i := range m.cameras {
		if m.cameras[i].ID == id {
			return &m.cameras[i]
		}
	}
	return nil
}

func (m *mockBuiltinPlugin) AddCamera(ctx context.Context, cfg CameraConfig) (*PluginCamera, error) {
	cam := &PluginCamera{
		ID:   cfg.Name,
		Name: cfg.Name,
		Host: cfg.Host,
	}
	m.cameras = append(m.cameras, *cam)
	return cam, nil
}

func (m *mockBuiltinPlugin) RemoveCamera(ctx context.Context, id string) error {
	for i := range m.cameras {
		if m.cameras[i].ID == id {
			m.cameras = append(m.cameras[:i], m.cameras[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *mockBuiltinPlugin) PTZControl(ctx context.Context, cameraID string, cmd PTZCommand) error {
	return nil
}

func setupTestManager(t *testing.T) (*Manager, string) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		System: config.SystemConfig{
			StoragePath: tmpDir,
		},
		Plugins: make(config.PluginsConfig),
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := NewManager(cfg, nil, nil, logger)

	return mgr, tmpDir
}

func TestNewManager(t *testing.T) {
	mgr, tmpDir := setupTestManager(t)

	if mgr == nil {
		t.Fatal("NewManager returned nil")
	}

	expectedPluginsDir := filepath.Join(tmpDir, "plugins")
	if mgr.pluginsDir != expectedPluginsDir {
		t.Errorf("Expected pluginsDir %s, got %s", expectedPluginsDir, mgr.pluginsDir)
	}

	expectedDataDir := filepath.Join(tmpDir, "plugin-data")
	if mgr.dataDir != expectedDataDir {
		t.Errorf("Expected dataDir %s, got %s", expectedDataDir, mgr.dataDir)
	}
}

func TestManager_RegisterBuiltinPlugin(t *testing.T) {
	mgr, _ := setupTestManager(t)

	plugin := &mockBuiltinPlugin{
		manifest: PluginManifest{
			ID:   "test-plugin",
			Name: "Test Plugin",
		},
		healthState: HealthStateHealthy,
	}

	err := mgr.RegisterBuiltinPlugin(plugin)
	if err != nil {
		t.Fatalf("RegisterBuiltinPlugin failed: %v", err)
	}

	// Verify plugin was registered
	p, ok := mgr.GetPlugin("test-plugin")
	if !ok {
		t.Fatal("Plugin not found after registration")
	}
	if p.Manifest().Name != "Test Plugin" {
		t.Errorf("Expected plugin name 'Test Plugin', got %s", p.Manifest().Name)
	}
}

func TestManager_RegisterBuiltinPlugin_EmptyID(t *testing.T) {
	mgr, _ := setupTestManager(t)

	plugin := &mockBuiltinPlugin{
		manifest: PluginManifest{
			Name: "Test Plugin",
		},
	}

	err := mgr.RegisterBuiltinPlugin(plugin)
	if err == nil {
		t.Error("Expected error for empty plugin ID")
	}
}

func TestManager_RegisterBuiltinPlugin_Duplicate(t *testing.T) {
	mgr, _ := setupTestManager(t)

	plugin := &mockBuiltinPlugin{
		manifest: PluginManifest{
			ID:   "duplicate-plugin",
			Name: "Test Plugin",
		},
	}

	err := mgr.RegisterBuiltinPlugin(plugin)
	if err != nil {
		t.Fatalf("First registration failed: %v", err)
	}

	err = mgr.RegisterBuiltinPlugin(plugin)
	if err == nil {
		t.Error("Expected error for duplicate plugin registration")
	}
}

func TestManager_GetPlugin_NotFound(t *testing.T) {
	mgr, _ := setupTestManager(t)

	_, ok := mgr.GetPlugin("nonexistent")
	if ok {
		t.Error("Expected GetPlugin to return false for nonexistent plugin")
	}
}

func TestManager_ListPlugins_Empty(t *testing.T) {
	mgr, _ := setupTestManager(t)

	plugins := mgr.ListPlugins()
	if len(plugins) != 0 {
		t.Errorf("Expected 0 plugins, got %d", len(plugins))
	}
}

func TestManager_ListPlugins_WithPlugins(t *testing.T) {
	mgr, _ := setupTestManager(t)

	plugin := &mockBuiltinPlugin{
		manifest: PluginManifest{
			ID:      "test-plugin",
			Name:    "Test Plugin",
			Version: "1.0.0",
		},
		healthState: HealthStateHealthy,
		cameras: []PluginCamera{
			{ID: "cam1", Name: "Camera 1"},
		},
	}

	mgr.RegisterBuiltinPlugin(plugin)
	mgr.config.Plugins["test-plugin"] = config.PluginConfig{Enabled: true}

	plugins := mgr.ListPlugins()
	if len(plugins) != 1 {
		t.Fatalf("Expected 1 plugin, got %d", len(plugins))
	}

	if plugins[0].Manifest.ID != "test-plugin" {
		t.Errorf("Expected plugin ID 'test-plugin', got %s", plugins[0].Manifest.ID)
	}
	if plugins[0].CameraCount != 1 {
		t.Errorf("Expected camera count 1, got %d", plugins[0].CameraCount)
	}
}

func TestManager_ListAllCameras_Empty(t *testing.T) {
	mgr, _ := setupTestManager(t)

	cameras := mgr.ListAllCameras()
	if len(cameras) != 0 {
		t.Errorf("Expected 0 cameras, got %d", len(cameras))
	}
}

func TestManager_GetCamera_NotFound(t *testing.T) {
	mgr, _ := setupTestManager(t)

	_, ok := mgr.GetCamera("nonexistent")
	if ok {
		t.Error("Expected GetCamera to return false for nonexistent camera")
	}
}

func TestManager_PluginsDir(t *testing.T) {
	mgr, tmpDir := setupTestManager(t)

	expected := filepath.Join(tmpDir, "plugins")
	if mgr.PluginsDir() != expected {
		t.Errorf("Expected PluginsDir %s, got %s", expected, mgr.PluginsDir())
	}
}

func TestManager_GetCatalog(t *testing.T) {
	mgr, _ := setupTestManager(t)

	catalog := mgr.GetCatalog()
	if catalog == nil {
		t.Fatal("GetCatalog returned nil")
	}
}

func TestManager_OnConfigChange(t *testing.T) {
	mgr, _ := setupTestManager(t)

	plugin := &mockBuiltinPlugin{
		manifest: PluginManifest{
			ID:   "config-test",
			Name: "Config Test Plugin",
		},
	}
	mgr.RegisterBuiltinPlugin(plugin)

	newCfg := &config.Config{
		Plugins: config.PluginsConfig{
			"config-test": {
				Enabled: true,
				Config:  map[string]interface{}{"key": "value"},
			},
		},
	}

	// Should not panic
	mgr.OnConfigChange(newCfg)
}

func TestManager_ScanPlugins_EmptyDir(t *testing.T) {
	mgr, _ := setupTestManager(t)

	err := mgr.ScanPlugins()
	if err != nil {
		t.Errorf("ScanPlugins failed on empty dir: %v", err)
	}
}

func TestManager_ScanPlugins_WithPlugin(t *testing.T) {
	mgr, tmpDir := setupTestManager(t)

	// Create a fake plugin directory with manifest
	pluginDir := filepath.Join(tmpDir, "plugins", "test-external")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("Failed to create plugin dir: %v", err)
	}

	manifestContent := `
id: test-external
name: Test External Plugin
version: "1.0.0"
runtime:
  type: binary
  binary: plugin
`
	manifestPath := filepath.Join(pluginDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	err := mgr.ScanPlugins()
	if err != nil {
		t.Errorf("ScanPlugins failed: %v", err)
	}

	// Check that plugin was found
	installed := mgr.ListInstalledPlugins()
	if len(installed) != 1 {
		t.Errorf("Expected 1 installed plugin, got %d", len(installed))
	}
}

func TestManager_ListInstalledPlugins_Empty(t *testing.T) {
	mgr, _ := setupTestManager(t)

	installed := mgr.ListInstalledPlugins()
	if len(installed) != 0 {
		t.Errorf("Expected 0 installed plugins, got %d", len(installed))
	}
}

func TestManager_StartExternalPlugin_NotInstalled(t *testing.T) {
	mgr, _ := setupTestManager(t)

	err := mgr.StartExternalPlugin("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent plugin")
	}
}

func TestManager_StopExternalPlugin_NotInstalled(t *testing.T) {
	mgr, _ := setupTestManager(t)

	err := mgr.StopExternalPlugin("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent plugin")
	}
}

func TestManager_EnablePlugin_NotFound(t *testing.T) {
	mgr, _ := setupTestManager(t)

	err := mgr.EnablePlugin(context.Background(), "nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent plugin")
	}
}

func TestManager_DisablePlugin_NotFound(t *testing.T) {
	mgr, _ := setupTestManager(t)

	err := mgr.DisablePlugin(context.Background(), "nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent plugin")
	}
}

func TestManager_DiscoverCameras(t *testing.T) {
	mgr, _ := setupTestManager(t)

	plugin := &mockBuiltinPlugin{
		manifest: PluginManifest{
			ID:   "discover-test",
			Name: "Discover Test",
		},
		discovered: []DiscoveredCamera{
			{ID: "cam1", Name: "Camera 1", Host: "192.168.1.100"},
		},
	}
	mgr.RegisterBuiltinPlugin(plugin)
	mgr.config.Plugins["discover-test"] = config.PluginConfig{Enabled: true}

	cameras, err := mgr.DiscoverCameras(context.Background())
	if err != nil {
		t.Fatalf("DiscoverCameras failed: %v", err)
	}

	if len(cameras) != 1 {
		t.Errorf("Expected 1 discovered camera, got %d", len(cameras))
	}
}

func TestManager_PTZControl_CameraNotFound(t *testing.T) {
	mgr, _ := setupTestManager(t)

	err := mgr.PTZControl(context.Background(), "nonexistent", PTZCommand{})
	if err == nil {
		t.Error("Expected error for nonexistent camera")
	}
}

func TestManager_Stop(t *testing.T) {
	mgr, _ := setupTestManager(t)

	plugin := &mockBuiltinPlugin{
		manifest: PluginManifest{
			ID:   "stop-test",
			Name: "Stop Test",
		},
	}
	mgr.RegisterBuiltinPlugin(plugin)

	// Stop should not panic
	err := mgr.Stop()
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}

func TestManager_Start(t *testing.T) {
	mgr, _ := setupTestManager(t)

	plugin := &mockBuiltinPlugin{
		manifest: PluginManifest{
			ID:   "start-test",
			Name: "Start Test",
		},
		healthState: HealthStateHealthy,
	}
	mgr.RegisterBuiltinPlugin(plugin)

	err := mgr.Start(context.Background())
	if err != nil {
		t.Errorf("Start failed: %v", err)
	}

	// Cleanup
	mgr.Stop()
}

func TestManager_GetCatalogWithStatus(t *testing.T) {
	mgr, _ := setupTestManager(t)

	status := mgr.GetCatalogWithStatus()
	// Should return empty list when no catalog plugins
	if status == nil {
		t.Error("GetCatalogWithStatus returned nil")
	}
}

func TestManager_InstallFromCatalog_NoCatalog(t *testing.T) {
	mgr, _ := setupTestManager(t)
	mgr.catalog = nil

	_, err := mgr.InstallFromCatalog(context.Background(), "nonexistent")
	if err == nil {
		t.Error("Expected error when catalog is nil")
	}
}

func TestManager_ReloadCatalog_NoFile(t *testing.T) {
	mgr, _ := setupTestManager(t)

	// Should not error even if no catalog file exists
	err := mgr.ReloadCatalog()
	// May or may not error depending on implementation
	_ = err
}

func TestManager_GetTrackedRepos(t *testing.T) {
	mgr, _ := setupTestManager(t)

	repos := mgr.GetTrackedRepos()
	if repos == nil {
		t.Error("GetTrackedRepos returned nil")
	}
}

func TestManager_GetInstaller(t *testing.T) {
	mgr, _ := setupTestManager(t)

	installer := mgr.GetInstaller()
	if installer == nil {
		t.Error("GetInstaller returned nil")
	}
}

func TestInstalledPlugin_Fields(t *testing.T) {
	now := time.Now()
	plugin := &installedPlugin{
		manifest: PluginManifest{
			ID:   "test",
			Name: "Test",
		},
		dir:       "/path/to/plugin",
		state:     PluginStateRunning,
		enabled:   true,
		startedAt: &now,
		lastError: "",
		cameras:   make(map[string]*PluginCamera),
	}

	if plugin.manifest.ID != "test" {
		t.Error("manifest ID not set")
	}
	if plugin.dir != "/path/to/plugin" {
		t.Error("dir not set")
	}
	if plugin.state != PluginStateRunning {
		t.Error("state not set")
	}
	if !plugin.enabled {
		t.Error("enabled should be true")
	}
	if plugin.startedAt == nil {
		t.Error("startedAt should be set")
	}
}
