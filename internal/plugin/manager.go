package plugin

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Spatial-NVR/SpatialNVR/internal/config"
	"github.com/Spatial-NVR/SpatialNVR/internal/events"
	"github.com/Spatial-NVR/SpatialNVR/internal/streaming"
)

// Ensure yaml is used
var _ = yaml.Unmarshal

// Manager handles plugin lifecycle, installation, and coordination.
// It supports both built-in Go plugins and external process plugins via JSON-RPC.
type Manager struct {
	pluginsDir string // Directory where plugins are installed
	dataDir    string // Directory for plugin data storage
	catalogDir string // Directory containing the plugin catalog

	// Built-in plugins (native Go implementations)
	builtinPlugins map[string]BuiltinPlugin

	// Installed external plugins
	installed map[string]*installedPlugin

	// Plugin installer for Git-based installation
	installer *Installer

	// Plugin catalog for marketplace
	catalog *Catalog

	// Plugin cameras aggregated from all plugins
	cameras map[string]*PluginCamera

	config   *config.Config
	go2rtc   *streaming.Go2RTCManager
	eventSvc *events.Service
	logger   *slog.Logger

	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc

	healthCheckInterval time.Duration
}

// BuiltinPlugin is implemented by Go plugins compiled into the NVR
type BuiltinPlugin interface {
	// Manifest returns the plugin's manifest
	Manifest() PluginManifest

	// Initialize prepares the plugin with configuration
	Initialize(ctx context.Context, cfg map[string]interface{}) error

	// Start begins plugin operation
	Start(ctx context.Context) error

	// Stop shuts down the plugin
	Stop() error

	// Health returns the plugin's health status
	Health() HealthStatus

	// OnConfigChange handles configuration updates
	OnConfigChange(cfg map[string]interface{})

	// DiscoverCameras searches for available cameras
	DiscoverCameras(ctx context.Context) ([]DiscoveredCamera, error)

	// ListCameras returns all cameras managed by this plugin
	ListCameras() []PluginCamera

	// GetCamera returns a specific camera
	GetCamera(id string) *PluginCamera

	// AddCamera adds a camera with the given configuration
	AddCamera(ctx context.Context, cfg CameraConfig) (*PluginCamera, error)

	// RemoveCamera removes a camera
	RemoveCamera(ctx context.Context, id string) error

	// PTZControl sends a PTZ command to a camera
	PTZControl(ctx context.Context, cameraID string, cmd PTZCommand) error
}

// installedPlugin represents a plugin installed on the system
type installedPlugin struct {
	manifest PluginManifest
	dir      string
	state    PluginState
	enabled  bool

	// Runtime state (only set when running)
	cmd       *exec.Cmd
	proxy     *PluginProxy
	cameras   map[string]*PluginCamera
	startedAt *time.Time
	lastError string

	// Log buffer for external plugins
	logBuffer []LogEntry
	logMu     sync.RWMutex

	mu sync.RWMutex
}

// LogEntry represents a single log line from a plugin
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
}

// NewManager creates a new plugin manager
func NewManager(
	cfg *config.Config,
	go2rtc *streaming.Go2RTCManager,
	eventSvc *events.Service,
	logger *slog.Logger,
) *Manager {
	// Default plugin directories
	pluginsDir := filepath.Join(cfg.System.StoragePath, "plugins")
	dataDir := filepath.Join(cfg.System.StoragePath, "plugin-data")

	mgr := &Manager{
		pluginsDir:          pluginsDir,
		dataDir:             dataDir,
		catalogDir:          cfg.System.StoragePath,
		builtinPlugins:      make(map[string]BuiltinPlugin),
		installed:           make(map[string]*installedPlugin),
		cameras:             make(map[string]*PluginCamera),
		config:              cfg,
		go2rtc:              go2rtc,
		eventSvc:            eventSvc,
		logger:              logger.With("component", "plugin-manager"),
		healthCheckInterval: 30 * time.Second,
	}

	// Create installer for Git-based plugin installation
	mgr.installer = NewInstaller(pluginsDir, logger)

	// Pass GitHub token from config to installer for higher API rate limits
	if cfg.System.Updates.GitHubToken != "" {
		mgr.installer.SetGitHubToken(cfg.System.Updates.GitHubToken)
	}

	// Load plugin catalog
	catalog, err := LoadCatalogFromDir(mgr.catalogDir)
	if err != nil {
		logger.Warn("Failed to load plugin catalog", "error", err)
		catalog = &Catalog{Version: "1.0", Plugins: []CatalogPlugin{}, Categories: make(map[string]CatalogCategory)}
	}
	mgr.catalog = catalog

	return mgr
}

