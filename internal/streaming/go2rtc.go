// Package streaming provides go2rtc integration for video streaming
package streaming

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
	"runtime"
	"sync"
	"time"
)

const (
	// DefaultGo2RTCPort is the default API port for go2rtc
	DefaultGo2RTCPort = 1984
	// DefaultRTSPPort is the default RTSP port
	DefaultRTSPPort = 8554
	// DefaultWebRTCPort is the default WebRTC port
	DefaultWebRTCPort = 8555
)

// Go2RTCManager manages the go2rtc subprocess
type Go2RTCManager struct {
	configPath string
	binaryPath string
	apiURL     string
	cmd        *exec.Cmd
	mu         sync.RWMutex
	running    bool
	logger     *slog.Logger

	// Circuit breaker for restarts
	restartCount    int       // Number of restarts in current window
	lastRestartTime time.Time // Time of last restart
	circuitOpen     bool      // True if circuit breaker is open (no more restarts)
}

// NewGo2RTCManager creates a new go2rtc manager
func NewGo2RTCManager(configPath, binaryPath string) *Go2RTCManager {
	return NewGo2RTCManagerWithPort(configPath, binaryPath, DefaultGo2RTCPort)
}

// NewGo2RTCManagerWithPort creates a new go2rtc manager with a specific API port
func NewGo2RTCManagerWithPort(configPath, binaryPath string, apiPort int) *Go2RTCManager {
	return &Go2RTCManager{
		configPath: configPath,
		binaryPath: binaryPath,
		apiURL:     fmt.Sprintf("http://localhost:%d", apiPort),
		logger:     slog.Default().With("component", "go2rtc"),
	}
}

// APIURL returns the API base URL for go2rtc
func (m *Go2RTCManager) APIURL() string {
	return m.apiURL
}

// Start starts the go2rtc process
func (m *Go2RTCManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("go2rtc is already running")
	}

	// Find binary path
	binaryPath := m.binaryPath
	if binaryPath == "" {
		var err error
		binaryPath, err = m.findBinary()
		if err != nil {
			return fmt.Errorf("go2rtc binary not found: %w", err)
		}
	}

	m.logger.Info("Starting go2rtc", "binary", binaryPath, "config", m.configPath)

	// Build command
	args := []string{}
	if m.configPath != "" {
		args = append(args, "-config", m.configPath)
	}

	m.cmd = exec.CommandContext(ctx, binaryPath, args...)
	m.cmd.Stdout = &logWriter{logger: m.logger, level: slog.LevelInfo}
	m.cmd.Stderr = &logWriter{logger: m.logger, level: slog.LevelError}

	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start go2rtc: %w", err)
	}

	m.running = true

	// Don't block on ready check - do it async and log when ready
	// go2rtc typically starts in under 1 second, blocking for 10s hurts startup
	go func() {
		if err := m.waitForReady(context.Background(), 10*time.Second); err != nil {
			m.logger.Error("go2rtc failed to become ready", "error", err)
			return
		}
		m.logger.Info("go2rtc is ready")
	}()

	// Monitor process in background
	go m.monitor()

	return nil
}

// Stop stops the go2rtc process
func (m *Go2RTCManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running || m.cmd == nil || m.cmd.Process == nil {
		return nil
	}

	m.logger.Info("Stopping go2rtc")

	// Try graceful shutdown first
	if err := m.cmd.Process.Signal(os.Interrupt); err != nil {
		m.logger.Warn("Failed to send interrupt signal", "error", err)
	}

	// Wait a bit for graceful shutdown
	done := make(chan error, 1)
	go func() {
		done <- m.cmd.Wait()
	}()

	select {
	case <-done:
		// Process exited
	case <-time.After(5 * time.Second):
		// Force kill
		m.logger.Warn("Force killing go2rtc")
		if err := m.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill go2rtc: %w", err)
		}
	}

	m.running = false
	m.cmd = nil

	return nil
}

// Reload reloads the go2rtc configuration
func (m *Go2RTCManager) Reload() error {
	m.mu.RLock()
	if !m.running {
		m.mu.RUnlock()
		return fmt.Errorf("go2rtc is not running")
	}
	m.mu.RUnlock()

	m.logger.Info("Reloading go2rtc configuration")

	// go2rtc supports config reload via API
	resp, err := http.Post(m.apiURL+"/api/config/reload", "application/json", nil)
	if err != nil {
		// If API reload fails, restart the process
		m.logger.Warn("API reload failed, restarting go2rtc", "error", err)
		return m.Restart()
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return m.Restart()
	}

	return nil
}

// Restart restarts the go2rtc process
func (m *Go2RTCManager) Restart() error {
	ctx := context.Background()
	if err := m.Stop(); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)
	return m.Start(ctx)
}

