package plugin

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
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Installer handles plugin installation from Git repositories
type Installer struct {
	pluginsDir  string
	cacheDir    string
	logger      *slog.Logger
	httpClient  *http.Client
	githubToken string // GitHub token for API rate limits

	// Tracked repositories for update checking
	repos map[string]*TrackedRepo
	mu    sync.RWMutex

	// Update check interval
	checkInterval time.Duration
	ctx           context.Context
	cancel        context.CancelFunc
}

// TrackedRepo represents a Git repository being tracked for updates
type TrackedRepo struct {
	URL            string    `json:"url" yaml:"url"`
	Owner          string    `json:"owner" yaml:"owner"`
	Repo           string    `json:"repo" yaml:"repo"`
	InstalledTag   string    `json:"installed_tag" yaml:"installed_tag"`
	LatestTag      string    `json:"latest_tag" yaml:"latest_tag"`
	LastCheck      time.Time `json:"last_check" yaml:"last_check"`
	UpdateAvailable bool     `json:"update_available" yaml:"update_available"`
	PluginID       string    `json:"plugin_id" yaml:"plugin_id"`
}

// GitHubRelease represents a GitHub release
type GitHubRelease struct {
	TagName     string         `json:"tag_name"`
	Name        string         `json:"name"`
	Draft       bool           `json:"draft"`
	Prerelease  bool           `json:"prerelease"`
	PublishedAt time.Time      `json:"published_at"`
	Assets      []GitHubAsset  `json:"assets"`
	Body        string         `json:"body"`
}

// GitHubAsset represents a release asset
type GitHubAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
	ContentType        string `json:"content_type"`
}

// NewInstaller creates a new plugin installer
func NewInstaller(pluginsDir string, logger *slog.Logger) *Installer {
	cacheDir := filepath.Join(pluginsDir, ".cache")

	// Check for GitHub token from environment
	githubToken := os.Getenv("GITHUB_TOKEN")

	return &Installer{
		pluginsDir:    pluginsDir,
		cacheDir:      cacheDir,
		logger:        logger.With("component", "plugin-installer"),
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		repos:         make(map[string]*TrackedRepo),
		checkInterval: 1 * time.Hour, // Check for updates hourly
		githubToken:   githubToken,
	}
}

// SetGitHubToken sets the GitHub token for API requests (higher rate limits)
func (i *Installer) SetGitHubToken(token string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.githubToken = token
}