// RegisterBuiltinPlugin adds a built-in Go plugin
func (m *Manager) RegisterBuiltinPlugin(p BuiltinPlugin) error {
	manifest := p.Manifest()
	if manifest.ID == "" {
		return fmt.Errorf("plugin ID cannot be empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.builtinPlugins[manifest.ID]; exists {
		return fmt.Errorf("plugin already registered: %s", manifest.ID)
	}

	m.builtinPlugins[manifest.ID] = p
	m.logger.Info("Builtin plugin registered", "id", manifest.ID, "name", manifest.Name)
	return nil
}

// Start initializes and starts all enabled plugins
func (m *Manager) Start(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)

	// Start the installer for update checking
	if err := m.installer.Start(ctx); err != nil {
		m.logger.Warn("Failed to start plugin installer", "error", err)
	}

	// Scan for installed external plugins
	if err := m.ScanPlugins(); err != nil {
		m.logger.Warn("Failed to scan plugins", "error", err)
	}

	// Start builtin plugins
	m.mu.RLock()
	plugins := make([]BuiltinPlugin, 0, len(m.builtinPlugins))
	for _, p := range m.builtinPlugins {
		plugins = append(plugins, p)
	}
	m.mu.RUnlock()

	for _, p := range plugins {
		manifest := p.Manifest()

		// Check if plugin is enabled in config
		pluginCfg, enabled := m.getPluginConfig(manifest.ID)
		if !enabled {
			m.logger.Debug("Plugin disabled, skipping", "id", manifest.ID)
			continue
		}

		// Initialize plugin
		if err := p.Initialize(m.ctx, pluginCfg.Config); err != nil {
			m.logger.Error("Failed to initialize plugin", "id", manifest.ID, "error", err)
			continue
		}

		// Start plugin
		if err := p.Start(m.ctx); err != nil {
			m.logger.Error("Failed to start plugin", "id", manifest.ID, "error", err)
			continue
		}

		m.logger.Info("Plugin started", "id", manifest.ID)

		// Sync cameras from this plugin
		go m.syncPluginCameras(manifest.ID, p)
	}

	// Start health check goroutine
	go m.healthCheckLoop()

	return nil
}

// Stop gracefully shuts down all plugins
func (m *Manager) Stop() error {
	if m.cancel != nil {
		m.cancel()
	}

	// Stop the installer
	if m.installer != nil {
		m.installer.Stop()
	}

	m.mu.RLock()
	plugins := make([]BuiltinPlugin, 0, len(m.builtinPlugins))
	for _, p := range m.builtinPlugins {
		plugins = append(plugins, p)
	}
	m.mu.RUnlock()

	var lastErr error
	for _, p := range plugins {
		manifest := p.Manifest()
		if err := p.Stop(); err != nil {
			m.logger.Error("Failed to stop plugin", "id", manifest.ID, "error", err)
			lastErr = err
		} else {
			m.logger.Info("Plugin stopped", "id", manifest.ID)
		}
	}

	return lastErr
}

// GetPlugin returns a plugin by ID
func (m *Manager) GetPlugin(id string) (BuiltinPlugin, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.builtinPlugins[id]
	return p, ok
}

// ListPlugins returns status for all registered plugins
func (m *Manager) ListPlugins() []PluginStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]PluginStatus, 0, len(m.builtinPlugins))

	for id, p := range m.builtinPlugins {
		manifest := p.Manifest()
		pluginCfg, enabled := m.getPluginConfigLocked(id)
		_ = pluginCfg // silence unused warning
		health := p.Health()

		state := PluginStateStopped
		if enabled {
			switch health.State {
			case HealthStateHealthy:
				state = PluginStateRunning
			case HealthStateUnhealthy:
				state = PluginStateError
			default:
				state = PluginStateRunning
			}
		}

		cameras := p.ListCameras()
		now := time.Now()

		result = append(result, PluginStatus{
			Manifest:    manifest,
			State:       state,
			Enabled:     enabled,
			Health:      health,
			CameraCount: len(cameras),
			StartedAt:   &now,
		})
	}

	return result
}

