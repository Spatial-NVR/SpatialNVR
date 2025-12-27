package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Spatial-NVR/SpatialNVR/sdk"
)

// PluginLoader handles discovery, installation, and lifecycle of plugins
type PluginLoader struct {
	pluginsDir string
	eventBus   *EventBus
	db         *sql.DB
	logger     *slog.Logger

	// Registered plugins (both builtin and loaded)
	plugins   map[string]*LoadedPlugin
	pluginsMu sync.RWMutex

	// Plugin startup order (topologically sorted)
	startupOrder []string

	// Configuration for plugins
	pluginConfigs map[string]map[string]interface{}

	// Context for plugin lifecycle
	ctx    context.Context
	cancel context.CancelFunc
}

// LoadedPlugin represents a plugin that's been loaded
type LoadedPlugin struct {
	Manifest   sdk.PluginManifest
	Plugin     sdk.Plugin
	Runtime    *sdk.PluginRuntime
	State      PluginState
	StartedAt  *time.Time
	LastError  string
	IsBuiltin  bool
	BinaryPath string // For external plugins
}

// PluginState represents the current state of a plugin
type PluginState string

const (
	PluginStateStopped  PluginState = "stopped"
	PluginStateStarting PluginState = "starting"
	PluginStateRunning  PluginState = "running"
	PluginStateStopping PluginState = "stopping"
	PluginStateError    PluginState = "error"
)

// BundledCorePlugins are the core plugins that ship with NVR
// These are automatically installed and cannot be removed
var BundledCorePlugins = []string{
	"nvr-core-api",
	"nvr-core-events",
	"nvr-core-config",
	"nvr-recording",
	"nvr-streaming",
	"nvr-detection",
	// Camera-specific plugins (to be added):
	// "nvr-camera-reolink",
	// "nvr-camera-wyze",
}

// CriticalPlugins are plugins that NVR cannot run without
var CriticalPlugins = []string{
	"nvr-core-api",
	"nvr-core-events",
	"nvr-core-config",
}

// NewPluginLoader creates a new plugin loader
func NewPluginLoader(pluginsDir string, eventBus *EventBus, db *sql.DB, logger *slog.Logger) *PluginLoader {
	return &PluginLoader{
		pluginsDir:    pluginsDir,
		eventBus:      eventBus,
		db:            db,
		logger:        logger.With("component", "plugin-loader"),
		plugins:       make(map[string]*LoadedPlugin),
		pluginConfigs: make(map[string]map[string]interface{}),
	}
}

// RegisterBuiltinPlugin registers a built-in Go plugin
func (l *PluginLoader) RegisterBuiltinPlugin(p sdk.Plugin) error {
	manifest := p.Manifest()
	if manifest.ID == "" {
		return fmt.Errorf("plugin ID cannot be empty")
	}

	l.pluginsMu.Lock()
	defer l.pluginsMu.Unlock()

	if _, exists := l.plugins[manifest.ID]; exists {
		return fmt.Errorf("plugin already registered: %s", manifest.ID)
	}

	l.plugins[manifest.ID] = &LoadedPlugin{
		Manifest:  manifest,
		Plugin:    p,
		State:     PluginStateStopped,
		IsBuiltin: true,
	}

	l.logger.Info("Registered builtin plugin", "id", manifest.ID, "version", manifest.Version)
	return nil
}

// SetPluginConfig sets the configuration for a plugin
func (l *PluginLoader) SetPluginConfig(pluginID string, config map[string]interface{}) {
	l.pluginsMu.Lock()
	defer l.pluginsMu.Unlock()
	l.pluginConfigs[pluginID] = config
}

// SavePluginConfig persists a plugin's configuration to disk
func (l *PluginLoader) SavePluginConfig(pluginID string) error {
	l.pluginsMu.RLock()
	config, ok := l.pluginConfigs[pluginID]
	l.pluginsMu.RUnlock()

	if !ok {
		return fmt.Errorf("no config found for plugin %s", pluginID)
	}

	// Create config directory if needed
	configDir := filepath.Join(l.pluginsDir, ".configs")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write config to file
	configPath := filepath.Join(configDir, pluginID+".json")
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	l.logger.Info("Saved plugin config", "plugin", pluginID, "path", configPath)
	return nil
}

