package integration

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/Spatial-NVR/SpatialNVR/internal/api"
	"github.com/Spatial-NVR/SpatialNVR/internal/config"
	"github.com/Spatial-NVR/SpatialNVR/internal/plugin"
)

// PluginTestEnv holds the plugin test environment
type PluginTestEnv struct {
	Config        *config.Config
	PluginManager *plugin.Manager
	Router        chi.Router
	Server        *httptest.Server
	TmpDir        string
}

// SetupPluginTestEnv creates a plugin test environment
func SetupPluginTestEnv(t *testing.T) *PluginTestEnv {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	storagePath := filepath.Join(tmpDir, "storage")

	// Create storage directory
	if err := os.MkdirAll(storagePath, 0755); err != nil {
		t.Fatalf("Failed to create storage directory: %v", err)
	}

	// Create a catalog file in the storage path
	catalogContent := `
version: "1.0"
updated: "2024-12-25"
plugins:
  - id: test-plugin
    name: Test Plugin
    description: A test plugin for unit testing
    author: Test Author
    repo: github.com/test/test-plugin
    category: cameras
    featured: true
    capabilities:
      - video
      - ptz
  - id: another-plugin
    name: Another Plugin
    description: Another test plugin
    repo: github.com/test/another-plugin
    category: integrations
categories:
  cameras:
    name: Camera Plugins
    description: Plugins for camera integrations
    icon: video
  integrations:
    name: Integrations
    description: External system integrations
    icon: plug
`
	catalogPath := filepath.Join(storagePath, "plugin-catalog.yaml")
	if err := os.WriteFile(catalogPath, []byte(catalogContent), 0644); err != nil {
		t.Fatalf("Failed to write catalog: %v", err)
	}

	// Create config
	cfg := &config.Config{
		Version: "1.0",
		System: config.SystemConfig{
			Name:        "Test NVR",
			Timezone:    "UTC",
			StoragePath: storagePath,
		},
		Cameras: []config.CameraConfig{},
		Plugins: make(config.PluginsConfig),
	}
	cfg.SetPath(configPath)

	// Create logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// Create plugin manager (without go2rtc and events for simplicity)
	pluginManager := plugin.NewManager(cfg, nil, nil, logger)

	// Create router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Setup plugin routes
	r.Route("/api/v1/plugins", func(r chi.Router) {
		r.Get("/catalog", handleGetCatalog(pluginManager))
		r.Get("/", handleListPlugins(pluginManager))
	})

	// Create test server
	server := httptest.NewServer(r)

	return &PluginTestEnv{
		Config:        cfg,
		PluginManager: pluginManager,
		Router:        r,
		Server:        server,
		TmpDir:        tmpDir,
	}
}

// Cleanup cleans up the test environment
func (e *PluginTestEnv) Cleanup() {
	e.Server.Close()
}

// Handler for getting catalog
func handleGetCatalog(pm *plugin.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		catalog := pm.GetCatalog()
		if catalog == nil {
			api.OK(w, map[string]interface{}{
				"version":    "1.0",
				"plugins":    []interface{}{},
				"categories": map[string]interface{}{},
			})
			return
		}

		pluginsWithStatus := pm.GetCatalogWithStatus()

		api.OK(w, map[string]interface{}{
			"version":    catalog.Version,
			"updated":    catalog.Updated,
			"plugins":    pluginsWithStatus,
			"categories": catalog.Categories,
		})
	}
}

// Handler for listing plugins
func handleListPlugins(pm *plugin.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pluginStatuses := pm.ListPlugins()

		plugins := make([]map[string]interface{}, 0, len(pluginStatuses))
		for _, p := range pluginStatuses {
			plugins = append(plugins, map[string]interface{}{
				"id":           p.Manifest.ID,
				"name":         p.Manifest.Name,
				"version":      p.Manifest.Version,
				"description":  p.Manifest.Description,
				"enabled":      p.Enabled,
				"status":       string(p.State),
				"health":       p.Health.State,
				"camera_count": p.CameraCount,
			})
		}
		api.OK(w, plugins)
	}
}