// EnablePlugin enables a plugin and starts it
func (m *Manager) EnablePlugin(ctx context.Context, id string) error {
	m.mu.Lock()
	p, ok := m.builtinPlugins[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("plugin not found: %s", id)
	}
	m.mu.Unlock()

	// Update config
	if m.config.Plugins == nil {
		m.config.Plugins = make(config.PluginsConfig)
	}
	pluginCfg := m.config.Plugins[id]
	pluginCfg.Enabled = true
	m.config.Plugins[id] = pluginCfg

	if err := m.config.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Initialize and start
	if err := p.Initialize(ctx, pluginCfg.Config); err != nil {
		return fmt.Errorf("failed to initialize plugin: %w", err)
	}

	if err := p.Start(ctx); err != nil {
		return fmt.Errorf("failed to start plugin: %w", err)
	}

	// Sync cameras
	go m.syncPluginCameras(id, p)

	m.logger.Info("Plugin enabled", "id", id)
	return nil
}

// DisablePlugin stops and disables a plugin
func (m *Manager) DisablePlugin(ctx context.Context, id string) error {
	m.mu.Lock()
	p, ok := m.builtinPlugins[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("plugin not found: %s", id)
	}
	m.mu.Unlock()

	// Stop the plugin
	if err := p.Stop(); err != nil {
		m.logger.Warn("Error stopping plugin", "id", id, "error", err)
	}

	// Remove cameras from this plugin
	m.removeCamerasForPlugin(id)

	// Update config
	if m.config.Plugins == nil {
		m.config.Plugins = make(config.PluginsConfig)
	}
	pluginCfg := m.config.Plugins[id]
	pluginCfg.Enabled = false
	m.config.Plugins[id] = pluginCfg

	if err := m.config.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	m.logger.Info("Plugin disabled", "id", id)
	return nil
}

// OnConfigChange handles configuration changes
func (m *Manager) OnConfigChange(cfg *config.Config) {
	m.mu.Lock()
	m.config = cfg
	m.mu.Unlock()

	// Notify all plugins of config changes
	m.mu.RLock()
	defer m.mu.RUnlock()

	for id, p := range m.builtinPlugins {
		pluginCfg, _ := m.getPluginConfigLocked(id)
		p.OnConfigChange(pluginCfg.Config)
	}
}

// ListAllCameras returns all cameras from all plugins
func (m *Manager) ListAllCameras() []*PluginCamera {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*PluginCamera, 0, len(m.cameras))
	for _, cam := range m.cameras {
		result = append(result, cam)
	}
	return result
}

// GetCamera returns a camera by ID
func (m *Manager) GetCamera(id string) (*PluginCamera, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cam, ok := m.cameras[id]
	return cam, ok
}

// DiscoverCameras triggers discovery on all enabled plugins
func (m *Manager) DiscoverCameras(ctx context.Context) ([]DiscoveredCamera, error) {
	m.mu.RLock()
	plugins := make([]BuiltinPlugin, 0)
	for id, p := range m.builtinPlugins {
		if _, enabled := m.getPluginConfigLocked(id); enabled {
			plugins = append(plugins, p)
		}
	}
	m.mu.RUnlock()

	var allDiscovered []DiscoveredCamera
	for _, p := range plugins {
		discovered, err := p.DiscoverCameras(ctx)
		if err != nil {
			m.logger.Warn("Discovery failed", "plugin", p.Manifest().ID, "error", err)
			continue
		}
		allDiscovered = append(allDiscovered, discovered...)
	}

	return allDiscovered, nil
}

// PTZControl sends a PTZ command to a camera
func (m *Manager) PTZControl(ctx context.Context, cameraID string, cmd PTZCommand) error {
	m.mu.RLock()
	cam, ok := m.cameras[cameraID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("camera not found: %s", cameraID)
	}

	plugin, ok := m.GetPlugin(cam.PluginID)
	if !ok {
		return fmt.Errorf("plugin not found: %s", cam.PluginID)
	}

	return plugin.PTZControl(ctx, cam.ID, cmd)
}