// LoadPluginConfigs loads all saved plugin configurations from disk
func (l *PluginLoader) LoadPluginConfigs() error {
	configDir := filepath.Join(l.pluginsDir, ".configs")
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		return nil // No configs to load
	}

	entries, err := os.ReadDir(configDir)
	if err != nil {
		return fmt.Errorf("failed to read config directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		pluginID := strings.TrimSuffix(entry.Name(), ".json")
		configPath := filepath.Join(configDir, entry.Name())

		data, err := os.ReadFile(configPath)
		if err != nil {
			l.logger.Warn("Failed to read plugin config", "plugin", pluginID, "error", err)
			continue
		}

		var config map[string]interface{}
		if err := json.Unmarshal(data, &config); err != nil {
			l.logger.Warn("Failed to parse plugin config", "plugin", pluginID, "error", err)
			continue
		}

		l.SetPluginConfig(pluginID, config)
		l.logger.Info("Loaded plugin config", "plugin", pluginID)
	}

	return nil
}

// Start initializes and starts all enabled plugins in dependency order
func (l *PluginLoader) Start(ctx context.Context) error {
	l.ctx, l.cancel = context.WithCancel(ctx)

	// Ensure plugins directory exists
	if err := os.MkdirAll(l.pluginsDir, 0755); err != nil {
		return fmt.Errorf("failed to create plugins directory: %w", err)
	}

	// Scan for external plugins
	if err := l.ScanExternalPlugins(); err != nil {
		l.logger.Warn("Failed to scan external plugins", "error", err)
	}

	// Build startup order based on dependencies
	order, err := l.buildStartupOrder()
	if err != nil {
		return fmt.Errorf("failed to build startup order: %w", err)
	}
	l.startupOrder = order

	l.logger.Info("Starting plugins", "order", order)

	// Start plugins in order
	for _, pluginID := range order {
		if err := l.startPlugin(pluginID); err != nil {
			if l.isCritical(pluginID) {
				return fmt.Errorf("critical plugin failed to start: %s: %w", pluginID, err)
			}
			l.logger.Error("Plugin failed to start", "id", pluginID, "error", err)
		}
	}

	// Start health check loop
	go l.healthCheckLoop()

	return nil
}

// Stop gracefully shuts down all plugins in reverse order
func (l *PluginLoader) Stop() error {
	if l.cancel != nil {
		l.cancel()
	}

	// Stop in reverse startup order
	l.pluginsMu.RLock()
	order := make([]string, len(l.startupOrder))
	copy(order, l.startupOrder)
	l.pluginsMu.RUnlock()

	// Reverse the order
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}

	var lastErr error
	for _, pluginID := range order {
		if err := l.stopPlugin(pluginID); err != nil {
			l.logger.Error("Failed to stop plugin", "id", pluginID, "error", err)
			lastErr = err
		}
	}

	return lastErr
}

// GetPlugin returns a loaded plugin by ID
func (l *PluginLoader) GetPlugin(id string) (*LoadedPlugin, bool) {
	l.pluginsMu.RLock()
	defer l.pluginsMu.RUnlock()
	p, ok := l.plugins[id]
	return p, ok
}

// ListPlugins returns all loaded plugins
func (l *PluginLoader) ListPlugins() []*LoadedPlugin {
	l.pluginsMu.RLock()
	defer l.pluginsMu.RUnlock()

	result := make([]*LoadedPlugin, 0, len(l.plugins))
	for _, p := range l.plugins {
		result = append(result, p)
	}
	return result
}

