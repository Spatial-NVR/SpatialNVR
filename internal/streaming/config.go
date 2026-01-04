package streaming

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Go2RTCConfig represents the go2rtc configuration file structure
type Go2RTCConfig struct {
	API     APIConfig              `yaml:"api,omitempty"`
	RTSP    RTSPConfig             `yaml:"rtsp,omitempty"`
	WebRTC  WebRTCConfig           `yaml:"webrtc,omitempty"`
	Streams map[string][]string    `yaml:"streams,omitempty"`
	Log     LogConfig              `yaml:"log,omitempty"`
}

// APIConfig represents go2rtc API configuration
type APIConfig struct {
	Listen   string `yaml:"listen,omitempty"`
	BasePath string `yaml:"base_path,omitempty"`
	Origin   string `yaml:"origin,omitempty"`
	TLSListen string `yaml:"tls_listen,omitempty"`
}

// RTSPConfig represents go2rtc RTSP configuration
type RTSPConfig struct {
	Listen       string `yaml:"listen,omitempty"`
	DefaultQuery string `yaml:"default_query,omitempty"`
}

// WebRTCConfig represents go2rtc WebRTC configuration
type WebRTCConfig struct {
	Listen     string   `yaml:"listen,omitempty"`
	Candidates []string `yaml:"candidates,omitempty"`
}

// LogConfig represents go2rtc logging configuration
type LogConfig struct {
	Level  string `yaml:"level,omitempty"`
	Format string `yaml:"format,omitempty"`
}

// CameraStream represents a camera's stream configuration
type CameraStream struct {
	ID       string
	Name     string
	URL      string
	Username string
	Password string
	SubURL   string // Sub-stream URL (optional)
}

// ConfigGenerator generates go2rtc configuration from camera configs
type ConfigGenerator struct {
	apiPort    int
	rtspPort   int
	webrtcPort int
}

// NewConfigGenerator creates a new config generator
func NewConfigGenerator() *ConfigGenerator {
	return &ConfigGenerator{
		apiPort:    DefaultGo2RTCPort,
		rtspPort:   DefaultRTSPPort,
		webrtcPort: DefaultWebRTCPort,
	}
}

// WithPorts sets custom ports for the generator
func (g *ConfigGenerator) WithPorts(api, rtsp, webrtc int) *ConfigGenerator {
	g.apiPort = api
	g.rtspPort = rtsp
	g.webrtcPort = webrtc
	return g
}

// Generate generates a go2rtc config from camera streams
func (g *ConfigGenerator) Generate(cameras []CameraStream) *Go2RTCConfig {
	config := &Go2RTCConfig{
		API: APIConfig{
			Listen:   fmt.Sprintf(":%d", g.apiPort),
			BasePath: "",
			// Allow all origins for development
			// go2rtc requires "*" to allow cross-origin WebSocket connections
			Origin:   "*",
		},
		RTSP: RTSPConfig{
			Listen:       fmt.Sprintf(":%d", g.rtspPort),
			DefaultQuery: "video&audio",
		},
		WebRTC: WebRTCConfig{
			Listen: fmt.Sprintf(":%d/tcp", g.webrtcPort),
			Candidates: []string{
				"stun:stun.l.google.com:19302",
			},
		},
		Streams: make(map[string][]string),
		Log: LogConfig{
			Level:  "info",
			Format: "text",
		},
	}

	for _, cam := range cameras {
		streamURL := g.buildStreamURL(cam.URL, cam.Username, cam.Password)
		streamName := sanitizeStreamName(cam.ID)
		rawStreamName := streamName + "_raw"

		// go2rtc stream configuration for audio transcoding:
		//
		// Problem: Many cameras output AAC at non-standard sample rates (e.g., 16kHz).
		// Browsers expect 44.1kHz or 48kHz for proper audio playback.
		//
		// Solution: Create two streams:
		// 1. Raw stream (_raw suffix): Direct RTSP connection to camera
		// 2. Main stream: Uses ffmpeg to transcode audio from raw stream
		//
		// The ffmpeg source copies video and transcodes audio to standard 48kHz.
		// This ensures ALL consumers (MSE, WebRTC, HLS) get properly encoded audio.
		//
		// For WebRTC specifically, go2rtc will further transcode to Opus as needed.

		// Raw stream - direct RTSP from camera (used as input for transcoding)
		config.Streams[rawStreamName] = []string{streamURL}

		// Main stream with multiple audio codec options:
		//
		// 1. exec:ffmpeg - Primary source with 48kHz AAC audio for MSE
		//    MSE requires AAC at standard sample rates (44.1kHz or 48kHz)
		//
		// 2. ffmpeg:...#audio=opus - Opus audio source for WebRTC
		//    WebRTC REQUIRES Opus codec for audio (doesn't support AAC)
		//    go2rtc will use this source when a WebRTC consumer connects
		//
		// FFmpeg args for primary source:
		// -hide_banner -v error: reduce noise
		// -fflags nobuffer -flags low_delay: minimize latency
		// -rtsp_transport tcp: use TCP for reliable delivery
		// -i rtsp://...: read from internal RTSP server (raw stream)
		// -c:v copy: pass video through unchanged
		// -c:a aac -ar 48000: transcode audio to 48kHz AAC
		// -f rtsp {output}: output back to go2rtc via RTSP
		config.Streams[streamName] = []string{
			// Primary source: 48kHz AAC for MSE consumers
			fmt.Sprintf("exec:ffmpeg -hide_banner -v error -fflags nobuffer -flags low_delay -rtsp_transport tcp -i rtsp://localhost:%d/%s -c:v copy -c:a aac -ar 48000 -f rtsp {output}", g.rtspPort, rawStreamName),
			// Opus audio source for WebRTC consumers
			fmt.Sprintf("ffmpeg:%s#audio=opus", streamName),
		}

		// Sub stream if available (raw, no transcoding needed for lower quality)
		if cam.SubURL != "" {
			subStreamURL := g.buildStreamURL(cam.SubURL, cam.Username, cam.Password)
			subStreamName := streamName + "_sub"
			config.Streams[subStreamName] = []string{subStreamURL}
		}
	}

	return config
}

