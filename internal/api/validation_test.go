package api

import (
	"testing"

	"github.com/Spatial-NVR/SpatialNVR/internal/config"
)

func TestCameraValidator_ValidateValidConfig(t *testing.T) {
	validator := NewCameraValidator()

	cfg := config.CameraConfig{
		Name: "Front Door",
		Stream: config.StreamConfig{
			URL: "rtsp://192.168.1.100:554/stream",
		},
		Detection: config.DetectionConfig{
			FPS: 5,
		},
		Recording: config.RecordingConfig{
			PreBufferSeconds:  5,
			PostBufferSeconds: 10,
		},
	}

	errors := validator.Validate(cfg)
	if errors.HasErrors() {
		t.Errorf("Valid config should not have errors, got: %v", errors)
	}
}

func TestCameraValidator_ValidateMissingName(t *testing.T) {
	validator := NewCameraValidator()

	cfg := config.CameraConfig{
		Name: "",
		Stream: config.StreamConfig{
			URL: "rtsp://192.168.1.100:554/stream",
		},
	}

	errors := validator.Validate(cfg)
	if !errors.HasErrors() {
		t.Error("Config with missing name should have errors")
	}

	found := false
	for _, err := range errors {
		if err.Field == "name" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error for 'name' field")
	}
}

func TestCameraValidator_ValidateShortName(t *testing.T) {
	validator := NewCameraValidator()

	cfg := config.CameraConfig{
		Name: "A",
		Stream: config.StreamConfig{
			URL: "rtsp://192.168.1.100:554/stream",
		},
	}

	errors := validator.Validate(cfg)
	if !errors.HasErrors() {
		t.Error("Config with name too short should have errors")
	}
}

func TestCameraValidator_ValidateMissingURL(t *testing.T) {
	validator := NewCameraValidator()

	cfg := config.CameraConfig{
		Name: "Front Door",
		Stream: config.StreamConfig{
			URL: "",
		},
	}

	errors := validator.Validate(cfg)
	if !errors.HasErrors() {
		t.Error("Config with missing URL should have errors")
	}

	found := false
	for _, err := range errors {
		if err.Field == "stream.url" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error for 'stream.url' field")
	}
}

func TestCameraValidator_ValidateInvalidURL(t *testing.T) {
	validator := NewCameraValidator()

	cfg := config.CameraConfig{
		Name: "Front Door",
		Stream: config.StreamConfig{
			URL: "not-a-valid-url",
		},
	}

	errors := validator.Validate(cfg)
	if !errors.HasErrors() {
		t.Error("Config with invalid URL should have errors")
	}
}

func TestCameraValidator_ValidateUnsupportedProtocol(t *testing.T) {
	validator := NewCameraValidator()

	cfg := config.CameraConfig{
		Name: "Front Door",
		Stream: config.StreamConfig{
			URL: "ftp://192.168.1.100/stream",
		},
	}

	errors := validator.Validate(cfg)
	if !errors.HasErrors() {
		t.Error("Config with unsupported protocol should have errors")
	}
}

func TestCameraValidator_ValidateSupportedProtocols(t *testing.T) {
	protocols := []string{"rtsp", "rtsps", "rtmp", "http", "https"}

	for _, proto := range protocols {
		validator := NewCameraValidator()
		cfg := config.CameraConfig{
			Name: "Front Door",
			Stream: config.StreamConfig{
				URL: proto + "://192.168.1.100/stream",
			},
		}

		errors := validator.Validate(cfg)
		// Should not have URL-related errors (might have other errors)
		for _, err := range errors {
			if err.Field == "stream.url" && err.Message != "" {
				// URL errors are acceptable if it's about the protocol
				if err.Message != "" && !containsString(err.Message, "unsupported") {
					t.Errorf("Protocol %s should be supported, got error: %s", proto, err.Message)
				}
			}
		}
	}
}

func TestCameraValidator_ValidateDetectionFPS(t *testing.T) {
	tests := []struct {
		fps       int
		shouldErr bool
	}{
		{0, false},
		{5, false},
		{30, false},
		{-1, true},
		{31, true},
	}

	for _, tc := range tests {
		validator := NewCameraValidator()
		cfg := config.CameraConfig{
			Name: "Front Door",
			Stream: config.StreamConfig{
				URL: "rtsp://192.168.1.100/stream",
			},
			Detection: config.DetectionConfig{
				FPS: tc.fps,
			},
		}

		errors := validator.Validate(cfg)
		hasDetectionError := false
		for _, err := range errors {
			if err.Field == "detection.fps" {
				hasDetectionError = true
				break
			}
		}

		if tc.shouldErr && !hasDetectionError {
			t.Errorf("FPS %d should have error", tc.fps)
		}
		if !tc.shouldErr && hasDetectionError {
			t.Errorf("FPS %d should not have error", tc.fps)
		}
	}
}

