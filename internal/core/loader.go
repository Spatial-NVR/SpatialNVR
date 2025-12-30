package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

	// Group plugins by dependency level for parallel start
	// Plugins with no dependencies can start in parallel
	// Plugins with dependencies wait for those to complete first
	levels := l.groupByDependencyLevel(order)

	for levelNum, levelPlugins := range levels {
		if len(levelPlugins) == 1 {
			// Single plugin, start directly
			pluginID := levelPlugins[0]
			if err := l.startPlugin(pluginID); err != nil {
				if l.isCritical(pluginID) {
					return fmt.Errorf("critical plugin failed to start: %s: %w", pluginID, err)
				}
				l.logger.Error("Plugin failed to start", "id", pluginID, "error", err)
			}
		} else {
			// Multiple plugins at this level, start in parallel
			l.logger.Debug("Starting plugins in parallel", "level", levelNum, "plugins", levelPlugins)

			var wg sync.WaitGroup
			errChan := make(chan error, len(levelPlugins))

			for _, pluginID := range levelPlugins {
				wg.Add(1)
				go func(id string) {
					defer wg.Done()
					if err := l.startPlugin(id); err != nil {
						if l.isCritical(id) {
							errChan <- fmt.Errorf("critical plugin failed to start: %s: %w", id, err)
						} else {
							l.logger.Error("Plugin failed to start", "id", id, "error", err)
						}
					}
				}(pluginID)
			}

			wg.Wait()
			close(errChan)

			// Check for critical failures
			for err := range errChan {
				if err != nil {
					return err
				}
			}
		}
	}

	// Start health check loop
	go l.healthCheckLoop()

	return nil
}