// Start begins the update checking loop
func (i *Installer) Start(ctx context.Context) error {
	i.ctx, i.cancel = context.WithCancel(ctx)

	// Create directories
	if err := os.MkdirAll(i.pluginsDir, 0755); err != nil {
		return fmt.Errorf("failed to create plugins directory: %w", err)
	}
	if err := os.MkdirAll(i.cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Load tracked repos
	if err := i.loadTrackedRepos(); err != nil {
		i.logger.Warn("Failed to load tracked repos", "error", err)
	}

	// Start update check loop
	go i.updateCheckLoop()

	return nil
}

// Stop stops the installer
func (i *Installer) Stop() {
	if i.cancel != nil {
		i.cancel()
	}
}

// InstallFromGitHub installs a plugin from a GitHub repository URL
// Supports formats:
//   - https://github.com/owner/repo
//   - github.com/owner/repo
//   - owner/repo
func (i *Installer) InstallFromGitHub(ctx context.Context, repoURL string) (*PluginManifest, error) {
	owner, repo, err := parseGitHubURL(repoURL)
	if err != nil {
		return nil, err
	}

	i.logger.Info("Installing plugin from GitHub", "owner", owner, "repo", repo)

	// Get latest release
	release, err := i.getLatestRelease(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest release: %w", err)
	}

	i.logger.Info("Found release", "tag", release.TagName, "name", release.Name)

	// Find the appropriate asset for this platform
	asset := i.findPlatformAsset(release.Assets)
	if asset == nil {
		// No binary release, try cloning the repo
		return i.installFromSource(ctx, owner, repo, release.TagName)
	}

	// Download and extract the asset
	return i.installFromAsset(ctx, owner, repo, release.TagName, asset)
}

// installFromAsset downloads and installs a pre-built plugin asset
func (i *Installer) installFromAsset(ctx context.Context, owner, repo, tag string, asset *GitHubAsset) (*PluginManifest, error) {
	i.logger.Info("Downloading plugin asset", "name", asset.Name, "size", asset.Size)

	// Download to cache
	cachePath := filepath.Join(i.cacheDir, asset.Name)
	if err := i.downloadFile(ctx, asset.BrowserDownloadURL, cachePath); err != nil {
		return nil, fmt.Errorf("failed to download asset: %w", err)
	}
	defer func() { _ = os.Remove(cachePath) }()

	// Extract to a temporary directory first so we can read the manifest
	tempDir := filepath.Join(i.cacheDir, fmt.Sprintf("temp-%s-%d", repo, time.Now().UnixNano()))
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Extract based on file type
	if strings.HasSuffix(asset.Name, ".tar.gz") || strings.HasSuffix(asset.Name, ".tgz") {
		if err := extractTarGz(cachePath, tempDir); err != nil {
			return nil, fmt.Errorf("failed to extract tarball: %w", err)
		}
	} else if strings.HasSuffix(asset.Name, ".zip") {
		if err := extractZip(cachePath, tempDir); err != nil {
			return nil, fmt.Errorf("failed to extract zip: %w", err)
		}
	} else {
		// Assume it's a binary, copy directly
		destPath := filepath.Join(tempDir, asset.Name)
		if err := copyFile(cachePath, destPath); err != nil {
			return nil, fmt.Errorf("failed to copy binary: %w", err)
		}
		_ = os.Chmod(destPath, 0755)
	}

	// Read manifest to get the plugin ID
	manifest, err := i.readManifest(tempDir)
	if err != nil {
		// Try to generate a basic manifest using repo name as fallback
		// Strip common suffixes like "-plugin" from repo name for cleaner ID
		pluginID := strings.TrimSuffix(repo, "-plugin")
		manifest = &PluginManifest{
			ID:      pluginID,
			Name:    pluginID,
			Version: stripVersionPrefix(tag),
			Runtime: PluginRuntime{Type: "binary", Binary: asset.Name},
		}
		// Write the generated manifest
		_ = i.writeManifest(tempDir, manifest)
	}

	// Use manifest ID for the final plugin directory (not repo name)
	pluginDir := filepath.Join(i.pluginsDir, manifest.ID)

	// Remove existing plugin directory if it exists
	_ = os.RemoveAll(pluginDir)

	// Move temp directory to final location
	if err := os.Rename(tempDir, pluginDir); err != nil {
		// If rename fails (cross-device), copy instead
		if err := copyDir(tempDir, pluginDir); err != nil {
			return nil, fmt.Errorf("failed to move plugin to final location: %w", err)
		}
	}

	// Track the repo for updates
	i.trackRepo(owner, repo, tag, manifest.ID)

	i.logger.Info("Plugin installed", "id", manifest.ID, "version", manifest.Version, "dir", pluginDir)
	return manifest, nil
}

// installFromSource clones and builds a plugin from source
func (i *Installer) installFromSource(ctx context.Context, owner, repo, tag string) (*PluginManifest, error) {
	i.logger.Info("Installing plugin from source", "owner", owner, "repo", repo, "tag", tag)

	// Clone to a temp directory first so we can read manifest and get the real ID
	tempDir := filepath.Join(i.cacheDir, fmt.Sprintf("src-%s-%d", repo, time.Now().UnixNano()))

	// Clone the repository (use --recurse-submodules to fetch submodules)
	cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)

	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--recurse-submodules", "--shallow-submodules", "--branch", tag, cloneURL, tempDir)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		// Try without tag (use default branch)
		cmd = exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--recurse-submodules", "--shallow-submodules", cloneURL, tempDir)
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("failed to clone repository: %w", err)
		}
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Initialize any submodules that weren't fetched (fallback for older git versions)
	submoduleCmd := exec.CommandContext(ctx, "git", "-c", "safe.directory=*", "submodule", "update", "--init", "--recursive")
	submoduleCmd.Dir = tempDir
	submoduleCmd.Stdout = io.Discard
	submoduleCmd.Stderr = io.Discard
	_ = submoduleCmd.Run() // Ignore errors - submodules may not exist

	// Read manifest first to determine runtime type
	manifest, err := i.readManifest(tempDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	// Handle different runtime types
	switch strings.ToLower(manifest.Runtime.Type) {
	case "python":
		i.logger.Info("Setting up Python plugin", "id", manifest.ID)
		// Run setup script if it exists to create venv and install dependencies
		if manifest.Runtime.Setup != "" {
			setupPath := filepath.Join(tempDir, manifest.Runtime.Setup)
			if _, err := os.Stat(setupPath); err == nil {
				i.logger.Info("Running plugin setup script", "id", manifest.ID, "setup", setupPath)
				setupCmd := exec.CommandContext(ctx, "/bin/bash", setupPath)
				setupCmd.Dir = tempDir
				setupCmd.Env = append(os.Environ(), "HOME=/tmp") // Some scripts need HOME
				if output, err := setupCmd.CombinedOutput(); err != nil {
					i.logger.Warn("Plugin setup script failed, trying manual venv setup", "id", manifest.ID, "error", err, "output", string(output))
				}
			}
		}

		// Ensure venv exists - create it if setup.sh didn't
		venvDir := filepath.Join(tempDir, "venv")
		venvBin := filepath.Join(venvDir, "bin", "python")
		if _, err := os.Stat(venvBin); os.IsNotExist(err) {
			i.logger.Info("Creating Python venv", "id", manifest.ID)
			venvCmd := exec.CommandContext(ctx, "python3", "-m", "venv", venvDir)
			venvCmd.Dir = tempDir
			if output, err := venvCmd.CombinedOutput(); err != nil {
				i.logger.Warn("Failed to create venv", "error", err, "output", string(output))
			}
		}

		// Install requirements if they exist
		reqPath := filepath.Join(tempDir, "requirements.txt")
		if _, err := os.Stat(reqPath); err == nil {
			pipPath := filepath.Join(venvDir, "bin", "pip")
			if _, err := os.Stat(pipPath); err == nil {
				i.logger.Info("Installing Python dependencies", "id", manifest.ID)
				pipCmd := exec.CommandContext(ctx, pipPath, "install", "-r", reqPath)
				pipCmd.Dir = tempDir
				if output, err := pipCmd.CombinedOutput(); err != nil {
					i.logger.Warn("Failed to install Python dependencies", "error", err, "output", string(output))
				}
			}
		}

	case "node":
		i.logger.Info("Installing Node.js plugin", "id", manifest.ID)
		// Check for package.json and run npm install
		if _, err := os.Stat(filepath.Join(tempDir, "package.json")); err == nil {
			i.logger.Info("Running npm install for Node.js plugin")
			npmCmd := exec.CommandContext(ctx, "npm", "install", "--production")
			npmCmd.Dir = tempDir
			if output, err := npmCmd.CombinedOutput(); err != nil {
				i.logger.Warn("npm install failed", "error", err, "output", string(output))
				// Continue anyway - maybe dependencies aren't required
			}
		}

	case "binary", "":
		// Check if it's a Go project and build it
		if _, err := os.Stat(filepath.Join(tempDir, "go.mod")); err == nil {
			i.logger.Info("Building Go plugin")
			buildCmd := exec.CommandContext(ctx, "go", "build", "-o", "plugin", ".")
			buildCmd.Dir = tempDir
			buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")
			if output, err := buildCmd.CombinedOutput(); err != nil {
				return nil, fmt.Errorf("failed to build plugin: %w\n%s", err, output)
			}
		} else {
			// No go.mod and not a specific runtime type - check if there's a pre-built binary
			i.logger.Debug("No go.mod found and no specific runtime, assuming pre-built plugin")
		}

	default:
		i.logger.Warn("Unknown runtime type, proceeding without build", "type", manifest.Runtime.Type)
	}

	// Use manifest ID for the final plugin directory (not repo name)
	pluginDir := filepath.Join(i.pluginsDir, manifest.ID)

	// Remove existing plugin directory if it exists
	_ = os.RemoveAll(pluginDir)

	// Move temp directory to final location
	if err := os.Rename(tempDir, pluginDir); err != nil {
		// If rename fails (cross-device), copy instead
		if err := copyDir(tempDir, pluginDir); err != nil {
			return nil, fmt.Errorf("failed to move plugin to final location: %w", err)
		}
	}

	// Track the repo
	i.trackRepo(owner, repo, tag, manifest.ID)

	i.logger.Info("Plugin installed from source", "id", manifest.ID, "version", manifest.Version, "runtime", manifest.Runtime.Type, "dir", pluginDir)
	return manifest, nil
}

