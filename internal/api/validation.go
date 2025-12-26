package api

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/Spatial-NVR/SpatialNVR/internal/config"
)

// ValidationError represents a validation error with field information
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationErrors holds multiple validation errors
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	var msgs []string
	for _, err := range e {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "; ")
}

// HasErrors returns true if there are validation errors
func (e ValidationErrors) HasErrors() bool {
	return len(e) > 0
}

// CameraValidator validates camera configuration
type CameraValidator struct {
	errors ValidationErrors
}

// NewCameraValidator creates a new camera validator
func NewCameraValidator() *CameraValidator {
	return &CameraValidator{
		errors: make(ValidationErrors, 0),
	}
}

// Validate validates a camera configuration
func (v *CameraValidator) Validate(cfg config.CameraConfig) ValidationErrors {
	v.errors = make(ValidationErrors, 0)

	v.validateName(cfg.Name)
	v.validateStreamURL(cfg.Stream.URL)
	v.validateSubStreamURL(cfg.Stream.SubURL)
	v.validateDetection(cfg.Detection)
	v.validateRecording(cfg.Recording)

	return v.errors
}

// ValidateUpdate validates a camera update (allows partial updates)
func (v *CameraValidator) ValidateUpdate(cfg config.CameraConfig) ValidationErrors {
	v.errors = make(ValidationErrors, 0)

	// Only validate fields that are set
	if cfg.Name != "" {
		v.validateName(cfg.Name)
	}
	if cfg.Stream.URL != "" {
		v.validateStreamURL(cfg.Stream.URL)
	}
	if cfg.Stream.SubURL != "" {
		v.validateSubStreamURL(cfg.Stream.SubURL)
	}

	return v.errors
}

func (v *CameraValidator) validateName(name string) {
	if name == "" {
		v.errors = append(v.errors, ValidationError{
			Field:   "name",
			Message: "camera name is required",
		})
		return
	}

	if len(name) < 2 {
		v.errors = append(v.errors, ValidationError{
			Field:   "name",
			Message: "camera name must be at least 2 characters",
		})
	}

	if len(name) > 100 {
		v.errors = append(v.errors, ValidationError{
			Field:   "name",
			Message: "camera name must be less than 100 characters",
		})
	}
}

func (v *CameraValidator) validateStreamURL(streamURL string) {
	if streamURL == "" {
		v.errors = append(v.errors, ValidationError{
			Field:   "stream.url",
			Message: "stream URL is required",
		})
		return
	}

	// Handle go2rtc prefixes (ffmpeg:, exec:, etc.)
	// These wrap the actual URL, so we validate the inner URL
	urlToValidate := streamURL
	validPrefixes := []string{"ffmpeg:", "exec:", "echo:", "expr:"}
	for _, prefix := range validPrefixes {
		if strings.HasPrefix(strings.ToLower(streamURL), prefix) {
			urlToValidate = streamURL[len(prefix):]
			break
		}
	}

	// If the URL is just a prefix command (like exec:), skip standard URL validation
	if urlToValidate == "" || strings.HasPrefix(urlToValidate, "#") {
		return
	}

	// Parse URL
	u, err := url.Parse(urlToValidate)
	if err != nil {
		v.errors = append(v.errors, ValidationError{
			Field:   "stream.url",
			Message: "invalid URL format",
		})
		return
	}

	// Check scheme
	validSchemes := map[string]bool{
		"rtsp":  true,
		"rtsps": true,
		"rtmp":  true,
		"http":  true,
		"https": true,
	}

	if !validSchemes[strings.ToLower(u.Scheme)] {
		v.errors = append(v.errors, ValidationError{
			Field:   "stream.url",
			Message: fmt.Sprintf("unsupported stream protocol '%s'. Supported: rtsp, rtsps, rtmp, http, https (with optional ffmpeg: prefix)", u.Scheme),
		})
	}

	// Check host
	if u.Host == "" {
		v.errors = append(v.errors, ValidationError{
			Field:   "stream.url",
			Message: "stream URL must include a host",
		})
	}
}

func (v *CameraValidator) validateSubStreamURL(subURL string) {
	if subURL == "" {
		return // Sub-stream is optional
	}

	// Handle go2rtc prefixes (ffmpeg:, exec:, etc.)
	urlToValidate := subURL
	validPrefixes := []string{"ffmpeg:", "exec:", "echo:", "expr:"}
	for _, prefix := range validPrefixes {
		if strings.HasPrefix(strings.ToLower(subURL), prefix) {
			urlToValidate = subURL[len(prefix):]
			break
		}
	}

	if urlToValidate == "" || strings.HasPrefix(urlToValidate, "#") {
		return
	}

	u, err := url.Parse(urlToValidate)
	if err != nil {
		v.errors = append(v.errors, ValidationError{
			Field:   "stream.sub_url",
			Message: "invalid sub-stream URL format",
		})
		return
	}

	validSchemes := map[string]bool{
		"rtsp":  true,
		"rtsps": true,
		"rtmp":  true,
		"http":  true,
		"https": true,
	}

	if !validSchemes[strings.ToLower(u.Scheme)] {
		v.errors = append(v.errors, ValidationError{
			Field:   "stream.sub_url",
			Message: fmt.Sprintf("unsupported sub-stream protocol '%s'", u.Scheme),
		})
	}
}

func (v *CameraValidator) validateDetection(cfg config.DetectionConfig) {
	if cfg.FPS < 0 || cfg.FPS > 30 {
		v.errors = append(v.errors, ValidationError{
			Field:   "detection.fps",
			Message: "detection FPS must be between 0 and 30",
		})
	}
}

func (v *CameraValidator) validateRecording(cfg config.RecordingConfig) {
	if cfg.PreBufferSeconds < 0 || cfg.PreBufferSeconds > 60 {
		v.errors = append(v.errors, ValidationError{
			Field:   "recording.pre_buffer_seconds",
			Message: "pre-buffer must be between 0 and 60 seconds",
		})
	}

	if cfg.PostBufferSeconds < 0 || cfg.PostBufferSeconds > 300 {
		v.errors = append(v.errors, ValidationError{
			Field:   "recording.post_buffer_seconds",
			Message: "post-buffer must be between 0 and 300 seconds",
		})
	}
}

// ValidateCameraID validates a camera ID format
func ValidateCameraID(id string) error {
	if id == "" {
		return fmt.Errorf("camera ID is required")
	}

	// ID should be alphanumeric with underscores
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_-]+$`, id)
	if !matched {
		return fmt.Errorf("camera ID must contain only letters, numbers, underscores, and hyphens")
	}

	if len(id) > 50 {
		return fmt.Errorf("camera ID must be less than 50 characters")
	}

	return nil
}

// SanitizeStreamURL removes credentials from a URL for logging
func SanitizeStreamURL(streamURL string) string {
	u, err := url.Parse(streamURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "[invalid-url]"
	}

	// Remove user info
	u.User = nil

	return u.String()
}
