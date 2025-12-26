package detection

import (
	"context"
	"testing"
	"time"

	"github.com/Spatial-NVR/SpatialNVR/internal/config"
)

func TestServiceConfig_Defaults(t *testing.T) {
	cfg := ServiceConfig{}

	if cfg.DetectionAddr != "" {
		t.Error("DetectionAddr should be empty by default")
	}
	if cfg.Go2RTCAddr != "" {
		t.Error("Go2RTCAddr should be empty by default")
	}
	if cfg.DefaultFPS != 0 {
		t.Error("DefaultFPS should be 0 by default")
	}
	if cfg.MinConfidence != 0 {
		t.Error("MinConfidence should be 0 by default")
	}
}

func TestServiceConfig_Values(t *testing.T) {
	cfg := ServiceConfig{
		DetectionAddr: "localhost:5100",
		Go2RTCAddr:    "http://localhost:1984",
		DefaultFPS:    10,
		MinConfidence: 0.6,
	}

	if cfg.DetectionAddr != "localhost:5100" {
		t.Errorf("Expected DetectionAddr 'localhost:5100', got %s", cfg.DetectionAddr)
	}
	if cfg.Go2RTCAddr != "http://localhost:1984" {
		t.Errorf("Expected Go2RTCAddr 'http://localhost:1984', got %s", cfg.Go2RTCAddr)
	}
	if cfg.DefaultFPS != 10 {
		t.Errorf("Expected DefaultFPS 10, got %d", cfg.DefaultFPS)
	}
	if cfg.MinConfidence != 0.6 {
		t.Errorf("Expected MinConfidence 0.6, got %f", cfg.MinConfidence)
	}
}

func TestDetectionError(t *testing.T) {
	err := &DetectionError{Message: "test error"}

	if err.Error() != "test error" {
		t.Errorf("Expected error message 'test error', got %s", err.Error())
	}
}

func TestDetectionError_Empty(t *testing.T) {
	err := &DetectionError{}

	if err.Error() != "" {
		t.Errorf("Expected empty error message, got %s", err.Error())
	}
}

func TestZone_ContainsPoint(t *testing.T) {
	// Create a square zone from (0,0) to (1,1)
	zone := Zone{
		ID:   "test-zone",
		Name: "Test Zone",
		Points: [][]float64{
			{0.0, 0.0},
			{1.0, 0.0},
			{1.0, 1.0},
			{0.0, 1.0},
		},
		Enabled: true,
	}

	tests := []struct {
		name     string
		x, y     float64
		expected bool
	}{
		{"center", 0.5, 0.5, true},
		{"corner", 0.0, 0.0, true},
		{"edge", 0.5, 0.0, true},
		{"outside left", -0.1, 0.5, false},
		{"outside right", 1.1, 0.5, false},
		{"outside top", 0.5, -0.1, false},
		{"outside bottom", 0.5, 1.1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := zone.ContainsPoint(tt.x, tt.y)
			if result != tt.expected {
				t.Errorf("ContainsPoint(%f, %f) = %v, want %v", tt.x, tt.y, result, tt.expected)
			}
		})
	}
}

func TestZone_ContainsPoint_Triangle(t *testing.T) {
	// Create a triangle zone
	zone := Zone{
		Points: [][]float64{
			{0.0, 0.0},
			{1.0, 0.0},
			{0.5, 1.0},
		},
		Enabled: true,
	}

	// Point inside triangle
	if !zone.ContainsPoint(0.5, 0.3) {
		t.Error("Expected point (0.5, 0.3) to be inside triangle")
	}

	// Point outside triangle
	if zone.ContainsPoint(0.1, 0.9) {
		t.Error("Expected point (0.1, 0.9) to be outside triangle")
	}
}

func TestZone_ContainsPoint_EmptyZone(t *testing.T) {
	zone := Zone{
		Points:  [][]float64{},
		Enabled: true,
	}

	// Empty zone should not contain any point
	if zone.ContainsPoint(0.5, 0.5) {
		t.Error("Empty zone should not contain any point")
	}
}

func TestZone_ContainsPoint_SinglePoint(t *testing.T) {
	zone := Zone{
		Points: [][]float64{
			{0.5, 0.5},
		},
		Enabled: true,
	}

	// Single point zone behavior
	result := zone.ContainsPoint(0.5, 0.5)
	// Behavior undefined for degenerate polygon, just ensure no panic
	_ = result
}

func TestZone_ContainsPoint_Line(t *testing.T) {
	zone := Zone{
		Points: [][]float64{
			{0.0, 0.0},
			{1.0, 1.0},
		},
		Enabled: true,
	}

	// Line zone behavior - should not panic
	result := zone.ContainsPoint(0.5, 0.5)
	_ = result
}

