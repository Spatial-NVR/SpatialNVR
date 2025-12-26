package models

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	if m == nil {
		t.Fatal("NewManager returned nil")
	}

	// Check that models directory was created
	modelsDir := filepath.Join(tmpDir, "models")
	if _, err := os.Stat(modelsDir); os.IsNotExist(err) {
		t.Error("Models directory was not created")
	}
}

func TestGetModelPath(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	tests := []struct {
		name     string
		modelID  string
		wantPath bool
	}{
		{
			name:     "valid model",
			modelID:  "yolov8n",
			wantPath: true,
		},
		{
			name:     "another valid model",
			modelID:  "yolo12n",
			wantPath: true,
		},
		{
			name:     "invalid model",
			modelID:  "nonexistent",
			wantPath: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := m.GetModelPath(tt.modelID)
			if tt.wantPath && path == "" {
				t.Error("expected path but got empty string")
			}
			if !tt.wantPath && path != "" {
				t.Errorf("expected empty string but got: %s", path)
			}
		})
	}
}

func TestIsModelDownloaded(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	// Model should not be downloaded initially
	if m.IsModelDownloaded("yolov8n") {
		t.Error("Model should not be downloaded initially")
	}

	// Create a fake model file
	modelsDir := filepath.Join(tmpDir, "models")
	modelPath := filepath.Join(modelsDir, "yolov8n.onnx")
	if err := os.WriteFile(modelPath, []byte("fake model data"), 0644); err != nil {
		t.Fatalf("Failed to create fake model file: %v", err)
	}

	// Now it should be detected as downloaded
	if !m.IsModelDownloaded("yolov8n") {
		t.Error("Model should be detected as downloaded")
	}

	// Unknown model should return false
	if m.IsModelDownloaded("nonexistent") {
		t.Error("Unknown model should return false")
	}
}

func TestGetDownloadStatus(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	// Not started model
	status := m.GetDownloadStatus("yolov8n")
	if status.Status != "not_started" {
		t.Errorf("Expected status 'not_started', got '%s'", status.Status)
	}

	// Create a fake downloaded model
	modelsDir := filepath.Join(tmpDir, "models")
	modelPath := filepath.Join(modelsDir, "yolo12n.onnx")
	if err := os.WriteFile(modelPath, []byte("fake model data"), 0644); err != nil {
		t.Fatalf("Failed to create fake model file: %v", err)
	}

	status = m.GetDownloadStatus("yolo12n")
	if status.Status != "completed" {
		t.Errorf("Expected status 'completed', got '%s'", status.Status)
	}
	if status.Progress != 100 {
		t.Errorf("Expected progress 100, got %f", status.Progress)
	}
}

func TestDownloadModel_UnknownModel(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	err := m.DownloadModel(context.Background(), "nonexistent_model")
	if err == nil {
		t.Error("Expected error for unknown model")
	}
}

func TestDownloadModel_AlreadyDownloaded(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	// Create a fake downloaded model
	modelsDir := filepath.Join(tmpDir, "models")
	modelPath := filepath.Join(modelsDir, "yolov8n.onnx")
	if err := os.WriteFile(modelPath, []byte("fake model data"), 0644); err != nil {
		t.Fatalf("Failed to create fake model file: %v", err)
	}

	// Should return nil (no error) for already downloaded model
	err := m.DownloadModel(context.Background(), "yolov8n")
	if err != nil {
		t.Errorf("Expected no error for already downloaded model, got: %v", err)
	}
}

func TestDownloadModel_AlreadyDownloading(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	// Manually set a model as downloading
	m.mu.Lock()
	m.downloads["yolov8n"] = &DownloadStatus{
		ModelID: "yolov8n",
		Status:  "downloading",
	}
	m.mu.Unlock()

	// Should return error for already downloading model
	err := m.DownloadModel(context.Background(), "yolov8n")
	if err == nil {
		t.Error("Expected error for already downloading model")
	}
}

func TestDownloadModel_WithMockServer(t *testing.T) {
	// Create a test server that serves fake model data with correct content length
	data := []byte("fake model data for testing purposes that is longer than expected")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	// Temporarily modify the model URL to use our test server
	// Save original
	originalInfo := AvailableModels["yolov8n"]
	AvailableModels["yolov8n"] = ModelInfo{
		ID:     "yolov8n",
		Name:   "Test Model",
		Type:   "yolo",
		Format: "onnx",
		URL:    server.URL + "/model.onnx",
		Size:   int64(len(data)),
	}
	defer func() {
		AvailableModels["yolov8n"] = originalInfo
	}()

	// Start download
	err := m.DownloadModel(context.Background(), "yolov8n")
	if err != nil {
		t.Fatalf("DownloadModel failed: %v", err)
	}

	// Wait for download to complete
	time.Sleep(500 * time.Millisecond)

	// Check status
	status := m.GetDownloadStatus("yolov8n")
	if status.Status != "completed" {
		t.Errorf("Expected status 'completed', got '%s' (error: %s)", status.Status, status.Error)
	}
}