// syncPluginCameras syncs cameras from a plugin
func (m *Manager) syncPluginCameras(pluginID string, p BuiltinPlugin) {
	cameras := p.ListCameras()

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, cam := range cameras {
		fullID := fmt.Sprintf("%s_%s", pluginID, cam.ID)
		camCopy := cam
		camCopy.PluginID = pluginID
		m.cameras[fullID] = &camCopy

		// Register with go2rtc
		if cam.MainStream != "" {
			if err := m.go2rtc.AddStream(fullID, cam.MainStream); err != nil {
				m.logger.Warn("Failed to register camera with go2rtc",
					"camera", fullID, "error", err)
			} else {
				m.logger.Debug("Registered camera with go2rtc", "camera", fullID)
			}
		}
	}

	m.logger.Info("Synced cameras from plugin", "plugin", pluginID, "count", len(cameras))
}

// removeCamerasForPlugin removes all cameras from a specific plugin
func (m *Manager) removeCamerasForPlugin(pluginID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	prefix := pluginID + "_"
	for id := range m.cameras {
		if len(id) > len(prefix) && id[:len(prefix)] == prefix {
			if err := m.go2rtc.RemoveStream(id); err != nil {
				m.logger.Warn("Failed to remove camera from go2rtc", "camera", id, "error", err)
			}
			delete(m.cameras, id)
		}
	}
}

// healthCheckLoop periodically checks plugin health
func (m *Manager) healthCheckLoop() {
	ticker := time.NewTicker(m.healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkHealth()
		}
	}
}

// checkHealth checks the health of all enabled plugins
func (m *Manager) checkHealth() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for id, p := range m.builtinPlugins {
		if _, enabled := m.getPluginConfigLocked(id); !enabled {
			continue
		}

		health := p.Health()
		if health.State != HealthStateHealthy {
			m.logger.Warn("Plugin unhealthy",
				"id", id,
				"state", health.State,
				"message", health.Message)
		}
	}
}

// getPluginConfig returns plugin config and enabled state (thread-safe)
func (m *Manager) getPluginConfig(id string) (config.PluginConfig, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getPluginConfigLocked(id)
}

// getPluginConfigLocked returns plugin config (caller must hold lock)
func (m *Manager) getPluginConfigLocked(id string) (config.PluginConfig, bool) {
	if m.config.Plugins == nil {
		return config.PluginConfig{}, false
	}
	cfg, ok := m.config.Plugins[id]
	return cfg, ok && cfg.Enabled
}

// getPluginConfigMap returns plugin config as a map
func (m *Manager) getPluginConfigMap(id string) map[string]interface{} {
	cfg, _ := m.getPluginConfig(id)
	return cfg.Config
}

// ============================================================================
// External Plugin Management
// ============================================================================

// ScanPlugins scans the plugins directory for installed external plugins
func (m *Manager) ScanPlugins() error {
	if err := os.MkdirAll(m.pluginsDir, 0755); err != nil {
		return fmt.Errorf("failed to create plugins directory: %w", err)
	}
	if err := os.MkdirAll(m.dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create plugin data directory: %w", err)
	}

	entries, err := os.ReadDir(m.pluginsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read plugins directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pluginDir := filepath.Join(m.pluginsDir, entry.Name())
		manifestPath := filepath.Join(pluginDir, "manifest.yaml")

		data, err := os.ReadFile(manifestPath)
		if err != nil {
			m.logger.Debug("Skipping directory (no manifest)", "dir", entry.Name())
			continue
		}

		var manifest PluginManifest
		if err := yaml.Unmarshal(data, &manifest); err != nil {
			m.logger.Warn("Invalid manifest", "dir", entry.Name(), "error", err)
			continue
		}

		m.mu.Lock()
		// Use getPluginConfigLocked to avoid deadlock (we already hold the lock)
		_, enabled := m.getPluginConfigLocked(manifest.ID)
		m.installed[manifest.ID] = &installedPlugin{
			manifest: manifest,
			dir:      pluginDir,
			state:    PluginStateStopped,
			enabled:  enabled,
			cameras:  make(map[string]*PluginCamera),
		}
		m.mu.Unlock()

		m.logger.Info("Found installed plugin", "id", manifest.ID, "version", manifest.Version)
	}

	return nil
}

// StartExternalPlugin starts an external plugin process
func (m *Manager) StartExternalPlugin(id string) error {
	m.mu.RLock()
	p, ok := m.installed[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("plugin not installed: %s", id)
	}

	return m.startExternalPlugin(p)
}

