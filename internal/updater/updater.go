package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"syscall"
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

// BackupInfo stores information about a backup for rollback
type BackupInfo struct {
	Component   string    `json:"component"`
	BackupPath  string    `json:"backup_path"`
	OriginalDir string    `json:"original_dir"`
	Version     string    `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
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
	CheckInterval      time.Duration `json:"check_interval"`      // How often to check for updates
	AutoUpdate         bool          `json:"auto_update"`         // Enable automatic updates
	AutoUpdateTime     string        `json:"auto_update_time"`    // Time to apply auto-updates (e.g., "03:00")
	IncludePrereleases bool          `json:"include_prereleases"`
	DataPath           string        `json:"data_path"`           // Where to store downloaded updates
	GitHubToken        string        `json:"github_token"`        // GitHub token for private repos
	ShutdownTimeout    time.Duration `json:"shutdown_timeout"`    // How long to wait for graceful shutdown
	HealthCheckURL     string        `json:"health_check_url"`    // URL to check after restart
	HealthCheckTimeout time.Duration `json:"health_check_timeout"` // How long to wait for health check
	MinDiskSpace       int64         `json:"min_disk_space"`      // Minimum free disk space in bytes
}

// Updater manages component updates
type Updater struct {
	config     Config
	components map[string]*Component
	mu         sync.RWMutex

	// Status tracking
	status   map[string]*UpdateStatus
	statusMu sync.RWMutex

	// Backup tracking for rollback
	backups   map[string]*BackupInfo
	backupsMu sync.RWMutex

	// Callbacks
	onUpdateAvailable func(component string, currentVersion, latestVersion string)
	onUpdateComplete  func(component string, version string)
	onRestartNeeded   func()

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
	if config.ShutdownTimeout == 0 {
		config.ShutdownTimeout = 30 * time.Second
	}
	if config.HealthCheckURL == "" {
		config.HealthCheckURL = "http://localhost:8080/health"
	}
	if config.HealthCheckTimeout == 0 {
		config.HealthCheckTimeout = 60 * time.Second
	}
	if config.MinDiskSpace == 0 {
		config.MinDiskSpace = 100 * 1024 * 1024 // 100MB minimum
	}

	return &Updater{
		config:     config,
		components: make(map[string]*Component),
		status:     make(map[string]*UpdateStatus),
		backups:    make(map[string]*BackupInfo),
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

// SetOnRestartNeeded sets the callback for when a restart is needed after updates
func (u *Updater) SetOnRestartNeeded(fn func()) {
	u.onRestartNeeded = fn
}

// SetGitHubToken updates the GitHub token dynamically
func (u *Updater) SetGitHubToken(token string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.config.GitHubToken = token
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

// Update downloads and installs an update for a component with atomic staging and rollback support
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

	// Pre-flight checks
	u.setStatus(componentName, "checking", 0, "Running pre-flight checks...")

	// Check disk space
	if err := u.checkDiskSpace(component.InstallPath); err != nil {
		u.setStatus(componentName, "error", 0, err.Error())
		return err
	}

	u.setStatus(componentName, "downloading", 5, "Fetching release info...")

	// Get release info
	release, err := u.getLatestRelease(ctx, component.Repository)
	if err != nil {
		u.setStatus(componentName, "error", 0, err.Error())
		return err
	}

	// Find the right asset and checksum
	asset := u.findAsset(release.Assets, component.AssetPattern)
	if asset == nil {
		err := fmt.Errorf("no matching asset found for pattern: %s", component.AssetPattern)
		u.setStatus(componentName, "error", 0, err.Error())
		return err
	}

	// Look for checksum file
	checksumAsset := u.findAsset(release.Assets, "checksums")
	var expectedChecksum string
	if checksumAsset != nil {
		expectedChecksum, err = u.fetchChecksum(ctx, checksumAsset.BrowserDownloadURL, asset.Name)
		if err != nil {
			u.logger.Warn("Could not fetch checksum, proceeding without verification", "error", err)
		}
	}

	u.setStatus(componentName, "downloading", 10, fmt.Sprintf("Downloading %s...", asset.Name))

	// Download to staging directory (atomic update preparation)
	stagingDir := filepath.Join(u.config.DataPath, "staging", componentName)
	if err := os.MkdirAll(stagingDir, 0755); err != nil {
		u.setStatus(componentName, "error", 0, fmt.Sprintf("Failed to create staging dir: %v", err))
		return err
	}
	defer os.RemoveAll(stagingDir) // Cleanup staging on exit

	downloadPath := filepath.Join(stagingDir, asset.Name)
	if err := u.downloadAsset(ctx, asset.BrowserDownloadURL, downloadPath, componentName); err != nil {
		u.setStatus(componentName, "error", 0, err.Error())
		return err
	}

	// Verify checksum if available
	if expectedChecksum != "" {
		u.setStatus(componentName, "verifying", 65, "Verifying checksum...")
		actualChecksum, err := u.calculateSHA256(downloadPath)
		if err != nil {
			u.setStatus(componentName, "error", 0, fmt.Sprintf("Checksum calculation failed: %v", err))
			return err
		}
		if actualChecksum != expectedChecksum {
			err := fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
			u.setStatus(componentName, "error", 0, err.Error())
			return err
		}
		u.logger.Info("Checksum verified", "component", componentName, "checksum", actualChecksum[:16]+"...")
	}

	u.setStatus(componentName, "extracting", 70, "Extracting update to staging...")

	// Extract to staging
	stagedInstallPath := filepath.Join(stagingDir, "install")
	if err := os.MkdirAll(stagedInstallPath, 0755); err != nil {
		u.setStatus(componentName, "error", 0, fmt.Sprintf("Failed to create staged install dir: %v", err))
		return err
	}

	if err := u.installUpdate(ctx, componentName, downloadPath, stagedInstallPath); err != nil {
		u.setStatus(componentName, "error", 0, err.Error())
		return err
	}

	u.setStatus(componentName, "backup", 80, "Creating backup...")

	// Create backup of current installation
	backupPath, err := u.createBackup(componentName, component.InstallPath, component.CurrentVersion)
	if err != nil {
		u.logger.Warn("Could not create backup, proceeding anyway", "error", err)
	} else {
		u.logger.Info("Backup created", "component", componentName, "path", backupPath)
	}

	u.setStatus(componentName, "installing", 90, "Atomically swapping to new version...")

	// Atomic swap: move staged files to final location
	if err := u.atomicSwap(stagedInstallPath, component.InstallPath); err != nil {
		u.setStatus(componentName, "error", 0, fmt.Sprintf("Atomic swap failed: %v", err))
		// Attempt rollback
		if backupPath != "" {
			if rollbackErr := u.Rollback(ctx, componentName); rollbackErr != nil {
				u.logger.Error("Rollback also failed", "error", rollbackErr)
			}
		}
		return err
	}

	// Update version info
	u.mu.Lock()
	component.CurrentVersion = component.LatestVersion
	component.UpdateAvail = false
	u.mu.Unlock()

	u.setStatus(componentName, "complete", 100,
		fmt.Sprintf("Updated to %s. Restarting...", component.LatestVersion))

	if u.onUpdateComplete != nil {
		u.onUpdateComplete(componentName, component.LatestVersion)
	}

	// Trigger restart if callback is set
	if u.onRestartNeeded != nil {
		u.onRestartNeeded()
	}

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

	// Add GitHub token for private repos if configured
	if u.config.GitHubToken != "" {
		req.Header.Set("Authorization", "Bearer "+u.config.GitHubToken)
	}

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

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

	// Add GitHub token for private repos if configured
	if u.config.GitHubToken != "" {
		req.Header.Set("Authorization", "Bearer "+u.config.GitHubToken)
		req.Header.Set("Accept", "application/octet-stream")
	}

	resp, err := u.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Create destination file
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

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
	defer func() { _ = file.Close() }()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer func() { _ = gzr.Close() }()

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
				_ = f.Close()
				return err
			}
			_ = f.Close()
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
	defer func() { _ = r.Close() }()

	for _, f := range r.File {
		target := filepath.Join(destPath, f.Name)

		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(target, 0755)
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
			_ = rc.Close()
			return err
		}

		_, err = io.Copy(out, rc)
		_ = rc.Close()
		_ = out.Close()
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
	defer func() { _ = in.Close() }()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

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
	_, _ = fmt.Sscanf(parts[0], "%d", &hour)
	_, _ = fmt.Sscanf(parts[1], "%d", &minute)

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

// ============================================================================
// Bulletproof Update Helpers
// ============================================================================

// checkDiskSpace verifies sufficient disk space is available
func (u *Updater) checkDiskSpace(path string) error {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		dir = u.config.DataPath
	}

	// Ensure directory exists for stat
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot access directory: %w", err)
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(dir, &stat); err != nil {
		return fmt.Errorf("cannot check disk space: %w", err)
	}

	// Available space in bytes
	available := stat.Bavail * uint64(stat.Bsize)

	if int64(available) < u.config.MinDiskSpace {
		return fmt.Errorf("insufficient disk space: %d MB available, need at least %d MB",
			available/(1024*1024), u.config.MinDiskSpace/(1024*1024))
	}

	u.logger.Debug("Disk space check passed", "available_mb", available/(1024*1024))
	return nil
}

// calculateSHA256 calculates the SHA256 checksum of a file
func (u *Updater) calculateSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// fetchChecksum downloads and parses a checksum file to find the checksum for a specific asset
func (u *Updater) fetchChecksum(ctx context.Context, checksumURL, assetName string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", checksumURL, nil)
	if err != nil {
		return "", err
	}

	if u.config.GitHubToken != "" {
		req.Header.Set("Authorization", "Bearer "+u.config.GitHubToken)
		req.Header.Set("Accept", "application/octet-stream")
	}

	resp, err := u.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("checksum fetch failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Parse checksum file (format: "checksum  filename" or "checksum filename")
	lines := strings.Split(string(body), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Try both formats
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			checksum := parts[0]
			filename := parts[len(parts)-1]
			if strings.Contains(filename, assetName) || filename == assetName {
				return checksum, nil
			}
		}
	}

	return "", fmt.Errorf("checksum not found for asset: %s", assetName)
}

// createBackup creates a backup of the current installation for rollback
func (u *Updater) createBackup(componentName, installPath, currentVersion string) (string, error) {
	// Check if there's anything to backup
	info, err := os.Stat(installPath)
	if os.IsNotExist(err) {
		return "", nil // Nothing to backup
	}
	if err != nil {
		return "", err
	}

	backupDir := filepath.Join(u.config.DataPath, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", err
	}

	timestamp := time.Now().Format("20060102-150405")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("%s-%s-%s", componentName, currentVersion, timestamp))

	// For files, copy directly; for directories, copy recursively
	if info.IsDir() {
		if err := u.copyDir(installPath, backupPath); err != nil {
			return "", err
		}
	} else {
		if err := u.copyFile(installPath, backupPath); err != nil {
			return "", err
		}
	}

	// Store backup info
	u.backupsMu.Lock()
	u.backups[componentName] = &BackupInfo{
		Component:   componentName,
		BackupPath:  backupPath,
		OriginalDir: installPath,
		Version:     currentVersion,
		CreatedAt:   time.Now(),
	}
	u.backupsMu.Unlock()

	return backupPath, nil
}

// copyDir recursively copies a directory
func (u *Updater) copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := u.copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := u.copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// atomicSwap atomically replaces the target with the source
func (u *Updater) atomicSwap(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("source does not exist: %w", err)
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	// For single files, use atomic rename
	if !srcInfo.IsDir() {
		// Check if destination exists
		if _, err := os.Stat(dst); err == nil {
			// Remove old file first
			if err := os.Remove(dst); err != nil {
				return fmt.Errorf("failed to remove old file: %w", err)
			}
		}
		return os.Rename(src, dst)
	}

	// For directories, we need to handle it differently
	// First, check if there are any files in the staged install path
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	// If there's only one entry and it matches a common binary name, move that
	if len(entries) == 1 {
		srcPath := filepath.Join(src, entries[0].Name())
		if !entries[0].IsDir() {
			// It's a single binary, move it directly
			if _, err := os.Stat(dst); err == nil {
				if err := os.Remove(dst); err != nil {
					return fmt.Errorf("failed to remove old file: %w", err)
				}
			}
			return os.Rename(srcPath, dst)
		}
	}

	// For multi-file directories, remove old and move new
	dstInfo, err := os.Stat(dst)
	if err == nil && dstInfo.IsDir() {
		// Create a temp backup in case rename fails
		tempDst := dst + ".old"
		if err := os.Rename(dst, tempDst); err != nil {
			return fmt.Errorf("failed to move old directory: %w", err)
		}

		if err := os.Rename(src, dst); err != nil {
			// Restore old directory
			_ = os.Rename(tempDst, dst)
			return fmt.Errorf("failed to move new directory: %w", err)
		}

		// Remove old directory
		_ = os.RemoveAll(tempDst)
		return nil
	}

	// Destination doesn't exist, just move
	return os.Rename(src, dst)
}

// Rollback restores a component from its backup
func (u *Updater) Rollback(ctx context.Context, componentName string) error {
	u.backupsMu.RLock()
	backup, ok := u.backups[componentName]
	u.backupsMu.RUnlock()

	if !ok {
		return fmt.Errorf("no backup available for %s", componentName)
	}

	u.logger.Info("Rolling back component", "component", componentName, "to_version", backup.Version)
	u.setStatus(componentName, "rolling_back", 0, fmt.Sprintf("Rolling back to %s...", backup.Version))

	// Check if backup exists
	if _, err := os.Stat(backup.BackupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup file does not exist: %s", backup.BackupPath)
	}

	// Remove current (potentially broken) installation
	if _, err := os.Stat(backup.OriginalDir); err == nil {
		if err := os.RemoveAll(backup.OriginalDir); err != nil {
			return fmt.Errorf("failed to remove broken installation: %w", err)
		}
	}

	// Restore from backup
	backupInfo, err := os.Stat(backup.BackupPath)
	if err != nil {
		return err
	}

	if backupInfo.IsDir() {
		if err := u.copyDir(backup.BackupPath, backup.OriginalDir); err != nil {
			return fmt.Errorf("failed to restore from backup: %w", err)
		}
	} else {
		if err := u.copyFile(backup.BackupPath, backup.OriginalDir); err != nil {
			return fmt.Errorf("failed to restore from backup: %w", err)
		}
	}

	// Update component version back
	u.mu.Lock()
	if component, ok := u.components[componentName]; ok {
		component.CurrentVersion = backup.Version
		component.UpdateAvail = true // Mark as update available since rollback happened
	}
	u.mu.Unlock()

	u.setStatus(componentName, "rolled_back", 100, fmt.Sprintf("Rolled back to %s", backup.Version))
	u.logger.Info("Rollback complete", "component", componentName, "version", backup.Version)

	return nil
}

// GetBackups returns available backups for all components
func (u *Updater) GetBackups() map[string]*BackupInfo {
	u.backupsMu.RLock()
	defer u.backupsMu.RUnlock()

	result := make(map[string]*BackupInfo, len(u.backups))
	for k, v := range u.backups {
		result[k] = v
	}
	return result
}

// CleanupOldBackups removes backups older than the specified duration
func (u *Updater) CleanupOldBackups(maxAge time.Duration) error {
	u.backupsMu.Lock()
	defer u.backupsMu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for component, backup := range u.backups {
		if backup.CreatedAt.Before(cutoff) {
			if err := os.RemoveAll(backup.BackupPath); err != nil {
				u.logger.Warn("Failed to cleanup backup", "component", component, "error", err)
			} else {
				delete(u.backups, component)
				u.logger.Info("Cleaned up old backup", "component", component, "created", backup.CreatedAt)
			}
		}
	}

	// Also cleanup any orphaned backup files
	backupDir := filepath.Join(u.config.DataPath, "backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return nil // Directory might not exist
	}

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(backupDir, entry.Name())
			if err := os.RemoveAll(path); err != nil {
				u.logger.Warn("Failed to cleanup orphaned backup", "path", path, "error", err)
			}
		}
	}

	return nil
}

// HealthCheck verifies the system is healthy after an update
func (u *Updater) HealthCheck(ctx context.Context) error {
	if u.config.HealthCheckURL == "" {
		return nil
	}

	u.logger.Info("Running health check", "url", u.config.HealthCheckURL)

	deadline := time.Now().Add(u.config.HealthCheckTimeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("health check timed out after %v", u.config.HealthCheckTimeout)
			}

			resp, err := u.client.Get(u.config.HealthCheckURL)
			if err != nil {
				u.logger.Debug("Health check failed, retrying", "error", err)
				continue
			}
			resp.Body.Close()

			if resp.StatusCode == 200 {
				u.logger.Info("Health check passed")
				return nil
			}

			u.logger.Debug("Health check returned non-200, retrying", "status", resp.StatusCode)
		}
	}
}
