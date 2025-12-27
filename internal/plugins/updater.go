package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Spatial-NVR/SpatialNVR/sdk"
)

const (
	versionsURL   = "https://raw.githubusercontent.com/Spatial-NVR/SpatialNVR/main/.versions/versions.json"
	checkInterval = 24 * time.Hour
	updateTimeout = 5 * time.Minute
)

// VersionInfo contains version information for a plugin
type VersionInfo struct {
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
}

// VersionsIndex contains all plugin version information
type VersionsIndex struct {
	CoreVersion string                 `json:"core_version"`
	UpdatedAt   string                 `json:"updated_at"`
	Plugins     map[string]VersionInfo `json:"plugins"`
}

// UpdateInfo contains information about an available update
type UpdateInfo struct {
	PluginID       string `json:"plugin_id"`
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	DownloadURL    string `json:"download_url"`
	CanAutoUpdate  bool   `json:"can_auto_update"`
}

// PluginInfo is a simplified plugin info for the updater
type PluginInfo struct {
	ID      string
	Version string
	Type    string // "builtin" or "external"
}

// PluginManager interface for interacting with the plugin system
type PluginManager interface {
	ListPluginInfo() []PluginInfo
	StopPlugin(id string) error
	StartPlugin(id string) error
	ReloadPlugin(id string) error
}

// Updater handles checking for and applying plugin updates
type Updater struct {
	manager      PluginManager
	pluginsPath  string
	httpClient   *http.Client
	mu           sync.RWMutex
	lastCheck    time.Time
	cachedIndex  *VersionsIndex
	updates      map[string]*UpdateInfo
	autoUpdate   bool
	logger       *slog.Logger
	stopCh       chan struct{}
}

// NewUpdater creates a new plugin updater
func NewUpdater(manager PluginManager, pluginsPath string, autoUpdate bool) *Updater {
	return &Updater{
		manager:     manager,
		pluginsPath: pluginsPath,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		updates:    make(map[string]*UpdateInfo),
		autoUpdate: autoUpdate,
		logger:     slog.Default().With("component", "plugin-updater"),
		stopCh:     make(chan struct{}),
	}
}

// Start begins periodic update checking
func (u *Updater) Start(ctx context.Context) {
	u.logger.Info("Starting plugin update checker", "interval", checkInterval)

	// Initial check after a short delay
	go func() {
		time.Sleep(30 * time.Second) // Wait for system to stabilize
		if err := u.CheckForUpdates(ctx); err != nil {
			u.logger.Error("Initial update check failed", "error", err)
		}
	}()

	// Periodic checks
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-u.stopCh:
			return
		case <-ticker.C:
			if err := u.CheckForUpdates(ctx); err != nil {
				u.logger.Error("Update check failed", "error", err)
			}
		}
	}
}

// Stop stops the update checker
func (u *Updater) Stop() {
	close(u.stopCh)
}

// CheckForUpdates checks for available plugin updates
func (u *Updater) CheckForUpdates(ctx context.Context) error {
	u.logger.Info("Checking for plugin updates")

	// Fetch versions index
	index, err := u.fetchVersionsIndex(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch versions index: %w", err)
	}

	u.mu.Lock()
	u.cachedIndex = index
	u.lastCheck = time.Now()
	u.mu.Unlock()

	// Compare with installed versions
	installed := u.manager.ListPluginInfo()
	updates := make(map[string]*UpdateInfo)

	for _, plugin := range installed {
		if latestInfo, ok := index.Plugins[plugin.ID]; ok {
			if isNewerVersion(latestInfo.Version, plugin.Version) {
				update := &UpdateInfo{
					PluginID:       plugin.ID,
					CurrentVersion: plugin.Version,
					LatestVersion:  latestInfo.Version,
					DownloadURL:    latestInfo.DownloadURL,
					CanAutoUpdate:  plugin.Type == "external", // Only external plugins can auto-update
				}
				updates[plugin.ID] = update
				u.logger.Info("Update available",
					"plugin", plugin.ID,
					"current", plugin.Version,
					"latest", latestInfo.Version,
				)
			}
		}
	}

	u.mu.Lock()
	u.updates = updates
	u.mu.Unlock()

	// Auto-update if enabled
	if u.autoUpdate {
		for _, update := range updates {
			if update.CanAutoUpdate {
				if err := u.ApplyUpdate(ctx, update.PluginID); err != nil {
					u.logger.Error("Auto-update failed", "plugin", update.PluginID, "error", err)
				}
			}
		}
	}

	return nil
}