func (m *Manager) startExternalPlugin(p *installedPlugin) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == PluginStateRunning {
		return nil
	}

	p.state = PluginStateStarting
	m.logger.Info("Starting external plugin", "id", p.manifest.ID)

	var cmd *exec.Cmd
	switch p.manifest.Runtime.Type {
	case "binary":
		// Try platform-specific binary first (e.g., plugin-linux-amd64)
		binaryPath := filepath.Join(p.dir, fmt.Sprintf("%s-%s-%s", p.manifest.Runtime.Binary, runtime.GOOS, runtime.GOARCH))
		if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
			// Fall back to generic binary name
			binaryPath = filepath.Join(p.dir, p.manifest.Runtime.Binary)
		} else {
			m.logger.Info("Using platform-specific binary", "path", binaryPath)
		}
		cmd = exec.CommandContext(m.ctx, binaryPath, p.manifest.Runtime.Args...)
	case "python":
		// Check if setup script needs to be run first
		if p.manifest.Runtime.Setup != "" {
			setupPath := filepath.Join(p.dir, p.manifest.Runtime.Setup)
			if _, err := os.Stat(setupPath); err == nil {
				// Check if setup has already been run (look for venv or setup marker)
				venvPath := filepath.Join(p.dir, "venv")
				setupMarker := filepath.Join(p.dir, ".setup_complete")
				if _, err := os.Stat(venvPath); os.IsNotExist(err) {
					if _, err := os.Stat(setupMarker); os.IsNotExist(err) {
						m.logger.Info("Running setup script for Python plugin", "plugin", p.manifest.ID)
						setupCmd := exec.CommandContext(m.ctx, "bash", setupPath)
						setupCmd.Dir = p.dir
						setupCmd.Env = append(os.Environ(),
							fmt.Sprintf("NVR_PLUGIN_ID=%s", p.manifest.ID),
							fmt.Sprintf("NVR_PLUGIN_DIR=%s", p.dir),
						)
						if output, err := setupCmd.CombinedOutput(); err != nil {
							m.logger.Error("Setup script failed", "plugin", p.manifest.ID, "error", err, "output", string(output))
							p.state = PluginStateError
							p.lastError = fmt.Sprintf("setup failed: %v", err)
							return fmt.Errorf("setup script failed: %w\n%s", err, output)
						}
						// Create marker so we don't run setup again
						_ = os.WriteFile(setupMarker, []byte(time.Now().Format(time.RFC3339)), 0644)
						m.logger.Info("Setup script completed", "plugin", p.manifest.ID)
					}
				}
			}
		}

		// Use GetEntryPoint() to support both entry_point and script fields
		entryPoint := filepath.Join(p.dir, p.manifest.Runtime.GetEntryPoint())

		// Check if venv exists and use its Python
		venvPython := filepath.Join(p.dir, "venv", "bin", "python3")
		pythonPath := "python3"
		if _, err := os.Stat(venvPython); err == nil {
			pythonPath = venvPython
			m.logger.Debug("Using virtual environment Python", "path", venvPython)
		}

		args := append([]string{entryPoint}, p.manifest.Runtime.Args...)
		cmd = exec.CommandContext(m.ctx, pythonPath, args...)
	case "node":
		entryPoint := filepath.Join(p.dir, p.manifest.Runtime.GetEntryPoint())
		args := append([]string{entryPoint}, p.manifest.Runtime.Args...)
		cmd = exec.CommandContext(m.ctx, "node", args...)
	default:
		p.state = PluginStateError
		p.lastError = fmt.Sprintf("unsupported runtime type: %s", p.manifest.Runtime.Type)
		return fmt.Errorf("unsupported runtime type: %s", p.manifest.Runtime.Type)
	}

	pluginDataDir := filepath.Join(m.dataDir, p.manifest.ID)
	_ = os.MkdirAll(pluginDataDir, 0755)

	cmd.Env = append(os.Environ(),
		fmt.Sprintf("NVR_PLUGIN_ID=%s", p.manifest.ID),
		fmt.Sprintf("NVR_PLUGIN_DIR=%s", p.dir),
		fmt.Sprintf("NVR_DATA_DIR=%s", pluginDataDir),
	)
	// Add plugin-specific environment variables from manifest
	if len(p.manifest.Runtime.Env) > 0 {
		cmd.Env = append(cmd.Env, p.manifest.Runtime.Env...)
		m.logger.Debug("Added plugin env vars", "id", p.manifest.ID, "count", len(p.manifest.Runtime.Env))
	}
	cmd.Dir = p.dir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		p.state = PluginStateError
		p.lastError = err.Error()
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		p.state = PluginStateError
		p.lastError = err.Error()
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		p.state = PluginStateError
		p.lastError = err.Error()
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		p.state = PluginStateError
		p.lastError = err.Error()
		return fmt.Errorf("failed to start plugin: %w", err)
	}

	p.cmd = cmd
	client := NewPluginClient(stdin, stdout, stderr)
	p.proxy = NewPluginProxy(client)

	go m.logPluginStderr(p.manifest.ID, stderr)

	pluginConfig := m.getPluginConfigMap(p.manifest.ID)
	initCtx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	defer cancel()

	if err := p.proxy.Initialize(initCtx, pluginConfig); err != nil {
		p.state = PluginStateError
		p.lastError = err.Error()
		_ = cmd.Process.Kill()
		return fmt.Errorf("failed to initialize plugin: %w", err)
	}

	now := time.Now()
	p.startedAt = &now
	p.state = PluginStateRunning
	p.enabled = true

	go m.syncExternalPluginCameras(p)
	go m.monitorPluginProcess(p)

	m.logger.Info("External plugin started", "id", p.manifest.ID)
	return nil
}

