// Package recording provides video recording and segment management for the NVR system
package recording

import (
	"context"
	"time"
)

// StorageTier represents the storage tier for a segment
type StorageTier string

const (
	StorageTierHot  StorageTier = "hot"  // Fast local storage
	StorageTierWarm StorageTier = "warm" // Local but compressed
	StorageTierCold StorageTier = "cold" // Archive/S3
)

// RecordingMode represents the recording mode
type RecordingMode string

const (
	RecordingModeContinuous RecordingMode = "continuous" // Always recording
	RecordingModeMotion     RecordingMode = "motion"     // Motion-triggered
	RecordingModeEvents     RecordingMode = "events"     // Event-triggered only
)

// RecorderState represents the state of a camera recorder
type RecorderState string

const (
	RecorderStateIdle     RecorderState = "idle"
	RecorderStateStarting RecorderState = "starting"
	RecorderStateRunning  RecorderState = "running"
	RecorderStateStopping RecorderState = "stopping"
	RecorderStateError    RecorderState = "error"
)

// Segment represents a recorded video segment
type Segment struct {
	ID              string      `json:"id" db:"id"`
	CameraID        string      `json:"camera_id" db:"camera_id"`
	StartTime       time.Time   `json:"start_time" db:"start_time"`
	EndTime         time.Time   `json:"end_time" db:"end_time"`
	Duration        float64     `json:"duration" db:"duration"` // seconds
	FilePath        string      `json:"file_path" db:"file_path"`
	FileSize        int64       `json:"file_size" db:"file_size"` // bytes
	StorageTier     StorageTier `json:"storage_tier" db:"storage_tier"`
	HasEvents       bool        `json:"has_events" db:"has_events"`
	EventCount      int         `json:"event_count" db:"event_count"`
	Codec           string      `json:"codec" db:"codec"`
	Resolution      string      `json:"resolution" db:"resolution"`
	Bitrate         int         `json:"bitrate" db:"bitrate"` // bps
	Thumbnail       string      `json:"thumbnail,omitempty" db:"thumbnail"`
	Checksum        string      `json:"checksum,omitempty" db:"checksum"` // SHA256
	RecordingMode   string      `json:"recording_mode" db:"recording_mode"`
	TriggerEventID  string      `json:"trigger_event_id,omitempty" db:"trigger_event_id"`
	CreatedAt       time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at" db:"updated_at"`
}

// SegmentMetadata holds extracted metadata from a segment file
type SegmentMetadata struct {
	Duration   float64 // seconds
	Codec      string
	Resolution string
	Bitrate    int
	FileSize   int64
	StartTime  time.Time
	EndTime    time.Time
}

// TimelineSegment represents a segment in timeline view
type TimelineSegment struct {
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	Type       string    `json:"type"` // recording, gap
	HasEvents  bool      `json:"has_events"`
	EventCount int       `json:"event_count"`
	SegmentIDs []string  `json:"segment_ids,omitempty"`
}

// Timeline represents timeline data for a camera
type Timeline struct {
	CameraID   string            `json:"camera_id"`
	StartTime  time.Time         `json:"start_time"`
	EndTime    time.Time         `json:"end_time"`
	Segments   []TimelineSegment `json:"segments"`
	TotalSize  int64             `json:"total_size"`
	TotalHours float64           `json:"total_hours"`
}

// RecorderStatus holds the status of a camera recorder
type RecorderStatus struct {
	CameraID        string        `json:"camera_id"`
	State           RecorderState `json:"state"`
	CurrentSegment  string        `json:"current_segment,omitempty"`
	SegmentStart    *time.Time    `json:"segment_start,omitempty"`
	BytesWritten    int64         `json:"bytes_written"`
	SegmentsCreated int           `json:"segments_created"`
	Uptime          float64       `json:"uptime"` // seconds
	LastError       string        `json:"last_error,omitempty"`
	LastErrorTime   *time.Time    `json:"last_error_time,omitempty"`
}

// RetentionStats holds retention cleanup statistics
type RetentionStats struct {
	SegmentsDeleted int   `json:"segments_deleted"`
	BytesFreed      int64 `json:"bytes_freed"`
	OldestRemaining time.Time `json:"oldest_remaining"`
	NewestRemaining time.Time `json:"newest_remaining"`
}

