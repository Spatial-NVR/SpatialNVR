// Package detection provides object detection capabilities for the NVR system
package detection

import (
	"context"
	"image"
	"time"
)

// ObjectType represents a detected object type
type ObjectType string

const (
	ObjectPerson  ObjectType = "person"
	ObjectVehicle ObjectType = "vehicle"
	ObjectAnimal  ObjectType = "animal"
	ObjectFace    ObjectType = "face"
	ObjectPlate   ObjectType = "license_plate"
	ObjectPackage ObjectType = "package"
	ObjectUnknown ObjectType = "unknown"
)

// BackendType represents a detection backend
type BackendType string

const (
	BackendNVIDIA    BackendType = "nvidia"
	BackendOpenVINO  BackendType = "openvino"
	BackendCoral     BackendType = "coral"
	BackendCoreML    BackendType = "coreml"
	BackendONNX      BackendType = "onnx"
	BackendFrigate   BackendType = "frigate"
	BackendCPU       BackendType = "cpu"
)

// ModelType represents a detection model type
type ModelType string

const (
	ModelYOLO12     ModelType = "yolo12"
	ModelYOLO11     ModelType = "yolo11"
	ModelYOLOv8     ModelType = "yolov8"
	ModelYOLONAS    ModelType = "yolonas"
	ModelMobileNet  ModelType = "mobilenet"
	ModelFrigatePlus ModelType = "frigate_plus"
	ModelFaceNet    ModelType = "facenet"
	ModelLPR        ModelType = "lpr"
)

// Detection represents a single detection result
type Detection struct {
	ID          string                 `json:"id"`
	CameraID    string                 `json:"camera_id"`
	ObjectType  ObjectType             `json:"object_type"`
	Label       string                 `json:"label"`
	Confidence  float64                `json:"confidence"`
	BoundingBox BoundingBox            `json:"bounding_box"`
	Timestamp   time.Time              `json:"timestamp"`
	FrameID     int64                  `json:"frame_id,omitempty"`
	TrackID     string                 `json:"track_id,omitempty"`
	ModelID     string                 `json:"model_id,omitempty"`
	Backend     BackendType            `json:"backend,omitempty"`
	Attributes  map[string]interface{} `json:"attributes,omitempty"`
	ZoneIDs     []string               `json:"zone_ids,omitempty"`
}

// BoundingBox represents a detection bounding box
type BoundingBox struct {
	X      float64 `json:"x"`      // Top-left X (0-1 normalized)
	Y      float64 `json:"y"`      // Top-left Y (0-1 normalized)
	Width  float64 `json:"width"`  // Width (0-1 normalized)
	Height float64 `json:"height"` // Height (0-1 normalized)
}

// Center returns the center point of the bounding box
func (b BoundingBox) Center() (float64, float64) {
	return b.X + b.Width/2, b.Y + b.Height/2
}

// Area returns the area of the bounding box
func (b BoundingBox) Area() float64 {
	return b.Width * b.Height
}

// Intersects checks if two bounding boxes intersect
func (b BoundingBox) Intersects(other BoundingBox) bool {
	return !(b.X+b.Width < other.X ||
		other.X+other.Width < b.X ||
		b.Y+b.Height < other.Y ||
		other.Y+other.Height < b.Y)
}

// IoU calculates Intersection over Union with another box
func (b BoundingBox) IoU(other BoundingBox) float64 {
	// Calculate intersection
	x1 := max(b.X, other.X)
	y1 := max(b.Y, other.Y)
	x2 := min(b.X+b.Width, other.X+other.Width)
	y2 := min(b.Y+b.Height, other.Y+other.Height)

	if x2 <= x1 || y2 <= y1 {
		return 0
	}

	intersection := (x2 - x1) * (y2 - y1)
	union := b.Area() + other.Area() - intersection

	if union == 0 {
		return 0
	}

	return intersection / union
}

// Frame represents a video frame for detection
type Frame struct {
	CameraID  string
	Timestamp time.Time
	FrameID   int64
	Image     image.Image
	Data      []byte // Raw image data (JPEG/PNG)
	Width     int
	Height    int
	Format    string // "jpeg", "png", "rgb", "bgr"
}