// StopExternalPlugin stops an external plugin
func (m *Manager) StopExternalPlugin(id string) error {
	m.mu.RLock()
	p, ok := m.installed[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("plugin not installed: %s", id)
	}

	return m.stopExternalPlugin(p)
}

func (m *Manager) stopExternalPlugin(p *installedPlugin) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state != PluginStateRunning {
		return nil
	}

	p.state = PluginStateStopping
	m.logger.Info("Stopping external plugin", "id", p.manifest.ID)

	if p.proxy != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = p.proxy.Shutdown(shutdownCtx)
		cancel()
		_ = p.proxy.Close()
		p.proxy = nil
	}

	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
		_, _ = p.cmd.Process.Wait()
		p.cmd = nil
	}

	for cameraID := range p.cameras {
		fullID := fmt.Sprintf("%s_%s", p.manifest.ID, cameraID)
		_ = m.go2rtc.RemoveStream(fullID)
	}
	p.cameras = make(map[string]*PluginCamera)

	m.mu.Lock()
	prefix := p.manifest.ID + "_"
	for id := range m.cameras {
		if len(id) > len(prefix) && id[:len(prefix)] == prefix {
			delete(m.cameras, id)
		}
	}
	m.mu.Unlock()

	p.state = PluginStateStopped
	p.startedAt = nil

	m.logger.Info("External plugin stopped", "id", p.manifest.ID)
	return nil
}

func (m *Manager) logPluginStderr(pluginID string, stderr io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := stderr.Read(buf)
		if n > 0 {
			output := string(buf[:n])
			m.logger.Debug("Plugin stderr", "id", pluginID, "output", output)

			// Store log in plugin's buffer
			m.mu.RLock()
			p, ok := m.installed[pluginID]
			m.mu.RUnlock()

			if ok {
				entry := LogEntry{
					Timestamp: time.Now(),
					Level:     "info",
					Message:   output,
				}
				p.logMu.Lock()
				p.logBuffer = append(p.logBuffer, entry)
				// Keep only last 1000 entries
				if len(p.logBuffer) > 1000 {
					p.logBuffer = p.logBuffer[len(p.logBuffer)-1000:]
				}
				p.logMu.Unlock()
			}
		}
		if err != nil {
			break
		}
	}
}

func (m *Manager) monitorPluginProcess(p *installedPlugin) {
	if p.cmd == nil {
		return
	}

	err := p.cmd.Wait()

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == PluginStateRunning {
		p.state = PluginStateError
		if err != nil {
			p.lastError = err.Error()
		} else {
			p.lastError = "plugin exited unexpectedly"
		}
		m.logger.Error("Plugin crashed", "id", p.manifest.ID, "error", p.lastError)

		if p.proxy != nil {
			_ = p.proxy.Close()
			p.proxy = nil
		}
	}
}

