package detection

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Client is an HTTP client for the detection service
type Client struct {
	mu         sync.RWMutex
	httpClient *http.Client
	baseURL    string
	logger     *slog.Logger

	// Stats
	requestCount int64
	errorCount   int64
	totalLatency time.Duration
}

// ClientConfig holds client configuration
type ClientConfig struct {
	Address    string
	Timeout    time.Duration
	MaxRetries int
}

// NewClient creates a new detection service client
func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	// Convert gRPC-style address to HTTP URL
	baseURL := fmt.Sprintf("http://%s", cfg.Address)

	return &Client{
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		baseURL: baseURL,
		logger:  slog.Default().With("component", "detection_client"),
	}, nil
}

// Detect sends an image for detection
func (c *Client) Detect(ctx context.Context, req *DetectRequest) (*DetectResponse, error) {
	c.mu.Lock()
	c.requestCount++
	c.mu.Unlock()

	start := time.Now()

	// Build HTTP request body
	body := map[string]interface{}{
		"camera_id":      req.CameraID,
		"min_confidence": req.MinConfidence,
	}

	// Encode image as base64
	if req.Frame != nil && len(req.Frame.Data) > 0 {
		body["image_data"] = base64.StdEncoding.EncodeToString(req.Frame.Data)
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		c.mu.Lock()
		c.errorCount++
		c.mu.Unlock()
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/detect", bytes.NewReader(jsonBody))
	if err != nil {
		c.mu.Lock()
		c.errorCount++
		c.mu.Unlock()
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.mu.Lock()
		c.errorCount++
		c.mu.Unlock()
		return nil, fmt.Errorf("detection request failed: %w", err)
	}
	defer resp.Body.Close()

	latency := time.Since(start)

	c.mu.Lock()
	c.totalLatency += latency
	c.mu.Unlock()

	// Parse response
	var result struct {
		Success        bool   `json:"success"`
		Error          string `json:"error"`
		CameraID       string `json:"camera_id"`
		Timestamp      int64  `json:"timestamp"`
		MotionDetected bool   `json:"motion_detected"`
		Detections     []struct {
			ObjectType string  `json:"object_type"`
			Label      string  `json:"label"`
			Confidence float64 `json:"confidence"`
			BBox       struct {
				X      float64 `json:"x"`
				Y      float64 `json:"y"`
				Width  float64 `json:"width"`
				Height float64 `json:"height"`
			} `json:"bbox"`
			TrackID    string            `json:"track_id"`
			Attributes map[string]string `json:"attributes"`
		} `json:"detections"`
		ProcessTimeMs float64 `json:"process_time_ms"`
		ModelID       string  `json:"model_id"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success && result.Error != "" {
		return nil, fmt.Errorf("detection failed: %s", result.Error)
	}

	// Convert to DetectResponse
	detections := make([]Detection, 0, len(result.Detections))
	for _, d := range result.Detections {
		// Convert string attributes to interface{} map
		attrs := make(map[string]interface{})
		for k, v := range d.Attributes {
			attrs[k] = v
		}

		detections = append(detections, Detection{
			ObjectType: ObjectType(d.ObjectType),
			Label:      d.Label,
			Confidence: d.Confidence,
			BoundingBox: BoundingBox{
				X:      d.BBox.X,
				Y:      d.BBox.Y,
				Width:  d.BBox.Width,
				Height: d.BBox.Height,
			},
			TrackID:    d.TrackID,
			Attributes: attrs,
			Timestamp:  time.Now(),
			Backend:    BackendONNX,
			ModelID:    result.ModelID,
		})
	}

	return &DetectResponse{
		CameraID:       result.CameraID,
		Timestamp:      time.UnixMilli(result.Timestamp),
		MotionDetected: result.MotionDetected,
		Detections:     detections,
		ProcessTimeMs:  result.ProcessTimeMs,
		Backend:        BackendONNX,
	}, nil
}

// GetMotionStatus returns motion detection statistics
func (c *Client) GetMotionStatus(ctx context.Context) (*MotionStatus, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/motion", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result MotionStatus
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ConfigureMotion updates motion detection configuration
func (c *Client) ConfigureMotion(ctx context.Context, config MotionConfig) error {
	jsonBody, err := json.Marshal(config)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/motion/config", bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// ResetMotion resets motion detection state for a camera
func (c *Client) ResetMotion(ctx context.Context, cameraID string) error {
	body := map[string]string{}
	if cameraID != "" {
		body["camera_id"] = cameraID
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/motion/reset", bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// DetectAsync sends an async detection request
func (c *Client) DetectAsync(req *DetectRequest) <-chan *DetectResponse {
	ch := make(chan *DetectResponse, 1)

	go func() {
		defer close(ch)
		resp, err := c.Detect(context.Background(), req)
		if err != nil {
			c.logger.Error("Async detection failed", "error", err)
			return
		}
		ch <- resp
	}()

	return ch
}

// GetStatus returns the service status
func (c *Client) GetStatus(ctx context.Context) (*ServiceStatus, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/status", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &ServiceStatus{Connected: false}, nil
	}
	defer resp.Body.Close()

	var result struct {
		Connected      bool    `json:"connected"`
		ProcessedCount int64   `json:"processed_count"`
		ErrorCount     int64   `json:"error_count"`
		AvgLatencyMs   float64 `json:"avg_latency_ms"`
		Uptime         float64 `json:"uptime"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return &ServiceStatus{Connected: false}, nil
	}

	return &ServiceStatus{
		Connected:      true,
		ProcessedCount: result.ProcessedCount,
		ErrorCount:     result.ErrorCount,
		AvgLatencyMs:   result.AvgLatencyMs,
	}, nil
}

// LoadModel loads a model on the detection service
func (c *Client) LoadModel(ctx context.Context, path string, modelType ModelType, backend BackendType) (string, error) {
	body := map[string]interface{}{
		"path":    path,
		"type":    string(modelType),
		"backend": string(backend),
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/load", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to load model: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Success bool   `json:"success"`
		ModelID string `json:"model_id"`
		Error   string `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if !result.Success {
		return "", fmt.Errorf("failed to load model: %s", result.Error)
	}

	return result.ModelID, nil
}

// UnloadModel unloads a model
func (c *Client) UnloadModel(ctx context.Context, modelID string) error {
	body := map[string]string{"model_id": modelID}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/unload", bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// GetBackends returns available backends
func (c *Client) GetBackends(ctx context.Context) ([]BackendInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/backends", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Backends []struct {
			Type      string `json:"type"`
			Available bool   `json:"available"`
			Version   string `json:"version"`
			Device    string `json:"device"`
		} `json:"backends"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	backends := make([]BackendInfo, 0, len(result.Backends))
	for _, b := range result.Backends {
		backends = append(backends, BackendInfo{
			Type:      BackendType(b.Type),
			Available: b.Available,
			Version:   b.Version,
			Device:    b.Device,
		})
	}

	return backends, nil
}

// GetModels returns loaded models
func (c *Client) GetModels(ctx context.Context) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Type    string `json:"type"`
			Backend string `json:"backend"`
			Path    string `json:"path"`
			Loaded  bool   `json:"loaded"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := make([]ModelInfo, 0, len(result.Models))
	for _, m := range result.Models {
		models = append(models, ModelInfo{
			ID:      m.ID,
			Name:    m.Name,
			Type:    ModelType(m.Type),
			Backend: BackendType(m.Backend),
			Path:    m.Path,
			Loaded:  m.Loaded,
		})
	}

	return models, nil
}

// Close closes the client connection
func (c *Client) Close() error {
	// HTTP client doesn't need explicit closing
	return nil
}

// Stats returns client statistics
func (c *Client) Stats() (requests int64, errors int64, avgLatency time.Duration) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	requests = c.requestCount
	errors = c.errorCount
	if requests > 0 {
		avgLatency = c.totalLatency / time.Duration(requests)
	}
	return
}

// StreamDetect opens a streaming detection connection
func (c *Client) StreamDetect(ctx context.Context) (DetectionStream, error) {
	// Would create a bidirectional stream
	return nil, fmt.Errorf("not implemented")
}

// DetectionStream represents a bidirectional detection stream
type DetectionStream interface {
	Send(*DetectRequest) error
	Recv() (*DetectResponse, error)
	io.Closer
}