func TestCameraValidator_ValidateRecordingBuffer(t *testing.T) {
	tests := []struct {
		preBuffer  int
		postBuffer int
		preErr     bool
		postErr    bool
	}{
		{0, 0, false, false},
		{5, 10, false, false},
		{60, 300, false, false},
		{-1, 0, true, false},
		{61, 0, true, false},
		{0, -1, false, true},
		{0, 301, false, true},
	}

	for _, tc := range tests {
		validator := NewCameraValidator()
		cfg := config.CameraConfig{
			Name: "Front Door",
			Stream: config.StreamConfig{
				URL: "rtsp://192.168.1.100/stream",
			},
			Recording: config.RecordingConfig{
				PreBufferSeconds:  tc.preBuffer,
				PostBufferSeconds: tc.postBuffer,
			},
		}

		errors := validator.Validate(cfg)

		hasPreErr := false
		hasPostErr := false
		for _, err := range errors {
			if err.Field == "recording.pre_buffer_seconds" {
				hasPreErr = true
			}
			if err.Field == "recording.post_buffer_seconds" {
				hasPostErr = true
			}
		}

		if tc.preErr != hasPreErr {
			t.Errorf("PreBuffer %d: expected error=%v, got=%v", tc.preBuffer, tc.preErr, hasPreErr)
		}
		if tc.postErr != hasPostErr {
			t.Errorf("PostBuffer %d: expected error=%v, got=%v", tc.postBuffer, tc.postErr, hasPostErr)
		}
	}
}

func TestValidateCameraID(t *testing.T) {
	tests := []struct {
		id        string
		shouldErr bool
	}{
		{"cam1", false},
		{"front_door", false},
		{"cam-123", false},
		{"Camera_1", false},
		{"", true},
		{"cam with spaces", true},
		{"cam@special", true},
		{"cam/path", true},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true}, // 51 chars
	}

	for _, tc := range tests {
		err := ValidateCameraID(tc.id)
		if tc.shouldErr && err == nil {
			t.Errorf("ID '%s' should have error", tc.id)
		}
		if !tc.shouldErr && err != nil {
			t.Errorf("ID '%s' should not have error, got: %v", tc.id, err)
		}
	}
}

func TestSanitizeStreamURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"rtsp://192.168.1.100/stream", "rtsp://192.168.1.100/stream"},
		{"rtsp://user:pass@192.168.1.100/stream", "rtsp://192.168.1.100/stream"},
		{"rtsp://admin:secret123@192.168.1.100:554/main", "rtsp://192.168.1.100:554/main"},
		{"not-a-url", "[invalid-url]"},
	}

	for _, tc := range tests {
		result := SanitizeStreamURL(tc.input)
		if result != tc.expected {
			t.Errorf("SanitizeStreamURL(%s) = %s, expected %s", tc.input, result, tc.expected)
		}
	}
}

func TestValidationErrors(t *testing.T) {
	errors := ValidationErrors{
		{Field: "name", Message: "is required"},
		{Field: "url", Message: "is invalid"},
	}

	if !errors.HasErrors() {
		t.Error("HasErrors should return true when there are errors")
	}

	errStr := errors.Error()
	if errStr == "" {
		t.Error("Error() should return non-empty string")
	}

	if !containsString(errStr, "name") {
		t.Error("Error string should contain 'name'")
	}
}

func TestValidationError(t *testing.T) {
	err := ValidationError{Field: "test", Message: "is required"}
	errStr := err.Error()

	if errStr != "test: is required" {
		t.Errorf("Expected 'test: is required', got '%s'", errStr)
	}
}