// getLatestRelease fetches the latest release from GitHub
func (i *Installer) getLatestRelease(ctx context.Context, owner, repo string) (*GitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "NVR-Plugin-Installer")

	// Add GitHub token if available (for higher rate limits)
	i.mu.RLock()
	token := i.githubToken
	i.mu.RUnlock()
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := i.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 404 {
		// No releases, return a dummy release for the default branch
		return &GitHubRelease{TagName: "main", Name: "Latest"}, nil
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error: %s - %s", resp.Status, string(body))
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to decode release: %w", err)
	}

	return &release, nil
}

// findPlatformAsset finds the appropriate binary asset for the current platform
// For cross-platform plugins (Python, Node.js), it will also match generic archives
func (i *Installer) findPlatformAsset(assets []GitHubAsset) *GitHubAsset {
	os := runtime.GOOS
	arch := runtime.GOARCH

	// Common naming patterns for platform-specific binaries
	patterns := []string{
		fmt.Sprintf("%s_%s", os, arch),
		fmt.Sprintf("%s-%s", os, arch),
		fmt.Sprintf("%s.%s", os, arch),
	}

	// Map arm64 to aarch64 which is also common
	if arch == "arm64" {
		patterns = append(patterns,
			fmt.Sprintf("%s_aarch64", os),
			fmt.Sprintf("%s-aarch64", os),
		)
	}

	// First, try to find platform-specific assets
	for _, asset := range assets {
		name := strings.ToLower(asset.Name)
		for _, pattern := range patterns {
			if strings.Contains(name, pattern) {
				return &asset
			}
		}
	}

	// If no platform-specific asset found, look for generic archives
	// These are likely cross-platform plugins (Python, Node.js, etc.)
	for _, asset := range assets {
		name := strings.ToLower(asset.Name)
		if strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz") || strings.HasSuffix(name, ".zip") {
			i.logger.Debug("Using generic archive asset for cross-platform plugin", "name", asset.Name)
			return &asset
		}
	}

	return nil
}