func (m *Manager) syncExternalPluginCameras(p *installedPlugin) {
	p.mu.RLock()
	if p.proxy == nil {
		p.mu.RUnlock()
		return
	}
	proxy := p.proxy
	pluginID := p.manifest.ID
	p.mu.RUnlock()

	ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	defer cancel()

	cameras, err := proxy.ListCameras(ctx)
	if err != nil {
		m.logger.Warn("Failed to list cameras from plugin", "id", pluginID, "error", err)
		return
	}

	p.mu.Lock()
	for _, cam := range cameras {
		cam.PluginID = pluginID
		camCopy := cam
		p.cameras[cam.ID] = &camCopy

		fullID := fmt.Sprintf("%s_%s", pluginID, cam.ID)
		if cam.MainStream != "" {
			if err := m.go2rtc.AddStream(fullID, cam.MainStream); err != nil {
				m.logger.Warn("Failed to register camera stream", "camera", fullID, "error", err)
			} else {
				m.logger.Info("Registered camera stream", "camera", fullID)
			}
		}

		m.mu.Lock()
		m.cameras[fullID] = &camCopy
		m.mu.Unlock()
	}
	p.mu.Unlock()

	m.logger.Info("Synced cameras from external plugin", "id", pluginID, "count", len(cameras))
}

// ListInstalledPlugins returns status for all installed external plugins
func (m *Manager) ListInstalledPlugins() []PluginStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]PluginStatus, 0, len(m.installed))
	for _, p := range m.installed {
		p.mu.RLock()
		status := PluginStatus{
			Manifest:    p.manifest,
			State:       p.state,
			Enabled:     p.enabled,
			CameraCount: len(p.cameras),
			LastError:   p.lastError,
			StartedAt:   p.startedAt,
		}
		p.mu.RUnlock()
		result = append(result, status)
	}
	return result
}

// PluginsDir returns the plugins directory path
func (m *Manager) PluginsDir() string {
	return m.pluginsDir
}

// GetInstalledPluginLogs returns the last n log entries for an installed plugin
func (m *Manager) GetInstalledPluginLogs(pluginID string, n int) []LogEntry {
	m.mu.RLock()
	p, ok := m.installed[pluginID]
	m.mu.RUnlock()

	if !ok {
		return nil
	}

	p.logMu.RLock()
	defer p.logMu.RUnlock()

	if n <= 0 || n > len(p.logBuffer) {
		n = len(p.logBuffer)
	}

	if n == 0 {
		return []LogEntry{}
	}

	// Return the last n entries
	start := len(p.logBuffer) - n
	if start < 0 {
		start = 0
	}

	result := make([]LogEntry, n)
	copy(result, p.logBuffer[start:])
	return result
}

// ============================================================================
// Installer Methods (delegated to Installer)
// ============================================================================

// InstallFromGitHub installs a plugin from a GitHub repository
func (m *Manager) InstallFromGitHub(ctx context.Context, repoURL string) (*PluginManifest, error) {
	manifest, err := m.installer.InstallFromGitHub(ctx, repoURL)
	if err != nil {
		return nil, err
	}

	// Rescan plugins to pick up the newly installed one
	if err := m.ScanPlugins(); err != nil {
		m.logger.Warn("Failed to rescan plugins after install", "error", err)
	}

	return manifest, nil
}

// UpdatePlugin updates a plugin to the latest version
func (m *Manager) UpdatePlugin(ctx context.Context, pluginID string) (*PluginManifest, error) {
	// Stop the plugin if running
	if err := m.StopExternalPlugin(pluginID); err != nil {
		m.logger.Debug("Plugin not running or failed to stop", "id", pluginID, "error", err)
	}

	manifest, err := m.installer.UpdatePlugin(ctx, pluginID)
	if err != nil {
		return nil, err
	}

	// Rescan and optionally restart
	if err := m.ScanPlugins(); err != nil {
		m.logger.Warn("Failed to rescan plugins after update", "error", err)
	}

	return manifest, nil
}

// UninstallPlugin removes a plugin
func (m *Manager) UninstallPlugin(pluginID string) error {
	// Stop the plugin if running
	if err := m.StopExternalPlugin(pluginID); err != nil {
		m.logger.Debug("Plugin not running or failed to stop", "id", pluginID, "error", err)
	}

	// Remove from installed list
	m.mu.Lock()
	delete(m.installed, pluginID)
	m.mu.Unlock()

	return m.installer.UninstallPlugin(pluginID)
}

// GetTrackedRepos returns all tracked repositories for update checking
func (m *Manager) GetTrackedRepos() []TrackedRepo {
	return m.installer.GetTrackedRepos()
}

// GetInstaller returns the plugin installer (for advanced use)
func (m *Manager) GetInstaller() *Installer {
	return m.installer
}