func TestZone_Fields(t *testing.T) {
	zone := Zone{
		ID:            "zone-1",
		Name:          "Front Yard",
		CameraID:      "cam-1",
		Points:        [][]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}},
		Objects:       []string{"person", "vehicle"},
		MinConfidence: 0.7,
		MinSize:       0.01,
		Enabled:       true,
	}

	if zone.ID != "zone-1" {
		t.Errorf("Expected ID 'zone-1', got %s", zone.ID)
	}
	if zone.Name != "Front Yard" {
		t.Errorf("Expected Name 'Front Yard', got %s", zone.Name)
	}
	if zone.CameraID != "cam-1" {
		t.Errorf("Expected CameraID 'cam-1', got %s", zone.CameraID)
	}
	if len(zone.Objects) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(zone.Objects))
	}
	if zone.MinConfidence != 0.7 {
		t.Errorf("Expected MinConfidence 0.7, got %f", zone.MinConfidence)
	}
	if zone.MinSize != 0.01 {
		t.Errorf("Expected MinSize 0.01, got %f", zone.MinSize)
	}
	if !zone.Enabled {
		t.Error("Expected Enabled true")
	}
}

func TestBoundingBox_Center(t *testing.T) {
	tests := []struct {
		name      string
		bbox      BoundingBox
		expectedX float64
		expectedY float64
	}{
		{
			name:      "unit box at origin",
			bbox:      BoundingBox{X: 0, Y: 0, Width: 1, Height: 1},
			expectedX: 0.5,
			expectedY: 0.5,
		},
		{
			name:      "offset box",
			bbox:      BoundingBox{X: 0.2, Y: 0.3, Width: 0.4, Height: 0.2},
			expectedX: 0.4,
			expectedY: 0.4,
		},
		{
			name:      "zero size box",
			bbox:      BoundingBox{X: 0.5, Y: 0.5, Width: 0, Height: 0},
			expectedX: 0.5,
			expectedY: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cx, cy := tt.bbox.Center()
			if cx != tt.expectedX {
				t.Errorf("Expected center X %f, got %f", tt.expectedX, cx)
			}
			if cy != tt.expectedY {
				t.Errorf("Expected center Y %f, got %f", tt.expectedY, cy)
			}
		})
	}
}

func TestBoundingBox_Area(t *testing.T) {
	tests := []struct {
		name     string
		bbox     BoundingBox
		expected float64
	}{
		{"unit box", BoundingBox{Width: 1, Height: 1}, 1.0},
		{"zero box", BoundingBox{Width: 0, Height: 0}, 0.0},
		{"rectangle", BoundingBox{Width: 0.5, Height: 0.2}, 0.1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			area := tt.bbox.Area()
			if area != tt.expected {
				t.Errorf("Expected area %f, got %f", tt.expected, area)
			}
		})
	}
}

func TestDetection_Fields(t *testing.T) {
	now := time.Now()
	det := Detection{
		ObjectType: ObjectPerson,
		Label:      "person",
		Confidence: 0.95,
		BoundingBox: BoundingBox{
			X: 0.1, Y: 0.2, Width: 0.3, Height: 0.4,
		},
		TrackID:    "track-123",
		Timestamp:  now,
		ModelID:    "yolov8",
		Backend:    BackendONNX,
		Attributes: map[string]interface{}{"color": "blue"},
	}

	if det.ObjectType != ObjectPerson {
		t.Errorf("Expected ObjectType person, got %s", det.ObjectType)
	}
	if det.Label != "person" {
		t.Errorf("Expected Label 'person', got %s", det.Label)
	}
	if det.Confidence != 0.95 {
		t.Errorf("Expected Confidence 0.95, got %f", det.Confidence)
	}
	if det.TrackID != "track-123" {
		t.Errorf("Expected TrackID 'track-123', got %s", det.TrackID)
	}
	if det.ModelID != "yolov8" {
		t.Errorf("Expected ModelID 'yolov8', got %s", det.ModelID)
	}
	if det.Backend != BackendONNX {
		t.Errorf("Expected Backend onnx, got %s", det.Backend)
	}
	if det.Attributes["color"] != "blue" {
		t.Error("Expected color attribute 'blue'")
	}
}

