package recording

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// SQLiteRepository implements Repository using SQLite
type SQLiteRepository struct {
	db *sql.DB
}

// NewSQLiteRepository creates a new SQLite repository
func NewSQLiteRepository(db *sql.DB) *SQLiteRepository {
	return &SQLiteRepository{db: db}
}

// InitSchema initializes the recordings table
func (r *SQLiteRepository) InitSchema(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS recordings (
			id TEXT PRIMARY KEY,
			camera_id TEXT NOT NULL,
			start_time INTEGER NOT NULL,
			end_time INTEGER NOT NULL,
			duration REAL NOT NULL,
			file_path TEXT NOT NULL,
			file_size INTEGER NOT NULL DEFAULT 0,
			storage_tier TEXT NOT NULL DEFAULT 'hot',
			has_events INTEGER NOT NULL DEFAULT 0,
			event_count INTEGER NOT NULL DEFAULT 0,
			codec TEXT,
			resolution TEXT,
			bitrate INTEGER,
			thumbnail TEXT,
			checksum TEXT,
			recording_mode TEXT,
			trigger_event_id TEXT,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_recordings_camera_time ON recordings(camera_id, start_time);
		CREATE INDEX IF NOT EXISTS idx_recordings_start_time ON recordings(start_time);
		CREATE INDEX IF NOT EXISTS idx_recordings_storage_tier ON recordings(storage_tier);
		CREATE INDEX IF NOT EXISTS idx_recordings_has_events ON recordings(has_events);
	`)
	return err
}

// Create inserts a new segment
func (r *SQLiteRepository) Create(ctx context.Context, segment *Segment) error {
	if segment.ID == "" {
		segment.ID = uuid.New().String()
	}
	now := time.Now()
	segment.CreatedAt = now
	segment.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO recordings (
			id, camera_id, start_time, end_time, duration, file_path,
			file_size, storage_tier, has_events, event_count, codec,
			resolution, bitrate, thumbnail, checksum, recording_mode,
			trigger_event_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		segment.ID,
		segment.CameraID,
		segment.StartTime.Unix(),
		segment.EndTime.Unix(),
		segment.Duration,
		segment.FilePath,
		segment.FileSize,
		segment.StorageTier,
		segment.HasEvents,
		segment.EventCount,
		segment.Codec,
		segment.Resolution,
		segment.Bitrate,
		segment.Thumbnail,
		segment.Checksum,
		segment.RecordingMode,
		segment.TriggerEventID,
		segment.CreatedAt.Unix(),
		segment.UpdatedAt.Unix(),
	)
	return err
}

// Get retrieves a segment by ID
func (r *SQLiteRepository) Get(ctx context.Context, id string) (*Segment, error) {
	segment := &Segment{}
	var startTime, endTime, createdAt, updatedAt int64
	var hasEvents int
	var thumbnail, checksum, recordingMode, triggerEventID sql.NullString

	err := r.db.QueryRowContext(ctx, `
		SELECT id, camera_id, start_time, end_time, duration, file_path,
			   file_size, storage_tier, has_events, event_count, codec,
			   resolution, bitrate, thumbnail, checksum, recording_mode,
			   trigger_event_id, created_at, updated_at
		FROM recordings WHERE id = ?
	`, id).Scan(
		&segment.ID,
		&segment.CameraID,
		&startTime,
		&endTime,
		&segment.Duration,
		&segment.FilePath,
		&segment.FileSize,
		&segment.StorageTier,
		&hasEvents,
		&segment.EventCount,
		&segment.Codec,
		&segment.Resolution,
		&segment.Bitrate,
		&thumbnail,
		&checksum,
		&recordingMode,
		&triggerEventID,
		&createdAt,
		&updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("segment not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	segment.StartTime = time.Unix(startTime, 0)
	segment.EndTime = time.Unix(endTime, 0)
	segment.CreatedAt = time.Unix(createdAt, 0)
	segment.UpdatedAt = time.Unix(updatedAt, 0)
	segment.HasEvents = hasEvents == 1
	segment.Thumbnail = thumbnail.String
	segment.Checksum = checksum.String
	segment.RecordingMode = recordingMode.String
	segment.TriggerEventID = triggerEventID.String

	return segment, nil
}

// Update updates a segment
func (r *SQLiteRepository) Update(ctx context.Context, segment *Segment) error {
	segment.UpdatedAt = time.Now()

	result, err := r.db.ExecContext(ctx, `
		UPDATE recordings SET
			camera_id = ?, start_time = ?, end_time = ?, duration = ?,
			file_path = ?, file_size = ?, storage_tier = ?, has_events = ?,
			event_count = ?, codec = ?, resolution = ?, bitrate = ?,
			thumbnail = ?, checksum = ?, recording_mode = ?,
			trigger_event_id = ?, updated_at = ?
		WHERE id = ?
	`,
		segment.CameraID,
		segment.StartTime.Unix(),
		segment.EndTime.Unix(),
		segment.Duration,
		segment.FilePath,
		segment.FileSize,
		segment.StorageTier,
		segment.HasEvents,
		segment.EventCount,
		segment.Codec,
		segment.Resolution,
		segment.Bitrate,
		segment.Thumbnail,
		segment.Checksum,
		segment.RecordingMode,
		segment.TriggerEventID,
		segment.UpdatedAt.Unix(),
		segment.ID,
	)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("segment not found: %s", segment.ID)
	}
	return nil
}

// Delete removes a segment by ID
func (r *SQLiteRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM recordings WHERE id = ?", id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("segment not found: %s", id)
	}
	return nil
}

// List retrieves segments with filtering and pagination
func (r *SQLiteRepository) List(ctx context.Context, opts ListOptions) ([]Segment, int, error) {
	var conditions []string
	var args []interface{}

	if opts.CameraID != "" {
		conditions = append(conditions, "camera_id = ?")
		args = append(args, opts.CameraID)
	}
	if opts.StartTime != nil {
		conditions = append(conditions, "start_time >= ?")
		args = append(args, opts.StartTime.Unix())
	}
	if opts.EndTime != nil {
		conditions = append(conditions, "end_time <= ?")
		args = append(args, opts.EndTime.Unix())
	}
	if opts.HasEvents != nil {
		conditions = append(conditions, "has_events = ?")
		if *opts.HasEvents {
			args = append(args, 1)
		} else {
			args = append(args, 0)
		}
	}
	if opts.Tier != nil {
		conditions = append(conditions, "storage_tier = ?")
		args = append(args, *opts.Tier)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Get total count
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM recordings %s", whereClause)
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Build order clause
	orderBy := "start_time"
	if opts.OrderBy != "" {
		switch opts.OrderBy {
		case "start_time", "end_time", "file_size", "duration":
			orderBy = opts.OrderBy
		}
	}
	orderDir := "ASC"
	if opts.OrderDesc {
		orderDir = "DESC"
	}

	// Build query with pagination
	query := fmt.Sprintf(`
		SELECT id, camera_id, start_time, end_time, duration, file_path,
			   file_size, storage_tier, has_events, event_count, codec,
			   resolution, bitrate, thumbnail, checksum, recording_mode,
			   trigger_event_id, created_at, updated_at
		FROM recordings %s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereClause, orderBy, orderDir)

	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	args = append(args, limit, opts.Offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var segments []Segment
	for rows.Next() {
		var seg Segment
		var startTime, endTime, createdAt, updatedAt int64
		var hasEvents int
		var thumbnail, checksum, recordingMode, triggerEventID sql.NullString

		if err := rows.Scan(
			&seg.ID,
			&seg.CameraID,
			&startTime,
			&endTime,
			&seg.Duration,
			&seg.FilePath,
			&seg.FileSize,
			&seg.StorageTier,
			&hasEvents,
			&seg.EventCount,
			&seg.Codec,
			&seg.Resolution,
			&seg.Bitrate,
			&thumbnail,
			&checksum,
			&recordingMode,
			&triggerEventID,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, 0, err
		}

		seg.StartTime = time.Unix(startTime, 0)
		seg.EndTime = time.Unix(endTime, 0)
		seg.CreatedAt = time.Unix(createdAt, 0)
		seg.UpdatedAt = time.Unix(updatedAt, 0)
		seg.HasEvents = hasEvents == 1
		seg.Thumbnail = thumbnail.String
		seg.Checksum = checksum.String
		seg.RecordingMode = recordingMode.String
		seg.TriggerEventID = triggerEventID.String

		segments = append(segments, seg)
	}

	return segments, total, rows.Err()
}

// DeleteBefore deletes segments before a given time for a camera
func (r *SQLiteRepository) DeleteBefore(ctx context.Context, cameraID string, before time.Time) (int, error) {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM recordings WHERE camera_id = ? AND end_time < ?
	`, cameraID, before.Unix())
	if err != nil {
		return 0, err
	}

	rows, _ := result.RowsAffected()
	return int(rows), nil
}

// UpdateTier updates the storage tier for multiple segments
func (r *SQLiteRepository) UpdateTier(ctx context.Context, ids []string, tier StorageTier) error {
	if len(ids) == 0 {
		return nil
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids)+2)
	args[0] = tier
	args[1] = time.Now().Unix()
	for i, id := range ids {
		placeholders[i] = "?"
		args[i+2] = id
	}

	query := fmt.Sprintf(`
		UPDATE recordings SET storage_tier = ?, updated_at = ?
		WHERE id IN (%s)
	`, strings.Join(placeholders, ","))

	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

// GetByTimeRange retrieves segments within a time range
func (r *SQLiteRepository) GetByTimeRange(ctx context.Context, cameraID string, start, end time.Time) ([]Segment, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, camera_id, start_time, end_time, duration, file_path,
			   file_size, storage_tier, has_events, event_count, codec,
			   resolution, bitrate, thumbnail, checksum, recording_mode,
			   trigger_event_id, created_at, updated_at
		FROM recordings
		WHERE camera_id = ? AND start_time < ? AND end_time > ?
		ORDER BY start_time ASC
	`, cameraID, end.Unix(), start.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanSegments(rows)
}

// GetOldestSegments retrieves the oldest segments for a camera
func (r *SQLiteRepository) GetOldestSegments(ctx context.Context, cameraID string, limit int) ([]Segment, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, camera_id, start_time, end_time, duration, file_path,
			   file_size, storage_tier, has_events, event_count, codec,
			   resolution, bitrate, thumbnail, checksum, recording_mode,
			   trigger_event_id, created_at, updated_at
		FROM recordings
		WHERE camera_id = ?
		ORDER BY start_time ASC
		LIMIT ?
	`, cameraID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanSegments(rows)
}

// GetTotalSize returns the total size of segments for a camera
func (r *SQLiteRepository) GetTotalSize(ctx context.Context, cameraID string) (int64, error) {
	var total sql.NullInt64
	err := r.db.QueryRowContext(ctx, `
		SELECT SUM(file_size) FROM recordings WHERE camera_id = ?
	`, cameraID).Scan(&total)
	return total.Int64, err
}

// GetSegmentCount returns the number of segments for a camera
func (r *SQLiteRepository) GetSegmentCount(ctx context.Context, cameraID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM recordings WHERE camera_id = ?
	`, cameraID).Scan(&count)
	return count, err
}

