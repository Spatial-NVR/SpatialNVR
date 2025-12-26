package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create a test config file
	configContent := `
version: "1.0"
system:
  name: "Test NVR"
  timezone: "America/New_York"
  storage_path: "/data"
  max_storage_gb: 500
  database:
    type: "sqlite"
    path: "/data/test.db"
cameras: []
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Version != "1.0" {
		t.Errorf("Expected version '1.0', got '%s'", cfg.Version)
	}

	if cfg.System.Name != "Test NVR" {
		t.Errorf("Expected name 'Test NVR', got '%s'", cfg.System.Name)
	}

	if cfg.System.Timezone != "America/New_York" {
		t.Errorf("Expected timezone 'America/New_York', got '%s'", cfg.System.Timezone)
	}

	if cfg.System.MaxStorageGB != 500 {
		t.Errorf("Expected max_storage_gb 500, got %d", cfg.System.MaxStorageGB)
	}
}

func TestLoadNonExistent(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("Expected error when loading non-existent file")
	}
}

func TestSave(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create initial config
	cfg := &Config{
		Version: "1.0",
		System: SystemConfig{
			Name:         "Test NVR",
			Timezone:     "UTC",
			StoragePath:  "/data",
			MaxStorageGB: 1000,
			Database: DatabaseConfig{
				Type: "sqlite",
				Path: "/data/nvr.db",
			},
		},
		Cameras: []CameraConfig{},
	}
	cfg.SetPath(configPath)

	err := cfg.Save()
	if err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}

	// Load and verify
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load saved config: %v", err)
	}

	if loaded.System.Name != cfg.System.Name {
		t.Errorf("Expected name '%s', got '%s'", cfg.System.Name, loaded.System.Name)
	}
}

func TestCameraOperations(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	cfg := &Config{
		Version: "1.0",
		System: SystemConfig{
			Name:        "Test NVR",
			Timezone:    "UTC",
			StoragePath: "/data",
		},
		Cameras: []CameraConfig{},
	}
	cfg.SetPath(configPath)

	// Test UpsertCamera (add new)
	cam := CameraConfig{
		ID:   "cam1",
		Name: "Front Door",
		Stream: StreamConfig{
			URL: "rtsp://192.168.1.100:554/stream",
		},
		Enabled: true,
	}

	err := cfg.UpsertCamera(cam)
	if err != nil {
		t.Fatalf("Failed to upsert camera: %v", err)
	}

	if len(cfg.Cameras) != 1 {
		t.Errorf("Expected 1 camera, got %d", len(cfg.Cameras))
	}

	// Test GetCamera
	retrieved := cfg.GetCamera("cam1")
	if retrieved == nil {
		t.Fatal("GetCamera returned nil for existing camera")
	}
	if retrieved.Name != "Front Door" {
		t.Errorf("Expected name 'Front Door', got '%s'", retrieved.Name)
	}

	// Test GetCamera non-existent
	nonExistent := cfg.GetCamera("nonexistent")
	if nonExistent != nil {
		t.Error("GetCamera should return nil for non-existent camera")
	}

	// Test UpsertCamera (update existing)
	cam.Name = "Back Door"
	err = cfg.UpsertCamera(cam)
	if err != nil {
		t.Fatalf("Failed to update camera: %v", err)
	}

	if len(cfg.Cameras) != 1 {
		t.Errorf("Expected 1 camera after update, got %d", len(cfg.Cameras))
	}

	retrieved = cfg.GetCamera("cam1")
	if retrieved.Name != "Back Door" {
		t.Errorf("Expected updated name 'Back Door', got '%s'", retrieved.Name)
	}

	// Test RemoveCamera
	err = cfg.RemoveCamera("cam1")
	if err != nil {
		t.Fatalf("Failed to remove camera: %v", err)
	}

	if len(cfg.Cameras) != 0 {
		t.Errorf("Expected 0 cameras after removal, got %d", len(cfg.Cameras))
	}

	// Test RemoveCamera non-existent
	err = cfg.RemoveCamera("nonexistent")
	if err == nil {
		t.Error("Expected error when removing non-existent camera")
	}
}

func TestOnChange(t *testing.T) {
	cfg := &Config{}

	callCount := 0
	cfg.OnChange(func(c *Config) {
		callCount++
	})

	// We can't easily test the watcher without writing files,
	// but we can verify the callback is registered
	if len(cfg.watchers) != 1 {
		t.Errorf("Expected 1 watcher, got %d", len(cfg.watchers))
	}
}

func TestCameraConfig(t *testing.T) {
	cam := CameraConfig{
		ID:      "test_cam",
		Name:    "Test Camera",
		Enabled: true,
		Stream: StreamConfig{
			URL:      "rtsp://192.168.1.100:554/stream",
			SubURL:   "rtsp://192.168.1.100:554/substream",
			Username: "admin",
			Password: "password",
		},
		Recording: RecordingConfig{
			Enabled:           true,
			PreBufferSeconds:  5,
			PostBufferSeconds: 10,
		},
		Detection: DetectionConfig{
			Enabled: true,
			FPS:     5,
		},
	}

	if cam.ID != "test_cam" {
		t.Errorf("Expected ID 'test_cam', got '%s'", cam.ID)
	}

	if cam.Stream.URL != "rtsp://192.168.1.100:554/stream" {
		t.Errorf("Unexpected stream URL: %s", cam.Stream.URL)
	}

	if cam.Recording.PreBufferSeconds != 5 {
		t.Errorf("Expected pre-buffer 5, got %d", cam.Recording.PreBufferSeconds)
	}
}

func TestPluginsConfig(t *testing.T) {
	plugins := make(PluginsConfig)

	plugins["test_plugin"] = PluginConfig{
		Enabled: true,
		Config: map[string]interface{}{
			"setting1": "value1",
		},
	}

	if !plugins["test_plugin"].Enabled {
		t.Error("Plugin should be enabled")
	}

	if plugins["test_plugin"].Config["setting1"] != "value1" {
		t.Error("Plugin config not set correctly")
	}
}

func TestSetDefaults(t *testing.T) {
	cfg := &Config{}
	cfg.setDefaults()

	if cfg.Version != "1.0" {
		t.Errorf("Expected default version '1.0', got '%s'", cfg.Version)
	}
	if cfg.System.Timezone != "UTC" {
		t.Errorf("Expected default timezone 'UTC', got '%s'", cfg.System.Timezone)
	}
	if cfg.System.StoragePath != "/data" {
		t.Errorf("Expected default storage path '/data', got '%s'", cfg.System.StoragePath)
	}
	if cfg.System.Database.Type != "sqlite" {
		t.Errorf("Expected default database type 'sqlite', got '%s'", cfg.System.Database.Type)
	}
	if cfg.System.Deployment.Type != "standalone" {
		t.Errorf("Expected default deployment type 'standalone', got '%s'", cfg.System.Deployment.Type)
	}
	if cfg.System.Logging.Level != "info" {
		t.Errorf("Expected default logging level 'info', got '%s'", cfg.System.Logging.Level)
	}
}

func TestSetDefaultsDoesNotOverwrite(t *testing.T) {
	cfg := &Config{
		Version: "2.0",
		System: SystemConfig{
			Timezone:    "America/New_York",
			StoragePath: "/custom/path",
			Database: DatabaseConfig{
				Type: "postgres",
			},
			Deployment: DeploymentConfig{
				Type: "distributed",
			},
			Logging: LoggingConfig{
				Level: "debug",
			},
		},
	}
	cfg.setDefaults()

	if cfg.Version != "2.0" {
		t.Errorf("Version was overwritten, got '%s'", cfg.Version)
	}
	if cfg.System.Timezone != "America/New_York" {
		t.Errorf("Timezone was overwritten, got '%s'", cfg.System.Timezone)
	}
	if cfg.System.StoragePath != "/custom/path" {
		t.Errorf("StoragePath was overwritten, got '%s'", cfg.System.StoragePath)
	}
	if cfg.System.Database.Type != "postgres" {
		t.Errorf("Database.Type was overwritten, got '%s'", cfg.System.Database.Type)
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Invalid YAML content
	invalidContent := `