// WriteToFile writes the configuration to a YAML file
func (g *ConfigGenerator) WriteToFile(config *Go2RTCConfig, path string) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Add header comment
	header := "# go2rtc configuration\n# Auto-generated by NVR System - manual edits may be overwritten\n\n"
	data = append([]byte(header), data...)

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// buildStreamURL builds a stream URL with credentials and transcoding options
func (g *ConfigGenerator) buildStreamURL(url, username, password string) string {
	result := url

	// Add credentials if provided AND URL doesn't already have credentials
	if username != "" && !urlHasCredentials(url) {
		// Parse and rebuild URL with credentials
		// Handle rtsp:// URLs
		if strings.HasPrefix(url, "rtsp://") {
			host := strings.TrimPrefix(url, "rtsp://")
			if password != "" {
				result = fmt.Sprintf("rtsp://%s:%s@%s", username, password, host)
			} else {
				result = fmt.Sprintf("rtsp://%s@%s", username, host)
			}
		} else if strings.HasPrefix(url, "http://") {
			// Handle http:// URLs
			host := strings.TrimPrefix(url, "http://")
			if password != "" {
				result = fmt.Sprintf("http://%s:%s@%s", username, password, host)
			} else {
				result = fmt.Sprintf("http://%s@%s", username, host)
			}
		} else if strings.HasPrefix(url, "https://") {
			// Handle https:// URLs
			host := strings.TrimPrefix(url, "https://")
			if password != "" {
				result = fmt.Sprintf("https://%s:%s@%s", username, password, host)
			} else {
				result = fmt.Sprintf("https://%s@%s", username, host)
			}
		}
	}

	return result
}

// urlHasCredentials checks if a URL already contains embedded credentials
func urlHasCredentials(urlStr string) bool {
	// Remove protocol prefix and check for @ before first /
	for _, proto := range []string{"rtsp://", "http://", "https://", "rtmp://"} {
		if strings.HasPrefix(urlStr, proto) {
			rest := strings.TrimPrefix(urlStr, proto)
			// Find the host part (before first /)
			slashIdx := strings.Index(rest, "/")
			hostPart := rest
			if slashIdx != -1 {
				hostPart = rest[:slashIdx]
			}
			// If there's an @ in the host part, credentials are present
			return strings.Contains(hostPart, "@")
		}
	}
	return false
}

// sanitizeStreamName ensures the stream name is valid for go2rtc
func sanitizeStreamName(name string) string {
	// Replace invalid characters with underscores
	replacer := strings.NewReplacer(
		" ", "_",
		"-", "_",
		".", "_",
		"/", "_",
		"\\", "_",
	)
	return strings.ToLower(replacer.Replace(name))
}

// GetStreamURL returns the go2rtc stream URL for a camera
func GetStreamURL(cameraID string, format string, apiPort int) string {
	streamName := sanitizeStreamName(cameraID)
	baseURL := fmt.Sprintf("http://localhost:%d", apiPort)

	switch format {
	case "rtsp":
		return fmt.Sprintf("rtsp://localhost:%d/%s", DefaultRTSPPort, streamName)
	case "webrtc":
		return fmt.Sprintf("%s/api/webrtc?src=%s", baseURL, streamName)
	case "hls":
		return fmt.Sprintf("%s/api/stream.m3u8?src=%s", baseURL, streamName)
	case "mse":
		return fmt.Sprintf("%s/api/ws?src=%s", baseURL, streamName)
	case "mjpeg":
		return fmt.Sprintf("%s/api/frame.jpeg?src=%s", baseURL, streamName)
	default:
		return fmt.Sprintf("%s/api/stream.m3u8?src=%s", baseURL, streamName)
	}
}