// StorageStats holds storage statistics
type StorageStats struct {
	TotalBytes     int64              `json:"total_bytes"`
	UsedBytes      int64              `json:"used_bytes"`
	AvailableBytes int64              `json:"available_bytes"`
	SegmentCount   int                `json:"segment_count"`
	ByCamera       map[string]int64   `json:"by_camera"`
	ByTier         map[StorageTier]int64 `json:"by_tier"`
}

// ListOptions holds options for listing segments
type ListOptions struct {
	CameraID  string
	StartTime *time.Time
	EndTime   *time.Time
	HasEvents *bool
	Tier      *StorageTier
	Limit     int
	Offset    int
	OrderBy   string // start_time, end_time, file_size
	OrderDesc bool
}

// RecordingService defines the interface for recording management
type RecordingService interface {
	// Lifecycle
	Start(ctx context.Context) error
	Stop() error

	// Camera recording control
	StartCamera(cameraID string) error
	StopCamera(cameraID string) error
	RestartCamera(cameraID string) error

	// Event-triggered recording
	TriggerEventRecording(cameraID, eventID string) error

	// Segment queries
	GetSegment(ctx context.Context, id string) (*Segment, error)
	ListSegments(ctx context.Context, opts ListOptions) ([]Segment, int, error)
	DeleteSegment(ctx context.Context, id string) error

	// Timeline queries
	GetTimeline(ctx context.Context, cameraID string, start, end time.Time) (*Timeline, error)
	GetTimelineSegments(ctx context.Context, cameraID string, start, end time.Time) ([]TimelineSegment, error)

	// Status
	GetRecorderStatus(cameraID string) (*RecorderStatus, error)
	GetAllRecorderStatus() map[string]*RecorderStatus
	GetStorageStats(ctx context.Context) (*StorageStats, error)

	// Retention
	RunRetention(ctx context.Context) (*RetentionStats, error)
}

// Repository defines the interface for segment persistence
type Repository interface {
	// Segment CRUD
	Create(ctx context.Context, segment *Segment) error
	Get(ctx context.Context, id string) (*Segment, error)
	Update(ctx context.Context, segment *Segment) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, opts ListOptions) ([]Segment, int, error)

	// Bulk operations
	DeleteBefore(ctx context.Context, cameraID string, before time.Time) (int, error)
	UpdateTier(ctx context.Context, ids []string, tier StorageTier) error

	// Query operations
	GetByTimeRange(ctx context.Context, cameraID string, start, end time.Time) ([]Segment, error)
	GetOldestSegments(ctx context.Context, cameraID string, limit int) ([]Segment, error)
	GetTotalSize(ctx context.Context, cameraID string) (int64, error)
	GetSegmentCount(ctx context.Context, cameraID string) (int, error)

	// Statistics
	GetStorageByCamera(ctx context.Context) (map[string]int64, error)
	GetStorageByTier(ctx context.Context) (map[StorageTier]int64, error)
}

// RingBuffer defines the interface for pre-event frame buffering
type RingBuffer interface {
	// Write adds data to the buffer
	Write(data []byte) error

	// Read reads all buffered data
	Read() []byte

	// Duration returns the current buffer duration
	Duration() time.Duration

	// Clear clears the buffer
	Clear()

	// Close closes the buffer
	Close() error
}

// SegmentHandler defines the interface for segment file operations
type SegmentHandler interface {
	// Create creates a new segment file path
	CreatePath(cameraID string, startTime time.Time) string

	// ExtractMetadata extracts metadata from a segment file
	ExtractMetadata(filePath string) (*SegmentMetadata, error)

	// GenerateThumbnail generates a thumbnail from a segment
	GenerateThumbnail(segmentPath, thumbnailPath string, offsetSeconds float64) error

	// CalculateChecksum calculates SHA256 checksum
	CalculateChecksum(filePath string) (string, error)

	// Delete deletes a segment file and its thumbnail
	Delete(segment *Segment) error
}

// FFmpegConfig holds FFmpeg process configuration
type FFmpegConfig struct {
	InputURL        string
	OutputPath      string
	SegmentDuration int    // seconds
	Codec           string // copy, libx264, etc.
	HWAccel         string // cuda, vaapi, videotoolbox, etc.
	ExtraArgs       []string
}

// StreamInfo holds information about a video stream
type StreamInfo struct {
	Codec      string
	Width      int
	Height     int
	FPS        float64
	Bitrate    int
	Duration   float64
	HasAudio   bool
	AudioCodec string
}