// DetectRequest represents a detection request
type DetectRequest struct {
	CameraID      string     `json:"camera_id"`
	Frame         *Frame     `json:"-"`
	ImageData     []byte     `json:"image_data,omitempty"`
	ModelIDs      []string   `json:"model_ids,omitempty"`
	MinConfidence float64    `json:"min_confidence"`
	Objects       []string   `json:"objects,omitempty"` // Filter for specific objects
	Zones         []string   `json:"zones,omitempty"`   // Filter for specific zones
}

// DetectResponse represents a detection response
type DetectResponse struct {
	CameraID       string       `json:"camera_id"`
	Timestamp      time.Time    `json:"timestamp"`
	FrameID        int64        `json:"frame_id,omitempty"`
	MotionDetected bool         `json:"motion_detected"`
	Detections     []Detection  `json:"detections"`
	ProcessTimeMs  float64      `json:"process_time_ms"`
	Backend        BackendType  `json:"backend"`
	ModelID        string       `json:"model_id"`
}

// ModelInfo represents information about a detection model
type ModelInfo struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Type         ModelType   `json:"type"`
	Backend      BackendType `json:"backend"`
	Path         string      `json:"path"`
	Version      string      `json:"version,omitempty"`
	InputSize    []int       `json:"input_size"` // [width, height]
	InputFormat  string      `json:"input_format"` // "nchw", "nhwc"
	PixelFormat  string      `json:"pixel_format"` // "rgb", "bgr"
	Classes      []string    `json:"classes,omitempty"`
	Loaded       bool        `json:"loaded"`
	LoadTime     *time.Time  `json:"load_time,omitempty"`
}

// BackendInfo represents information about a detection backend
type BackendInfo struct {
	Type        BackendType `json:"type"`
	Available   bool        `json:"available"`
	Version     string      `json:"version,omitempty"`
	Device      string      `json:"device,omitempty"`
	DeviceIndex int         `json:"device_index,omitempty"`
	Memory      int64       `json:"memory,omitempty"` // bytes
	Compute     string      `json:"compute,omitempty"` // CUDA version, etc.
}

// ServiceStatus represents the detection service status
type ServiceStatus struct {
	Connected    bool           `json:"connected"`
	Backends     []BackendInfo  `json:"backends"`
	Models       []ModelInfo    `json:"models"`
	QueueSize    int            `json:"queue_size"`
	ProcessedCount int64        `json:"processed_count"`
	ErrorCount   int64          `json:"error_count"`
	AvgLatencyMs float64        `json:"avg_latency_ms"`
	Uptime       float64        `json:"uptime"` // seconds
}

// Zone represents a detection zone for filtering
type Zone struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	CameraID      string      `json:"camera_id"`
	Enabled       bool        `json:"enabled"`
	Points        [][]float64 `json:"points"` // Polygon points [[x,y], ...]
	Objects       []string    `json:"objects"` // Objects to detect
	MinConfidence float64     `json:"min_confidence"`
	MinSize       float64     `json:"min_size,omitempty"` // Minimum object size (0-1)
}

// ContainsPoint checks if a point is inside the zone polygon
func (z Zone) ContainsPoint(x, y float64) bool {
	if len(z.Points) < 3 {
		return false
	}

	// Ray casting algorithm
	n := len(z.Points)
	inside := false

	j := n - 1
	for i := 0; i < n; i++ {
		xi, yi := z.Points[i][0], z.Points[i][1]
		xj, yj := z.Points[j][0], z.Points[j][1]

		if ((yi > y) != (yj > y)) && (x < (xj-xi)*(y-yi)/(yj-yi)+xi) {
			inside = !inside
		}
		j = i
	}

	return inside
}

// ContainsBox checks if a bounding box overlaps with the zone
func (z Zone) ContainsBox(box BoundingBox) bool {
	cx, cy := box.Center()
	return z.ContainsPoint(cx, cy)
}