version: "1.0"
  bad indentation
cameras: []
`
	err := os.WriteFile(configPath, []byte(invalidContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err = Load(configPath)
	if err == nil {
		t.Error("Expected error when loading invalid YAML")
	}
}

func TestGetPath(t *testing.T) {
	cfg := &Config{}
	cfg.SetPath("/custom/path/config.yaml")

	path := cfg.GetPath()
	if path != "/custom/path/config.yaml" {
		t.Errorf("Expected path '/custom/path/config.yaml', got '%s'", path)
	}
}

func TestLoadWithCameras(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
version: "1.0"
system:
  name: "Test NVR"
cameras:
  - id: "cam1"
    name: "Front Door"
    enabled: true
    stream:
      url: "rtsp://192.168.1.100:554/stream"
      username: "admin"
      password: "test123"
  - id: "cam2"
    name: "Back Door"
    enabled: false
    stream:
      url: "rtsp://192.168.1.101:554/stream"
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if len(cfg.Cameras) != 2 {
		t.Errorf("Expected 2 cameras, got %d", len(cfg.Cameras))
	}

	cam1 := cfg.GetCamera("cam1")
	if cam1 == nil {
		t.Fatal("Camera cam1 not found")
	}
	if cam1.Name != "Front Door" {
		t.Errorf("Expected name 'Front Door', got '%s'", cam1.Name)
	}
	if !cam1.Enabled {
		t.Error("Camera cam1 should be enabled")
	}
}

func TestConfigTypes(t *testing.T) {
	// Test all config types can be instantiated
	_ = SystemConfig{}
	_ = DatabaseConfig{}
	_ = DeploymentConfig{}
	_ = LoggingConfig{}
	_ = CameraConfig{}
	_ = StreamConfig{}
	_ = LocationConfig{}
	_ = RecordingConfig{}
	_ = RetentionConfig{}
	_ = DetectionConfig{}
	_ = ZoneConfig{}
	_ = FiltersConfig{}
	_ = NotifyConfig{}
	_ = AudioConfig{}
	_ = PTZConfig{}
	_ = PTZPreset{}
	_ = AdvancedConfig{}
	_ = MotionConfig{}
	_ = DetectorsConfig{}
	_ = YOLOConfig{}
	_ = FaceConfig{}
	_ = LPRConfig{}
	_ = LPRFormat{}
	_ = NotificationsConfig{}
	_ = QuietHoursConfig{}
	_ = NotificationChannels{}
	_ = PushConfig{}
	_ = EmailConfig{}
	_ = SMTPConfig{}
	_ = WebhookConfig{}
	_ = DiscordConfig{}
	_ = StorageConfig{}
	_ = StorageRetention{}
	_ = TierConfig{}
	_ = S3Config{}
	_ = PreferencesConfig{}
	_ = UIPreferences{}
	_ = DashboardConfig{}
	_ = TimelinePreferences{}
	_ = EventPreferences{}
}

func TestFullConfigYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
version: "1.0"
system:
  name: "Full Test NVR"
  timezone: "America/Los_Angeles"
  storage_path: "/data/recordings"
  max_storage_gb: 2000
  database:
    type: "postgres"
    host: "localhost"
    port: 5432
    database: "nvr"
    username: "nvr_user"
    password: "secret123"
  deployment:
    mode: "distributed"
  logging:
    level: "debug"
    format: "json"
cameras:
  - id: "garage"
    name: "Garage Camera"
    enabled: true
    manufacturer: "Hikvision"
    model: "DS-2CD2343G2-I"
    stream:
      url: "rtsp://192.168.1.50:554/Streaming/Channels/101"
      sub_url: "rtsp://192.168.1.50:554/Streaming/Channels/102"
      username: "admin"
      password: "campass"
      auth_type: "digest"
    location:
      lat: 37.7749
      lon: -122.4194
      description: "North side of garage"
    recording:
      enabled: true
      mode: "continuous"
      pre_buffer_seconds: 5
      post_buffer_seconds: 30
      segment_duration: 60
      retention:
        default_days: 30
        events_days: 90
      codec: "h264"
      resolution: "1920x1080"
      fps: 15
      bitrate: 4000000
    detection:
      enabled: true
      fps: 5
      models:
        - "yolo12"
      zones:
        - id: "driveway"
          name: "Driveway Zone"
          enabled: true
          points: [[0.1, 0.1], [0.9, 0.1], [0.9, 0.9], [0.1, 0.9]]
          objects:
            - "person"
            - "car"
          min_confidence: 0.6
          min_size: 0.01
      filters:
        min_area: 100
        stationary_threshold: 30
        track_objects: true
        max_disappeared: 10
      notifications:
        enabled: true
        channels:
          - "push"
          - "email"
        events:
          person: true
          car: true
        cooldown_seconds: 60
    motion:
      enabled: true
      method: "mog2"
      threshold: 0.25
      min_area: 500
      mask_zones: true
      pre_motion_buffer: 30
      post_motion_buffer: 60
      cooldown_seconds: 5
    audio:
      enabled: true
      detect:
        - "glass_break"
        - "scream"
      sensitivity: 0.7
    ptz:
      enabled: true
      presets:
        - id: "home"
          name: "Home Position"
          pan: 0.0
          tilt: 0.0
          zoom: 1.0
        - id: "driveway"
          name: "Driveway View"
          pan: 45.0
          tilt: -10.0
          zoom: 2.0
    advanced:
      hwaccel: "videotoolbox"
      timeout_seconds: 10
      retry_attempts: 3
      timestamp_overlay: true
      timestamp_format: "%Y-%m-%d %H:%M:%S"
detectors:
  yolo12:
    type: "onnx"
    model: "yolov12n.onnx"
    device: "GPU"
    confidence: 0.5
    iou: 0.45
    classes:
      - "person"
      - "car"
      - "dog"
      - "cat"
  face_recognition:
    type: "insightface"
    model: "buffalo_l"
    device: "GPU"
    det_size: [640, 640]
    confidence: 0.6
  lpr:
    type: "easyocr"
    device: "GPU"
    lang: "en"
    confidence: 0.7
    formats:
      - country: "US"
        regex: "^[A-Z0-9]{1,7}$"
plugins:
  reolink:
    enabled: true
    config:
      scan_interval: 300
      default_password: "admin"
notifications:
  enabled: true
  quiet_hours:
    enabled: true
    start: "22:00"
    end: "07:00"
  channels:
    push:
      enabled: true
      service: "firebase"
    email:
      enabled: true
      smtp:
        host: "smtp.gmail.com"
        port: 587
        username: "nvr@example.com"
        password: "smtppass"
        from: "nvr@example.com"
        to:
          - "alerts@example.com"
    webhook:
      enabled: false
      url: "https://hooks.example.com/nvr"
      method: "POST"
      headers:
        Authorization: "Bearer token123"
    discord:
      enabled: true
      webhook_url: "https://discord.com/api/webhooks/..."
storage:
  recordings: "/data/recordings"
  thumbnails: "/data/thumbnails"
  snapshots: "/data/snapshots"
  exports: "/data/exports"
  retention:
    default_days: 30
    tiers:
      warm:
        duration_days: 7
        location: "/archive/warm"
      cold:
        duration_days: 30
        location: "s3://nvr-archive"
  s3:
    enabled: true
    endpoint: "https://s3.amazonaws.com"
    region: "us-west-2"
    bucket: "nvr-archive"
    access_key: "AKIAIOSFODNN7EXAMPLE"
    secret_key: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
preferences:
  ui:
    theme: "dark"
    language: "en"
    dashboard:
      grid_columns: 4
      show_fps: true
      show_bitrate: true
  timeline:
    default_range_hours: 24
    thumbnail_interval_seconds: 60
  events:
    auto_acknowledge_after_days: 7
    group_similar_events: true
    group_window_seconds: 300
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify various fields
	if cfg.System.Name != "Full Test NVR" {
		t.Errorf("Expected system name 'Full Test NVR', got '%s'", cfg.System.Name)
	}
	if cfg.System.Database.Type != "postgres" {
		t.Errorf("Expected database type 'postgres', got '%s'", cfg.System.Database.Type)
	}
	if cfg.System.Database.Host != "localhost" {
		t.Errorf("Expected database host 'localhost', got '%s'", cfg.System.Database.Host)
	}

	// Check camera
	if len(cfg.Cameras) != 1 {
		t.Errorf("Expected 1 camera, got %d", len(cfg.Cameras))
	}
	cam := cfg.GetCamera("garage")
	if cam == nil {
		t.Fatal("Camera 'garage' not found")
	}
	if cam.Manufacturer != "Hikvision" {
		t.Errorf("Expected manufacturer 'Hikvision', got '%s'", cam.Manufacturer)
	}
	if cam.Motion.Method != "mog2" {
		t.Errorf("Expected motion method 'mog2', got '%s'", cam.Motion.Method)
	}
	if len(cam.PTZ.Presets) != 2 {
		t.Errorf("Expected 2 PTZ presets, got %d", len(cam.PTZ.Presets))
	}
	if cam.Advanced.HWAccel != "videotoolbox" {
		t.Errorf("Expected hwaccel 'videotoolbox', got '%s'", cam.Advanced.HWAccel)
	}

	// Check detectors
	if cfg.Detectors.YOLO12.Model != "yolov12n.onnx" {
		t.Errorf("Expected YOLO12 model 'yolov12n.onnx', got '%s'", cfg.Detectors.YOLO12.Model)
	}
	if cfg.Detectors.FaceRecognition.Model != "buffalo_l" {
		t.Errorf("Expected face recognition model 'buffalo_l', got '%s'", cfg.Detectors.FaceRecognition.Model)
	}
	if len(cfg.Detectors.LPR.Formats) != 1 {
		t.Errorf("Expected 1 LPR format, got %d", len(cfg.Detectors.LPR.Formats))
	}

	// Check plugins
	if !cfg.Plugins["reolink"].Enabled {
		t.Error("Reolink plugin should be enabled")
	}

	// Check notifications
	if !cfg.Notifications.Enabled {
		t.Error("Notifications should be enabled")
	}
	if !cfg.Notifications.QuietHours.Enabled {
		t.Error("Quiet hours should be enabled")
	}
	if cfg.Notifications.QuietHours.Start != "22:00" {
		t.Errorf("Expected quiet hours start '22:00', got '%s'", cfg.Notifications.QuietHours.Start)
	}
	if !cfg.Notifications.Channels.Push.Enabled {
		t.Error("Push notifications should be enabled")
	}
	if !cfg.Notifications.Channels.Email.Enabled {
		t.Error("Email notifications should be enabled")
	}
	if !cfg.Notifications.Channels.Discord.Enabled {
		t.Error("Discord notifications should be enabled")
	}

	// Check storage
	if cfg.Storage.Recordings != "/data/recordings" {
		t.Errorf("Expected recordings path '/data/recordings', got '%s'", cfg.Storage.Recordings)
	}
	if !cfg.Storage.S3.Enabled {
		t.Error("S3 storage should be enabled")
	}
	if cfg.Storage.S3.Bucket != "nvr-archive" {
		t.Errorf("Expected S3 bucket 'nvr-archive', got '%s'", cfg.Storage.S3.Bucket)
	}

	// Check preferences
	if cfg.Preferences.UI.Theme != "dark" {
		t.Errorf("Expected theme 'dark', got '%s'", cfg.Preferences.UI.Theme)
	}
	if cfg.Preferences.UI.Dashboard.GridColumns != 4 {
		t.Errorf("Expected grid columns 4, got %d", cfg.Preferences.UI.Dashboard.GridColumns)
	}
	if cfg.Preferences.Timeline.DefaultRangeHours != 24 {
		t.Errorf("Expected default range 24 hours, got %d", cfg.Preferences.Timeline.DefaultRangeHours)
	}
}

func TestEncryptDecrypt(t *testing.T) {
	key := []byte("test-encryption-key-32-bytes!!") // Exactly 32 bytes
	if len(key) != 32 {
		// Pad to 32 bytes
		key = append(key, make([]byte, 32-len(key))...)
	}
	plaintext := "secret password"

	encrypted, err := encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encryption failed: %v", err)
	}

	if encrypted == plaintext {
		t.Error("Encrypted text should not equal plaintext")
	}

	decrypted, err := decrypt(key, encrypted)
	if err != nil {
		t.Fatalf("Decryption failed: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("Expected decrypted '%s', got '%s'", plaintext, decrypted)
	}
}

func TestDecryptInvalidData(t *testing.T) {
	key := []byte("12345678901234567890123456789012") // Exactly 32 bytes

	// Invalid base64
	_, err := decrypt(key, "not-valid-base64!!!")
	if err == nil {
		t.Error("Expected error for invalid base64")
	}

	// Too short ciphertext
	_, err = decrypt(key, "YWJj") // "abc" in base64
	if err == nil {
		t.Error("Expected error for too short ciphertext")
	}
}

func TestGetEncryptionKey(t *testing.T) {
	// Test with environment variable
	originalKey := os.Getenv("NVR_ENCRYPTION_KEY")
	defer os.Setenv("NVR_ENCRYPTION_KEY", originalKey)

	// Test with valid base64 key
	testKey := make([]byte, 32)
	for i := range testKey {
		testKey[i] = byte(i)
	}
	os.Setenv("NVR_ENCRYPTION_KEY", "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")
	key := getEncryptionKey()
	if len(key) != 32 {
		t.Errorf("Expected 32-byte key, got %d bytes", len(key))
	}

	// Test with invalid key (wrong length after decoding)
	os.Setenv("NVR_ENCRYPTION_KEY", "dGVzdA==") // "test" in base64 (4 bytes)
	key = getEncryptionKey()
	// Should fall back to default key
	if len(key) != 32 {
		t.Errorf("Expected 32-byte default key, got %d bytes", len(key))
	}

	// Test with invalid base64
	os.Setenv("NVR_ENCRYPTION_KEY", "not-valid-base64!!!")
	key = getEncryptionKey()
	if len(key) != 32 {
		t.Errorf("Expected 32-byte default key, got %d bytes", len(key))
	}

	// Test without environment variable
	os.Unsetenv("NVR_ENCRYPTION_KEY")
	key = getEncryptionKey()
	if len(key) != 32 {
		t.Errorf("Expected 32-byte default key, got %d bytes", len(key))
	}
}

func TestLoadWithEncryptedPassword(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// First, create a config with plaintext password
	configContent := `
