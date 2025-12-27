package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Component represents an updatable component
type Component struct {
	Name           string    `json:"name"`
	CurrentVersion string    `json:"current_version"`
	LatestVersion  string    `json:"latest_version,omitempty"`
	UpdateAvail    bool      `json:"update_available"`
	Repository     string    `json:"repository"`
	AssetPattern   string    `json:"asset_pattern"` // e.g., "spatialnvr-linux-{arch}"
	InstallPath    string    `json:"install_path"`
	LastChecked    time.Time `json:"last_checked"`
	AutoUpdate     bool      `json:"auto_update"`
}

// GitHubRelease represents a GitHub release API response
type GitHubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Prerelease  bool          `json:"prerelease"`
	Draft       bool          `json:"draft"`
	PublishedAt time.Time     `json:"published_at"`
	Assets      []GitHubAsset `json:"assets"`
}

// GitHubAsset represents a release asset
type GitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	ContentType        string `json:"content_type"`
}

// UpdateStatus represents the current update status
type UpdateStatus struct {
	Component   string    `json:"component"`
	Status      string    `json:"status"` // checking, downloading, extracting, installing, complete, error
	Progress    float64   `json:"progress"`
	Message     string    `json:"message"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
	Error       string    `json:"error,omitempty"`
}

// Config holds updater configuration
type Config struct {
	CheckInterval    time.Duration `json:"check_interval"`    // How often to check for updates
	AutoUpdate       bool          `json:"auto_update"`       // Enable automatic updates
	AutoUpdateTime   string        `json:"auto_update_time"`  // Time to apply auto-updates (e.g., "03:00")
	IncludePrereleases bool        `json:"include_prereleases"`
	DataPath         string        `json:"data_path"`         // Where to store downloaded updates
}

// Updater manages component updates
type Updater struct {
	config     Config
	components map[string]*Component
	mu         sync.RWMutex

	// Status tracking
	status   map[string]*UpdateStatus
	statusMu sync.RWMutex

	// Callbacks
	onUpdateAvailable func(component string, currentVersion, latestVersion string)
	onUpdateComplete  func(component string, version string)

	// HTTP client for GitHub API
	client *http.Client

	// Stop channel
	stopCh chan struct{}

	logger *slog.Logger
}

// NewUpdater creates a new updater instance
func NewUpdater(config Config, logger *slog.Logger) *Updater {
	if config.CheckInterval == 0 {
		config.CheckInterval = 6 * time.Hour
	}
	if config.DataPath == "" {
		config.DataPath = "/data/updates"
	}

	return &Updater{
		config:     config,
		components: make(map[string]*Component),
		status:     make(map[string]*UpdateStatus),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		stopCh: make(chan struct{}),
		logger: logger,
	}
}

// RegisterComponent registers a component for update tracking
func (u *Updater) RegisterComponent(c Component) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.components[c.Name] = &c
}

// SetOnUpdateAvailable sets the callback for when updates are available
func (u *Updater) SetOnUpdateAvailable(fn func(component, currentVersion, latestVersion string)) {
	u.onUpdateAvailable = fn
}

// SetOnUpdateComplete sets the callback for when an update completes
func (u *Updater) SetOnUpdateComplete(fn func(component, version string)) {
	u.onUpdateComplete = fn
}

// Start begins the update checking loop
func (u *Updater) Start(ctx context.Context) error {
	// Ensure data directory exists
	if err := os.MkdirAll(u.config.DataPath, 0755); err != nil {
		return fmt.Errorf("failed to create update directory: %w", err)
	}

	// Initial check
	go u.checkAllUpdates(ctx)

	// Periodic check
	go func() {
		ticker := time.NewTicker(u.config.CheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-u.stopCh:
				return
			case <-ticker.C:
				u.checkAllUpdates(ctx)

				// Check if we should auto-update
				if u.config.AutoUpdate {
					u.maybeAutoUpdate(ctx)
				}
			}
		}
	}()

	return nil
}

// Stop stops the updater
func (u *Updater) Stop() {
	close(u.stopCh)
}

// checkAllUpdates checks for updates on all registered components
func (u *Updater) checkAllUpdates(ctx context.Context) {
	u.mu.RLock()
	components := make([]*Component, 0, len(u.components))
	for _, c := range u.components {
		components = append(components, c)
	}
	u.mu.RUnlock()

	for _, c := range components {
		if err := u.CheckUpdate(ctx, c.Name); err != nil {
			u.logger.Error("Failed to check update", "component", c.Name, "error", err)
		}
	}
}

// CheckUpdate checks for updates for a specific component
func (u *Updater) CheckUpdate(ctx context.Context, componentName string) error {
	u.mu.RLock()
	component, ok := u.components[componentName]
	u.mu.RUnlock()

	if !ok {
		return fmt.Errorf("unknown component: %s", componentName)
	}

	u.setStatus(componentName, "checking", 0, "Checking for updates...")

	// Fetch latest release from GitHub
	release, err := u.getLatestRelease(ctx, component.Repository)
	if err != nil {
		u.setStatus(componentName, "error", 0, err.Error())
		return err
	}

	// Update component info
	u.mu.Lock()
	component.LastChecked = time.Now()
	component.LatestVersion = strings.TrimPrefix(release.TagName, "v")
	component.UpdateAvail = component.LatestVersion != component.CurrentVersion
	u.mu.Unlock()

	if component.UpdateAvail {
		u.setStatus(componentName, "available", 0,
			fmt.Sprintf("Update available: %s -> %s", component.CurrentVersion, component.LatestVersion))

		if u.onUpdateAvailable != nil {
			u.onUpdateAvailable(componentName, component.CurrentVersion, component.LatestVersion)
		}
	} else {
		u.setStatus(componentName, "current", 100, "Up to date")
	}

	return nil
}

// GetComponents returns all registered components
func (u *Updater) GetComponents() []Component {
	u.mu.RLock()
	defer u.mu.RUnlock()

	result := make([]Component, 0, len(u.components))
	for _, c := range u.components {
		result = append(result, *c)
	}
	return result
}

// GetUpdateStatus returns the current update status for a component
func (u *Updater) GetUpdateStatus(componentName string) *UpdateStatus {
	u.statusMu.RLock()
	defer u.statusMu.RUnlock()

	if status, ok := u.status[componentName]; ok {
		return status
	}
	return nil
}

// GetAllUpdateStatus returns status for all components
func (u *Updater) GetAllUpdateStatus() map[string]*UpdateStatus {
	u.statusMu.RLock()
	defer u.statusMu.RUnlock()

	result := make(map[string]*UpdateStatus, len(u.status))
	for k, v := range u.status {
		result[k] = v
	}
	return result
}

// Update downloads and installs an update for a component
func (u *Updater) Update(ctx context.Context, componentName string) error {
	u.mu.RLock()
	component, ok := u.components[componentName]
	u.mu.RUnlock()

	if !ok {
		return fmt.Errorf("unknown component: %s", componentName)
	}

	if !component.UpdateAvail {
		return fmt.Errorf("no update available for %s", componentName)
	}

	u.setStatus(componentName, "downloading", 0, "Fetching release info...")

	// Get release info
	release, err := u.getLatestRelease(ctx, component.Repository)
	if err != nil {
		u.setStatus(componentName, "error", 0, err.Error())
		return err
	}

	// Find the right asset
	asset := u.findAsset(release.Assets, component.AssetPattern)
	if asset == nil {
		err := fmt.Errorf("no matching asset found for pattern: %s", component.AssetPattern)
		u.setStatus(componentName, "error", 0, err.Error())
		return err
	}

	u.setStatus(componentName, "downloading", 10, fmt.Sprintf("Downloading %s...", asset.Name))

	// Download the asset
	downloadPath := filepath.Join(u.config.DataPath, asset.Name)
	if err := u.downloadAsset(ctx, asset.BrowserDownloadURL, downloadPath, componentName); err != nil {
		u.setStatus(componentName, "error", 0, err.Error())
		return err
	}

	u.setStatus(componentName, "extracting", 70, "Extracting update...")

	// Extract/install the update
	if err := u.installUpdate(ctx, componentName, downloadPath, component.InstallPath); err != nil {
		u.setStatus(componentName, "error", 0, err.Error())
		return err
	}

	// Update version info
	u.mu.Lock()
	component.CurrentVersion = component.LatestVersion
	component.UpdateAvail = false
	u.mu.Unlock()

	u.setStatus(componentName, "complete", 100,
		fmt.Sprintf("Updated to %s. Restart required.", component.LatestVersion))

	if u.onUpdateComplete != nil {
		u.onUpdateComplete(componentName, component.LatestVersion)
	}

	// Cleanup download
	os.Remove(downloadPath)

	return nil
}

// UpdateAll updates all components that have updates available
func (u *Updater) UpdateAll(ctx context.Context) error {
	u.mu.RLock()
	var toUpdate []string
	for name, c := range u.components {
		if c.UpdateAvail && c.AutoUpdate {
			toUpdate = append(toUpdate, name)
		}
	}
	u.mu.RUnlock()

	var errs []error
	for _, name := range toUpdate {
		if err := u.Update(ctx, name); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("some updates failed: %v", errs)
	}
	return nil
}

// getLatestRelease fetches the latest release from GitHub
func (u *Updater) getLatestRelease(ctx context.Context, repo string) (*GitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "SpatialNVR-Updater")

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to parse release: %w", err)
	}

	return &release, nil
}

// findAsset finds the matching asset for the current platform
func (u *Updater) findAsset(assets []GitHubAsset, pattern string) *GitHubAsset {
	// Replace {arch} and {os} in pattern
	arch := runtime.GOARCH
	goos := runtime.GOOS

	pattern = strings.ReplaceAll(pattern, "{arch}", arch)
	pattern = strings.ReplaceAll(pattern, "{os}", goos)

	for _, asset := range assets {
		if strings.Contains(asset.Name, pattern) {
			return &asset
		}
	}
	return nil
}

// downloadAsset downloads a release asset with progress tracking
func (u *Updater) downloadAsset(ctx context.Context, url, destPath, componentName string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := u.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Create destination file
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Track progress
	totalSize := resp.ContentLength
	var downloaded int64

	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := resp.Body.Read(buf)
		if n > 0 {
			_, writeErr := out.Write(buf[:n])
			if writeErr != nil {
				return writeErr
			}
			downloaded += int64(n)

			if totalSize > 0 {
				progress := 10 + (float64(downloaded)/float64(totalSize))*60 // 10-70%
				u.setStatus(componentName, "downloading", progress,
					fmt.Sprintf("Downloading... %d%%", int(progress)))
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	return nil
}

// installUpdate extracts and installs the update
func (u *Updater) installUpdate(ctx context.Context, componentName, archivePath, installPath string) error {
	// Determine archive type and extract
	if strings.HasSuffix(archivePath, ".tar.gz") || strings.HasSuffix(archivePath, ".tgz") {
		return u.extractTarGz(archivePath, installPath)
	} else if strings.HasSuffix(archivePath, ".zip") {
		return u.extractZip(archivePath, installPath)
	} else {
		// Assume it's a binary, just copy it
		return u.copyFile(archivePath, installPath)
	}
}

// extractTarGz extracts a tar.gz archive
func (u *Updater) extractTarGz(archivePath, destPath string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(destPath, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}

	return nil
}

// extractZip extracts a zip archive
func (u *Updater) extractZip(archivePath, destPath string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		target := filepath.Join(destPath, f.Name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(target, 0755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

// copyFile copies a single file
func (u *Updater) copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	// Make executable
	return os.Chmod(dst, 0755)
}

// setStatus updates the status for a component
func (u *Updater) setStatus(component, status string, progress float64, message string) {
	u.statusMu.Lock()
	defer u.statusMu.Unlock()

	s, ok := u.status[component]
	if !ok {
		s = &UpdateStatus{
			Component: component,
			StartedAt: time.Now(),
		}
		u.status[component] = s
	}

	s.Status = status
	s.Progress = progress
	s.Message = message

	if status == "complete" || status == "error" {
		s.CompletedAt = time.Now()
	}
	if status == "error" {
		s.Error = message
	}
}

// maybeAutoUpdate checks if it's time for auto-updates
func (u *Updater) maybeAutoUpdate(ctx context.Context) {
	if u.config.AutoUpdateTime == "" {
		return
	}

	// Parse auto-update time
	now := time.Now()
	parts := strings.Split(u.config.AutoUpdateTime, ":")
	if len(parts) != 2 {
		return
	}

	var hour, minute int
	fmt.Sscanf(parts[0], "%d", &hour)
	fmt.Sscanf(parts[1], "%d", &minute)

	// Check if we're within 5 minutes of the scheduled time
	if now.Hour() == hour && now.Minute() >= minute && now.Minute() < minute+5 {
		u.logger.Info("Running scheduled auto-update")
		if err := u.UpdateAll(ctx); err != nil {
			u.logger.Error("Auto-update failed", "error", err)
		}
	}
}

// NeedsRestart returns true if any component was updated and needs a restart
func (u *Updater) NeedsRestart() bool {
	u.statusMu.RLock()
	defer u.statusMu.RUnlock()

	for _, s := range u.status {
		if s.Status == "complete" {
			return true
		}
	}
	return false
}

// GetPendingUpdates returns components that have updates available
func (u *Updater) GetPendingUpdates() []Component {
	u.mu.RLock()
	defer u.mu.RUnlock()

	var result []Component
	for _, c := range u.components {
		if c.UpdateAvail {
			result = append(result, *c)
		}
	}
	return result
}