// GetStorageByCamera returns total storage used by each camera
func (r *SQLiteRepository) GetStorageByCamera(ctx context.Context) (map[string]int64, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT camera_id, SUM(file_size) as total_size
		FROM recordings
		GROUP BY camera_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int64)
	for rows.Next() {
		var cameraID string
		var totalSize int64
		if err := rows.Scan(&cameraID, &totalSize); err != nil {
			return nil, err
		}
		result[cameraID] = totalSize
	}
	return result, rows.Err()
}

// GetStorageByTier returns total storage used by each tier
func (r *SQLiteRepository) GetStorageByTier(ctx context.Context) (map[StorageTier]int64, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT storage_tier, SUM(file_size) as total_size
		FROM recordings
		GROUP BY storage_tier
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[StorageTier]int64)
	for rows.Next() {
		var tier StorageTier
		var totalSize int64
		if err := rows.Scan(&tier, &totalSize); err != nil {
			return nil, err
		}
		result[tier] = totalSize
	}
	return result, rows.Err()
}

// scanSegments is a helper to scan multiple segments from rows
func (r *SQLiteRepository) scanSegments(rows *sql.Rows) ([]Segment, error) {
	var segments []Segment
	for rows.Next() {
		var seg Segment
		var startTime, endTime, createdAt, updatedAt int64
		var hasEvents int
		var thumbnail, checksum, recordingMode, triggerEventID sql.NullString

		if err := rows.Scan(
			&seg.ID,
			&seg.CameraID,
			&startTime,
			&endTime,
			&seg.Duration,
			&seg.FilePath,
			&seg.FileSize,
			&seg.StorageTier,
			&hasEvents,
			&seg.EventCount,
			&seg.Codec,
			&seg.Resolution,
			&seg.Bitrate,
			&thumbnail,
			&checksum,
			&recordingMode,
			&triggerEventID,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, err
		}

		seg.StartTime = time.Unix(startTime, 0)
		seg.EndTime = time.Unix(endTime, 0)
		seg.CreatedAt = time.Unix(createdAt, 0)
		seg.UpdatedAt = time.Unix(updatedAt, 0)
		seg.HasEvents = hasEvents == 1
		seg.Thumbnail = thumbnail.String
		seg.Checksum = checksum.String
		seg.RecordingMode = recordingMode.String
		seg.TriggerEventID = triggerEventID.String

		segments = append(segments, seg)
	}
	return segments, rows.Err()
}
