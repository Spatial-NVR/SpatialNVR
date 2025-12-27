// Package config provides configuration management for the NVR system
package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

// Config represents the main NVR configuration
type Config struct {
	Version     string            `yaml:"version"`
	System      SystemConfig      `yaml:"system"`
	Cameras     []CameraConfig    `yaml:"cameras"`
	Detectors   DetectorsConfig   `yaml:"detectors"`
	Plugins     PluginsConfig     `yaml:"plugins"`
	Notifications NotificationsConfig `yaml:"notifications"`
	Storage     StorageConfig     `yaml:"storage"`
	Preferences PreferencesConfig `yaml:"preferences"`

	// Internal fields
	mu        sync.RWMutex `yaml:"-"`
	path      string       `yaml:"-"`
	watchers  []func(*Config) `yaml:"-"`
	encKey    []byte       `yaml:"-"`
}

// SystemConfig holds system-wide settings
type SystemConfig struct {
	Name        string         `yaml:"name"`
	Timezone    string         `yaml:"timezone"`
	StoragePath string         `yaml:"storage_path"`
	MaxStorageGB int           `yaml:"max_storage_gb"`
	Database    DatabaseConfig `yaml:"database"`
	Deployment  DeploymentConfig `yaml:"deployment"`
	Logging     LoggingConfig  `yaml:"logging"`
}