// downloadFile downloads a file from a URL
func (i *Installer) downloadFile(ctx context.Context, urlStr, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return err
	}

	// Add GitHub token for private repo downloads
	i.mu.RLock()
	token := i.githubToken
	i.mu.RUnlock()
	if token != "" && strings.Contains(urlStr, "github.com") {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/octet-stream")
	}

	resp, err := i.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, resp.Body)
	return err
}

// readManifest reads a plugin manifest from a directory
func (i *Installer) readManifest(dir string) (*PluginManifest, error) {
	manifestPath := filepath.Join(dir, "manifest.yaml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		// Try manifest.json
		manifestPath = filepath.Join(dir, "manifest.json")
		data, err = os.ReadFile(manifestPath)
		if err != nil {
			return nil, fmt.Errorf("manifest not found")
		}
		var manifest PluginManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return nil, err
		}
		return &manifest, nil
	}

	var manifest PluginManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

// writeManifest writes a plugin manifest to a directory
func (i *Installer) writeManifest(dir string, manifest *PluginManifest) error {
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "manifest.yaml"), data, 0644)
}

// trackRepo adds a repository to the tracked list
func (i *Installer) trackRepo(owner, repo, tag, pluginID string) {
	i.mu.Lock()
	defer i.mu.Unlock()

	key := fmt.Sprintf("%s/%s", owner, repo)
	i.repos[key] = &TrackedRepo{
		URL:          fmt.Sprintf("https://github.com/%s/%s", owner, repo),
		Owner:        owner,
		Repo:         repo,
		InstalledTag: tag,
		LatestTag:    tag,
		LastCheck:    time.Now(),
		PluginID:     pluginID,
	}

	_ = i.saveTrackedRepos()
}

