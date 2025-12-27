package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/Spatial-NVR/SpatialNVR/internal/database"
)

// Service manages events
type Service struct {
	db          *database.DB
	logger      *slog.Logger
	subscribers []chan *Event
	mu          sync.RWMutex
}

// NewService creates a new event service
func NewService(db *database.DB) *Service {
	return &Service{
		db:          db,
		logger:      slog.Default().With("component", "event_service"),
		subscribers: make([]chan *Event, 0),
	}
}

// Subscribe returns a channel that receives new events
func (s *Service) Subscribe() chan *Event {
	ch := make(chan *Event, 100)
	s.mu.Lock()
	s.subscribers = append(s.subscribers, ch)
	s.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscription
func (s *Service) Unsubscribe(ch chan *Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, sub := range s.subscribers {
		if sub == ch {
			s.subscribers = append(s.subscribers[:i], s.subscribers[i+1:]...)
			close(ch)
			break
		}
	}
}

// Create creates a new event
func (s *Service) Create(ctx context.Context, event *Event) error {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	var tagsJSON, metadataJSON []byte
	var err error

	if len(event.Tags) > 0 {
		tagsJSON, err = json.Marshal(event.Tags)
		if err != nil {
			return fmt.Errorf("failed to marshal tags: %w", err)
		}
	}

	if event.Metadata != nil {
		metadataJSON = event.Metadata
	}

	var endTimestamp *int64
	if event.EndTimestamp != nil {
		ts := event.EndTimestamp.Unix()
		endTimestamp = &ts
	}

	acknowledged := 0
	if event.Acknowledged {
		acknowledged = 1
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO events (
			id, camera_id, event_type, label, timestamp, end_timestamp,
			confidence, thumbnail_path, video_segment_id, metadata,
			acknowledged, tags, notes, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		event.ID, event.CameraID, event.EventType, event.Label, event.Timestamp.Unix(), endTimestamp,
		event.Confidence, event.ThumbnailPath, event.VideoSegmentID, metadataJSON,
		acknowledged, tagsJSON, event.Notes, event.CreatedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to create event: %w", err)
	}

	// Notify subscribers
	s.notifySubscribers(event)

	s.logger.Info("Event created", "id", event.ID, "type", event.EventType, "camera", event.CameraID)
	return nil
}

// Get retrieves an event by ID
func (s *Service) Get(ctx context.Context, id string) (*Event, error) {
	event := &Event{}
	var timestamp, createdAt int64
	var endTimestamp sql.NullInt64
	var confidence sql.NullFloat64
	var label, thumbnailPath, videoSegmentID, notes sql.NullString
	var metadataJSON, tagsJSON sql.NullString
	var acknowledged int

	err := s.db.QueryRowContext(ctx, `
		SELECT id, camera_id, event_type, label, timestamp, end_timestamp,
		       confidence, thumbnail_path, video_segment_id, metadata,
		       acknowledged, tags, notes, created_at
		FROM events WHERE id = ?
	`, id).Scan(
		&event.ID, &event.CameraID, &event.EventType, &label, &timestamp, &endTimestamp,
		&confidence, &thumbnailPath, &videoSegmentID, &metadataJSON,
		&acknowledged, &tagsJSON, &notes, &createdAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("event not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	event.Timestamp = time.Unix(timestamp, 0)
	event.CreatedAt = time.Unix(createdAt, 0)
	event.Acknowledged = acknowledged == 1

	if endTimestamp.Valid {
		t := time.Unix(endTimestamp.Int64, 0)
		event.EndTimestamp = &t
	}
	if confidence.Valid {
		event.Confidence = confidence.Float64
	}
	if label.Valid {
		event.Label = label.String
	}
	if thumbnailPath.Valid {
		event.ThumbnailPath = thumbnailPath.String
	}
	if videoSegmentID.Valid {
		event.VideoSegmentID = videoSegmentID.String
	}
	if notes.Valid {
		event.Notes = notes.String
	}
	if metadataJSON.Valid {
		event.Metadata = json.RawMessage(metadataJSON.String)
	}
	if tagsJSON.Valid {
		_ = json.Unmarshal([]byte(tagsJSON.String), &event.Tags)
	}

	return event, nil
}

// List retrieves events with filters
func (s *Service) List(ctx context.Context, opts ListOptions) ([]*Event, int, error) {
	// Build query
	query := `SELECT id, camera_id, event_type, label, timestamp, end_timestamp,
	                 confidence, thumbnail_path, video_segment_id, metadata,
	                 acknowledged, tags, notes, created_at
	          FROM events WHERE 1=1`
	countQuery := `SELECT COUNT(*) FROM events WHERE 1=1`
	args := []interface{}{}

	if opts.CameraID != "" {
		query += " AND camera_id = ?"
		countQuery += " AND camera_id = ?"
		args = append(args, opts.CameraID)
	}

	if opts.EventType != "" {
		query += " AND event_type = ?"
		countQuery += " AND event_type = ?"
		args = append(args, opts.EventType)
	}

	if !opts.StartTime.IsZero() {
		query += " AND timestamp >= ?"
		countQuery += " AND timestamp >= ?"
		args = append(args, opts.StartTime.Unix())
	}

	if !opts.EndTime.IsZero() {
		query += " AND timestamp <= ?"
		countQuery += " AND timestamp <= ?"
		args = append(args, opts.EndTime.Unix())
	}

	// Get total count
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	var totalCount int
	if err := s.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&totalCount); err != nil {
		return nil, 0, err
	}

	// Add ordering and pagination
	query += " ORDER BY timestamp DESC"

	limit := 50
	if opts.Limit > 0 && opts.Limit <= 1000 {
		limit = opts.Limit
	}
	query += " LIMIT ?"
	args = append(args, limit)

	if opts.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, opts.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	events := []*Event{}
	for rows.Next() {
		event := &Event{}
		var timestamp, createdAt int64
		var endTimestamp sql.NullInt64
		var confidence sql.NullFloat64
		var label, thumbnailPath, videoSegmentID, notes sql.NullString
		var metadataJSON, tagsJSON sql.NullString
		var acknowledged int

		if err := rows.Scan(
			&event.ID, &event.CameraID, &event.EventType, &label, &timestamp, &endTimestamp,
			&confidence, &thumbnailPath, &videoSegmentID, &metadataJSON,
			&acknowledged, &tagsJSON, &notes, &createdAt,
		); err != nil {
			return nil, 0, err
		}

		event.Timestamp = time.Unix(timestamp, 0)
		event.CreatedAt = time.Unix(createdAt, 0)
		event.Acknowledged = acknowledged == 1

		if endTimestamp.Valid {
			t := time.Unix(endTimestamp.Int64, 0)
			event.EndTimestamp = &t
		}
		if confidence.Valid {
			event.Confidence = confidence.Float64
		}
		if label.Valid {
			event.Label = label.String
		}
		if thumbnailPath.Valid {
			event.ThumbnailPath = thumbnailPath.String
		}
		if videoSegmentID.Valid {
			event.VideoSegmentID = videoSegmentID.String
		}
		if notes.Valid {
			event.Notes = notes.String
		}
		if metadataJSON.Valid {
			event.Metadata = json.RawMessage(metadataJSON.String)
		}
		if tagsJSON.Valid {
			_ = json.Unmarshal([]byte(tagsJSON.String), &event.Tags)
		}

		events = append(events, event)
	}

	return events, totalCount, rows.Err()
}

// Acknowledge acknowledges an event
func (s *Service) Acknowledge(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE events SET acknowledged = 1 WHERE id = ?", id)
	return err
}

// Delete deletes an event
func (s *Service) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM events WHERE id = ?", id)
	return err
}

// GetStats returns event statistics
func (s *Service) GetStats(ctx context.Context, cameraID string) (map[string]interface{}, error) {
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	var today, unacknowledged, total int

	// Count today's events
	query := "SELECT COUNT(*) FROM events WHERE timestamp >= ?"
	args := []interface{}{todayStart.Unix()}
	if cameraID != "" {
		query += " AND camera_id = ?"
		args = append(args, cameraID)
	}
	_ = s.db.QueryRowContext(ctx, query, args...).Scan(&today)

	// Count unacknowledged
	query = "SELECT COUNT(*) FROM events WHERE acknowledged = 0"
	args = []interface{}{}
	if cameraID != "" {
		query += " AND camera_id = ?"
		args = append(args, cameraID)
	}
	_ = s.db.QueryRowContext(ctx, query, args...).Scan(&unacknowledged)

	// Count total
	query = "SELECT COUNT(*) FROM events"
	args = []interface{}{}
	if cameraID != "" {
		query += " WHERE camera_id = ?"
		args = append(args, cameraID)
	}
	_ = s.db.QueryRowContext(ctx, query, args...).Scan(&total)

	return map[string]interface{}{
		"today":          today,
		"unacknowledged": unacknowledged,
		"total":          total,
	}, nil
}

// CreateMotionEvent creates a motion event
func (s *Service) CreateMotionEvent(ctx context.Context, cameraID string, confidence float64, thumbnail string) (*Event, error) {
	event := &Event{
		CameraID:      cameraID,
		EventType:     EventMotion,
		Label:         "motion",
		Confidence:    confidence,
		ThumbnailPath: thumbnail,
		Timestamp:     time.Now(),
	}
	if err := s.Create(ctx, event); err != nil {
		return nil, err
	}
	return event, nil
}

// CreateDoorbellEvent creates a doorbell ring event
func (s *Service) CreateDoorbellEvent(ctx context.Context, cameraID string, thumbnail string) (*Event, error) {
	event := &Event{
		CameraID:      cameraID,
		EventType:     EventDoorbell,
		Label:         "doorbell",
		Confidence:    1.0,
		ThumbnailPath: thumbnail,
		Timestamp:     time.Now(),
	}
	if err := s.Create(ctx, event); err != nil {
		return nil, err
	}
	return event, nil
}

// CreateAudioEvent creates an audio detection event
func (s *Service) CreateAudioEvent(ctx context.Context, cameraID, audioType string, confidence float64) (*Event, error) {
	metadata, _ := json.Marshal(map[string]interface{}{"audio_type": audioType})
	event := &Event{
		CameraID:   cameraID,
		EventType:  EventAudio,
		Label:      audioType,
		Confidence: confidence,
		Metadata:   metadata,
		Timestamp:  time.Now(),
	}
	if err := s.Create(ctx, event); err != nil {
		return nil, err
	}
	return event, nil
}

func (s *Service) notifySubscribers(event *Event) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, ch := range s.subscribers {
		select {
		case ch <- event:
		default:
		}
	}
}

// ====================
// Motion Zone Methods
// ====================

// CreateZone creates a new motion zone
func (s *Service) CreateZone(ctx context.Context, zone *MotionZone) error {
	if zone.ID == "" {
		zone.ID = uuid.New().String()
	}
	now := time.Now()
	zone.CreatedAt = now
	zone.UpdatedAt = now

	// Serialize points and object_types to JSON
	pointsJSON, err := json.Marshal(zone.Points)
	if err != nil {
		return fmt.Errorf("failed to marshal points: %w", err)
	}

	var objectTypesJSON []byte
	if len(zone.ObjectTypes) > 0 {
		objectTypesJSON, err = json.Marshal(zone.ObjectTypes)
		if err != nil {
			return fmt.Errorf("failed to marshal object types: %w", err)
		}
	}

	enabled := 0
	if zone.Enabled {
		enabled = 1
	}
	notifications := 0
	if zone.Notifications {
		notifications = 1
	}
	recording := 0
	if zone.Recording {
		recording = 1
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO motion_zones (
			id, camera_id, name, enabled, points, object_types,
			min_confidence, min_size, sensitivity, cooldown_seconds,
			notifications, recording, color, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		zone.ID, zone.CameraID, zone.Name, enabled, pointsJSON, objectTypesJSON,
		zone.MinConfidence, zone.MinSize, zone.Sensitivity, zone.Cooldown,
		notifications, recording, zone.Color, zone.CreatedAt.Unix(), zone.UpdatedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to create zone: %w", err)
	}

	s.logger.Info("Zone created", "id", zone.ID, "camera", zone.CameraID, "name", zone.Name)
	return nil
}

// GetZone retrieves a zone by ID
func (s *Service) GetZone(ctx context.Context, id string) (*MotionZone, error) {
	zone := &MotionZone{}
	var enabled, notifications, recording int
	var createdAt, updatedAt int64
	var pointsJSON, objectTypesJSON, color sql.NullString
	var minSize sql.NullFloat64

	err := s.db.QueryRowContext(ctx, `
		SELECT id, camera_id, name, enabled, points, object_types,
		       min_confidence, min_size, sensitivity, cooldown_seconds,
		       notifications, recording, color, created_at, updated_at
		FROM motion_zones WHERE id = ?
	`, id).Scan(
		&zone.ID, &zone.CameraID, &zone.Name, &enabled, &pointsJSON, &objectTypesJSON,
		&zone.MinConfidence, &minSize, &zone.Sensitivity, &zone.Cooldown,
		&notifications, &recording, &color, &createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("zone not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	zone.Enabled = enabled == 1
	zone.Notifications = notifications == 1
	zone.Recording = recording == 1
	zone.CreatedAt = time.Unix(createdAt, 0)
	zone.UpdatedAt = time.Unix(updatedAt, 0)

	if minSize.Valid {
		zone.MinSize = minSize.Float64
	}
	if color.Valid {
		zone.Color = color.String
	}
	if pointsJSON.Valid {
		_ = json.Unmarshal([]byte(pointsJSON.String), &zone.Points)
	}
	if objectTypesJSON.Valid {
		_ = json.Unmarshal([]byte(objectTypesJSON.String), &zone.ObjectTypes)
	}

	return zone, nil
}

// ListZones retrieves zones for a camera
func (s *Service) ListZones(ctx context.Context, cameraID string) ([]*MotionZone, error) {
	query := `SELECT id, camera_id, name, enabled, points, object_types,
	                 min_confidence, min_size, sensitivity, cooldown_seconds,
	                 notifications, recording, color, created_at, updated_at
	          FROM motion_zones`
	args := []interface{}{}

	if cameraID != "" {
		query += " WHERE camera_id = ?"
		args = append(args, cameraID)
	}
	query += " ORDER BY name ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	zones := []*MotionZone{}
	for rows.Next() {
		zone := &MotionZone{}
		var enabled, notifications, recording int
		var createdAt, updatedAt int64
		var pointsJSON, objectTypesJSON, color sql.NullString
		var minSize sql.NullFloat64

		if err := rows.Scan(
			&zone.ID, &zone.CameraID, &zone.Name, &enabled, &pointsJSON, &objectTypesJSON,
			&zone.MinConfidence, &minSize, &zone.Sensitivity, &zone.Cooldown,
			&notifications, &recording, &color, &createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}

		zone.Enabled = enabled == 1
		zone.Notifications = notifications == 1
		zone.Recording = recording == 1
		zone.CreatedAt = time.Unix(createdAt, 0)
		zone.UpdatedAt = time.Unix(updatedAt, 0)

		if minSize.Valid {
			zone.MinSize = minSize.Float64
		}
		if color.Valid {
			zone.Color = color.String
		}
		if pointsJSON.Valid {
			_ = json.Unmarshal([]byte(pointsJSON.String), &zone.Points)
		}
		if objectTypesJSON.Valid {
			_ = json.Unmarshal([]byte(objectTypesJSON.String), &zone.ObjectTypes)
		}

		zones = append(zones, zone)
	}

	return zones, rows.Err()
}

// UpdateZone updates an existing zone
func (s *Service) UpdateZone(ctx context.Context, zone *MotionZone) error {
	zone.UpdatedAt = time.Now()

	pointsJSON, err := json.Marshal(zone.Points)
	if err != nil {
		return fmt.Errorf("failed to marshal points: %w", err)
	}

	var objectTypesJSON []byte
	if len(zone.ObjectTypes) > 0 {
		objectTypesJSON, err = json.Marshal(zone.ObjectTypes)
		if err != nil {
			return fmt.Errorf("failed to marshal object types: %w", err)
		}
	}

	enabled := 0
	if zone.Enabled {
		enabled = 1
	}
	notifications := 0
	if zone.Notifications {
		notifications = 1
	}
	recording := 0
	if zone.Recording {
		recording = 1
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE motion_zones SET
			name = ?, enabled = ?, points = ?, object_types = ?,
			min_confidence = ?, min_size = ?, sensitivity = ?, cooldown_seconds = ?,
			notifications = ?, recording = ?, color = ?, updated_at = ?
		WHERE id = ?
	`,
		zone.Name, enabled, pointsJSON, objectTypesJSON,
		zone.MinConfidence, zone.MinSize, zone.Sensitivity, zone.Cooldown,
		notifications, recording, zone.Color, zone.UpdatedAt.Unix(), zone.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update zone: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("zone not found: %s", zone.ID)
	}

	s.logger.Info("Zone updated", "id", zone.ID)
	return nil
}

// DeleteZone deletes a zone
func (s *Service) DeleteZone(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM motion_zones WHERE id = ?", id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("zone not found: %s", id)
	}

	s.logger.Info("Zone deleted", "id", id)
	return nil
}

// GetEnabledZonesForCamera returns all enabled zones for a camera
func (s *Service) GetEnabledZonesForCamera(ctx context.Context, cameraID string) ([]*MotionZone, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, camera_id, name, enabled, points, object_types,
		       min_confidence, min_size, sensitivity, cooldown_seconds,
		       notifications, recording, color, created_at, updated_at
		FROM motion_zones
		WHERE camera_id = ? AND enabled = 1
		ORDER BY name ASC
	`, cameraID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	zones := []*MotionZone{}
	for rows.Next() {
		zone := &MotionZone{}
		var enabled, notifications, recording int
		var createdAt, updatedAt int64
		var pointsJSON, objectTypesJSON, color sql.NullString
		var minSize sql.NullFloat64

		if err := rows.Scan(
			&zone.ID, &zone.CameraID, &zone.Name, &enabled, &pointsJSON, &objectTypesJSON,
			&zone.MinConfidence, &minSize, &zone.Sensitivity, &zone.Cooldown,
			&notifications, &recording, &color, &createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}

		zone.Enabled = enabled == 1
		zone.Notifications = notifications == 1
		zone.Recording = recording == 1
		zone.CreatedAt = time.Unix(createdAt, 0)
		zone.UpdatedAt = time.Unix(updatedAt, 0)

		if minSize.Valid {
			zone.MinSize = minSize.Float64
		}
		if color.Valid {
			zone.Color = color.String
		}
		if pointsJSON.Valid {
			_ = json.Unmarshal([]byte(pointsJSON.String), &zone.Points)
		}
		if objectTypesJSON.Valid {
			_ = json.Unmarshal([]byte(objectTypesJSON.String), &zone.ObjectTypes)
		}

		zones = append(zones, zone)
	}

	return zones, rows.Err()
}