version: "1.0"
system:
  name: "Test NVR"
cameras:
  - id: "cam1"
    name: "Test Camera"
    enabled: true
    stream:
      url: "rtsp://192.168.1.100:554/stream"
      password: "plaintext_password"
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Password should remain plaintext after load (only encrypted on save)
	cam := cfg.GetCamera("cam1")
	if cam == nil {
		t.Fatal("Camera not found")
	}
	if cam.Stream.Password != "plaintext_password" {
		t.Errorf("Expected plaintext password, got '%s'", cam.Stream.Password)
	}
}

func TestSaveCreatesValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	cfg := &Config{
		Version: "1.0",
		System: SystemConfig{
			Name:        "Test NVR",
			Timezone:    "UTC",
			StoragePath: "/data",
		},
		Cameras: []CameraConfig{
			{
				ID:      "cam1",
				Name:    "Test Camera",
				Enabled: true,
				Stream: StreamConfig{
					URL:      "rtsp://192.168.1.100:554/stream",
					Password: "secret",
				},
			},
		},
		encKey: []byte("12345678901234567890123456789012"), // Set encryption key for test
	}
	cfg.SetPath(configPath)

	err := cfg.Save()
	if err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Read the saved file
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read saved config: %v", err)
	}

	// Should contain header
	if !strings.Contains(string(data), "# NVR System Configuration") {
		t.Error("Saved config should contain header comment")
	}

	// Password should be encrypted
	if strings.Contains(string(data), "secret") && !strings.Contains(string(data), "encrypted:") {
		t.Error("Password should be encrypted in saved config")
	}
}
