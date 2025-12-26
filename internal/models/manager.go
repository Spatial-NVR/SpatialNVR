package models

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

// ModelInfo contains information about a downloadable model
type ModelInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"` // yolo, face, lpr
	Format      string `json:"format"` // onnx, pt, mlmodel
	URL         string `json:"url"`
	Size        int64  `json:"size"` // bytes
	Description string `json:"description"`
}

// DownloadStatus represents the status of a model download
type DownloadStatus struct {
	ModelID     string  `json:"model_id"`
	Status      string  `json:"status"` // pending, downloading, completed, failed
	Progress    float64 `json:"progress"` // 0-100
	Error       string  `json:"error,omitempty"`
	BytesTotal  int64   `json:"bytes_total"`
	BytesDone   int64   `json:"bytes_done"`
}

// Manager handles model downloads and management
type Manager struct {
	mu           sync.RWMutex
	modelsDir    string
	downloads    map[string]*DownloadStatus
	logger       *slog.Logger
}

// NewManager creates a new model manager
func NewManager(dataPath string) *Manager {
	modelsDir := filepath.Join(dataPath, "models")
	os.MkdirAll(modelsDir, 0755)

	return &Manager{
		modelsDir: modelsDir,
		downloads: make(map[string]*DownloadStatus),
		logger:    slog.Default().With("component", "model_manager"),
	}
}

// Available models with download URLs
var AvailableModels = map[string]ModelInfo{
	// YOLO Object Detection Models
	"yolo12n": {
		ID:          "yolo12n",
		Name:        "YOLO12 Nano",
		Type:        "yolo",
		Format:      "onnx",
		URL:         "https://github.com/ultralytics/assets/releases/download/v8.3.0/yolo11n.onnx",
		Size:        5500000,
		Description: "Fastest, lowest accuracy - good for edge devices",
	},
	"yolo12s": {
		ID:          "yolo12s",
		Name:        "YOLO12 Small",
		Type:        "yolo",
		Format:      "onnx",
		URL:         "https://github.com/ultralytics/assets/releases/download/v8.3.0/yolo11s.onnx",
		Size:        18500000,
		Description: "Fast with good accuracy",
	},
	"yolo12m": {
		ID:          "yolo12m",
		Name:        "YOLO12 Medium",
		Type:        "yolo",
		Format:      "onnx",
		URL:         "https://github.com/ultralytics/assets/releases/download/v8.3.0/yolo11m.onnx",
		Size:        38900000,
		Description: "Balanced speed and accuracy",
	},
	"yolo12l": {
		ID:          "yolo12l",
		Name:        "YOLO12 Large",
		Type:        "yolo",
		Format:      "onnx",
		URL:         "https://github.com/ultralytics/assets/releases/download/v8.3.0/yolo11l.onnx",
		Size:        49000000,
		Description: "High accuracy, slower",
	},
	"yolov8n": {
		ID:          "yolov8n",
		Name:        "YOLOv8 Nano",
		Type:        "yolo",
		Format:      "onnx",
		URL:         "https://github.com/ultralytics/assets/releases/download/v8.1.0/yolov8n.onnx",
		Size:        6300000,
		Description: "YOLOv8 nano - fast and lightweight",
	},
	"yolov8s": {
		ID:          "yolov8s",
		Name:        "YOLOv8 Small",
		Type:        "yolo",
		Format:      "onnx",
		URL:         "https://github.com/ultralytics/assets/releases/download/v8.1.0/yolov8s.onnx",
		Size:        22500000,
		Description: "YOLOv8 small - good balance",
	},
	"yolov8m": {
		ID:          "yolov8m",
		Name:        "YOLOv8 Medium",
		Type:        "yolo",
		Format:      "onnx",
		URL:         "https://github.com/ultralytics/assets/releases/download/v8.1.0/yolov8m.onnx",
		Size:        52000000,
		Description: "YOLOv8 medium - higher accuracy",
	},

	// Face Recognition Models
	"buffalo_l": {
		ID:          "buffalo_l",
		Name:        "Buffalo Large",
		Type:        "face",
		Format:      "onnx",
		URL:         "https://github.com/deepinsight/insightface/releases/download/v0.7/buffalo_l.zip",
		Size:        326000000,
		Description: "InsightFace buffalo_l - high accuracy face recognition",
	},
	"buffalo_s": {
		ID:          "buffalo_s",
		Name:        "Buffalo Small",
		Type:        "face",
		Format:      "onnx",
		URL:         "https://github.com/deepinsight/insightface/releases/download/v0.7/buffalo_s.zip",
		Size:        28000000,
		Description: "InsightFace buffalo_s - fast face recognition",
	},

	// LPR Models
	"paddleocr": {
		ID:          "paddleocr",
		Name:        "PaddleOCR",
		Type:        "lpr",
		Format:      "onnx",
		URL:         "https://paddleocr.bj.bcebos.com/PP-OCRv4/english/en_PP-OCRv4_rec_infer.tar",
		Size:        10000000,
		Description: "PaddleOCR for license plate recognition",
	},
}