// groupByDependencyLevel groups plugins into levels where each level
// contains plugins whose dependencies are all in previous levels
func (l *PluginLoader) groupByDependencyLevel(order []string) [][]string {
	l.pluginsMu.RLock()
	defer l.pluginsMu.RUnlock()

	var levels [][]string
	started := make(map[string]bool)

	remaining := make([]string, len(order))
	copy(remaining, order)

	for len(remaining) > 0 {
		var currentLevel []string
		var nextRemaining []string

		for _, pluginID := range remaining {
			lp, ok := l.plugins[pluginID]
			if !ok {
				continue
			}

			// Check if all dependencies have been started
			allDepsStarted := true
			for _, dep := range lp.Manifest.Dependencies {
				if !started[dep] {
					allDepsStarted = false
					break
				}
			}

			if allDepsStarted {
				currentLevel = append(currentLevel, pluginID)
			} else {
				nextRemaining = append(nextRemaining, pluginID)
			}
		}

		if len(currentLevel) == 0 && len(nextRemaining) > 0 {
			// No progress made, break to avoid infinite loop
			// Add remaining to current level
			currentLevel = nextRemaining
			nextRemaining = nil
		}

		// Mark all in current level as started
		for _, id := range currentLevel {
			started[id] = true
		}

		levels = append(levels, currentLevel)
		remaining = nextRemaining
	}

	return levels
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
		_ = l.eventBus.PublishPluginError(pluginID, err)
		return fmt.Errorf("failed to initialize plugin: %w", err)
	}

	// Start plugin
	if err := lp.Plugin.Start(initCtx); err != nil {
		l.pluginsMu.Lock()
		lp.State = PluginStateError
		lp.LastError = err.Error()
		l.pluginsMu.Unlock()
		_ = l.eventBus.PublishPluginError(pluginID, err)
		return fmt.Errorf("failed to start plugin: %w", err)
	}

	now := time.Now()
	l.pluginsMu.Lock()
	lp.State = PluginStateRunning
	lp.StartedAt = &now
	lp.Runtime = runtime
	lp.LastError = ""
	l.pluginsMu.Unlock()

	_ = l.eventBus.PublishPluginStarted(pluginID, lp.Manifest.Version)
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

	// Allow stopping plugins that are running OR in error state
	if lp.State != PluginStateRunning && lp.State != PluginStateError {
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

	_ = l.eventBus.PublishPluginStopped(pluginID)
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

	l.logger.Debug("Scanning plugins directory", "dir", l.pluginsDir, "entries", len(entries))

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pluginDir := filepath.Join(l.pluginsDir, entry.Name())
		manifestPath := filepath.Join(pluginDir, "manifest.yaml")

		l.logger.Debug("Checking plugin directory", "dir", entry.Name(), "manifestPath", manifestPath)

		data, err := os.ReadFile(manifestPath)
		if err != nil {
			l.logger.Debug("Skipping directory (no manifest)", "dir", entry.Name(), "error", err)
			continue
		}

		var manifest sdk.PluginManifest
		if err := yaml.Unmarshal(data, &manifest); err != nil {
			l.logger.Warn("Invalid manifest", "dir", entry.Name(), "error", err)
			continue
		}

		l.logger.Debug("Found manifest", "dir", entry.Name(), "id", manifest.ID, "runtime", manifest.Runtime.Type)

		l.pluginsMu.Lock()
		if _, exists := l.plugins[manifest.ID]; exists {
			l.logger.Debug("Plugin already registered, skipping", "id", manifest.ID)
			l.pluginsMu.Unlock()
			continue
		}

		var binaryPath string
		var extPlugin *ExternalPlugin

		// Handle different runtime types
		switch manifest.Runtime.Type {
		case "python":
			// For Python plugins, run setup script if exists, then use python interpreter
			scriptPath := manifest.Runtime.Script
			if scriptPath == "" {
				scriptPath = manifest.Runtime.EntryPoint
			}
			if scriptPath == "" {
				l.logger.Warn("Python plugin missing script path", "id", manifest.ID)
				l.pluginsMu.Unlock()
				continue
			}

			// Run setup script if it exists (creates venv, installs deps)
			if manifest.Runtime.Setup != "" {
				setupPath := filepath.Join(pluginDir, manifest.Runtime.Setup)
				if _, err := os.Stat(setupPath); err == nil {
					l.logger.Info("Running plugin setup script", "id", manifest.ID, "setup", setupPath)
					setupCmd := exec.Command("/bin/sh", setupPath)
					setupCmd.Dir = pluginDir
					if output, err := setupCmd.CombinedOutput(); err != nil {
						l.logger.Warn("Plugin setup script failed", "id", manifest.ID, "error", err, "output", string(output))
						// Continue anyway - setup might have partially succeeded
					}
				}
			}

			// Check for venv python, fallback to system python
			venvPython := filepath.Join(pluginDir, "venv", "bin", "python")
			pythonPath := "python3"
			if _, err := os.Stat(venvPython); err == nil {
				pythonPath = venvPython
			}

			// For Python, the "binary" is actually the python interpreter + script
			fullScriptPath := filepath.Join(pluginDir, scriptPath)
			binaryPath = pythonPath // Store python path for reference
			extPlugin = NewExternalPluginWithArgs(manifest, pythonPath, []string{fullScriptPath}, pluginDir)

			l.logger.Info("Registered Python plugin", "id", manifest.ID, "version", manifest.Version, "python", pythonPath, "script", scriptPath)

		case "node":
			// For Node.js plugins
			scriptPath := manifest.Runtime.Script
			if scriptPath == "" {
				scriptPath = manifest.Runtime.EntryPoint
			}
			if scriptPath == "" {
				l.logger.Warn("Node plugin missing script path", "id", manifest.ID)
				l.pluginsMu.Unlock()
				continue
			}

			fullScriptPath := filepath.Join(pluginDir, scriptPath)
			binaryPath = "node"
			extPlugin = NewExternalPluginWithArgs(manifest, "node", []string{fullScriptPath}, pluginDir)

			l.logger.Info("Registered Node.js plugin", "id", manifest.ID, "version", manifest.Version, "script", scriptPath)

		default:
			// Binary plugins (go, binary, or unspecified)
			binaryPath = l.findPluginBinary(pluginDir, manifest.ID)
			if binaryPath == "" {
				l.logger.Warn("Plugin binary not found", "id", manifest.ID, "dir", pluginDir)
				l.pluginsMu.Unlock()
				continue
			}
			extPlugin = NewExternalPlugin(manifest, binaryPath, pluginDir)
			l.logger.Info("Registered external plugin", "id", manifest.ID, "version", manifest.Version, "binary", binaryPath)
		}

		// Register the plugin
		l.plugins[manifest.ID] = &LoadedPlugin{
			Manifest:   manifest,
			Plugin:     extPlugin,
			State:      PluginStateStopped,
			IsBuiltin:  false,
			BinaryPath: binaryPath,
		}
		l.pluginsMu.Unlock()
	}

	return nil
}