// IsRunning returns whether go2rtc is running
func (m *Go2RTCManager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// GetAPIURL returns the go2rtc API URL
func (m *Go2RTCManager) GetAPIURL() string {
	return m.apiURL
}

// GetStreams returns the list of configured streams
func (m *Go2RTCManager) GetStreams() (map[string]interface{}, error) {
	resp, err := http.Get(m.apiURL + "/api/streams")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	// For now, just return empty map - will implement proper parsing
	return make(map[string]interface{}), nil
}

// AddStream adds a stream to go2rtc
func (m *Go2RTCManager) AddStream(name, url string) error {
	endpoint := fmt.Sprintf("%s/api/streams?name=%s&src=%s", m.apiURL, name, url)
	resp, err := http.Post(endpoint, "application/json", nil)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to add stream: %s", string(body))
	}

	return nil
}

// RemoveStream removes a stream from go2rtc
func (m *Go2RTCManager) RemoveStream(name string) error {
	req, err := http.NewRequest(http.MethodDelete, m.apiURL+"/api/streams?name="+name, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	return nil
}

// GetWebRTCURL returns the WebRTC WebSocket URL for a stream
func (m *Go2RTCManager) GetWebRTCURL(streamName string) string {
	return fmt.Sprintf("ws://localhost:%d/api/ws?src=%s", DefaultGo2RTCPort, streamName)
}

// GetBackchannelURL returns the backchannel WebSocket URL for a stream
// This enables sending audio from client to camera
func (m *Go2RTCManager) GetBackchannelURL(streamName string) string {
	return fmt.Sprintf("ws://localhost:%d/api/ws?src=%s&backchannel=1", DefaultGo2RTCPort, streamName)
}

// GetStreamInfo gets information about a stream from go2rtc
func (m *Go2RTCManager) GetStreamInfo(name string) (map[string]interface{}, error) {
	resp, err := http.Get(fmt.Sprintf("%s/api/streams?name=%s", m.apiURL, name))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}

// waitForReady waits for go2rtc to be ready
func (m *Go2RTCManager) waitForReady(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for go2rtc to start")
		case <-ticker.C:
			resp, err := http.Get(m.apiURL + "/api")
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
	}
}

// monitor monitors the go2rtc process and restarts if it crashes
func (m *Go2RTCManager) monitor() {
	if m.cmd == nil {
		return
	}

	err := m.cmd.Wait()

	m.mu.Lock()
	wasRunning := m.running
	m.running = false

	// Circuit breaker constants
	const (
		maxRestarts     = 5               // Max restarts before circuit opens
		resetWindow     = 5 * time.Minute // Window to reset restart count
		circuitCooldown = 2 * time.Minute // Time before circuit closes again
	)

	// Check if we should reset the restart count (long enough since last restart)
	if time.Since(m.lastRestartTime) > resetWindow {
		m.restartCount = 0
		m.circuitOpen = false
	}

	// Check circuit breaker
	if m.circuitOpen {
		// Check if cooldown has passed
		if time.Since(m.lastRestartTime) > circuitCooldown {
			m.logger.Info("Circuit breaker cooldown passed, allowing restart attempt")
			m.circuitOpen = false
			m.restartCount = 0
		} else {
			m.mu.Unlock()
			m.logger.Warn("Circuit breaker open, not restarting go2rtc",
				"cooldown_remaining", circuitCooldown-time.Since(m.lastRestartTime))
			return
		}
	}

	m.mu.Unlock()

	if wasRunning {
		m.logger.Error("go2rtc exited unexpectedly", "error", err)

		m.mu.Lock()
		m.restartCount++
		m.lastRestartTime = time.Now()

		if m.restartCount >= maxRestarts {
			m.circuitOpen = true
			m.mu.Unlock()
			m.logger.Error("Circuit breaker opened - too many go2rtc restarts",
				"restarts", m.restartCount,
				"cooldown", circuitCooldown)
			return
		}

		// Exponential backoff: 2s, 4s, 8s, 16s, 32s
		backoff := time.Duration(1<<m.restartCount) * time.Second
		if backoff > 32*time.Second {
			backoff = 32 * time.Second
		}
		m.mu.Unlock()

		m.logger.Info("Restarting go2rtc with backoff",
			"attempt", m.restartCount,
			"backoff", backoff)

		time.Sleep(backoff)
		if err := m.Start(context.Background()); err != nil {
			m.logger.Error("Failed to restart go2rtc", "error", err)
		}
	}
}

// findBinary finds the go2rtc binary
func (m *Go2RTCManager) findBinary() (string, error) {
	// Check common locations
	locations := []string{
		"./bin/go2rtc",
		"./go2rtc",
		"/usr/local/bin/go2rtc",
		"/usr/bin/go2rtc",
	}

	// Add OS-specific binary name
	if runtime.GOOS == "windows" {
		locations = append([]string{"./bin/go2rtc.exe", "./go2rtc.exe"}, locations...)
	}

	// Check if go2rtc is in PATH
	if path, err := exec.LookPath("go2rtc"); err == nil {
		return path, nil
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			abs, err := filepath.Abs(loc)
			if err == nil {
				return abs, nil
			}
			return loc, nil
		}
	}

	return "", fmt.Errorf("go2rtc binary not found in any standard location")
}

// logWriter wraps slog for cmd output
type logWriter struct {
	logger *slog.Logger
	level  slog.Level
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	msg := string(p)
	if len(msg) > 0 && msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-1]
	}
	if msg != "" {
		w.logger.Log(context.Background(), w.level, msg)
	}
	return len(p), nil
}