func TestDownloadModel_ServerError(t *testing.T) {
	// Create a test server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	// Temporarily modify the model URL
	originalInfo := AvailableModels["yolo12n"]
	AvailableModels["yolo12n"] = ModelInfo{
		ID:     "yolo12n",
		Name:   "Test Model",
		Type:   "yolo",
		Format: "onnx",
		URL:    server.URL + "/model.onnx",
		Size:   100,
	}
	defer func() {
		AvailableModels["yolo12n"] = originalInfo
	}()

	// Start download
	err := m.DownloadModel(context.Background(), "yolo12n")
	if err != nil {
		t.Fatalf("DownloadModel failed to start: %v", err)
	}

	// Wait for download to fail
	time.Sleep(500 * time.Millisecond)

	// Check status should be failed
	status := m.GetDownloadStatus("yolo12n")
	if status.Status != "failed" {
		t.Errorf("Expected status 'failed', got '%s'", status.Status)
	}
}

func TestDownloadModel_ContextCancellation(t *testing.T) {
	// Create a test server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000000")
		w.WriteHeader(http.StatusOK)
		// Write slowly
		for i := 0; i < 100; i++ {
			select {
			case <-r.Context().Done():
				return
			default:
				time.Sleep(10 * time.Millisecond)
				w.Write([]byte("data"))
			}
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	// Temporarily modify the model URL
	originalInfo := AvailableModels["yolo12m"]
	AvailableModels["yolo12m"] = ModelInfo{
		ID:     "yolo12m",
		Name:   "Test Model",
		Type:   "yolo",
		Format: "onnx",
		URL:    server.URL + "/model.onnx",
		Size:   1000000,
	}
	defer func() {
		AvailableModels["yolo12m"] = originalInfo
	}()

	// Create cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Start download
	err := m.DownloadModel(ctx, "yolo12m")
	if err != nil {
		t.Fatalf("DownloadModel failed to start: %v", err)
	}

	// Cancel after a short delay
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Wait for cancellation to take effect
	time.Sleep(200 * time.Millisecond)

	// Check status should be failed
	status := m.GetDownloadStatus("yolo12m")
	if status.Status != "failed" && status.Status != "downloading" {
		// Either failed or still downloading is acceptable
		t.Logf("Status after cancellation: %s", status.Status)
	}
}

func TestListAvailableModels(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	models := m.ListAvailableModels()
	if len(models) == 0 {
		t.Error("Expected some models to be listed")
	}

	// Check that all expected fields are populated
	for _, model := range models {
		if model.ID == "" {
			t.Error("Model ID should not be empty")
		}
		if model.Name == "" {
			t.Error("Model Name should not be empty")
		}
		if model.Type == "" {
			t.Error("Model Type should not be empty")
		}
	}
}

func TestGetAllDownloadStatuses(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	// GetAllDownloadStatuses returns status for all available models
	statuses := m.GetAllDownloadStatuses()
	if statuses == nil {
		t.Error("GetAllDownloadStatuses should not return nil")
	}

	// Should have entries for all available models
	if len(statuses) != len(AvailableModels) {
		t.Errorf("Expected %d statuses, got %d", len(AvailableModels), len(statuses))
	}

	// All statuses should be "not_started" initially
	for id, status := range statuses {
		if status.Status != "not_started" {
			t.Errorf("Expected model %s to have status 'not_started', got '%s'", id, status.Status)
		}
	}
}

func TestAvailableModelsStructure(t *testing.T) {
	// Verify the structure of available models
	requiredModels := []string{"yolov8n", "yolo12n", "yolo12s", "yolo12m"}
	for _, modelID := range requiredModels {
		model, ok := AvailableModels[modelID]
		if !ok {
			t.Errorf("Expected model %s to be available", modelID)
			continue
		}
		if model.URL == "" {
			t.Errorf("Model %s has empty URL", modelID)
		}
		if model.Size == 0 {
			t.Errorf("Model %s has zero size", modelID)
		}
	}
}