// findPluginBinary finds the executable binary in a plugin directory
func (l *PluginLoader) findPluginBinary(pluginDir string, pluginID string) string {
	// Platform suffix for current OS/arch (e.g., "-linux-amd64")
	platformSuffix := fmt.Sprintf("-%s-%s", runtime.GOOS, runtime.GOARCH)

	// Build candidates list with platform-specific binaries first
	baseCandidates := []string{
		pluginID,                              // e.g., "reolink"
		pluginID + "-plugin",                  // e.g., "reolink-plugin"
		filepath.Base(pluginDir),              // directory name
		filepath.Base(pluginDir) + "-plugin", // directory name + "-plugin"
	}

	// Check platform-specific binaries first, then generic ones
	var candidates []string
	for _, base := range baseCandidates {
		candidates = append(candidates, base+platformSuffix) // e.g., "reolink-plugin-linux-amd64"
	}
	candidates = append(candidates, baseCandidates...) // Then generic names

	for _, name := range candidates {
		path := filepath.Join(pluginDir, name)
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() && isExecutable(info) {
			l.logger.Debug("Found plugin binary", "path", path)
			return path
		}
	}

	// Fallback: look for any executable file (excluding scripts and common non-binaries)
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		name := entry.Name()
		// Skip directories, manifests, and shell scripts
		if entry.IsDir() || name == "manifest.yaml" || strings.HasSuffix(name, ".sh") ||
			strings.HasSuffix(name, ".py") || strings.HasSuffix(name, ".js") ||
			strings.HasSuffix(name, ".go") || strings.HasSuffix(name, ".yaml") ||
			strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".json") ||
			strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".txt") {
			continue
		}

		path := filepath.Join(pluginDir, name)
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() && isExecutable(info) {
			l.logger.Debug("Found plugin binary (fallback)", "path", path)
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
			_ = l.eventBus.Publish(SubjectPluginHealth, map[string]interface{}{
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
// If the plugin is already running (update case), it will be stopped and restarted (hot-reload).
func (l *PluginLoader) ScanAndStart(ctx context.Context, pluginID string) error {
	// Check if plugin is already running (update case)
	l.pluginsMu.RLock()
	existingPlugin, wasRunning := l.plugins[pluginID]
	if wasRunning && existingPlugin.State == PluginStateRunning {
		l.pluginsMu.RUnlock()
		l.logger.Info("Hot-reload: stopping existing plugin for update", "id", pluginID)
		if err := l.stopPlugin(pluginID); err != nil {
			l.logger.Warn("Failed to stop plugin during hot-reload", "id", pluginID, "error", err)
		}
		// Unregister so it can be re-scanned with new binary
		l.pluginsMu.Lock()
		delete(l.plugins, pluginID)
		l.pluginsMu.Unlock()
	} else {
		l.pluginsMu.RUnlock()
	}

	// Rescan external plugins to pick up newly installed/updated ones
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
	return l.unregisterPlugin(pluginID, false)
}

// ForceUnregisterPlugin removes a plugin from the loader even if it's running
// This is used during uninstallation when we need to clean up regardless of state
func (l *PluginLoader) ForceUnregisterPlugin(pluginID string) error {
	return l.unregisterPlugin(pluginID, true)
}

func (l *PluginLoader) unregisterPlugin(pluginID string, force bool) error {
	l.pluginsMu.Lock()
	defer l.pluginsMu.Unlock()

	lp, ok := l.plugins[pluginID]
	if !ok {
		return nil // Already not registered
	}

	if lp.IsBuiltin {
		return fmt.Errorf("cannot unregister builtin plugin: %s", pluginID)
	}

	// Check if plugin is still running (unless force is set)
	if !force && (lp.State == PluginStateRunning || lp.State == PluginStateStarting) {
		return fmt.Errorf("plugin is still running or starting, stop it first: %s", pluginID)
	}

	// If force and plugin is running, try to stop it first
	if force && lp.State == PluginStateRunning {
		l.logger.Warn("Force unregistering running plugin", "id", pluginID)
		// Stop the plugin without holding the lock (unlock temporarily)
		l.pluginsMu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = lp.Plugin.Stop(ctx)
		cancel()
		if lp.Runtime != nil {
			lp.Runtime.Stop()
		}
		l.pluginsMu.Lock()
	}

	delete(l.plugins, pluginID)
	l.logger.Info("Plugin unregistered", "id", pluginID, "force", force)
	return nil
}
