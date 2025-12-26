package plugin

import (
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// Catalog represents the plugin marketplace catalog
type Catalog struct {
	Version  string            `json:"version" yaml:"version"`
	Updated  string            `json:"updated" yaml:"updated"`
	Plugins  []CatalogPlugin   `json:"plugins" yaml:"plugins"`
	Categories map[string]CatalogCategory `json:"categories" yaml:"categories"`

	mu sync.RWMutex
}

// CatalogPlugin represents a plugin in the catalog
type CatalogPlugin struct {
	ID               string   `json:"id" yaml:"id"`
	Name             string   `json:"name" yaml:"name"`
	Description      string   `json:"description" yaml:"description"`
	Author           string   `json:"author" yaml:"author"`
	Repo             string   `json:"repo" yaml:"repo"`
	Icon             string   `json:"icon" yaml:"icon"`
	Category         string   `json:"category" yaml:"category"`
	Featured         bool     `json:"featured" yaml:"featured"`
	Capabilities     []string `json:"capabilities" yaml:"capabilities"`
	SupportedDevices []string `json:"supported_devices" yaml:"supported_devices"`
}

// CatalogCategory represents a category in the catalog
type CatalogCategory struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	Icon        string `json:"icon" yaml:"icon"`
}

// CatalogPluginStatus combines catalog info with installation status
type CatalogPluginStatus struct {
	CatalogPlugin
	Installed       bool   `json:"installed"`
	InstalledVersion string `json:"installed_version,omitempty"`
	LatestVersion   string `json:"latest_version,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	Enabled         bool   `json:"enabled"`
	State           string `json:"state,omitempty"`
}

// LoadCatalog loads the plugin catalog from a file
func LoadCatalog(path string) (*Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var catalog Catalog
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return nil, err
	}

	return &catalog, nil
}

// LoadCatalogFromDir tries to find and load catalog from common locations
func LoadCatalogFromDir(baseDir string) (*Catalog, error) {
	// Try common locations
	paths := []string{
		filepath.Join(baseDir, "plugin-catalog.yaml"),
		filepath.Join(baseDir, "..", "plugin-catalog.yaml"),
		"/app/plugin-catalog.yaml",
		"plugin-catalog.yaml",
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return LoadCatalog(path)
		}
	}

	// Return empty catalog if not found
	return &Catalog{
		Version:    "1.0",
		Plugins:    []CatalogPlugin{},
		Categories: make(map[string]CatalogCategory),
	}, nil
}

// GetPlugins returns all plugins in the catalog
func (c *Catalog) GetPlugins() []CatalogPlugin {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Plugins
}

// GetPlugin returns a specific plugin by ID
func (c *Catalog) GetPlugin(id string) *CatalogPlugin {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, p := range c.Plugins {
		if p.ID == id {
			return &p
		}
	}
	return nil
}

// GetFeaturedPlugins returns featured plugins
func (c *Catalog) GetFeaturedPlugins() []CatalogPlugin {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var featured []CatalogPlugin
	for _, p := range c.Plugins {
		if p.Featured {
			featured = append(featured, p)
		}
	}
	return featured
}

// GetPluginsByCategory returns plugins in a specific category
func (c *Catalog) GetPluginsByCategory(category string) []CatalogPlugin {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var plugins []CatalogPlugin
	for _, p := range c.Plugins {
		if p.Category == category {
			plugins = append(plugins, p)
		}
	}
	return plugins
}

// GetCategories returns all categories
func (c *Catalog) GetCategories() map[string]CatalogCategory {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Categories
}