// loadTrackedRepos loads the tracked repos from disk
func (i *Installer) loadTrackedRepos() error {
	path := filepath.Join(i.pluginsDir, "repos.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return yaml.Unmarshal(data, &i.repos)
}

// saveTrackedRepos saves the tracked repos to disk
func (i *Installer) saveTrackedRepos() error {
	// Don't write empty repos file
	if len(i.repos) == 0 {
		return nil
	}
	data, err := yaml.Marshal(i.repos)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(i.pluginsDir, "repos.yaml"), data, 0644)
}

// updateCheckLoop periodically checks for plugin updates
func (i *Installer) updateCheckLoop() {
	ticker := time.NewTicker(i.checkInterval)
	defer ticker.Stop()

	// Check immediately on start
	i.checkForUpdates()

	for {
		select {
		case <-i.ctx.Done():
			return
		case <-ticker.C:
			i.checkForUpdates()
		}
	}
}

// checkForUpdates checks all tracked repos for new releases
func (i *Installer) checkForUpdates() {
	i.mu.RLock()
	repos := make([]*TrackedRepo, 0, len(i.repos))
	for _, r := range i.repos {
		repos = append(repos, r)
	}
	i.mu.RUnlock()

	for _, repo := range repos {
		ctx, cancel := context.WithTimeout(i.ctx, 30*time.Second)
		release, err := i.getLatestRelease(ctx, repo.Owner, repo.Repo)
		cancel()

		if err != nil {
			i.logger.Debug("Failed to check for updates", "repo", repo.URL, "error", err)
			continue
		}

		i.mu.Lock()
		repo.LatestTag = release.TagName
		repo.LastCheck = time.Now()
		repo.UpdateAvailable = repo.LatestTag != repo.InstalledTag
		i.mu.Unlock()

		if repo.UpdateAvailable {
			i.logger.Info("Plugin update available",
				"plugin", repo.PluginID,
				"installed", repo.InstalledTag,
				"latest", repo.LatestTag)
		}
	}

	i.mu.Lock()
	_ = i.saveTrackedRepos()
	i.mu.Unlock()
}

// GetTrackedRepos returns all tracked repositories
func (i *Installer) GetTrackedRepos() []TrackedRepo {
	i.mu.RLock()
	defer i.mu.RUnlock()

	repos := make([]TrackedRepo, 0, len(i.repos))
	for _, r := range i.repos {
		repos = append(repos, *r)
	}
	return repos
}

// UpdatePlugin updates a plugin to the latest version
func (i *Installer) UpdatePlugin(ctx context.Context, pluginID string) (*PluginManifest, error) {
	i.mu.RLock()
	var repo *TrackedRepo
	for _, r := range i.repos {
		if r.PluginID == pluginID {
			repo = r
			break
		}
	}
	i.mu.RUnlock()

	if repo == nil {
		return nil, fmt.Errorf("plugin not tracked: %s", pluginID)
	}

	return i.InstallFromGitHub(ctx, repo.URL)
}