func TestDetectRequest_Fields(t *testing.T) {
	frame := &Frame{
		Data:      []byte{1, 2, 3},
		Width:     1920,
		Height:    1080,
		Timestamp: time.Now(),
	}

	req := &DetectRequest{
		CameraID:      "cam-1",
		Frame:         frame,
		MinConfidence: 0.5,
		Objects:       []string{"person", "vehicle"},
		Zones:         []string{"zone-1", "zone-2"},
	}

	if req.CameraID != "cam-1" {
		t.Errorf("Expected CameraID 'cam-1', got %s", req.CameraID)
	}
	if req.MinConfidence != 0.5 {
		t.Errorf("Expected MinConfidence 0.5, got %f", req.MinConfidence)
	}
	if len(req.Objects) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(req.Objects))
	}
	if len(req.Zones) != 2 {
		t.Errorf("Expected 2 zones, got %d", len(req.Zones))
	}
}

func TestDetectResponse_Fields(t *testing.T) {
	now := time.Now()
	resp := &DetectResponse{
		CameraID:       "cam-1",
		Timestamp:      now,
		MotionDetected: true,
		Detections: []Detection{
			{ObjectType: ObjectPerson, Confidence: 0.9},
		},
		ProcessTimeMs: 25.5,
		ModelID:       "yolov8",
	}

	if resp.CameraID != "cam-1" {
		t.Errorf("Expected CameraID 'cam-1', got %s", resp.CameraID)
	}
	if !resp.MotionDetected {
		t.Error("Expected MotionDetected true")
	}
	if len(resp.Detections) != 1 {
		t.Errorf("Expected 1 detection, got %d", len(resp.Detections))
	}
	if resp.ProcessTimeMs != 25.5 {
		t.Errorf("Expected ProcessTimeMs 25.5, got %f", resp.ProcessTimeMs)
	}
}

func TestFrame_Fields(t *testing.T) {
	now := time.Now()
	frame := &Frame{
		Data:      []byte{1, 2, 3, 4, 5},
		Width:     1920,
		Height:    1080,
		Format:    "jpeg",
		Timestamp: now,
	}

	if len(frame.Data) != 5 {
		t.Errorf("Expected Data length 5, got %d", len(frame.Data))
	}
	if frame.Width != 1920 {
		t.Errorf("Expected Width 1920, got %d", frame.Width)
	}
	if frame.Height != 1080 {
		t.Errorf("Expected Height 1080, got %d", frame.Height)
	}
	if frame.Format != "jpeg" {
		t.Errorf("Expected Format 'jpeg', got %s", frame.Format)
	}
}

func TestObjectType_Constants(t *testing.T) {
	tests := []struct {
		objectType ObjectType
		expected   string
	}{
		{ObjectPerson, "person"},
		{ObjectVehicle, "vehicle"},
		{ObjectAnimal, "animal"},
		{ObjectPackage, "package"},
		{ObjectFace, "face"},
	}

	for _, tt := range tests {
		if string(tt.objectType) != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, string(tt.objectType))
		}
	}
}

func TestModelType_Constants(t *testing.T) {
	tests := []struct {
		modelType ModelType
		expected  string
	}{
		{ModelYOLOv8, "yolov8"},
		{ModelYOLO11, "yolo11"},
		{ModelYOLO12, "yolo12"},
		{ModelYOLONAS, "yolonas"},
		{ModelMobileNet, "mobilenet"},
		{ModelFrigatePlus, "frigate_plus"},
		{ModelFaceNet, "facenet"},
		{ModelLPR, "lpr"},
	}

	for _, tt := range tests {
		if string(tt.modelType) != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, string(tt.modelType))
		}
	}
}

func TestBackendType_Constants(t *testing.T) {
	tests := []struct {
		backendType BackendType
		expected    string
	}{
		{BackendONNX, "onnx"},
		{BackendNVIDIA, "nvidia"},
		{BackendOpenVINO, "openvino"},
		{BackendCoreML, "coreml"},
		{BackendCoral, "coral"},
		{BackendFrigate, "frigate"},
		{BackendCPU, "cpu"},
	}

	for _, tt := range tests {
		if string(tt.backendType) != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, string(tt.backendType))
		}
	}
}

func TestServiceStatus_Fields(t *testing.T) {
	status := &ServiceStatus{
		Connected:       true,
		ProcessedCount:  1000,
		ErrorCount:      5,
		AvgLatencyMs:    25.5,
		Uptime:          3600.0,
		QueueSize:       10,
	}

	if !status.Connected {
		t.Error("Expected Connected true")
	}
	if status.ProcessedCount != 1000 {
		t.Errorf("Expected ProcessedCount 1000, got %d", status.ProcessedCount)
	}
	if status.ErrorCount != 5 {
		t.Errorf("Expected ErrorCount 5, got %d", status.ErrorCount)
	}
	if status.AvgLatencyMs != 25.5 {
		t.Errorf("Expected AvgLatencyMs 25.5, got %f", status.AvgLatencyMs)
	}
	if status.Uptime != 3600.0 {
		t.Errorf("Expected Uptime 3600, got %f", status.Uptime)
	}
	if status.QueueSize != 10 {
		t.Errorf("Expected QueueSize 10, got %d", status.QueueSize)
	}
}

