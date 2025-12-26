package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCatalog(t *testing.T) {
	// Create a temporary catalog file
	tmpDir := t.TempDir()
	catalogPath := filepath.Join(tmpDir, "plugin-catalog.yaml")

	catalogContent := `
version: "1.0"
updated: "2024-12-25"
plugins:
  - id: test-plugin
    name: Test Plugin
    description: A test plugin
    author: Test Author
    repo: github.com/test/plugin
    category: cameras
    featured: true
    capabilities:
      - video
      - ptz
  - id: another-plugin
    name: Another Plugin
    description: Another test plugin
    repo: github.com/test/another
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

	if err := os.WriteFile(catalogPath, []byte(catalogContent), 0644); err != nil {
		t.Fatalf("Failed to write test catalog: %v", err)
	}

	// Test LoadCatalog
	catalog, err := LoadCatalog(catalogPath)
	if err != nil {
		t.Fatalf("LoadCatalog failed: %v", err)
	}

	// Verify catalog structure
	if catalog.Version != "1.0" {
		t.Errorf("Expected version 1.0, got %s", catalog.Version)
	}

	if catalog.Updated != "2024-12-25" {
		t.Errorf("Expected updated 2024-12-25, got %s", catalog.Updated)
	}

	if len(catalog.Plugins) != 2 {
		t.Errorf("Expected 2 plugins, got %d", len(catalog.Plugins))
	}

	if len(catalog.Categories) != 2 {
		t.Errorf("Expected 2 categories, got %d", len(catalog.Categories))
	}

	// Test GetPlugin
	plugin := catalog.GetPlugin("test-plugin")
	if plugin == nil {
		t.Fatal("Expected to find test-plugin")
	}

	if plugin.Name != "Test Plugin" {
		t.Errorf("Expected name 'Test Plugin', got %s", plugin.Name)
	}

	if !plugin.Featured {
		t.Error("Expected plugin to be featured")
	}

	if len(plugin.Capabilities) != 2 {
		t.Errorf("Expected 2 capabilities, got %d", len(plugin.Capabilities))
	}

	// Test GetPlugin for non-existent plugin
	nonExistent := catalog.GetPlugin("non-existent")
	if nonExistent != nil {
		t.Error("Expected nil for non-existent plugin")
	}

	// Test GetFeaturedPlugins
	featured := catalog.GetFeaturedPlugins()
	if len(featured) != 1 {
		t.Errorf("Expected 1 featured plugin, got %d", len(featured))
	}

	if featured[0].ID != "test-plugin" {
		t.Errorf("Expected featured plugin ID 'test-plugin', got %s", featured[0].ID)
	}

	// Test GetPluginsByCategory
	cameraPlugins := catalog.GetPluginsByCategory("cameras")
	if len(cameraPlugins) != 1 {
		t.Errorf("Expected 1 camera plugin, got %d", len(cameraPlugins))
	}

	integrationPlugins := catalog.GetPluginsByCategory("integrations")
	if len(integrationPlugins) != 1 {
		t.Errorf("Expected 1 integration plugin, got %d", len(integrationPlugins))
	}

	// Test GetCategories
	categories := catalog.GetCategories()
	if len(categories) != 2 {
		t.Errorf("Expected 2 categories, got %d", len(categories))
	}

	if cat, ok := categories["cameras"]; !ok {
		t.Error("Expected cameras category")
	} else if cat.Name != "Camera Plugins" {
		t.Errorf("Expected category name 'Camera Plugins', got %s", cat.Name)
	}
}

func TestLoadCatalogFromDir(t *testing.T) {
	// Create a temporary directory with a catalog
	tmpDir := t.TempDir()
	catalogPath := filepath.Join(tmpDir, "plugin-catalog.yaml")

	catalogContent := `
version: "2.0"
plugins:
  - id: dir-test
    name: Dir Test
    description: Test from directory
    repo: github.com/test/dir
    category: cameras
categories:
  cameras:
    name: Cameras
`

	if err := os.WriteFile(catalogPath, []byte(catalogContent), 0644); err != nil {
		t.Fatalf("Failed to write test catalog: %v", err)
	}

	// Test LoadCatalogFromDir
	catalog, err := LoadCatalogFromDir(tmpDir)
	if err != nil {
		t.Fatalf("LoadCatalogFromDir failed: %v", err)
	}

	if catalog.Version != "2.0" {
		t.Errorf("Expected version 2.0, got %s", catalog.Version)
	}

	if len(catalog.Plugins) != 1 {
		t.Errorf("Expected 1 plugin, got %d", len(catalog.Plugins))
	}
}

func TestLoadCatalogFromDir_NotFound(t *testing.T) {
	// Test with non-existent directory - should return empty catalog
	catalog, err := LoadCatalogFromDir("/non/existent/path")
	if err != nil {
		t.Fatalf("Expected no error for missing catalog, got %v", err)
	}

	if catalog.Version != "1.0" {
		t.Errorf("Expected default version 1.0, got %s", catalog.Version)
	}

	if len(catalog.Plugins) != 0 {
		t.Errorf("Expected 0 plugins, got %d", len(catalog.Plugins))
	}
}

func TestLoadCatalog_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	catalogPath := filepath.Join(tmpDir, "invalid-catalog.yaml")

	invalidContent := `
this is not valid yaml: [
`

	if err := os.WriteFile(catalogPath, []byte(invalidContent), 0644); err != nil {
		t.Fatalf("Failed to write invalid catalog: %v", err)
	}

	_, err := LoadCatalog(catalogPath)
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}
}