// Tests

func TestGetCatalogEndpoint(t *testing.T) {
	env := SetupPluginTestEnv(t)
	defer env.Cleanup()

	resp, err := http.Get(env.Server.URL + "/api/v1/plugins/catalog")
	if err != nil {
		t.Fatalf("Failed to get catalog: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result api.Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !result.Success {
		t.Error("Expected successful response")
	}

	// Verify catalog data
	data := result.Data.(map[string]interface{})
	if data["version"] != "1.0" {
		t.Errorf("Expected version 1.0, got %v", data["version"])
	}

	plugins := data["plugins"].([]interface{})
	if len(plugins) != 2 {
		t.Errorf("Expected 2 plugins, got %d", len(plugins))
	}

	categories := data["categories"].(map[string]interface{})
	if len(categories) != 2 {
		t.Errorf("Expected 2 categories, got %d", len(categories))
	}
}

func TestCatalogPluginStatus(t *testing.T) {
	env := SetupPluginTestEnv(t)
	defer env.Cleanup()

	resp, err := http.Get(env.Server.URL + "/api/v1/plugins/catalog")
	if err != nil {
		t.Fatalf("Failed to get catalog: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result api.Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	data := result.Data.(map[string]interface{})
	plugins := data["plugins"].([]interface{})

	// All plugins should be not installed initially
	for _, p := range plugins {
		pluginData := p.(map[string]interface{})
		if pluginData["installed"].(bool) {
			t.Errorf("Plugin %s should not be installed", pluginData["id"])
		}
	}
}

func TestListPluginsEndpoint(t *testing.T) {
	env := SetupPluginTestEnv(t)
	defer env.Cleanup()

	resp, err := http.Get(env.Server.URL + "/api/v1/plugins")
	if err != nil {
		t.Fatalf("Failed to list plugins: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result api.Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !result.Success {
		t.Error("Expected successful response")
	}

	// Should be empty initially (no builtin plugins registered in test)
	plugins := result.Data.([]interface{})
	if len(plugins) != 0 {
		t.Errorf("Expected 0 plugins, got %d", len(plugins))
	}
}

func TestCatalogCategories(t *testing.T) {
	env := SetupPluginTestEnv(t)
	defer env.Cleanup()

	resp, err := http.Get(env.Server.URL + "/api/v1/plugins/catalog")
	if err != nil {
		t.Fatalf("Failed to get catalog: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result api.Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	data := result.Data.(map[string]interface{})
	categories := data["categories"].(map[string]interface{})

	// Check cameras category
	cameras := categories["cameras"].(map[string]interface{})
	if cameras["name"] != "Camera Plugins" {
		t.Errorf("Expected cameras category name 'Camera Plugins', got %v", cameras["name"])
	}

	// Check integrations category
	integrations := categories["integrations"].(map[string]interface{})
	if integrations["name"] != "Integrations" {
		t.Errorf("Expected integrations category name 'Integrations', got %v", integrations["name"])
	}
}

func TestCatalogFeaturedPlugin(t *testing.T) {
	env := SetupPluginTestEnv(t)
	defer env.Cleanup()

	resp, err := http.Get(env.Server.URL + "/api/v1/plugins/catalog")
	if err != nil {
		t.Fatalf("Failed to get catalog: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result api.Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	data := result.Data.(map[string]interface{})
	plugins := data["plugins"].([]interface{})

	// Find the featured plugin
	var featuredCount int
	for _, p := range plugins {
		pluginData := p.(map[string]interface{})
		if featured, ok := pluginData["featured"].(bool); ok && featured {
			featuredCount++
			if pluginData["id"] != "test-plugin" {
				t.Errorf("Expected featured plugin 'test-plugin', got %v", pluginData["id"])
			}
		}
	}

	if featuredCount != 1 {
		t.Errorf("Expected 1 featured plugin, got %d", featuredCount)
	}
}