// GetAvailableUpdates returns all available updates
func (u *Updater) GetAvailableUpdates() []*UpdateInfo {
	u.mu.RLock()
	defer u.mu.RUnlock()

	updates := make([]*UpdateInfo, 0, len(u.updates))
	for _, update := range u.updates {
		updates = append(updates, update)
	}
	return updates
}

// GetLastCheck returns the time of the last update check
func (u *Updater) GetLastCheck() time.Time {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.lastCheck
}

// GetVersionsIndex returns the cached versions index
func (u *Updater) GetVersionsIndex() *VersionsIndex {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.cachedIndex
}

// ApplyUpdate downloads and applies an update for a plugin
func (u *Updater) ApplyUpdate(ctx context.Context, pluginID string) error {
	u.mu.RLock()
	update, ok := u.updates[pluginID]
	u.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no update available for plugin %s", pluginID)
	}

	if !update.CanAutoUpdate {
		return fmt.Errorf("plugin %s cannot be auto-updated (builtin plugin)", pluginID)
	}

	u.logger.Info("Applying update", "plugin", pluginID, "version", update.LatestVersion)

	// Create timeout context
	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	// Download plugin
	tempFile, err := u.downloadPlugin(ctx, update.DownloadURL)
	if err != nil {
		return fmt.Errorf("failed to download plugin: %w", err)
	}
	defer os.Remove(tempFile)

	// Stop the plugin if running
	if err := u.manager.StopPlugin(pluginID); err != nil {
		u.logger.Warn("Failed to stop plugin before update", "plugin", pluginID, "error", err)
	}

	// Extract to plugins directory
	pluginDir := filepath.Join(u.pluginsPath, pluginID)
	if err := u.extractPlugin(tempFile, pluginDir); err != nil {
		return fmt.Errorf("failed to extract plugin: %w", err)
	}

	// Reload the plugin
	if err := u.manager.ReloadPlugin(pluginID); err != nil {
		return fmt.Errorf("failed to reload plugin: %w", err)
	}

	// Remove from updates list
	u.mu.Lock()
	delete(u.updates, pluginID)
	u.mu.Unlock()

	u.logger.Info("Update applied successfully", "plugin", pluginID, "version", update.LatestVersion)
	return nil
}

func (u *Updater) fetchVersionsIndex(ctx context.Context) (*VersionsIndex, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", versionsURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var index VersionsIndex
	if err := json.NewDecoder(resp.Body).Decode(&index); err != nil {
		return nil, err
	}

	return &index, nil
}

func (u *Updater) downloadPlugin(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	// Create temp file
	tmpFile, err := os.CreateTemp("", "plugin-*.tar.gz")
	if err != nil {
		return "", err
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", err
	}

	tmpFile.Close()
	return tmpFile.Name(), nil
}

func (u *Updater) extractPlugin(archivePath, destDir string) error {
	// Backup existing plugin
	backupDir := destDir + ".backup"
	if _, err := os.Stat(destDir); err == nil {
		os.RemoveAll(backupDir)
		if err := os.Rename(destDir, backupDir); err != nil {
			return fmt.Errorf("failed to backup existing plugin: %w", err)
		}
	}

	// Create destination directory
	if err := os.MkdirAll(destDir, 0755); err != nil {
		// Restore backup on failure
		if _, err := os.Stat(backupDir); err == nil {
			os.Rename(backupDir, destDir)
		}
		return err
	}

	// Extract archive using tar command
	cmd := exec.Command("tar", "-xzf", archivePath, "-C", destDir, "--strip-components=1")
	if err := cmd.Run(); err != nil {
		// Restore backup on failure
		os.RemoveAll(destDir)
		if _, err := os.Stat(backupDir); err == nil {
			os.Rename(backupDir, destDir)
		}
		return fmt.Errorf("failed to extract archive: %w", err)
	}

	// Remove backup on success
	os.RemoveAll(backupDir)
	return nil
}

