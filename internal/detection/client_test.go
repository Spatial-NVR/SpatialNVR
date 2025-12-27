package detection

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	client, err := NewClient(ClientConfig{
		Address:    "localhost:5100",
		Timeout:    10 * time.Second,
		MaxRetries: 3,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if client == nil {
		t.Fatal("NewClient returned nil")
	}
	if client.baseURL != "http://localhost:5100" {
		t.Errorf("Expected baseURL 'http://localhost:5100', got %s", client.baseURL)
	}
}

func TestNewClient_DefaultTimeout(t *testing.T) {
	client, err := NewClient(ClientConfig{
		Address: "localhost:5100",
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if client.httpClient.Timeout != 30*time.Second {
		t.Errorf("Expected default timeout 30s, got %v", client.httpClient.Timeout)
	}
}

func TestClient_Detect_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/detect" {
			t.Errorf("Unexpected request: %s %s", r.Method, r.URL.Path)
		}

		response := map[string]interface{}{
			"success":         true,
			"camera_id":       "cam1",
			"timestamp":       time.Now().UnixMilli(),
			"motion_detected": true,
			"detections": []map[string]interface{}{
				{
					"object_type": "person",
					"label":       "person",
					"confidence":  0.95,
					"bbox": map[string]float64{
						"x":      0.1,
						"y":      0.2,
						"width":  0.3,
						"height": 0.4,
					},
					"track_id": "track1",
					"attributes": map[string]string{
						"color": "blue",
					},
				},
			},
			"process_time_ms": 25.5,
			"model_id":        "yolov8",
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		Address: server.URL[7:], // Remove "http://"
	})

	resp, err := client.Detect(context.Background(), &DetectRequest{
		CameraID:      "cam1",
		MinConfidence: 0.5,
		Frame:         &Frame{Data: []byte{1, 2, 3}},
	})
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}

	if resp.CameraID != "cam1" {
		t.Errorf("Expected CameraID 'cam1', got %s", resp.CameraID)
	}
	if !resp.MotionDetected {
		t.Error("Expected MotionDetected true")
	}
	if len(resp.Detections) != 1 {
		t.Errorf("Expected 1 detection, got %d", len(resp.Detections))
	}
	if resp.Detections[0].ObjectType != ObjectPerson {
		t.Errorf("Expected ObjectType person, got %s", resp.Detections[0].ObjectType)
	}
	if resp.Detections[0].Confidence != 0.95 {
		t.Errorf("Expected confidence 0.95, got %f", resp.Detections[0].Confidence)
	}
}

func TestClient_Detect_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"success": false,
			"error":   "model not loaded",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		Address: server.URL[7:],
	})

	_, err := client.Detect(context.Background(), &DetectRequest{
		CameraID: "cam1",
	})
	if err == nil {
		t.Error("Expected error, got nil")
	}
}

func TestClient_GetStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/status" {
			t.Errorf("Unexpected request: %s %s", r.Method, r.URL.Path)
		}

		response := map[string]interface{}{
			"connected":       true,
			"processed_count": 1000,
			"error_count":     5,
			"avg_latency_ms":  25.5,
			"uptime":          3600.0,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		Address: server.URL[7:],
	})

	status, err := client.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if !status.Connected {
		t.Error("Expected Connected true")
	}
	if status.ProcessedCount != 1000 {
		t.Errorf("Expected ProcessedCount 1000, got %d", status.ProcessedCount)
	}
}

func TestClient_GetStatus_Unreachable(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		Address: "localhost:59999", // Unlikely to be in use
		Timeout: 100 * time.Millisecond,
	})

	status, err := client.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus should not error: %v", err)
	}
	if status.Connected {
		t.Error("Expected Connected false for unreachable server")
	}
}

func TestClient_GetMotionStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := MotionStatus{
			FramesProcessed: 1000,
			MotionDetected:  50,
			MotionRate:      0.05,
			AvgLatencyMs:    25.5,
			CamerasTracked:  3,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		Address: server.URL[7:],
	})

	status, err := client.GetMotionStatus(context.Background())
	if err != nil {
		t.Fatalf("GetMotionStatus failed: %v", err)
	}
	if status.FramesProcessed != 1000 {
		t.Errorf("Expected FramesProcessed 1000, got %d", status.FramesProcessed)
	}
	if status.MotionDetected != 50 {
		t.Errorf("Expected MotionDetected 50, got %d", status.MotionDetected)
	}
}