// startPlugin starts a single plugin
func (l *PluginLoader) startPlugin(pluginID string) error {
	l.pluginsMu.Lock()
	lp, ok := l.plugins[pluginID]
	if !ok {
		l.pluginsMu.Unlock()
		return fmt.Errorf("plugin not found: %s", pluginID)
	}

	if lp.State == PluginStateRunning {
		l.pluginsMu.Unlock()
		return nil
	}

	lp.State = PluginStateStarting
	isExternal := !lp.IsBuiltin
	l.pluginsMu.Unlock()

	l.logger.Info("Starting plugin", "id", pluginID)

	// Get plugin config
	config := l.pluginConfigs[pluginID]
	if config == nil {
		config = make(map[string]interface{})
	}

	// Create runtime
	runtime := sdk.NewPluginRuntime(pluginID, l.eventBus.Conn(), l.db, config, l.logger)

	// For external plugins, use a timeout to prevent hanging
	var initCtx context.Context
	var initCancel context.CancelFunc
	if isExternal {
		initCtx, initCancel = context.WithTimeout(l.ctx, 30*time.Second)
		defer initCancel()
	} else {
		initCtx = l.ctx
	}

	// Initialize plugin
	if err := lp.Plugin.Initialize(initCtx, runtime); err != nil {
		l.pluginsMu.Lock()
		lp.State = PluginStateError
		lp.LastError = err.Error()
		l.pluginsMu.Unlock()
		l.eventBus.PublishPluginError(pluginID, err)
		return fmt.Errorf("failed to initialize plugin: %w", err)
	}

	// Start plugin
	if err := lp.Plugin.Start(initCtx); err != nil {
		l.pluginsMu.Lock()
		lp.State = PluginStateError
		lp.LastError = err.Error()
		l.pluginsMu.Unlock()
		l.eventBus.PublishPluginError(pluginID, err)
		return fmt.Errorf("failed to start plugin: %w", err)
	}

	now := time.Now()
	l.pluginsMu.Lock()
	lp.State = PluginStateRunning
	lp.StartedAt = &now
	lp.Runtime = runtime
	lp.LastError = ""
	l.pluginsMu.Unlock()

	l.eventBus.PublishPluginStarted(pluginID, lp.Manifest.Version)
	l.logger.Info("Plugin started", "id", pluginID, "version", lp.Manifest.Version)

	return nil
}

// stopPlugin stops a single plugin
func (l *PluginLoader) stopPlugin(pluginID string) error {
	l.pluginsMu.Lock()
	lp, ok := l.plugins[pluginID]
	if !ok {
		l.pluginsMu.Unlock()
		return nil
	}

	if lp.State != PluginStateRunning {
		l.pluginsMu.Unlock()
		return nil
	}

	lp.State = PluginStateStopping
	l.pluginsMu.Unlock()

	l.logger.Info("Stopping plugin", "id", pluginID)

	// Create timeout context
	stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop plugin
	if err := lp.Plugin.Stop(stopCtx); err != nil {
		l.logger.Warn("Plugin stop returned error", "id", pluginID, "error", err)
	}

	// Stop runtime
	if lp.Runtime != nil {
		lp.Runtime.Stop()
	}

	l.pluginsMu.Lock()
	lp.State = PluginStateStopped
	lp.StartedAt = nil
	lp.Runtime = nil
	l.pluginsMu.Unlock()

	l.eventBus.PublishPluginStopped(pluginID)
	l.logger.Info("Plugin stopped", "id", pluginID)

	return nil
}