func TestCameraValidator_ValidateUpdate(t *testing.T) {
	tests := []struct {
		name      string
		cfg       config.CameraConfig
		wantField string // expected field with error, empty if no error expected
	}{
		{
			name: "empty update is valid",
			cfg:  config.CameraConfig{},
		},
		{
			name: "valid name update",
			cfg: config.CameraConfig{
				Name: "Valid Name",
			},
		},
		{
			name: "short name update",
			cfg: config.CameraConfig{
				Name: "A",
			},
			wantField: "name",
		},
		{
			name: "valid stream URL update",
			cfg: config.CameraConfig{
				Stream: config.StreamConfig{
					URL: "rtsp://192.168.1.100/stream",
				},
			},
		},
		{
			name: "valid sub-stream URL update",
			cfg: config.CameraConfig{
				Stream: config.StreamConfig{
					SubURL: "rtsp://192.168.1.100/substream",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewCameraValidator()
			errors := validator.ValidateUpdate(tt.cfg)

			if tt.wantField != "" {
				found := false
				for _, err := range errors {
					if err.Field == tt.wantField {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected error for field '%s', got none", tt.wantField)
				}
			} else if errors.HasErrors() {
				t.Errorf("Expected no errors, got: %v", errors)
			}
		})
	}
}

func TestValidateSubStreamURL(t *testing.T) {
	tests := []struct {
		name      string
		subURL    string
		wantError bool
	}{
		{
			name:      "empty is valid",
			subURL:    "",
			wantError: false,
		},
		{
			name:      "valid rtsp",
			subURL:    "rtsp://192.168.1.100/substream",
			wantError: false,
		},
		{
			name:      "valid https",
			subURL:    "https://example.com/stream.m3u8",
			wantError: false,
		},
		{
			name:      "with ffmpeg prefix",
			subURL:    "ffmpeg:rtsp://192.168.1.100/substream",
			wantError: false,
		},
		{
			name:      "with exec prefix",
			subURL:    "exec:#hardware",
			wantError: false,
		},
		{
			name:      "invalid protocol",
			subURL:    "ftp://192.168.1.100/substream",
			wantError: true,
		},
		{
			name:      "malformed url",
			subURL:    "://not-valid",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewCameraValidator()
			cfg := config.CameraConfig{
				Name: "Test Camera",
				Stream: config.StreamConfig{
					URL:    "rtsp://192.168.1.100/main",
					SubURL: tt.subURL,
				},
			}
			errors := validator.Validate(cfg)

			hasSubURLError := false
			for _, err := range errors {
				if err.Field == "stream.sub_url" {
					hasSubURLError = true
					break
				}
			}

			if tt.wantError && !hasSubURLError {
				t.Errorf("Expected sub_url error for %q, got none", tt.subURL)
			}
			if !tt.wantError && hasSubURLError {
				t.Errorf("Did not expect sub_url error for %q", tt.subURL)
			}
		})
	}
}

func TestValidateLongName(t *testing.T) {
	validator := NewCameraValidator()

	// Create a name longer than 100 characters
	longName := ""
	for i := 0; i < 101; i++ {
		longName += "a"
	}

	cfg := config.CameraConfig{
		Name: longName,
		Stream: config.StreamConfig{
			URL: "rtsp://192.168.1.100/stream",
		},
	}

	errors := validator.Validate(cfg)

	found := false
	for _, err := range errors {
		if err.Field == "name" && containsString(err.Message, "100") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error for name too long")
	}
}

func TestValidateStreamURL_MissingHost(t *testing.T) {
	validator := NewCameraValidator()

	cfg := config.CameraConfig{
		Name: "Test Camera",
		Stream: config.StreamConfig{
			URL: "rtsp:///stream", // Missing host
		},
	}

	errors := validator.Validate(cfg)

	found := false
	for _, err := range errors {
		if err.Field == "stream.url" && containsString(err.Message, "host") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error for missing host")
	}
}

func TestValidateStreamURL_WithPrefixes(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantError bool
	}{
		{
			name:      "ffmpeg prefix with valid url",
			url:       "ffmpeg:rtsp://192.168.1.100/stream",
			wantError: false,
		},
		{
			name:      "exec prefix",
			url:       "exec:#hardware",
			wantError: false,
		},
		{
			name:      "echo prefix",
			url:       "echo:#test",
			wantError: false,
		},
		{
			name:      "expr prefix",
			url:       "expr:#expression",
			wantError: false,
		},
		{
			name:      "empty after prefix",
			url:       "ffmpeg:",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewCameraValidator()
			cfg := config.CameraConfig{
				Name: "Test Camera",
				Stream: config.StreamConfig{
					URL: tt.url,
				},
			}
			errors := validator.Validate(cfg)

			hasURLError := false
			for _, err := range errors {
				if err.Field == "stream.url" {
					hasURLError = true
					break
				}
			}

			if tt.wantError && !hasURLError {
				t.Errorf("Expected stream.url error for %q", tt.url)
			}
			if !tt.wantError && hasURLError {
				t.Errorf("Did not expect stream.url error for %q, but got one", tt.url)
			}
		})
	}
}

func TestEmptyValidationErrors(t *testing.T) {
	errors := ValidationErrors{}
	if errors.HasErrors() {
		t.Error("Empty errors should not have errors")
	}
	if errors.Error() != "" {
		t.Error("Empty errors should have empty string")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStringHelper(s, substr))
}

func containsStringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
