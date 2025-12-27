// Package core provides port management for NVR services.
// Standard services (go2rtc, Web UI) use their conventional ports.
// Internal services use ports starting at 12000 to avoid conflicts.
package core

import (
	"fmt"
	"net"
	"sync"
)

// Default port assignments
const (
	// Core services
	DefaultAPIPort  = 8080  // Main NVR API and Web UI (standard web port)
	DefaultNATSPort = 4222  // Embedded NATS event bus (standard NATS port)

	// Web UI - same as API (static files served by Go backend)
	DefaultWebUIPort = 8080 // Web UI served on same port as API

	// Streaming services - standard go2rtc ports
	DefaultGo2RTCAPIPort    = 1984 // go2rtc API (standard)
	DefaultGo2RTCRTSPPort   = 8554 // go2rtc RTSP (standard)
	DefaultGo2RTCWebRTCPort = 8555 // go2rtc WebRTC (standard)

	// Plugin services - internal ports (12000+ range)
	DefaultSpatialPort   = 12020 // Spatial tracking plugin
	DefaultDetectionPort = 12021 // Detection service (gRPC)

	// Reserved range for dynamic allocation
	DynamicPortStart = 12100
	DynamicPortEnd   = 12999
)

// PortManager handles port allocation and conflict detection
type PortManager struct {
	mu           sync.RWMutex
	allocated    map[int]string // port -> service name
	nextDynamic  int
}

// NewPortManager creates a new port manager
func NewPortManager() *PortManager {
	return &PortManager{
		allocated:   make(map[int]string),
		nextDynamic: DynamicPortStart,
	}
}

// globalPortManager is the singleton port manager
var (
	globalPortManager     *PortManager
	globalPortManagerOnce sync.Once
	currentPortConfig     *PortConfig
	portConfigMu          sync.RWMutex
)

// GetPortManager returns the global port manager
func GetPortManager() *PortManager {
	globalPortManagerOnce.Do(func() {
		globalPortManager = NewPortManager()
	})
	return globalPortManager
}

// IsPortAvailable checks if a port is available for binding
func IsPortAvailable(port int) bool {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// Reserve reserves a port for a service
// Returns the port and true if successful, or 0 and false if the port is taken
func (pm *PortManager) Reserve(port int, serviceName string) (int, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Check if already allocated by us
	if existing, ok := pm.allocated[port]; ok {
		if existing == serviceName {
			return port, true // Same service already has it
		}
		return 0, false // Different service has it
	}

	// Check if port is actually available on the system
	if !IsPortAvailable(port) {
		return 0, false
	}

	pm.allocated[port] = serviceName
	return port, true
}

// ReserveOrFind reserves the preferred port or finds an available one
func (pm *PortManager) ReserveOrFind(preferredPort int, serviceName string) (int, error) {
	// Try preferred port first
	if port, ok := pm.Reserve(preferredPort, serviceName); ok {
		return port, nil
	}

	// Find next available dynamic port
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for port := pm.nextDynamic; port <= DynamicPortEnd; port++ {
		if _, exists := pm.allocated[port]; !exists && IsPortAvailable(port) {
			pm.allocated[port] = serviceName
			pm.nextDynamic = port + 1
			return port, nil
		}
	}

	return 0, fmt.Errorf("no available ports for service %s", serviceName)
}

// Release releases a port
func (pm *PortManager) Release(port int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.allocated, port)
}

// GetAllocated returns all allocated ports
func (pm *PortManager) GetAllocated() map[int]string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := make(map[int]string, len(pm.allocated))
	for k, v := range pm.allocated {
		result[k] = v
	}
	return result
}

// PortConfig holds the resolved port configuration for all services
type PortConfig struct {
	API          int `json:"api"`
	NATS         int `json:"nats"`
	WebUI        int `json:"web_ui"`
	Go2RTCAPI    int `json:"go2rtc_api"`
	Go2RTCRTSP   int `json:"go2rtc_rtsp"`
	Go2RTCWebRTC int `json:"go2rtc_webrtc"`
	Spatial      int `json:"spatial"`
	Detection    int `json:"detection"`
}

// ResolveAllPorts allocates all required ports, finding alternatives if needed
func (pm *PortManager) ResolveAllPorts() (*PortConfig, error) {
	config := &PortConfig{}
	var err error

	// Core services
	config.API, err = pm.ReserveOrFind(DefaultAPIPort, "nvr-api")
	if err != nil {
		return nil, fmt.Errorf("failed to allocate API port: %w", err)
	}

	config.NATS, err = pm.ReserveOrFind(DefaultNATSPort, "nats")
	if err != nil {
		return nil, fmt.Errorf("failed to allocate NATS port: %w", err)
	}

	config.WebUI, err = pm.ReserveOrFind(DefaultWebUIPort, "web-ui")
	if err != nil {
		return nil, fmt.Errorf("failed to allocate WebUI port: %w", err)
	}

	// Streaming services
	config.Go2RTCAPI, err = pm.ReserveOrFind(DefaultGo2RTCAPIPort, "go2rtc-api")
	if err != nil {
		return nil, fmt.Errorf("failed to allocate go2rtc API port: %w", err)
	}

	config.Go2RTCRTSP, err = pm.ReserveOrFind(DefaultGo2RTCRTSPPort, "go2rtc-rtsp")
	if err != nil {
		return nil, fmt.Errorf("failed to allocate go2rtc RTSP port: %w", err)
	}

	config.Go2RTCWebRTC, err = pm.ReserveOrFind(DefaultGo2RTCWebRTCPort, "go2rtc-webrtc")
	if err != nil {
		return nil, fmt.Errorf("failed to allocate go2rtc WebRTC port: %w", err)
	}

	// Plugin services
	config.Spatial, err = pm.ReserveOrFind(DefaultSpatialPort, "spatial-tracking")
	if err != nil {
		return nil, fmt.Errorf("failed to allocate Spatial port: %w", err)
	}

	config.Detection, err = pm.ReserveOrFind(DefaultDetectionPort, "detection")
	if err != nil {
		return nil, fmt.Errorf("failed to allocate Detection port: %w", err)
	}

	// Store for global access
	SetCurrentPortConfig(config)

	return config, nil
}

// SetCurrentPortConfig stores the current port configuration
func SetCurrentPortConfig(config *PortConfig) {
	portConfigMu.Lock()
	defer portConfigMu.Unlock()
	currentPortConfig = config
}

// GetCurrentPortConfig returns the current port configuration
func GetCurrentPortConfig() *PortConfig {
	portConfigMu.RLock()
	defer portConfigMu.RUnlock()
	if currentPortConfig == nil {
		// Return defaults if not set yet
		return &PortConfig{
			API:          DefaultAPIPort,
			NATS:         DefaultNATSPort,
			WebUI:        DefaultWebUIPort,
			Go2RTCAPI:    DefaultGo2RTCAPIPort,
			Go2RTCRTSP:   DefaultGo2RTCRTSPPort,
			Go2RTCWebRTC: DefaultGo2RTCWebRTCPort,
			Spatial:      DefaultSpatialPort,
			Detection:    DefaultDetectionPort,
		}
	}
	return currentPortConfig
}