// ScanExternalPlugins scans the plugins directory for external plugins
func (l *PluginLoader) ScanExternalPlugins() error {
	entries, err := os.ReadDir(l.pluginsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pluginDir := filepath.Join(l.pluginsDir, entry.Name())
		manifestPath := filepath.Join(pluginDir, "manifest.yaml")

		data, err := os.ReadFile(manifestPath)
		if err != nil {
			l.logger.Debug("Skipping directory (no manifest)", "dir", entry.Name())
			continue
		}

		var manifest sdk.PluginManifest
		if err := yaml.Unmarshal(data, &manifest); err != nil {
			l.logger.Warn("Invalid manifest", "dir", entry.Name(), "error", err)
			continue
		}

		l.pluginsMu.Lock()
		if _, exists := l.plugins[manifest.ID]; !exists {
			// Find the plugin binary
			binaryPath := l.findPluginBinary(pluginDir, manifest.ID)
			if binaryPath == "" {
				l.logger.Warn("Plugin binary not found", "id", manifest.ID, "dir", pluginDir)
				l.pluginsMu.Unlock()
				continue
			}

			// Create external plugin wrapper
			extPlugin := NewExternalPlugin(manifest, binaryPath)

			// Register the plugin
			l.plugins[manifest.ID] = &LoadedPlugin{
				Manifest:   manifest,
				Plugin:     extPlugin,
				State:      PluginStateStopped,
				IsBuiltin:  false,
				BinaryPath: binaryPath,
			}

			l.logger.Info("Registered external plugin", "id", manifest.ID, "version", manifest.Version, "binary", binaryPath)
		}
		l.pluginsMu.Unlock()
	}

	return nil
}

// findPluginBinary finds the executable binary in a plugin directory
func (l *PluginLoader) findPluginBinary(pluginDir string, pluginID string) string {
	// Common binary names to look for, in order of preference
	candidates := []string{
		pluginID,                                // e.g., "reolink"
		pluginID + "-plugin",                    // e.g., "reolink-plugin"
		filepath.Base(pluginDir),                // directory name
		filepath.Base(pluginDir) + "-plugin",   // directory name + "-plugin"
	}

	for _, name := range candidates {
		path := filepath.Join(pluginDir, name)
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() && isExecutable(info) {
			return path
		}
	}

	// Fallback: look for any executable file
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "manifest.yaml" {
			continue
		}

		path := filepath.Join(pluginDir, entry.Name())
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() && isExecutable(info) {
			return path
		}
	}

	return ""
}

// isExecutable checks if a file is executable
func isExecutable(info os.FileInfo) bool {
	mode := info.Mode()
	// Check if any execute bit is set
	return mode&0111 != 0
}

// buildStartupOrder builds a topologically sorted startup order
func (l *PluginLoader) buildStartupOrder() ([]string, error) {
	l.pluginsMu.RLock()
	defer l.pluginsMu.RUnlock()

	// Build dependency graph
	graph := make(map[string][]string)
	inDegree := make(map[string]int)

	// First, initialize all plugins in the graph
	for id := range l.plugins {
		graph[id] = []string{}
		inDegree[id] = 0
	}

	// Then add edges for dependencies
	for id, lp := range l.plugins {
		for _, dep := range lp.Manifest.Dependencies {
			// Check if dependency exists
			if _, exists := l.plugins[dep]; !exists {
				l.logger.Warn("Plugin has missing dependency, treating as optional",
					"plugin", id, "missing_dependency", dep)
				continue // Skip missing dependencies - they may be external/optional
			}
			graph[dep] = append(graph[dep], id)
			inDegree[id]++
		}
	}

	// Kahn's algorithm for topological sort
	var queue []string
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}

	// Sort queue for deterministic order
	sort.Strings(queue)

	var order []string
	for len(queue) > 0 {
		// Pop from queue
		id := queue[0]
		queue = queue[1:]
		order = append(order, id)

		// Reduce in-degree of dependents
		for _, dependent := range graph[id] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
				sort.Strings(queue) // Keep queue sorted
			}
		}
	}

	// Check for cycles
	if len(order) != len(l.plugins) {
		// Find which plugins weren't ordered (they have remaining dependencies)
		missing := []string{}
		for id := range l.plugins {
			found := false
			for _, orderedID := range order {
				if id == orderedID {
					found = true
					break
				}
			}
			if !found {
				missing = append(missing, id)
			}
		}
		return nil, fmt.Errorf("circular dependency detected involving plugins: %v", missing)
	}

	return order, nil
}

