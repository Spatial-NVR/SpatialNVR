package nvrspatialtracking

import (
	"encoding/json"
	"math"
	"time"
)

// Point represents a 2D coordinate
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Distance calculates distance between two points
func (p Point) Distance(other Point) float64 {
	dx := p.X - other.X
	dy := p.Y - other.Y
	return math.Sqrt(dx*dx + dy*dy)
}

// Polygon is a series of points forming a closed shape
type Polygon []Point

// MarshalJSON custom marshaler for Polygon to handle empty slices
func (p Polygon) MarshalJSON() ([]byte, error) {
	if p == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]Point(p))
}

// ContainsPoint checks if a point is inside the polygon using ray casting
func (p Polygon) ContainsPoint(pt Point) bool {
	if len(p) < 3 {
		return false
	}

	n := len(p)
	inside := false

	j := n - 1
	for i := 0; i < n; i++ {
		xi, yi := p[i].X, p[i].Y
		xj, yj := p[j].X, p[j].Y

		if ((yi > pt.Y) != (yj > pt.Y)) &&
			(pt.X < (xj-xi)*(pt.Y-yi)/(yj-yi)+xi) {
			inside = !inside
		}
		j = i
	}

	return inside
}

// Intersects checks if two polygons overlap
func (p Polygon) Intersects(other Polygon) bool {
	// Check if any vertex of p is inside other
	for _, pt := range p {
		if other.ContainsPoint(pt) {
			return true
		}
	}

	// Check if any vertex of other is inside p
	for _, pt := range other {
		if p.ContainsPoint(pt) {
			return true
		}
	}

	return false
}

// SpatialMap represents a floor plan or area where cameras are positioned
type SpatialMap struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	ImageURL  string    `json:"image_url,omitempty"`
	Width     float64   `json:"width"`  // Logical width in units
	Height    float64   `json:"height"` // Logical height in units
	Scale     float64   `json:"scale"`  // Units per meter (for distance calculations)
	Metadata  MapMeta   `json:"metadata,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MapMeta contains optional metadata for a spatial map
type MapMeta struct {
	Building string `json:"building,omitempty"`
	Floor    string `json:"floor,omitempty"`
	Area     string `json:"area,omitempty"`
}

// CameraPlacement represents where a camera is positioned on a spatial map
type CameraPlacement struct {
	ID              string    `json:"id"`
	CameraID        string    `json:"camera_id"` // Reference to NVR camera
	MapID           string    `json:"map_id"`    // Which spatial map
	Position        Point     `json:"position"`  // Position on map
	Rotation        float64   `json:"rotation"`  // Degrees, 0 = pointing right
	FOVAngle        float64   `json:"fov_angle"` // Field of view angle (degrees)
	FOVDepth        float64   `json:"fov_depth"` // How far camera sees (units)
	CoveragePolygon Polygon   `json:"coverage_polygon,omitempty"` // Optional manual override
	MountHeight     float64   `json:"mount_height,omitempty"`     // Height in meters
	TiltAngle       float64   `json:"tilt_angle,omitempty"`       // Vertical tilt
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// CalculateFOVPolygon generates the camera's field of view polygon
func (cp *CameraPlacement) CalculateFOVPolygon() Polygon {
	if len(cp.CoveragePolygon) > 0 {
		return cp.CoveragePolygon
	}

	halfAngle := cp.FOVAngle / 2.0 * math.Pi / 180.0
	rotation := cp.Rotation * math.Pi / 180.0

	leftAngle := rotation - halfAngle
	rightAngle := rotation + halfAngle

	leftPoint := Point{
		X: cp.Position.X + cp.FOVDepth*math.Cos(leftAngle),
		Y: cp.Position.Y + cp.FOVDepth*math.Sin(leftAngle),
	}
	rightPoint := Point{
		X: cp.Position.X + cp.FOVDepth*math.Cos(rightAngle),
		Y: cp.Position.Y + cp.FOVDepth*math.Sin(rightAngle),
	}

	return Polygon{cp.Position, leftPoint, rightPoint}
}

// TransitionType defines how cameras are connected
type TransitionType string

const (
	TransitionOverlap  TransitionType = "overlap"  // Cameras share common view
	TransitionAdjacent TransitionType = "adjacent" // Cameras touch but don't overlap
	TransitionGap      TransitionType = "gap"      // Space between camera views
)

// EdgeDirection represents which edge of the camera frame
type EdgeDirection string

const (
	EdgeTop    EdgeDirection = "top"
	EdgeBottom EdgeDirection = "bottom"
	EdgeLeft   EdgeDirection = "left"
	EdgeRight  EdgeDirection = "right"
)

// ZoneDefinition defines an area on the camera frame
type ZoneDefinition struct {
	Edge  EdgeDirection `json:"edge"`
	Start float64       `json:"start"` // 0.0 to 1.0, start position along edge
	End   float64       `json:"end"`   // 0.0 to 1.0, end position along edge
}

// CameraTransition defines how objects move between cameras
type CameraTransition struct {
	ID            string         `json:"id"`
	FromCameraID  string         `json:"from_camera_id"`
	ToCameraID    string         `json:"to_camera_id"`
	Type          TransitionType `json:"type"`
	Bidirectional bool           `json:"bidirectional"` // If true, works both ways

	// For overlap transitions
	OverlapZone Polygon `json:"overlap_zone,omitempty"`

	// For gap transitions
	ExpectedTransitTime float64 `json:"expected_transit_time,omitempty"` // Seconds
	TransitTimeVariance float64 `json:"transit_time_variance,omitempty"` // +/- seconds

	// Exit/entry zones on camera frames
	ExitZone  *ZoneDefinition `json:"exit_zone,omitempty"`
	EntryZone *ZoneDefinition `json:"entry_zone,omitempty"`

	// Learned statistics
	AvgTransitTime     float64 `json:"avg_transit_time,omitempty"`
	SuccessRate        float64 `json:"success_rate,omitempty"`
	TotalHandoffs      int     `json:"total_handoffs"`
	SuccessfulHandoffs int     `json:"successful_handoffs"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TrackState represents the current state of a global track