func TestBackendInfo_Fields(t *testing.T) {
	info := BackendInfo{
		Type:      BackendONNX,
		Available: true,
		Version:   "1.14.0",
		Device:    "CPU",
	}

	if info.Type != BackendONNX {
		t.Errorf("Expected Type onnx, got %s", info.Type)
	}
	if !info.Available {
		t.Error("Expected Available true")
	}
	if info.Version != "1.14.0" {
		t.Errorf("Expected Version '1.14.0', got %s", info.Version)
	}
	if info.Device != "CPU" {
		t.Errorf("Expected Device 'CPU', got %s", info.Device)
	}
}

func TestModelInfo_Fields(t *testing.T) {
	info := ModelInfo{
		ID:       "model-123",
		Name:     "YOLOv8",
		Type:     ModelYOLOv8,
		Backend:  BackendONNX,
		Path:     "/models/yolov8.onnx",
		Loaded:   true,
		Classes:  []string{"person", "car"},
	}

	if info.ID != "model-123" {
		t.Errorf("Expected ID 'model-123', got %s", info.ID)
	}
	if info.Name != "YOLOv8" {
		t.Errorf("Expected Name 'YOLOv8', got %s", info.Name)
	}
	if info.Type != ModelYOLOv8 {
		t.Errorf("Expected Type yolov8, got %s", info.Type)
	}
	if !info.Loaded {
		t.Error("Expected Loaded true")
	}
	if len(info.Classes) != 2 {
		t.Errorf("Expected 2 classes, got %d", len(info.Classes))
	}
}

func TestNewService_WithEmptyConfig(t *testing.T) {
	cfg := &config.Config{
		Cameras: []config.CameraConfig{},
	}

	svcCfg := ServiceConfig{}

	// This will fail to connect, but should not panic
	_, err := NewService(cfg, nil, svcCfg)
	// May error due to failed connection attempt
	if err != nil {
		t.Logf("NewService error (expected without detection server): %v", err)
	}
}

func TestService_Stop_NotRunning(t *testing.T) {
	cfg := &config.Config{}
	svc := &Service{
		config:  cfg,
		streams: make(map[string]context.CancelFunc),
		running: false,
	}

	err := svc.Stop()
	if err != nil {
		t.Errorf("Stop should not error when not running: %v", err)
	}
}

func TestService_StopCamera_NotRunning(t *testing.T) {
	cfg := &config.Config{}
	svc := &Service{
		config:  cfg,
		streams: make(map[string]context.CancelFunc),
	}

	err := svc.StopCamera("nonexistent")
	if err != nil {
		t.Errorf("StopCamera should not error for nonexistent camera: %v", err)
	}
}

func TestMotionStatus_Fields(t *testing.T) {
	status := MotionStatus{
		FramesProcessed: 5000,
		MotionDetected:  250,
		MotionRate:      0.05,
		AvgLatencyMs:    15.5,
		CamerasTracked:  4,
	}

	if status.FramesProcessed != 5000 {
		t.Errorf("Expected FramesProcessed 5000, got %d", status.FramesProcessed)
	}
	if status.MotionDetected != 250 {
		t.Errorf("Expected MotionDetected 250, got %d", status.MotionDetected)
	}
	if status.MotionRate != 0.05 {
		t.Errorf("Expected MotionRate 0.05, got %f", status.MotionRate)
	}
	if status.AvgLatencyMs != 15.5 {
		t.Errorf("Expected AvgLatencyMs 15.5, got %f", status.AvgLatencyMs)
	}
	if status.CamerasTracked != 4 {
		t.Errorf("Expected CamerasTracked 4, got %d", status.CamerasTracked)
	}
}

func TestMotionConfig_Fields(t *testing.T) {
	cfg := MotionConfig{
		Enabled:        true,
		Method:         "frame_diff",
		Threshold:      0.05,
		PixelThreshold: 25,
	}

	if !cfg.Enabled {
		t.Error("Expected Enabled true")
	}
	if cfg.Method != "frame_diff" {
		t.Errorf("Expected Method 'frame_diff', got %s", cfg.Method)
	}
	if cfg.Threshold != 0.05 {
		t.Errorf("Expected Threshold 0.05, got %f", cfg.Threshold)
	}
	if cfg.PixelThreshold != 25 {
		t.Errorf("Expected PixelThreshold 25, got %d", cfg.PixelThreshold)
	}
}
