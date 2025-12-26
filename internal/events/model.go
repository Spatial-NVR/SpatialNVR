// Package events provides event management functionality
package events

import (
	"encoding/json"
	"time"
)

// EventType represents the type of event
type EventType string

const (
	EventMotion      EventType = "motion"
	EventPerson      EventType = "person"
	EventVehicle     EventType = "vehicle"
	EventAnimal      EventType = "animal"
	EventFace        EventType = "face"
	EventLPR         EventType = "license_plate"
	EventAudio       EventType = "audio"
	EventStateChange EventType = "state_change"
	EventDoorbell    EventType = "doorbell"
	EventLineCross   EventType = "line_cross"
	EventZoneEnter   EventType = "zone_enter"
	EventZoneExit    EventType = "zone_exit"
	EventTamper      EventType = "tamper"
)

// Event represents a detected event
type Event struct {
	ID             string          `json:"id"`
	CameraID       string          `json:"camera_id"`
	EventType      EventType       `json:"event_type"`
	Label          string          `json:"label,omitempty"`
	Timestamp      time.Time       `json:"timestamp"`
	EndTimestamp   *time.Time      `json:"end_timestamp,omitempty"`
	Confidence     float64         `json:"confidence"`
	ThumbnailPath  string          `json:"thumbnail_path,omitempty"`
	VideoSegmentID string          `json:"video_segment_id,omitempty"`
	ZoneID         string          `json:"zone_id,omitempty"`
	ZoneName       string          `json:"zone_name,omitempty"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
	Acknowledged   bool            `json:"acknowledged"`
	AcknowledgedAt *time.Time      `json:"acknowledged_at,omitempty"`
	Tags           []string        `json:"tags,omitempty"`
	Notes          string          `json:"notes,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

// ListOptions represents filters for querying events
type ListOptions struct {
	CameraID  string    `json:"camera_id,omitempty"`
	EventType EventType `json:"event_type,omitempty"`
	StartTime time.Time `json:"start_time,omitempty"`
	EndTime   time.Time `json:"end_time,omitempty"`
	ZoneID    string    `json:"zone_id,omitempty"`
	Limit     int       `json:"limit,omitempty"`
	Offset    int       `json:"offset,omitempty"`
}

// MotionZone represents a detection zone configuration
type MotionZone struct {
	ID            string    `json:"id"`
	CameraID      string    `json:"camera_id"`
	Name          string    `json:"name"`
	Enabled       bool      `json:"enabled"`
	Points        []Point   `json:"points"`
	ObjectTypes   []string  `json:"object_types,omitempty"`
	MinConfidence float64   `json:"min_confidence"`
	MinSize       float64   `json:"min_size,omitempty"`
	Sensitivity   int       `json:"sensitivity"`
	Cooldown      int       `json:"cooldown_seconds"`
	Notifications bool      `json:"notifications"`
	Recording     bool      `json:"recording"`
	Color         string    `json:"color,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Point represents a 2D point with normalized coordinates
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// DoorbellEvent represents a doorbell ring event
type DoorbellEvent struct {
	EventID      string     `json:"event_id"`
	CameraID     string     `json:"camera_id"`
	Timestamp    time.Time  `json:"timestamp"`
	Answered     bool       `json:"answered"`
	AnsweredAt   *time.Time `json:"answered_at,omitempty"`
	AnsweredBy   string     `json:"answered_by,omitempty"`
	Duration     int        `json:"duration_seconds,omitempty"`
	ThumbnailURL string     `json:"thumbnail_url,omitempty"`
}

// AudioEvent represents an audio detection event
type AudioEvent struct {
	EventID      string    `json:"event_id"`
	CameraID     string    `json:"camera_id"`
	Timestamp    time.Time `json:"timestamp"`
	AudioType    string    `json:"audio_type"`
	Confidence   float64   `json:"confidence"`
	Duration     float64   `json:"duration_seconds"`
	AudioClipURL string    `json:"audio_clip_url,omitempty"`
}