// ============================================================================
// Catalog Methods
// ============================================================================

// GetCatalog returns the plugin catalog
func (m *Manager) GetCatalog() *Catalog {
	return m.catalog
}

// GetCatalogWithStatus returns catalog plugins with installation status
func (m *Manager) GetCatalogWithStatus() []CatalogPluginStatus {
	if m.catalog == nil {
		return []CatalogPluginStatus{}
	}

	catalogPlugins := m.catalog.GetPlugins()
	result := make([]CatalogPluginStatus, 0, len(catalogPlugins))

	// Get tracked repos for version info
	trackedRepos := m.installer.GetTrackedRepos()
	repoVersions := make(map[string]TrackedRepo)
	for _, repo := range trackedRepos {
		repoVersions[repo.PluginID] = repo
	}

	for _, cp := range catalogPlugins {
		status := CatalogPluginStatus{
			CatalogPlugin: cp,
			Installed:     false,
			Enabled:       false,
		}

		// Check if it's a builtin plugin
		m.mu.RLock()
		if bp, ok := m.builtinPlugins[cp.ID]; ok {
			status.Installed = true
			status.InstalledVersion = bp.Manifest().Version
			status.LatestVersion = bp.Manifest().Version
			_, enabled := m.getPluginConfigLocked(cp.ID)
			status.Enabled = enabled
			status.State = string(PluginStateRunning)
		}
		// Check if it's an installed external plugin
		if ip, ok := m.installed[cp.ID]; ok {
			status.Installed = true
			status.InstalledVersion = ip.manifest.Version
			status.Enabled = ip.enabled
			status.State = string(ip.state)
		}
		m.mu.RUnlock()

		// Check tracked repo for version info (only for non-builtin plugins)
		// Builtin plugins should use their embedded version, not tracked repo versions
		m.mu.RLock()
		_, isBuiltin := m.builtinPlugins[cp.ID]
		m.mu.RUnlock()
		if repo, ok := repoVersions[cp.ID]; ok && !isBuiltin {
			status.InstalledVersion = repo.InstalledTag
			status.LatestVersion = repo.LatestTag
			status.UpdateAvailable = repo.UpdateAvailable
		}

		result = append(result, status)
	}

	return result
}

// InstallFromCatalog installs a plugin from the catalog by ID
func (m *Manager) InstallFromCatalog(ctx context.Context, pluginID string) (*PluginManifest, error) {
	if m.catalog == nil {
		return nil, fmt.Errorf("catalog not loaded")
	}

	plugin := m.catalog.GetPlugin(pluginID)
	if plugin == nil {
		return nil, fmt.Errorf("plugin not found in catalog: %s", pluginID)
	}

	return m.InstallAndStart(ctx, plugin.Repo)
}

// InstallAndStart installs a plugin and starts it immediately (hot-reload)
func (m *Manager) InstallAndStart(ctx context.Context, repoURL string) (*PluginManifest, error) {
	manifest, err := m.installer.InstallFromGitHub(ctx, repoURL)
	if err != nil {
		return nil, err
	}

	// Rescan to pick up the new plugin
	if err := m.ScanPlugins(); err != nil {
		m.logger.Warn("Failed to rescan plugins after install", "error", err)
	}

	// Auto-enable and start the plugin
	m.mu.RLock()
	plugin, ok := m.installed[manifest.ID]
	m.mu.RUnlock()

	if ok {
		// Update config to enable
		if m.config.Plugins == nil {
			m.config.Plugins = make(config.PluginsConfig)
		}
		pluginCfg := m.config.Plugins[manifest.ID]
		pluginCfg.Enabled = true
		m.config.Plugins[manifest.ID] = pluginCfg
		_ = m.config.Save()

		// Start the plugin
		if err := m.startExternalPlugin(plugin); err != nil {
			m.logger.Warn("Failed to auto-start plugin", "id", manifest.ID, "error", err)
		} else {
			m.logger.Info("Plugin installed and started (hot-reload)", "id", manifest.ID)
		}
	}

	return manifest, nil
}

// ReloadCatalog reloads the plugin catalog from disk
func (m *Manager) ReloadCatalog() error {
	catalog, err := LoadCatalogFromDir(m.catalogDir)
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.catalog = catalog
	m.mu.Unlock()

	m.logger.Info("Plugin catalog reloaded", "plugins", len(catalog.Plugins))
	return nil
}