// DatabaseConfig holds database settings
type DatabaseConfig struct {
	Type     string `yaml:"type"` // sqlite or postgres
	Path     string `yaml:"path"` // SQLite path
	Host     string `yaml:"host,omitempty"`
	Port     int    `yaml:"port,omitempty"`
	Database string `yaml:"database,omitempty"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// DeploymentConfig holds deployment settings
type DeploymentConfig struct {
	Type string `yaml:"type"` // standalone (default) or distributed
}

// LoggingConfig holds logging settings
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// CameraConfig holds configuration for a single camera
type CameraConfig struct {
	ID           string           `yaml:"id" json:"id"`
	Name         string           `yaml:"name" json:"name"`
	Enabled      bool             `yaml:"enabled" json:"enabled"`
	Stream       StreamConfig     `yaml:"stream" json:"stream"`
	Manufacturer string           `yaml:"manufacturer,omitempty" json:"manufacturer,omitempty"`
	Model        string           `yaml:"model,omitempty" json:"model,omitempty"`
	Location     LocationConfig   `yaml:"location,omitempty" json:"location,omitempty"`
	Recording    RecordingConfig  `yaml:"recording" json:"recording"`
	Detection    DetectionConfig  `yaml:"detection" json:"detection"`
	Motion       MotionConfig     `yaml:"motion,omitempty" json:"motion,omitempty"`
	Audio        AudioConfig      `yaml:"audio,omitempty" json:"audio,omitempty"`
	PTZ          PTZConfig        `yaml:"ptz,omitempty" json:"ptz,omitempty"`
	Advanced     AdvancedConfig   `yaml:"advanced,omitempty" json:"advanced,omitempty"`
}

// MotionConfig holds motion detection settings for a camera
type MotionConfig struct {
	Enabled          bool    `yaml:"enabled" json:"enabled"`
	Method           string  `yaml:"method,omitempty" json:"method,omitempty"`               // frame_diff, mog2, knn
	Threshold        float64 `yaml:"threshold,omitempty" json:"threshold,omitempty"`         // Percentage of frame change (0.0-1.0)
	MinArea          int     `yaml:"min_area,omitempty" json:"min_area,omitempty"`           // Minimum contour area in pixels
	MaskZones        bool    `yaml:"mask_zones,omitempty" json:"mask_zones,omitempty"`       // Apply detection zones as motion mask
	PreMotionBuffer  int     `yaml:"pre_motion_buffer,omitempty" json:"pre_motion_buffer,omitempty"`   // Frames to buffer before motion
	PostMotionBuffer int     `yaml:"post_motion_buffer,omitempty" json:"post_motion_buffer,omitempty"` // Frames to buffer after motion stops
	CooldownSeconds  int     `yaml:"cooldown_seconds,omitempty" json:"cooldown_seconds,omitempty"`     // Minimum time between motion events
}

// StreamConfig holds camera stream settings
type StreamConfig struct {
	URL      string `yaml:"url" json:"url"`
	SubURL   string `yaml:"sub_url,omitempty" json:"sub_url,omitempty"`
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
	Password string `yaml:"password,omitempty" json:"password,omitempty"`
	AuthType string `yaml:"auth_type,omitempty" json:"auth_type,omitempty"` // basic, digest, none
}

// LocationConfig holds camera location info
type LocationConfig struct {
	Lat         float64 `yaml:"lat,omitempty" json:"lat,omitempty"`
	Lon         float64 `yaml:"lon,omitempty" json:"lon,omitempty"`
	Description string  `yaml:"description,omitempty" json:"description,omitempty"`
}

// RecordingConfig holds recording settings
type RecordingConfig struct {
	Enabled           bool            `yaml:"enabled" json:"enabled"`
	Mode              string          `yaml:"mode" json:"mode"` // continuous, motion, events
	PreBufferSeconds  int             `yaml:"pre_buffer_seconds" json:"pre_buffer_seconds"`
	PostBufferSeconds int             `yaml:"post_buffer_seconds" json:"post_buffer_seconds"`
	SegmentDuration   int             `yaml:"segment_duration" json:"segment_duration"` // seconds, default 60
	Retention         RetentionConfig `yaml:"retention" json:"retention"`
	Codec             string          `yaml:"codec,omitempty" json:"codec,omitempty"`
	Resolution        string          `yaml:"resolution,omitempty" json:"resolution,omitempty"`
	FPS               int             `yaml:"fps,omitempty" json:"fps,omitempty"`
	Bitrate           int             `yaml:"bitrate,omitempty" json:"bitrate,omitempty"`
}

// RetentionConfig holds retention settings
type RetentionConfig struct {
	DefaultDays int `yaml:"default_days" json:"default_days"`
	EventsDays  int `yaml:"events_days" json:"events_days"`
}

// DetectionConfig holds AI detection settings for a camera
type DetectionConfig struct {
	Enabled       bool             `yaml:"enabled" json:"enabled"`
	FPS           int              `yaml:"fps" json:"fps"`
	Models        []string         `yaml:"models" json:"models"`
	Zones         []ZoneConfig     `yaml:"zones,omitempty" json:"zones,omitempty"`
	Filters       FiltersConfig    `yaml:"filters,omitempty" json:"filters,omitempty"`
	Notifications NotifyConfig     `yaml:"notifications,omitempty" json:"notifications,omitempty"`
}

// ZoneConfig holds detection zone settings
type ZoneConfig struct {
	ID            string      `yaml:"id"`
	Name          string      `yaml:"name"`
	Enabled       bool        `yaml:"enabled"`
	Points        [][]float64 `yaml:"points"` // Normalized 0-1 coordinates
	Objects       []string    `yaml:"objects"`
	MinConfidence float64     `yaml:"min_confidence"`
	MinSize       float64     `yaml:"min_size,omitempty"`
}

// FiltersConfig holds detection filter settings
type FiltersConfig struct {
	MinArea             int  `yaml:"min_area,omitempty"`
	StationaryThreshold int  `yaml:"stationary_threshold,omitempty"`
	TrackObjects        bool `yaml:"track_objects,omitempty"`
	MaxDisappeared      int  `yaml:"max_disappeared,omitempty"`
}

// NotifyConfig holds per-camera notification settings
type NotifyConfig struct {
	Enabled         bool     `yaml:"enabled"`
	Channels        []string `yaml:"channels"`
	Events          map[string]bool `yaml:"events"`
	CooldownSeconds int      `yaml:"cooldown_seconds"`
}

// AudioConfig holds audio detection settings
type AudioConfig struct {
	Enabled     bool     `yaml:"enabled"`
	Detect      []string `yaml:"detect,omitempty"`
	Sensitivity float64  `yaml:"sensitivity,omitempty"`
}

// PTZConfig holds PTZ settings
type PTZConfig struct {
	Enabled bool          `yaml:"enabled"`
	Presets []PTZPreset   `yaml:"presets,omitempty"`
}

// PTZPreset holds a PTZ preset
type PTZPreset struct {
	ID   string  `yaml:"id"`
	Name string  `yaml:"name"`
	Pan  float64 `yaml:"pan"`
	Tilt float64 `yaml:"tilt"`
	Zoom float64 `yaml:"zoom"`
}

// AdvancedConfig holds advanced camera settings
type AdvancedConfig struct {
	HWAccel          string `yaml:"hwaccel,omitempty"`
	TimeoutSeconds   int    `yaml:"timeout_seconds,omitempty"`
	RetryAttempts    int    `yaml:"retry_attempts,omitempty"`
	TimestampOverlay bool   `yaml:"timestamp_overlay,omitempty"`
	TimestampFormat  string `yaml:"timestamp_format,omitempty"`
}

// DetectorsConfig holds AI detector configurations
type DetectorsConfig struct {
	YOLO12          YOLOConfig       `yaml:"yolo12,omitempty"`
	YOLO11          YOLOConfig       `yaml:"yolo11,omitempty"`
	FaceRecognition FaceConfig       `yaml:"face_recognition,omitempty"`
	LPR             LPRConfig        `yaml:"lpr,omitempty"`
}

// YOLOConfig holds YOLO model settings
type YOLOConfig struct {
	Type       string   `yaml:"type"`
	Model      string   `yaml:"model"`
	Device     string   `yaml:"device"`
	Confidence float64  `yaml:"confidence"`
	IOU        float64  `yaml:"iou,omitempty"`
	Classes    []string `yaml:"classes,omitempty"`
}

// FaceConfig holds face recognition settings
type FaceConfig struct {
	Type       string `yaml:"type"`
	Model      string `yaml:"model"`
	Device     string `yaml:"device"`
	DetSize    []int  `yaml:"det_size,omitempty"`
	Confidence float64 `yaml:"confidence"`
}

// LPRConfig holds license plate recognition settings
type LPRConfig struct {
	Type       string       `yaml:"type"`
	Device     string       `yaml:"device"`
	Lang       string       `yaml:"lang"`
	Confidence float64      `yaml:"confidence"`
	Formats    []LPRFormat  `yaml:"formats,omitempty"`
}

// LPRFormat holds plate format validation
type LPRFormat struct {
	Country string `yaml:"country"`
	Regex   string `yaml:"regex"`
}

// PluginsConfig holds plugin configurations
type PluginsConfig map[string]PluginConfig

// PluginConfig holds a single plugin's configuration
type PluginConfig struct {
	Enabled bool                   `yaml:"enabled"`
	Config  map[string]interface{} `yaml:"config,omitempty"`
}

// NotificationsConfig holds notification settings
type NotificationsConfig struct {
	Enabled    bool                     `yaml:"enabled"`
	QuietHours QuietHoursConfig         `yaml:"quiet_hours,omitempty"`
	Channels   NotificationChannels     `yaml:"channels"`
}

// QuietHoursConfig holds quiet hours settings
type QuietHoursConfig struct {
	Enabled bool   `yaml:"enabled"`
	Start   string `yaml:"start"`
	End     string `yaml:"end"`
}

// NotificationChannels holds channel configurations
type NotificationChannels struct {
	Push    PushConfig    `yaml:"push,omitempty"`
	Email   EmailConfig   `yaml:"email,omitempty"`
	Webhook WebhookConfig `yaml:"webhook,omitempty"`
	Discord DiscordConfig `yaml:"discord,omitempty"`
}

// PushConfig holds push notification settings
type PushConfig struct {
	Enabled bool   `yaml:"enabled"`
	Service string `yaml:"service,omitempty"`
}

// EmailConfig holds email notification settings
type EmailConfig struct {
	Enabled bool       `yaml:"enabled"`
	SMTP    SMTPConfig `yaml:"smtp,omitempty"`
}

// SMTPConfig holds SMTP settings
type SMTPConfig struct {
	Host     string   `yaml:"host"`
	Port     int      `yaml:"port"`
	Username string   `yaml:"username"`
	Password string   `yaml:"password"`
	From     string   `yaml:"from"`
	To       []string `yaml:"to"`
}

// WebhookConfig holds webhook settings
type WebhookConfig struct {
	Enabled bool              `yaml:"enabled"`
	URL     string            `yaml:"url,omitempty"`
	Method  string            `yaml:"method,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty"`
}

// DiscordConfig holds Discord webhook settings
type DiscordConfig struct {
	Enabled    bool   `yaml:"enabled"`
	WebhookURL string `yaml:"webhook_url,omitempty"`
}

// StorageConfig holds storage settings
type StorageConfig struct {
	Recordings string               `yaml:"recordings"`
	Thumbnails string               `yaml:"thumbnails"`
	Snapshots  string               `yaml:"snapshots"`
	Exports    string               `yaml:"exports"`
	Retention  StorageRetention     `yaml:"retention"`
	S3         S3Config             `yaml:"s3,omitempty"`
}

// StorageRetention holds retention settings
type StorageRetention struct {
	DefaultDays int                  `yaml:"default_days"`
	Tiers       map[string]TierConfig `yaml:"tiers,omitempty"`
}

// TierConfig holds storage tier settings
type TierConfig struct {
	DurationDays int    `yaml:"duration_days"`
	Location     string `yaml:"location"`
}

// S3Config holds S3 storage settings
type S3Config struct {
	Enabled   bool   `yaml:"enabled"`
	Endpoint  string `yaml:"endpoint,omitempty"`
	Region    string `yaml:"region,omitempty"`
	Bucket    string `yaml:"bucket,omitempty"`
	AccessKey string `yaml:"access_key,omitempty"`
	SecretKey string `yaml:"secret_key,omitempty"`
}

// PreferencesConfig holds user preferences
type PreferencesConfig struct {
	UI       UIPreferences       `yaml:"ui" json:"ui"`
	Timeline TimelinePreferences `yaml:"timeline" json:"timeline"`
	Events   EventPreferences    `yaml:"events" json:"events"`
}

// UIPreferences holds UI settings
type UIPreferences struct {
	Theme     string          `yaml:"theme" json:"theme"`
	Language  string          `yaml:"language" json:"language"`
	Dashboard DashboardConfig `yaml:"dashboard" json:"dashboard"`
}

// DashboardConfig holds dashboard settings
type DashboardConfig struct {
	GridColumns int  `yaml:"grid_columns" json:"grid_columns"`
	ShowFPS     bool `yaml:"show_fps" json:"show_fps"`
	ShowBitrate bool `yaml:"show_bitrate" json:"show_bitrate"`
}

// TimelinePreferences holds timeline settings
type TimelinePreferences struct {
	DefaultRangeHours        int `yaml:"default_range_hours" json:"default_range_hours"`
	ThumbnailIntervalSeconds int `yaml:"thumbnail_interval_seconds" json:"thumbnail_interval_seconds"`
}

// EventPreferences holds event settings
type EventPreferences struct {
	AutoAcknowledgeAfterDays int  `yaml:"auto_acknowledge_after_days" json:"auto_acknowledge_after_days"`
	GroupSimilarEvents       bool `yaml:"group_similar_events" json:"group_similar_events"`
	GroupWindowSeconds       int  `yaml:"group_window_seconds" json:"group_window_seconds"`
}

// Load loads configuration from a YAML file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	cfg.path = path
	cfg.encKey = getEncryptionKey()

	// Decrypt sensitive fields
	if err := cfg.decryptSecrets(); err != nil {
		return nil, fmt.Errorf("failed to decrypt secrets: %w", err)
	}

	// Set defaults
	cfg.setDefaults()

	return &cfg, nil
}

// Save saves the configuration to a YAML file
func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.saveUnlocked()
}

// saveUnlocked saves without acquiring lock (caller must hold lock)
func (c *Config) saveUnlocked() error {
	// Create a copy for saving (without mutex)
	cfgCopy := &Config{
		Version:       c.Version,
		System:        c.System,
		Cameras:       c.Cameras,
		Detectors:     c.Detectors,
		Plugins:       c.Plugins,
		Notifications: c.Notifications,
		Storage:       c.Storage,
		Preferences:   c.Preferences,
		path:          c.path,
		encKey:        c.encKey,
	}
	if err := cfgCopy.encryptSecrets(); err != nil {
		return fmt.Errorf("failed to encrypt secrets: %w", err)
	}

	data, err := yaml.Marshal(cfgCopy)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Add header
	header := "# NVR System Configuration\n# Auto-generated - manual edits are preserved\n\n"
	data = append([]byte(header), data...)

	// Atomic write
	tmpPath := c.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return os.Rename(tmpPath, c.path)
}

// Watch starts watching for configuration file changes
func (c *Config) Watch() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	go func() {
		defer watcher.Close()

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					time.Sleep(100 * time.Millisecond) // Debounce
					c.reload()
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				slog.Error("Config watch error", "error", err)
			}
		}
	}()

	return watcher.Add(c.path)
}

// OnChange registers a callback for config changes
func (c *Config) OnChange(fn func(*Config)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.watchers = append(c.watchers, fn)
}

// reload reloads the configuration from disk
func (c *Config) reload() {
	newCfg, err := Load(c.path)
	if err != nil {
		slog.Error("Failed to reload config", "error", err)
		return
	}

	c.mu.Lock()
	// Copy fields individually to avoid copying the mutex
	c.Version = newCfg.Version
	c.System = newCfg.System
	c.Cameras = newCfg.Cameras
	c.Detectors = newCfg.Detectors
	c.Plugins = newCfg.Plugins
	c.Notifications = newCfg.Notifications
	c.Storage = newCfg.Storage
	c.Preferences = newCfg.Preferences
	c.encKey = newCfg.encKey
	watchers := c.watchers
	c.mu.Unlock()

	slog.Info("Configuration reloaded")

	for _, fn := range watchers {
		fn(c)
	}
}

// GetCamera returns a camera by ID
func (c *Config) GetCamera(id string) *CameraConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for i := range c.Cameras {
		if c.Cameras[i].ID == id {
			return &c.Cameras[i]
		}
	}
	return nil
}

// UpsertCamera adds or updates a camera
func (c *Config) UpsertCamera(cam CameraConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.Cameras {
		if c.Cameras[i].ID == cam.ID {
			c.Cameras[i] = cam
			return c.saveUnlocked()
		}
	}

	c.Cameras = append(c.Cameras, cam)
	return c.saveUnlocked()
}

// RemoveCamera removes a camera by ID
func (c *Config) RemoveCamera(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.Cameras {
		if c.Cameras[i].ID == id {
			c.Cameras = append(c.Cameras[:i], c.Cameras[i+1:]...)
			return c.saveUnlocked()
		}
	}

	return fmt.Errorf("camera not found: %s", id)
}

// SetPath sets the path for the config file (used for saving)
func (c *Config) SetPath(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.path = path
}

// GetPath returns the current config file path
func (c *Config) GetPath() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.path
}

// setDefaults sets default values for unset fields
func (c *Config) setDefaults() {
	if c.Version == "" {
		c.Version = "1.0"
	}
	if c.System.Timezone == "" {
		c.System.Timezone = "UTC"
	}
	if c.System.StoragePath == "" {
		c.System.StoragePath = "/data"
	}
	if c.System.Database.Type == "" {
		c.System.Database.Type = "sqlite"
	}
	if c.System.Deployment.Type == "" {
		c.System.Deployment.Type = "standalone"
	}
	if c.System.Logging.Level == "" {
		c.System.Logging.Level = "info"
	}
}

// encryptSecrets encrypts sensitive fields
func (c *Config) encryptSecrets() error {
	for i := range c.Cameras {
		if c.Cameras[i].Stream.Password != "" && !strings.HasPrefix(c.Cameras[i].Stream.Password, "encrypted:") {
			encrypted, err := encrypt(c.encKey, c.Cameras[i].Stream.Password)
			if err != nil {
				return err
			}
			c.Cameras[i].Stream.Password = "encrypted:" + encrypted
		}
	}
	return nil
}

// decryptSecrets decrypts sensitive fields
func (c *Config) decryptSecrets() error {
	for i := range c.Cameras {
		if strings.HasPrefix(c.Cameras[i].Stream.Password, "encrypted:") {
			encrypted := strings.TrimPrefix(c.Cameras[i].Stream.Password, "encrypted:")
			decrypted, err := decrypt(c.encKey, encrypted)
			if err != nil {
				return err
			}
			c.Cameras[i].Stream.Password = decrypted
		}
	}
	return nil
}

// getEncryptionKey returns the encryption key from environment or generates one
func getEncryptionKey() []byte {
	keyStr := os.Getenv("NVR_ENCRYPTION_KEY")
	if keyStr != "" {
		key, err := base64.StdEncoding.DecodeString(keyStr)
		if err == nil && len(key) == 32 {
			return key
		}
	}

	// Default key (should be replaced in production)
	// Must be exactly 32 bytes for AES-256
	return []byte("nvr-default-key-change-in-prod!!")
}

// encrypt encrypts a string using AES-GCM
func encrypt(key []byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decrypt decrypts a string using AES-GCM
func decrypt(key []byte, ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	if len(data) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertextBytes := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