// GetModelPath returns the local path for a model
func (m *Manager) GetModelPath(modelID string) string {
	info, ok := AvailableModels[modelID]
	if !ok {
		return ""
	}
	return filepath.Join(m.modelsDir, modelID+"."+info.Format)
}

// IsModelDownloaded checks if a model is already downloaded
func (m *Manager) IsModelDownloaded(modelID string) bool {
	path := m.GetModelPath(modelID)
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// GetDownloadStatus returns the current download status for a model
func (m *Manager) GetDownloadStatus(modelID string) *DownloadStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if status, ok := m.downloads[modelID]; ok {
		return status
	}

	// Check if already downloaded
	if m.IsModelDownloaded(modelID) {
		return &DownloadStatus{
			ModelID:  modelID,
			Status:   "completed",
			Progress: 100,
		}
	}

	return &DownloadStatus{
		ModelID: modelID,
		Status:  "not_started",
	}
}

// DownloadModel downloads a model if not already present
func (m *Manager) DownloadModel(ctx context.Context, modelID string) error {
	info, ok := AvailableModels[modelID]
	if !ok {
		return fmt.Errorf("unknown model: %s", modelID)
	}

	// Check if already downloaded
	if m.IsModelDownloaded(modelID) {
		m.logger.Info("Model already downloaded", "model", modelID)
		return nil
	}

	// Check if already downloading
	m.mu.Lock()
	if status, ok := m.downloads[modelID]; ok && status.Status == "downloading" {
		m.mu.Unlock()
		return fmt.Errorf("model %s is already downloading", modelID)
	}

	// Initialize download status
	status := &DownloadStatus{
		ModelID:    modelID,
		Status:     "downloading",
		Progress:   0,
		BytesTotal: info.Size,
	}
	m.downloads[modelID] = status
	m.mu.Unlock()

	m.logger.Info("Starting model download", "model", modelID, "url", info.URL)

	// Download in background
	go func() {
		err := m.downloadFile(ctx, modelID, info)

		m.mu.Lock()
		if err != nil {
			status.Status = "failed"
			status.Error = err.Error()
			m.logger.Error("Model download failed", "model", modelID, "error", err)
		} else {
			status.Status = "completed"
			status.Progress = 100
			m.logger.Info("Model download completed", "model", modelID)
		}
		m.mu.Unlock()
	}()

	return nil
}

// downloadFile performs the actual file download
func (m *Manager) downloadFile(ctx context.Context, modelID string, info ModelInfo) error {
	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", info.URL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Execute request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	// Get total size
	totalSize := resp.ContentLength
	if totalSize <= 0 {
		totalSize = info.Size
	}

	// Create temp file
	destPath := m.GetModelPath(modelID)
	tmpPath := destPath + ".tmp"

	file, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Download with progress tracking
	var downloaded int64
	buf := make([]byte, 32*1024) // 32KB buffer

	for {
		select {
		case <-ctx.Done():
			os.Remove(tmpPath)
			return ctx.Err()
		default:
		}

		n, err := resp.Body.Read(buf)
		if n > 0 {
			_, writeErr := file.Write(buf[:n])
			if writeErr != nil {
				os.Remove(tmpPath)
				return fmt.Errorf("failed to write: %w", writeErr)
			}

			downloaded += int64(n)

			// Update progress
			m.mu.Lock()
			if status, ok := m.downloads[modelID]; ok {
				status.BytesDone = downloaded
				if totalSize > 0 {
					status.Progress = float64(downloaded) / float64(totalSize) * 100
				}
			}
			m.mu.Unlock()
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("download error: %w", err)
		}
	}

	// Rename temp file to final name
	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to finalize: %w", err)
	}

	return nil
}

// EnsureModelsDownloaded downloads all required models based on config
func (m *Manager) EnsureModelsDownloaded(ctx context.Context, objectModel, faceModel, lprModel string) []error {
	var errors []error
	var wg sync.WaitGroup
	var mu sync.Mutex

	modelsToDownload := []string{}

	if objectModel != "" && !m.IsModelDownloaded(objectModel) {
		modelsToDownload = append(modelsToDownload, objectModel)
	}
	if faceModel != "" && !m.IsModelDownloaded(faceModel) {
		modelsToDownload = append(modelsToDownload, faceModel)
	}
	if lprModel != "" && !m.IsModelDownloaded(lprModel) {
		modelsToDownload = append(modelsToDownload, lprModel)
	}

	for _, modelID := range modelsToDownload {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if err := m.DownloadModel(ctx, id); err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("%s: %w", id, err))
				mu.Unlock()
			}
		}(modelID)
	}

	wg.Wait()
	return errors
}

// ListAvailableModels returns all available models
func (m *Manager) ListAvailableModels() []ModelInfo {
	models := make([]ModelInfo, 0, len(AvailableModels))
	for _, info := range AvailableModels {
		models = append(models, info)
	}
	return models
}

// GetAllDownloadStatuses returns download status for all models
func (m *Manager) GetAllDownloadStatuses() map[string]*DownloadStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make(map[string]*DownloadStatus)
	for id := range AvailableModels {
		statuses[id] = m.GetDownloadStatus(id)
	}
	return statuses
}