// isCritical checks if a plugin is critical
func (l *PluginLoader) isCritical(pluginID string) bool {
	for _, id := range CriticalPlugins {
		if id == pluginID {
			return true
		}
	}

	l.pluginsMu.RLock()
	defer l.pluginsMu.RUnlock()
	if lp, ok := l.plugins[pluginID]; ok {
		return lp.Manifest.Critical
	}
	return false
}

// healthCheckLoop periodically checks plugin health
func (l *PluginLoader) healthCheckLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-l.ctx.Done():
			return
		case <-ticker.C:
			l.checkHealth()
		}
	}
}

// checkHealth checks the health of all running plugins
func (l *PluginLoader) checkHealth() {
	l.pluginsMu.RLock()
	defer l.pluginsMu.RUnlock()

	for id, lp := range l.plugins {
		if lp.State != PluginStateRunning {
			continue
		}

		health := lp.Plugin.Health()
		if health.State != sdk.HealthStateHealthy {
			l.logger.Warn("Plugin unhealthy",
				"id", id,
				"state", health.State,
				"message", health.Message)

			// Publish health event
			l.eventBus.Publish(SubjectPluginHealth, map[string]interface{}{
				"plugin_id": id,
				"state":     health.State,
				"message":   health.Message,
				"timestamp": time.Now(),
			})
		}
	}
}

// RestartPlugin restarts a plugin
func (l *PluginLoader) RestartPlugin(ctx context.Context, pluginID string) error {
	if err := l.stopPlugin(pluginID); err != nil {
		return fmt.Errorf("failed to stop plugin: %w", err)
	}

	time.Sleep(100 * time.Millisecond) // Brief pause

	if err := l.startPlugin(pluginID); err != nil {
		return fmt.Errorf("failed to start plugin: %w", err)
	}

	return nil
}

// EnablePlugin enables and starts a plugin
func (l *PluginLoader) EnablePlugin(ctx context.Context, pluginID string) error {
	l.pluginsMu.RLock()
	lp, ok := l.plugins[pluginID]
	l.pluginsMu.RUnlock()

	if !ok {
		return fmt.Errorf("plugin not found: %s", pluginID)
	}

	if lp.State == PluginStateRunning {
		return nil
	}

	return l.startPlugin(pluginID)
}

// DisablePlugin stops and disables a plugin
func (l *PluginLoader) DisablePlugin(ctx context.Context, pluginID string) error {
	if l.isCritical(pluginID) {
		return fmt.Errorf("cannot disable critical plugin: %s", pluginID)
	}

	return l.stopPlugin(pluginID)
}

// ScanAndStart rescans the plugins directory for new plugins and starts the specified plugin.
// This is useful after installing a new plugin to make it immediately available without restart.
func (l *PluginLoader) ScanAndStart(ctx context.Context, pluginID string) error {
	// Rescan external plugins to pick up newly installed ones
	if err := l.ScanExternalPlugins(); err != nil {
		l.logger.Warn("Failed to scan external plugins", "error", err)
	}

	// Check if the plugin is now registered
	l.pluginsMu.RLock()
	_, ok := l.plugins[pluginID]
	l.pluginsMu.RUnlock()

	if !ok {
		return fmt.Errorf("plugin not found after scan: %s", pluginID)
	}

	// Start the plugin
	return l.startPlugin(pluginID)
}

// UnregisterPlugin removes a plugin from the loader (for uninstallation)
func (l *PluginLoader) UnregisterPlugin(pluginID string) error {
	l.pluginsMu.Lock()
	defer l.pluginsMu.Unlock()

	lp, ok := l.plugins[pluginID]
	if !ok {
		return nil // Already not registered
	}

	if lp.IsBuiltin {
		return fmt.Errorf("cannot unregister builtin plugin: %s", pluginID)
	}

	if lp.State == PluginStateRunning {
		return fmt.Errorf("plugin is still running, stop it first: %s", pluginID)
	}

	delete(l.plugins, pluginID)
	l.logger.Info("Plugin unregistered", "id", pluginID)
	return nil
}