// UninstallPlugin removes a plugin
func (i *Installer) UninstallPlugin(pluginID string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.logger.Info("Uninstalling plugin", "id", pluginID)

	// Find and remove from tracked repos
	var repoKey string
	for key, r := range i.repos {
		if r.PluginID == pluginID {
			repoKey = key
			break
		}
	}

	if repoKey != "" {
		delete(i.repos, repoKey)
		_ = i.saveTrackedRepos()
		i.logger.Debug("Removed plugin from tracked repos", "id", pluginID, "key", repoKey)
	}

	// Find the plugin directory by scanning for manifest with matching ID
	// This handles both old-style (repo name) and new-style (manifest ID) directories
	pluginDir := i.findPluginDirectory(pluginID)
	if pluginDir == "" {
		// Fallback to direct ID match (the pluginID might be the directory name)
		fallbackDir := filepath.Join(i.pluginsDir, pluginID)
		if _, err := os.Stat(fallbackDir); err == nil {
			pluginDir = fallbackDir
			i.logger.Debug("Using fallback directory path", "id", pluginID, "dir", pluginDir)
		}
	}

	if pluginDir == "" {
		i.logger.Warn("Plugin directory not found, may already be uninstalled", "id", pluginID)
		return nil // Not an error - plugin may already be uninstalled
	}

	// Verify directory exists before attempting removal
	if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
		i.logger.Warn("Plugin directory does not exist, may already be uninstalled", "id", pluginID, "dir", pluginDir)
		return nil // Not an error - already gone
	}

	if err := os.RemoveAll(pluginDir); err != nil {
		return fmt.Errorf("failed to remove plugin directory %s: %w", pluginDir, err)
	}

	i.logger.Info("Plugin uninstalled successfully", "id", pluginID, "dir", pluginDir)
	return nil
}

// findPluginDirectory scans for a plugin directory with a manifest matching the given ID
// It also checks if the pluginID matches a directory name directly (for backwards compatibility)
func (i *Installer) findPluginDirectory(pluginID string) string {
	entries, err := os.ReadDir(i.pluginsDir)
	if err != nil {
		i.logger.Debug("Failed to read plugins directory", "error", err)
		return ""
	}

	// First pass: check manifest IDs
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirPath := filepath.Join(i.pluginsDir, entry.Name())

		// Try manifest.yaml first
		manifestPath := filepath.Join(dirPath, "manifest.yaml")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			// Try manifest.json as fallback
			manifestPath = filepath.Join(dirPath, "manifest.json")
			data, err = os.ReadFile(manifestPath)
			if err != nil {
				continue
			}
		}

		var manifest PluginManifest
		if strings.HasSuffix(manifestPath, ".json") {
			if err := json.Unmarshal(data, &manifest); err != nil {
				continue
			}
		} else {
			if err := yaml.Unmarshal(data, &manifest); err != nil {
				continue
			}
		}

		if manifest.ID == pluginID {
			i.logger.Debug("Found plugin by manifest ID", "id", pluginID, "dir", dirPath)
			return dirPath
		}
	}

	// Second pass: check if pluginID matches a directory name directly
	// This handles cases where the directory name doesn't match the manifest ID
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if entry.Name() == pluginID {
			dirPath := filepath.Join(i.pluginsDir, entry.Name())
			i.logger.Debug("Found plugin by directory name", "id", pluginID, "dir", dirPath)
			return dirPath
		}
	}

	i.logger.Debug("Plugin directory not found", "id", pluginID)
	return ""
}

// parseGitHubURL extracts owner and repo from various GitHub URL formats
func parseGitHubURL(url string) (owner, repo string, err error) {
	// Remove common prefixes
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "github.com/")
	url = strings.TrimSuffix(url, ".git")
	url = strings.TrimSuffix(url, "/")

	// Match owner/repo pattern
	re := regexp.MustCompile(`^([^/]+)/([^/]+)$`)
	matches := re.FindStringSubmatch(url)
	if matches == nil {
		return "", "", fmt.Errorf("invalid GitHub URL format: %s", url)
	}

	return matches[1], matches[2], nil
}

// Helper functions for archive extraction

func extractTarGz(src, dest string) error {
	cmd := exec.Command("tar", "-xzf", src, "-C", dest)
	return cmd.Run()
}

func extractZip(src, dest string) error {
	cmd := exec.Command("unzip", "-o", src, "-d", dest)
	return cmd.Run()
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, in)
	return err
}

// copyDir recursively copies a directory from src to dest
func copyDir(src, dest string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dest, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, destPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, destPath); err != nil {
				return err
			}
			// Preserve executable permissions
			info, err := entry.Info()
			if err == nil {
				_ = os.Chmod(destPath, info.Mode())
			}
		}
	}

	return nil
}

// stripVersionPrefix removes common version prefixes like "v" from version strings
// e.g., "v1.2.0" -> "1.2.0"
func stripVersionPrefix(version string) string {
	return strings.TrimPrefix(version, "v")
}
