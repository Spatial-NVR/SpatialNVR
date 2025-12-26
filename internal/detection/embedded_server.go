package detection

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// EmbeddedServer is a built-in detection server that runs in-process
// It provides a basic detection backend for the NVR when no external
// detection service is configured
type EmbeddedServer struct {
	mu       sync.RWMutex
	server   *http.Server
	listener net.Listener
	logger   *slog.Logger
	port     int

	// Stats
	startTime      time.Time
	processedCount int64
	errorCount     int64

	// Models (mock for now)
	models []ModelInfo
}

// EmbeddedServerConfig holds embedded server configuration
type EmbeddedServerConfig struct {
	Port   int
	Logger *slog.Logger
}

// NewEmbeddedServer creates a new embedded detection server
func NewEmbeddedServer(cfg EmbeddedServerConfig) *EmbeddedServer {
	if cfg.Port == 0 {
		cfg.Port = 50051
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &EmbeddedServer{
		port:      cfg.Port,
		logger:    cfg.Logger.With("component", "embedded-detection"),
		models:    []ModelInfo{},
		startTime: time.Now(),
	}
}

// Start starts the embedded detection server
func (s *EmbeddedServer) Start(ctx context.Context) error {
	r := chi.NewRouter()

	// Detection endpoints
	r.Post("/detect", s.handleDetect)
	r.Get("/status", s.handleStatus)
	r.Get("/models", s.handleListModels)
	r.Post("/load", s.handleLoadModel)
	r.Post("/unload", s.handleUnloadModel)
	r.Get("/backends", s.handleListBackends)
	r.Get("/motion", s.handleMotionStatus)
	r.Post("/motion/config", s.handleMotionConfig)
	r.Post("/motion/reset", s.handleMotionReset)

	// Create listener
	addr := fmt.Sprintf(":%d", s.port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	s.listener = listener

	// Get actual port (in case 0 was used)
	s.port = listener.Addr().(*net.TCPAddr).Port

	s.server = &http.Server{
		Handler: r,
	}

	s.startTime = time.Now()
	s.logger.Info("Embedded detection server starting", "port", s.port)

	// Run server in background
	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			s.logger.Error("Embedded detection server error", "error", err)
		}
	}()

	return nil
}

// Stop stops the embedded server
func (s *EmbeddedServer) Stop(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// Port returns the port the server is listening on
func (s *EmbeddedServer) Port() int {
	return s.port
}

// Address returns the full address (localhost:port)
func (s *EmbeddedServer) Address() string {
	return fmt.Sprintf("localhost:%d", s.port)
}

// HTTP Handlers

func (s *EmbeddedServer) handleDetect(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.processedCount++
	s.mu.Unlock()

	var req struct {
		CameraID      string  `json:"camera_id"`
		ImageData     string  `json:"image_data"` // Base64 encoded
		MinConfidence float64 `json:"min_confidence"`
		Objects       []string `json:"objects"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Decode image to verify it's valid
	if req.ImageData != "" {
		_, err := base64.StdEncoding.DecodeString(req.ImageData)
		if err != nil {
			s.respondError(w, http.StatusBadRequest, "Invalid image data")
			return
		}
	}

	// For now, return empty detections (no model loaded yet)
	// In the future, this would run actual inference
	response := map[string]interface{}{
		"success":         true,
		"camera_id":       req.CameraID,
		"timestamp":       time.Now().UnixMilli(),
		"motion_detected": false,
		"detections":      []interface{}{},
		"process_time_ms": 0.5,
		"model_id":        "",
	}

	s.respondJSON(w, response)
}

func (s *EmbeddedServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	processed := s.processedCount
	errors := s.errorCount
	s.mu.RUnlock()

	uptime := time.Since(s.startTime).Seconds()

	avgLatency := 0.0
	if processed > 0 {
		avgLatency = 0.5 // Mock value
	}

	response := map[string]interface{}{
		"connected":       true,
		"processed_count": processed,
		"error_count":     errors,
		"avg_latency_ms":  avgLatency,
		"uptime":          uptime,
	}

	s.respondJSON(w, response)
}

func (s *EmbeddedServer) handleListModels(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	models := s.models
	s.mu.RUnlock()

	// Convert to response format
	modelList := make([]map[string]interface{}, 0, len(models))
	for _, m := range models {
		modelList = append(modelList, map[string]interface{}{
			"id":      m.ID,
			"name":    m.Name,
			"type":    string(m.Type),
			"backend": string(m.Backend),
			"path":    m.Path,
			"loaded":  m.Loaded,
		})
	}

	s.respondJSON(w, map[string]interface{}{
		"models": modelList,
	})
}

func (s *EmbeddedServer) handleLoadModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path    string `json:"path"`
		Type    string `json:"type"`
		Backend string `json:"backend"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Generate model ID
	modelID := fmt.Sprintf("model_%d", time.Now().UnixNano())

	s.mu.Lock()
	s.models = append(s.models, ModelInfo{
		ID:      modelID,
		Name:    req.Path,
		Type:    ModelType(req.Type),
		Backend: BackendType(req.Backend),
		Path:    req.Path,
		Loaded:  true,
	})
	s.mu.Unlock()

	s.logger.Info("Model loaded", "model_id", modelID, "path", req.Path)

	s.respondJSON(w, map[string]interface{}{
		"success":  true,
		"model_id": modelID,
	})
}

func (s *EmbeddedServer) handleUnloadModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ModelID string `json:"model_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	s.mu.Lock()
	for i, m := range s.models {
		if m.ID == req.ModelID {
			s.models = append(s.models[:i], s.models[i+1:]...)
			break
		}
	}
	s.mu.Unlock()

	s.logger.Info("Model unloaded", "model_id", req.ModelID)

	s.respondJSON(w, map[string]interface{}{
		"success": true,
	})
}

func (s *EmbeddedServer) handleListBackends(w http.ResponseWriter, r *http.Request) {
	// Report available backends based on platform
	backends := []map[string]interface{}{
		{
			"type":      "cpu",
			"available": true,
			"version":   "1.0.0",
			"device":    "CPU",
		},
	}

	// Could add GPU/NPU detection here in the future

	s.respondJSON(w, map[string]interface{}{
		"backends": backends,
	})
}

func (s *EmbeddedServer) handleMotionStatus(w http.ResponseWriter, r *http.Request) {
	s.respondJSON(w, map[string]interface{}{
		"enabled":  true,
		"cameras":  map[string]interface{}{},
		"settings": map[string]interface{}{},
	})
}

func (s *EmbeddedServer) handleMotionConfig(w http.ResponseWriter, r *http.Request) {
	// Accept any config for now
	s.respondJSON(w, map[string]interface{}{
		"success": true,
	})
}

func (s *EmbeddedServer) handleMotionReset(w http.ResponseWriter, r *http.Request) {
	s.respondJSON(w, map[string]interface{}{
		"success": true,
	})
}

// Helper methods

func (s *EmbeddedServer) respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *EmbeddedServer) respondError(w http.ResponseWriter, status int, message string) {
	s.mu.Lock()
	s.errorCount++
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"error":   message,
	})
}