func TestClient_ConfigureMotion(t *testing.T) {
	var receivedConfig MotionConfig
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/motion/config" {
			t.Errorf("Unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&receivedConfig)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		Address: server.URL[7:],
	})

	config := MotionConfig{
		Enabled:        true,
		Method:         "frame_diff",
		Threshold:      0.05,
		PixelThreshold: 25,
	}

	err := client.ConfigureMotion(context.Background(), config)
	if err != nil {
		t.Fatalf("ConfigureMotion failed: %v", err)
	}
	if receivedConfig.Threshold != 0.05 {
		t.Errorf("Expected Threshold 0.05, got %f", receivedConfig.Threshold)
	}
}

func TestClient_ResetMotion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/motion/reset" {
			t.Errorf("Unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		Address: server.URL[7:],
	})

	err := client.ResetMotion(context.Background(), "cam1")
	if err != nil {
		t.Fatalf("ResetMotion failed: %v", err)
	}
}

func TestClient_LoadModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/load" {
			t.Errorf("Unexpected request: %s %s", r.Method, r.URL.Path)
		}

		response := map[string]interface{}{
			"success":  true,
			"model_id": "model_123",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		Address: server.URL[7:],
	})

	modelID, err := client.LoadModel(context.Background(), "/path/to/model", ModelYOLOv8, BackendONNX)
	if err != nil {
		t.Fatalf("LoadModel failed: %v", err)
	}
	if modelID != "model_123" {
		t.Errorf("Expected modelID 'model_123', got %s", modelID)
	}
}

func TestClient_LoadModel_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"success": false,
			"error":   "model file not found",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		Address: server.URL[7:],
	})

	_, err := client.LoadModel(context.Background(), "/path/to/model", ModelYOLOv8, BackendONNX)
	if err == nil {
		t.Error("Expected error, got nil")
	}
}

func TestClient_UnloadModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/unload" {
			t.Errorf("Unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		Address: server.URL[7:],
	})

	err := client.UnloadModel(context.Background(), "model_123")
	if err != nil {
		t.Fatalf("UnloadModel failed: %v", err)
	}
}

func TestClient_GetBackends(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"backends": []map[string]interface{}{
				{
					"type":      "onnx",
					"available": true,
					"version":   "1.14",
					"device":    "CPU",
				},
				{
					"type":      "nvidia",
					"available": true,
					"version":   "8.5",
					"device":    "CUDA:0",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		Address: server.URL[7:],
	})

	backends, err := client.GetBackends(context.Background())
	if err != nil {
		t.Fatalf("GetBackends failed: %v", err)
	}
	if len(backends) != 2 {
		t.Errorf("Expected 2 backends, got %d", len(backends))
	}
	if backends[0].Type != BackendONNX {
		t.Errorf("Expected first backend ONNX, got %s", backends[0].Type)
	}
}

func TestClient_GetModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"models": []map[string]interface{}{
				{
					"id":      "model_1",
					"name":    "YOLOv8",
					"type":    "yolov8",
					"backend": "onnx",
					"path":    "/models/yolov8.onnx",
					"loaded":  true,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		Address: server.URL[7:],
	})

	models, err := client.GetModels(context.Background())
	if err != nil {
		t.Fatalf("GetModels failed: %v", err)
	}
	if len(models) != 1 {
		t.Errorf("Expected 1 model, got %d", len(models))
	}
	if models[0].Name != "YOLOv8" {
		t.Errorf("Expected model name 'YOLOv8', got %s", models[0].Name)
	}
}

func TestClient_Close(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		Address: "localhost:5100",
	})

	err := client.Close()
	if err != nil {
		t.Errorf("Close should not error: %v", err)
	}
}

func TestClient_Stats(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		Address: "localhost:5100",
	})

	requests, errors, avgLatency := client.Stats()
	if requests != 0 {
		t.Errorf("Expected 0 requests, got %d", requests)
	}
	if errors != 0 {
		t.Errorf("Expected 0 errors, got %d", errors)
	}
	if avgLatency != 0 {
		t.Errorf("Expected 0 avgLatency, got %v", avgLatency)
	}
}

func TestClient_DetectAsync(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"success":         true,
			"camera_id":       "cam1",
			"timestamp":       time.Now().UnixMilli(),
			"motion_detected": false,
			"detections":      []map[string]interface{}{},
			"process_time_ms": 10.0,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, _ := NewClient(ClientConfig{
		Address: server.URL[7:],
	})

	ch := client.DetectAsync(&DetectRequest{CameraID: "cam1"})
	resp := <-ch

	if resp == nil {
		t.Fatal("Expected response, got nil")
	}
	if resp.CameraID != "cam1" {
		t.Errorf("Expected CameraID 'cam1', got %s", resp.CameraID)
	}
}

func TestClient_StreamDetect_NotImplemented(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		Address: "localhost:5100",
	})

	_, err := client.StreamDetect(context.Background())
	if err == nil {
		t.Error("Expected error for unimplemented method")
	}
}