// Detector defines the interface for detection backends
type Detector interface {
	// Name returns the detector name
	Name() string

	// Type returns the backend type
	Type() BackendType

	// IsAvailable checks if the backend is available
	IsAvailable() bool

	// GetInfo returns backend information
	GetInfo() BackendInfo

	// LoadModel loads a model
	LoadModel(ctx context.Context, path string, modelType ModelType) (string, error)

	// UnloadModel unloads a model
	UnloadModel(ctx context.Context, modelID string) error

	// Detect performs detection on an image
	Detect(ctx context.Context, req *DetectRequest) (*DetectResponse, error)

	// GetModels returns loaded models
	GetModels() []ModelInfo

	// Close closes the detector
	Close() error
}

// DetectionService defines the interface for the detection service
type DetectionService interface {
	// Start starts the detection service
	Start(ctx context.Context) error

	// Stop stops the detection service
	Stop() error

	// Detect performs detection on a frame
	Detect(ctx context.Context, req *DetectRequest) (*DetectResponse, error)

	// DetectAsync performs async detection
	DetectAsync(req *DetectRequest) <-chan *DetectResponse

	// GetStatus returns service status
	GetStatus() *ServiceStatus

	// LoadModel loads a model
	LoadModel(ctx context.Context, path string, modelType ModelType, backend BackendType) (string, error)

	// UnloadModel unloads a model
	UnloadModel(ctx context.Context, modelID string) error

	// GetBackends returns available backends
	GetBackends() []BackendInfo

	// GetModels returns loaded models
	GetModels() []ModelInfo
}

// FrameGrabber defines the interface for grabbing frames from cameras
type FrameGrabber interface {
	// GrabFrame grabs a single frame from a camera
	GrabFrame(ctx context.Context, cameraID string) (*Frame, error)

	// StartStream starts a continuous frame stream
	StartStream(ctx context.Context, cameraID string, fps int) (<-chan *Frame, error)

	// StopStream stops a frame stream
	StopStream(cameraID string) error

	// Close closes the frame grabber
	Close() error
}

// TrackedObject represents an object being tracked across frames
type TrackedObject struct {
	TrackID     string       `json:"track_id"`
	ObjectType  ObjectType   `json:"object_type"`
	Label       string       `json:"label"`
	FirstSeen   time.Time    `json:"first_seen"`
	LastSeen    time.Time    `json:"last_seen"`
	Detections  []Detection  `json:"detections,omitempty"`
	BoundingBox BoundingBox  `json:"bounding_box"` // Current position
	Velocity    [2]float64   `json:"velocity,omitempty"` // Pixels per second [vx, vy]
	Stationary  bool         `json:"stationary"`
	FramesMissed int         `json:"frames_missed"`
}

// ObjectTracker defines the interface for object tracking
type ObjectTracker interface {
	// Update updates tracking with new detections
	Update(detections []Detection, timestamp time.Time) []TrackedObject

	// GetTracks returns current tracks
	GetTracks() []TrackedObject

	// GetTrack returns a specific track
	GetTrack(trackID string) (*TrackedObject, bool)

	// Reset resets the tracker
	Reset()
}

// MotionStatus represents motion detection statistics
type MotionStatus struct {
	FramesProcessed  int     `json:"frames_processed"`
	MotionDetected   int     `json:"motion_detected"`
	MotionSkipped    int     `json:"motion_skipped"`
	MotionRate       float64 `json:"motion_rate"`
	AvgLatencyMs     float64 `json:"avg_latency_ms"`
	LastMotionTime   float64 `json:"last_motion_time"`
	CamerasTracked   int     `json:"cameras_tracked"`
	Config           MotionConfig `json:"config"`
}

// MotionConfig represents motion detection configuration
type MotionConfig struct {
	Enabled        bool    `json:"enabled"`
	Method         string  `json:"method"`          // "frame_diff", "mog2", "knn"
	Threshold      float64 `json:"threshold"`       // 0-1, percentage of pixels changed
	PixelThreshold int     `json:"pixel_threshold"` // Per-pixel difference threshold
}

// MotionRegion represents a region where motion was detected
type MotionRegion struct {
	X         float64 `json:"x"`         // Normalized 0-1
	Y         float64 `json:"y"`         // Normalized 0-1
	Width     float64 `json:"width"`     // Normalized 0-1
	Height    float64 `json:"height"`    // Normalized 0-1
	Intensity float64 `json:"intensity"` // Motion intensity 0-1
}

// Helper functions
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