// isNewerVersion compares semantic versions
// Returns true if latest is newer than current
func isNewerVersion(latest, current string) bool {
	// Normalize versions
	latest = strings.TrimPrefix(latest, "v")
	current = strings.TrimPrefix(current, "v")

	latestParts := strings.Split(latest, ".")
	currentParts := strings.Split(current, ".")

	// Compare each part
	maxLen := len(latestParts)
	if len(currentParts) > maxLen {
		maxLen = len(currentParts)
	}

	for i := 0; i < maxLen; i++ {
		latestNum := 0
		currentNum := 0

		if i < len(latestParts) {
			fmt.Sscanf(latestParts[i], "%d", &latestNum)
		}
		if i < len(currentParts) {
			fmt.Sscanf(currentParts[i], "%d", &currentNum)
		}

		if latestNum > currentNum {
			return true
		}
		if latestNum < currentNum {
			return false
		}
	}

	return false
}

// HTTP Handlers for plugin updates API

// UpdatesHandler handles GET /api/v1/plugins/updates
type UpdatesHandler struct {
	updater *Updater
}

// NewUpdatesHandler creates a new updates handler
func NewUpdatesHandler(updater *Updater) *UpdatesHandler {
	return &UpdatesHandler{updater: updater}
}

// ServeHTTP handles the request
func (h *UpdatesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.getUpdates(w, r)
	case http.MethodPost:
		h.checkUpdates(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *UpdatesHandler) getUpdates(w http.ResponseWriter, r *http.Request) {
	updates := h.updater.GetAvailableUpdates()
	lastCheck := h.updater.GetLastCheck()

	response := map[string]interface{}{
		"updates":    updates,
		"last_check": lastCheck,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *UpdatesHandler) checkUpdates(w http.ResponseWriter, r *http.Request) {
	if err := h.updater.CheckForUpdates(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.getUpdates(w, r)
}

// ApplyUpdateHandler handles POST /api/v1/plugins/{id}/update
type ApplyUpdateHandler struct {
	updater *Updater
}

// NewApplyUpdateHandler creates a new apply update handler
func NewApplyUpdateHandler(updater *Updater) *ApplyUpdateHandler {
	return &ApplyUpdateHandler{updater: updater}
}

// Handle applies an update for a specific plugin
func (h *ApplyUpdateHandler) Handle(w http.ResponseWriter, r *http.Request, pluginID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := h.updater.ApplyUpdate(r.Context(), pluginID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "updated",
	})
}

// PluginLoaderAdapter adapts the core.PluginLoader to the PluginManager interface
type PluginLoaderAdapter struct {
	listFunc   func() []sdk.PluginManifest
	stopFunc   func(id string) error
	startFunc  func(id string) error
	reloadFunc func(id string) error
}

// NewPluginLoaderAdapter creates a new adapter
func NewPluginLoaderAdapter(
	listFunc func() []sdk.PluginManifest,
	stopFunc func(id string) error,
	startFunc func(id string) error,
	reloadFunc func(id string) error,
) *PluginLoaderAdapter {
	return &PluginLoaderAdapter{
		listFunc:   listFunc,
		stopFunc:   stopFunc,
		startFunc:  startFunc,
		reloadFunc: reloadFunc,
	}
}

// ListPluginInfo returns plugin info from manifests
func (a *PluginLoaderAdapter) ListPluginInfo() []PluginInfo {
	manifests := a.listFunc()
	info := make([]PluginInfo, 0, len(manifests))
	for _, m := range manifests {
		info = append(info, PluginInfo{
			ID:      m.ID,
			Version: m.Version,
			Type:    "plugin", // Type is determined by how it was loaded, not in manifest
		})
	}
	return info
}

// StopPlugin stops a plugin
func (a *PluginLoaderAdapter) StopPlugin(id string) error {
	return a.stopFunc(id)
}

// StartPlugin starts a plugin
func (a *PluginLoaderAdapter) StartPlugin(id string) error {
	return a.startFunc(id)
}

// ReloadPlugin reloads a plugin
func (a *PluginLoaderAdapter) ReloadPlugin(id string) error {
	if err := a.stopFunc(id); err != nil {
		return err
	}
	return a.startFunc(id)
}