type TrackState string

const (
	TrackStateActive    TrackState = "active"    // Currently visible in a camera
	TrackStateTransit   TrackState = "transit"   // In gap between cameras
	TrackStatePending   TrackState = "pending"   // Waiting for handoff match
	TrackStateLost      TrackState = "lost"      // No match found, may recover
	TrackStateCompleted TrackState = "completed" // Track finished
)

// GlobalTrack represents a tracked object across multiple cameras
type GlobalTrack struct {
	ID                string    `json:"id"`
	FirstSeen         time.Time `json:"first_seen"`
	LastSeen          time.Time `json:"last_seen"`
	CurrentCameraID   string    `json:"current_camera_id"`
	CurrentLocalTrack string    `json:"current_local_track"` // Track ID from detection

	// Object identification
	ObjectType string `json:"object_type"` // "person", "vehicle", etc.

	// Appearance for Re-ID (stored as base64 encoded embedding)
	Embedding     []byte  `json:"embedding,omitempty"`
	EmbeddingConf float64 `json:"embedding_confidence"`

	// Visual attributes for quick matching
	DominantColors  []string `json:"dominant_colors,omitempty"`
	EstimatedHeight float64  `json:"estimated_height,omitempty"`

	// State
	State TrackState `json:"state"`

	// Predictions
	PredictedNext    string     `json:"predicted_next_camera,omitempty"`
	PredictedArrival *time.Time `json:"predicted_arrival,omitempty"`

	// History
	Path []TrackSegment `json:"path"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TrackSegment represents a portion of a track within a single camera
type TrackSegment struct {
	ID            string            `json:"id"`
	GlobalTrackID string            `json:"global_track_id"`
	CameraID      string            `json:"camera_id"`
	LocalTrackID  string            `json:"local_track_id"`
	EnteredAt     time.Time         `json:"entered_at"`
	ExitedAt      *time.Time        `json:"exited_at,omitempty"`
	ExitDirection EdgeDirection     `json:"exit_direction,omitempty"`
	ExitPosition  *Point            `json:"exit_position,omitempty"`
	BoundingBoxes []BoundingBoxSample `json:"bounding_boxes,omitempty"`
}

// BoundingBoxSample is a sampled position within a camera
type BoundingBoxSample struct {
	Timestamp time.Time `json:"timestamp"`
	X         float64   `json:"x"` // Normalized 0-1
	Y         float64   `json:"y"` // Normalized 0-1
	Width     float64   `json:"width"`
	Height    float64   `json:"height"`
}

// PendingHandoff represents an object that exited one camera and is expected in another
type PendingHandoff struct {
	ID             string         `json:"id"`
	GlobalTrackID  string         `json:"global_track_id"`
	FromCameraID   string         `json:"from_camera_id"`
	ToCameraIDs    []string       `json:"to_camera_ids"` // Possible destination cameras
	TransitionType TransitionType `json:"transition_type"`
	ExitedAt       time.Time      `json:"exited_at"`
	ExpectedBy     time.Time      `json:"expected_by"` // Deadline for match
	ExitDirection  EdgeDirection  `json:"exit_direction"`
	ExitPosition   Point          `json:"exit_position"`
	Embedding      []byte         `json:"embedding,omitempty"`
	DominantColors []string       `json:"dominant_colors,omitempty"`
}

// CalibrationSession represents an active calibration
type CalibrationSession struct {
	ID           string    `json:"id"`
	CameraID     string    `json:"camera_id"`
	StartedAt    time.Time `json:"started_at"`
	Status       string    `json:"status"` // "pending", "in_progress", "completed"
	Instructions string    `json:"instructions"`
}

// HandoffTestResult contains results from a handoff test
type HandoffTestResult struct {
	FromCameraID   string         `json:"from_camera_id"`
	ToCameraID     string         `json:"to_camera_id"`
	TransitionType TransitionType `json:"transition_type"`
	ExpectedTime   float64        `json:"expected_time_seconds"`
	Status         string         `json:"status"`
	Message        string         `json:"message"`
}

// Analytics contains aggregate tracking statistics
type Analytics struct {
	TotalTracks        int                  `json:"total_tracks"`
	ActiveTracks       int                  `json:"active_tracks"`
	TotalHandoffs      int                  `json:"total_handoffs"`
	SuccessfulHandoffs int                  `json:"successful_handoffs"`
	OverallSuccessRate float64              `json:"overall_success_rate"`
	TransitionStats    []TransitionStat     `json:"transition_stats"`
	HourlyActivity     []HourlyActivityStat `json:"hourly_activity"`
}

// MapAnalytics contains analytics for a specific map (matches frontend SpatialAnalytics type)
type MapAnalytics struct {
	ActiveTracks       int      `json:"active_tracks"`
	TotalTracks        int      `json:"total_tracks"`
	SuccessfulHandoffs int      `json:"successful_handoffs"`
	FailedHandoffs     int      `json:"failed_handoffs"`
	TotalHandoffs      int      `json:"total_handoffs,omitempty"`
	AverageTransitTime float64  `json:"average_transit_time"`
	CoverageGaps       []string `json:"coverage_gaps"`
}

// TransitionStat contains stats for a specific transition
type TransitionStat struct {
	TransitionID   string  `json:"transition_id"`
	FromCameraID   string  `json:"from_camera_id"`
	ToCameraID     string  `json:"to_camera_id"`
	Type           string  `json:"type"`
	TotalHandoffs  int     `json:"total_handoffs"`
	SuccessRate    float64 `json:"success_rate"`
	AvgTransitTime float64 `json:"avg_transit_time"`
}

// HourlyActivityStat contains hourly tracking activity
type HourlyActivityStat struct {
	Hour       int `json:"hour"`
	TrackCount int `json:"track_count"`
}

// TrackPath represents a track's journey on the spatial map
type TrackPath struct {
	TrackID   string     `json:"track_id"`
	MapID     string     `json:"map_id"`
	Waypoints []Waypoint `json:"waypoints"`
}

// Waypoint is a point along a track's path
type Waypoint struct {
	Timestamp  time.Time `json:"timestamp"`
	CameraID   string    `json:"camera_id"`
	Position   Point     `json:"position"` // Position on spatial map
	Confidence float64   `json:"confidence"`
}
