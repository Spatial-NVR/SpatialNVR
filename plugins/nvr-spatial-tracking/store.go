package nvrspatialtracking

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Store handles database operations for spatial tracking
type Store struct {
	db       *sql.DB
	dataPath string // Path for storing images
}

// NewStore creates a new Store instance
func NewStore(db *sql.DB) *Store {
	return &Store{
		db:       db,
		dataPath: "/tmp/nvr-spatial-data", // Default path
	}
}

// SetDataPath sets the path for storing images and other data
func (s *Store) SetDataPath(path string) {
	s.dataPath = path
}

// Migrate creates the database schema
func (s *Store) Migrate(ctx context.Context) error {
	migrations := []string{
		// Spatial maps table
		`CREATE TABLE IF NOT EXISTS spatial_maps (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			image_url TEXT,
			width REAL NOT NULL,
			height REAL NOT NULL,
			scale REAL NOT NULL DEFAULT 1.0,
			metadata_json TEXT,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,

		// Camera placements table
		`CREATE TABLE IF NOT EXISTS camera_placements (
			id TEXT PRIMARY KEY,
			camera_id TEXT NOT NULL,
			map_id TEXT NOT NULL REFERENCES spatial_maps(id) ON DELETE CASCADE,
			position_x REAL NOT NULL,
			position_y REAL NOT NULL,
			rotation REAL NOT NULL DEFAULT 0,
			fov_angle REAL NOT NULL DEFAULT 90,
			fov_depth REAL NOT NULL DEFAULT 100,
			coverage_polygon_json TEXT,
			mount_height REAL,
			tilt_angle REAL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(camera_id, map_id)
		)`,

		// Camera transitions table
		`CREATE TABLE IF NOT EXISTS camera_transitions (
			id TEXT PRIMARY KEY,
			from_camera_id TEXT NOT NULL,
			to_camera_id TEXT NOT NULL,
			transition_type TEXT NOT NULL CHECK(transition_type IN ('overlap', 'adjacent', 'gap')),
			bidirectional INTEGER NOT NULL DEFAULT 1,
			overlap_zone_json TEXT,
			expected_transit_time REAL,
			transit_time_variance REAL,
			exit_zone_json TEXT,
			entry_zone_json TEXT,
			avg_transit_time REAL,
			success_rate REAL,
			total_handoffs INTEGER NOT NULL DEFAULT 0,
			successful_handoffs INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(from_camera_id, to_camera_id)
		)`,

		// Global tracks table
		`CREATE TABLE IF NOT EXISTS global_tracks (
			id TEXT PRIMARY KEY,
			first_seen TIMESTAMP NOT NULL,
			last_seen TIMESTAMP NOT NULL,
			current_camera_id TEXT,
			current_local_track TEXT,
			object_type TEXT NOT NULL,
			embedding BLOB,
			embedding_confidence REAL,
			dominant_colors_json TEXT,
			estimated_height REAL,
			state TEXT NOT NULL CHECK(state IN ('active', 'transit', 'pending', 'lost', 'completed')),
			predicted_next_camera TEXT,
			predicted_arrival TIMESTAMP,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,

		// Track segments table
		`CREATE TABLE IF NOT EXISTS track_segments (
			id TEXT PRIMARY KEY,
			global_track_id TEXT NOT NULL REFERENCES global_tracks(id) ON DELETE CASCADE,
			camera_id TEXT NOT NULL,
			local_track_id TEXT NOT NULL,
			entered_at TIMESTAMP NOT NULL,
			exited_at TIMESTAMP,
			exit_direction TEXT,
			exit_position_x REAL,
			exit_position_y REAL,
			bounding_boxes_json TEXT,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,

		// Pending handoffs table
		`CREATE TABLE IF NOT EXISTS pending_handoffs (
			id TEXT PRIMARY KEY,
			global_track_id TEXT NOT NULL REFERENCES global_tracks(id) ON DELETE CASCADE,
			from_camera_id TEXT NOT NULL,
			to_camera_ids_json TEXT NOT NULL,
			transition_type TEXT NOT NULL,
			exited_at TIMESTAMP NOT NULL,
			expected_by TIMESTAMP NOT NULL,
			exit_direction TEXT,
			exit_position_x REAL,
			exit_position_y REAL,
			embedding BLOB,
			dominant_colors_json TEXT,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,

		// Indexes for performance
		`CREATE INDEX IF NOT EXISTS idx_camera_placements_map_id ON camera_placements(map_id)`,
		`CREATE INDEX IF NOT EXISTS idx_camera_placements_camera_id ON camera_placements(camera_id)`,
		`CREATE INDEX IF NOT EXISTS idx_camera_transitions_from ON camera_transitions(from_camera_id)`,
		`CREATE INDEX IF NOT EXISTS idx_camera_transitions_to ON camera_transitions(to_camera_id)`,
		`CREATE INDEX IF NOT EXISTS idx_global_tracks_state ON global_tracks(state)`,
		`CREATE INDEX IF NOT EXISTS idx_global_tracks_current_camera ON global_tracks(current_camera_id)`,
		`CREATE INDEX IF NOT EXISTS idx_track_segments_global_track ON track_segments(global_track_id)`,
		`CREATE INDEX IF NOT EXISTS idx_track_segments_camera ON track_segments(camera_id)`,
		`CREATE INDEX IF NOT EXISTS idx_pending_handoffs_expected_by ON pending_handoffs(expected_by)`,
	}

	for _, migration := range migrations {
		if _, err := s.db.Exec(migration); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	return nil
}

// ============================================================================
// Spatial Map CRUD
// ============================================================================

// CreateMap creates a new spatial map
func (s *Store) CreateMap(m *SpatialMap) error {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	m.CreatedAt = time.Now()
	m.UpdatedAt = m.CreatedAt

	metaJSON, err := json.Marshal(m.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO spatial_maps (id, name, image_url, width, height, scale, metadata_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, m.ID, m.Name, m.ImageURL, m.Width, m.Height, m.Scale, string(metaJSON), m.CreatedAt, m.UpdatedAt)

	return err
}

// GetMap retrieves a spatial map by ID
func (s *Store) GetMap(id string) (*SpatialMap, error) {
	var m SpatialMap
	var metaJSON string

	err := s.db.QueryRow(`
		SELECT id, name, image_url, width, height, scale, metadata_json, created_at, updated_at
		FROM spatial_maps WHERE id = ?
	`, id).Scan(&m.ID, &m.Name, &m.ImageURL, &m.Width, &m.Height, &m.Scale, &metaJSON, &m.CreatedAt, &m.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if metaJSON != "" {
		if err := json.Unmarshal([]byte(metaJSON), &m.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &m, nil
}

// ListMaps returns all spatial maps
func (s *Store) ListMaps() ([]SpatialMap, error) {
	rows, err := s.db.Query(`
		SELECT id, name, image_url, width, height, scale, metadata_json, created_at, updated_at
		FROM spatial_maps ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var maps []SpatialMap
	for rows.Next() {
		var m SpatialMap
		var metaJSON string

		if err := rows.Scan(&m.ID, &m.Name, &m.ImageURL, &m.Width, &m.Height, &m.Scale, &metaJSON, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}

		if metaJSON != "" {
			if err := json.Unmarshal([]byte(metaJSON), &m.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		maps = append(maps, m)
	}

	return maps, rows.Err()
}

// UpdateMap updates a spatial map
func (s *Store) UpdateMap(m *SpatialMap) error {
	m.UpdatedAt = time.Now()

	metaJSON, err := json.Marshal(m.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	result, err := s.db.Exec(`
		UPDATE spatial_maps SET name = ?, image_url = ?, width = ?, height = ?, scale = ?, metadata_json = ?, updated_at = ?
		WHERE id = ?
	`, m.Name, m.ImageURL, m.Width, m.Height, m.Scale, string(metaJSON), m.UpdatedAt, m.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errors.New("map not found")
	}

	return nil
}

// DeleteMap deletes a spatial map
func (s *Store) DeleteMap(id string) error {
	result, err := s.db.Exec(`DELETE FROM spatial_maps WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errors.New("map not found")
	}

	return nil
}

// ============================================================================
// Camera Placement CRUD
// ============================================================================

// CreatePlacement creates a new camera placement
func (s *Store) CreatePlacement(cp *CameraPlacement) error {
	if cp.ID == "" {
		cp.ID = uuid.New().String()
	}
	cp.CreatedAt = time.Now()
	cp.UpdatedAt = cp.CreatedAt

	coverageJSON, err := json.Marshal(cp.CoveragePolygon)
	if err != nil {
		return fmt.Errorf("failed to marshal coverage polygon: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO camera_placements (id, camera_id, map_id, position_x, position_y, rotation, fov_angle, fov_depth, coverage_polygon_json, mount_height, tilt_angle, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, cp.ID, cp.CameraID, cp.MapID, cp.Position.X, cp.Position.Y, cp.Rotation, cp.FOVAngle, cp.FOVDepth, string(coverageJSON), cp.MountHeight, cp.TiltAngle, cp.CreatedAt, cp.UpdatedAt)

	return err
}

// GetPlacement retrieves a camera placement by ID
func (s *Store) GetPlacement(id string) (*CameraPlacement, error) {
	var cp CameraPlacement
	var coverageJSON string
	var mountHeight, tiltAngle sql.NullFloat64

	err := s.db.QueryRow(`
		SELECT id, camera_id, map_id, position_x, position_y, rotation, fov_angle, fov_depth, coverage_polygon_json, mount_height, tilt_angle, created_at, updated_at
		FROM camera_placements WHERE id = ?
	`, id).Scan(&cp.ID, &cp.CameraID, &cp.MapID, &cp.Position.X, &cp.Position.Y, &cp.Rotation, &cp.FOVAngle, &cp.FOVDepth, &coverageJSON, &mountHeight, &tiltAngle, &cp.CreatedAt, &cp.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if coverageJSON != "" {
		if err := json.Unmarshal([]byte(coverageJSON), &cp.CoveragePolygon); err != nil {
			return nil, fmt.Errorf("failed to unmarshal coverage polygon: %w", err)
		}
	}

	if mountHeight.Valid {
		cp.MountHeight = mountHeight.Float64
	}
	if tiltAngle.Valid {
		cp.TiltAngle = tiltAngle.Float64
	}

	return &cp, nil
}

// GetPlacementByCameraID retrieves a camera placement by camera ID
func (s *Store) GetPlacementByCameraID(cameraID string) (*CameraPlacement, error) {
	var cp CameraPlacement
	var coverageJSON string
	var mountHeight, tiltAngle sql.NullFloat64

	err := s.db.QueryRow(`
		SELECT id, camera_id, map_id, position_x, position_y, rotation, fov_angle, fov_depth, coverage_polygon_json, mount_height, tilt_angle, created_at, updated_at
		FROM camera_placements WHERE camera_id = ?
	`, cameraID).Scan(&cp.ID, &cp.CameraID, &cp.MapID, &cp.Position.X, &cp.Position.Y, &cp.Rotation, &cp.FOVAngle, &cp.FOVDepth, &coverageJSON, &mountHeight, &tiltAngle, &cp.CreatedAt, &cp.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if coverageJSON != "" {
		if err := json.Unmarshal([]byte(coverageJSON), &cp.CoveragePolygon); err != nil {
			return nil, fmt.Errorf("failed to unmarshal coverage polygon: %w", err)
		}
	}

	if mountHeight.Valid {
		cp.MountHeight = mountHeight.Float64
	}
	if tiltAngle.Valid {
		cp.TiltAngle = tiltAngle.Float64
	}

	return &cp, nil
}

// ListPlacementsByMap returns all camera placements for a map
func (s *Store) ListPlacementsByMap(mapID string) ([]CameraPlacement, error) {
	rows, err := s.db.Query(`
		SELECT id, camera_id, map_id, position_x, position_y, rotation, fov_angle, fov_depth, coverage_polygon_json, mount_height, tilt_angle, created_at, updated_at
		FROM camera_placements WHERE map_id = ? ORDER BY created_at
	`, mapID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var placements []CameraPlacement
	for rows.Next() {
		var cp CameraPlacement
		var coverageJSON string
		var mountHeight, tiltAngle sql.NullFloat64

		if err := rows.Scan(&cp.ID, &cp.CameraID, &cp.MapID, &cp.Position.X, &cp.Position.Y, &cp.Rotation, &cp.FOVAngle, &cp.FOVDepth, &coverageJSON, &mountHeight, &tiltAngle, &cp.CreatedAt, &cp.UpdatedAt); err != nil {
			return nil, err
		}

		if coverageJSON != "" {
			if err := json.Unmarshal([]byte(coverageJSON), &cp.CoveragePolygon); err != nil {
				return nil, fmt.Errorf("failed to unmarshal coverage polygon: %w", err)
			}
		}

		if mountHeight.Valid {
			cp.MountHeight = mountHeight.Float64
		}
		if tiltAngle.Valid {
			cp.TiltAngle = tiltAngle.Float64
		}

		placements = append(placements, cp)
	}

	return placements, rows.Err()
}

// UpdatePlacement updates a camera placement
func (s *Store) UpdatePlacement(cp *CameraPlacement) error {
	cp.UpdatedAt = time.Now()

	coverageJSON, err := json.Marshal(cp.CoveragePolygon)
	if err != nil {
		return fmt.Errorf("failed to marshal coverage polygon: %w", err)
	}

	result, err := s.db.Exec(`
		UPDATE camera_placements SET camera_id = ?, map_id = ?, position_x = ?, position_y = ?, rotation = ?, fov_angle = ?, fov_depth = ?, coverage_polygon_json = ?, mount_height = ?, tilt_angle = ?, updated_at = ?
		WHERE id = ?
	`, cp.CameraID, cp.MapID, cp.Position.X, cp.Position.Y, cp.Rotation, cp.FOVAngle, cp.FOVDepth, string(coverageJSON), cp.MountHeight, cp.TiltAngle, cp.UpdatedAt, cp.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errors.New("placement not found")
	}

	return nil
}

// DeletePlacement deletes a camera placement
func (s *Store) DeletePlacement(id string) error {
	result, err := s.db.Exec(`DELETE FROM camera_placements WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errors.New("placement not found")
	}

	return nil
}

// ============================================================================
// Camera Transition CRUD
// ============================================================================

// CreateTransition creates a new camera transition
func (s *Store) CreateTransition(ct *CameraTransition) error {
	if ct.ID == "" {
		ct.ID = uuid.New().String()
	}
	ct.CreatedAt = time.Now()
	ct.UpdatedAt = ct.CreatedAt

	overlapJSON, _ := json.Marshal(ct.OverlapZone)
	exitJSON, _ := json.Marshal(ct.ExitZone)
	entryJSON, _ := json.Marshal(ct.EntryZone)

	_, err := s.db.Exec(`
		INSERT INTO camera_transitions (id, from_camera_id, to_camera_id, transition_type, bidirectional, overlap_zone_json, expected_transit_time, transit_time_variance, exit_zone_json, entry_zone_json, avg_transit_time, success_rate, total_handoffs, successful_handoffs, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, ct.ID, ct.FromCameraID, ct.ToCameraID, ct.Type, ct.Bidirectional, string(overlapJSON), ct.ExpectedTransitTime, ct.TransitTimeVariance, string(exitJSON), string(entryJSON), ct.AvgTransitTime, ct.SuccessRate, ct.TotalHandoffs, ct.SuccessfulHandoffs, ct.CreatedAt, ct.UpdatedAt)

	return err
}

// GetTransition retrieves a camera transition by ID
func (s *Store) GetTransition(id string) (*CameraTransition, error) {
	var ct CameraTransition
	var overlapJSON, exitJSON, entryJSON string
	var expectedTime, variance, avgTime, successRate sql.NullFloat64

	err := s.db.QueryRow(`
		SELECT id, from_camera_id, to_camera_id, transition_type, bidirectional, overlap_zone_json, expected_transit_time, transit_time_variance, exit_zone_json, entry_zone_json, avg_transit_time, success_rate, total_handoffs, successful_handoffs, created_at, updated_at
		FROM camera_transitions WHERE id = ?
	`, id).Scan(&ct.ID, &ct.FromCameraID, &ct.ToCameraID, &ct.Type, &ct.Bidirectional, &overlapJSON, &expectedTime, &variance, &exitJSON, &entryJSON, &avgTime, &successRate, &ct.TotalHandoffs, &ct.SuccessfulHandoffs, &ct.CreatedAt, &ct.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if overlapJSON != "" {
		_ = json.Unmarshal([]byte(overlapJSON), &ct.OverlapZone)
	}
	if exitJSON != "" {
		_ = json.Unmarshal([]byte(exitJSON), &ct.ExitZone)
	}
	if entryJSON != "" {
		_ = json.Unmarshal([]byte(entryJSON), &ct.EntryZone)
	}

	if expectedTime.Valid {
		ct.ExpectedTransitTime = expectedTime.Float64
	}
	if variance.Valid {
		ct.TransitTimeVariance = variance.Float64
	}
	if avgTime.Valid {
		ct.AvgTransitTime = avgTime.Float64
	}
	if successRate.Valid {
		ct.SuccessRate = successRate.Float64
	}

	return &ct, nil
}

// GetTransitionByCameras retrieves a transition by from/to camera IDs
func (s *Store) GetTransitionByCameras(fromCameraID, toCameraID string) (*CameraTransition, error) {
	var ct CameraTransition
	var overlapJSON, exitJSON, entryJSON string
	var expectedTime, variance, avgTime, successRate sql.NullFloat64

	// Check both directions if bidirectional
	err := s.db.QueryRow(`
		SELECT id, from_camera_id, to_camera_id, transition_type, bidirectional, overlap_zone_json, expected_transit_time, transit_time_variance, exit_zone_json, entry_zone_json, avg_transit_time, success_rate, total_handoffs, successful_handoffs, created_at, updated_at
		FROM camera_transitions
		WHERE (from_camera_id = ? AND to_camera_id = ?) OR (bidirectional = 1 AND from_camera_id = ? AND to_camera_id = ?)
	`, fromCameraID, toCameraID, toCameraID, fromCameraID).Scan(&ct.ID, &ct.FromCameraID, &ct.ToCameraID, &ct.Type, &ct.Bidirectional, &overlapJSON, &expectedTime, &variance, &exitJSON, &entryJSON, &avgTime, &successRate, &ct.TotalHandoffs, &ct.SuccessfulHandoffs, &ct.CreatedAt, &ct.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if overlapJSON != "" {
		_ = json.Unmarshal([]byte(overlapJSON), &ct.OverlapZone)
	}
	if exitJSON != "" {
		_ = json.Unmarshal([]byte(exitJSON), &ct.ExitZone)
	}
	if entryJSON != "" {
		_ = json.Unmarshal([]byte(entryJSON), &ct.EntryZone)
	}

	if expectedTime.Valid {
		ct.ExpectedTransitTime = expectedTime.Float64
	}
	if variance.Valid {
		ct.TransitTimeVariance = variance.Float64
	}
	if avgTime.Valid {
		ct.AvgTransitTime = avgTime.Float64
	}
	if successRate.Valid {
		ct.SuccessRate = successRate.Float64
	}

	return &ct, nil
}

// ListTransitions returns all camera transitions
func (s *Store) ListTransitions() ([]CameraTransition, error) {
	rows, err := s.db.Query(`
		SELECT id, from_camera_id, to_camera_id, transition_type, bidirectional, overlap_zone_json, expected_transit_time, transit_time_variance, exit_zone_json, entry_zone_json, avg_transit_time, success_rate, total_handoffs, successful_handoffs, created_at, updated_at
		FROM camera_transitions ORDER BY created_at
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var transitions []CameraTransition
	for rows.Next() {
		var ct CameraTransition
		var overlapJSON, exitJSON, entryJSON string
		var expectedTime, variance, avgTime, successRate sql.NullFloat64

		if err := rows.Scan(&ct.ID, &ct.FromCameraID, &ct.ToCameraID, &ct.Type, &ct.Bidirectional, &overlapJSON, &expectedTime, &variance, &exitJSON, &entryJSON, &avgTime, &successRate, &ct.TotalHandoffs, &ct.SuccessfulHandoffs, &ct.CreatedAt, &ct.UpdatedAt); err != nil {
			return nil, err
		}

		if overlapJSON != "" {
			_ = json.Unmarshal([]byte(overlapJSON), &ct.OverlapZone)
		}
		if exitJSON != "" {
			_ = json.Unmarshal([]byte(exitJSON), &ct.ExitZone)
		}
		if entryJSON != "" {
			_ = json.Unmarshal([]byte(entryJSON), &ct.EntryZone)
		}

		if expectedTime.Valid {
			ct.ExpectedTransitTime = expectedTime.Float64
		}
		if variance.Valid {
			ct.TransitTimeVariance = variance.Float64
		}
		if avgTime.Valid {
			ct.AvgTransitTime = avgTime.Float64
		}
		if successRate.Valid {
			ct.SuccessRate = successRate.Float64
		}

		transitions = append(transitions, ct)
	}

	return transitions, rows.Err()
}

// ListTransitionsFromCamera returns all transitions from a specific camera
func (s *Store) ListTransitionsFromCamera(cameraID string) ([]CameraTransition, error) {
	rows, err := s.db.Query(`
		SELECT id, from_camera_id, to_camera_id, transition_type, bidirectional, overlap_zone_json, expected_transit_time, transit_time_variance, exit_zone_json, entry_zone_json, avg_transit_time, success_rate, total_handoffs, successful_handoffs, created_at, updated_at
		FROM camera_transitions
		WHERE from_camera_id = ? OR (bidirectional = 1 AND to_camera_id = ?)
		ORDER BY created_at
	`, cameraID, cameraID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var transitions []CameraTransition
	for rows.Next() {
		var ct CameraTransition
		var overlapJSON, exitJSON, entryJSON string
		var expectedTime, variance, avgTime, successRate sql.NullFloat64

		if err := rows.Scan(&ct.ID, &ct.FromCameraID, &ct.ToCameraID, &ct.Type, &ct.Bidirectional, &overlapJSON, &expectedTime, &variance, &exitJSON, &entryJSON, &avgTime, &successRate, &ct.TotalHandoffs, &ct.SuccessfulHandoffs, &ct.CreatedAt, &ct.UpdatedAt); err != nil {
			return nil, err
		}

		if overlapJSON != "" {
			_ = json.Unmarshal([]byte(overlapJSON), &ct.OverlapZone)
		}
		if exitJSON != "" {
			_ = json.Unmarshal([]byte(exitJSON), &ct.ExitZone)
		}
		if entryJSON != "" {
			_ = json.Unmarshal([]byte(entryJSON), &ct.EntryZone)
		}

		if expectedTime.Valid {
			ct.ExpectedTransitTime = expectedTime.Float64
		}
		if variance.Valid {
			ct.TransitTimeVariance = variance.Float64
		}
		if avgTime.Valid {
			ct.AvgTransitTime = avgTime.Float64
		}
		if successRate.Valid {
			ct.SuccessRate = successRate.Float64
		}

		transitions = append(transitions, ct)
	}

	return transitions, rows.Err()
}

// UpdateTransition updates a camera transition
func (s *Store) UpdateTransition(ct *CameraTransition) error {
	ct.UpdatedAt = time.Now()

	overlapJSON, _ := json.Marshal(ct.OverlapZone)
	exitJSON, _ := json.Marshal(ct.ExitZone)
	entryJSON, _ := json.Marshal(ct.EntryZone)

	result, err := s.db.Exec(`
		UPDATE camera_transitions SET from_camera_id = ?, to_camera_id = ?, transition_type = ?, bidirectional = ?, overlap_zone_json = ?, expected_transit_time = ?, transit_time_variance = ?, exit_zone_json = ?, entry_zone_json = ?, avg_transit_time = ?, success_rate = ?, total_handoffs = ?, successful_handoffs = ?, updated_at = ?
		WHERE id = ?
	`, ct.FromCameraID, ct.ToCameraID, ct.Type, ct.Bidirectional, string(overlapJSON), ct.ExpectedTransitTime, ct.TransitTimeVariance, string(exitJSON), string(entryJSON), ct.AvgTransitTime, ct.SuccessRate, ct.TotalHandoffs, ct.SuccessfulHandoffs, ct.UpdatedAt, ct.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errors.New("transition not found")
	}

	return nil
}

// RecordHandoff updates transition statistics after a handoff attempt
func (s *Store) RecordHandoff(transitionID string, transitTime float64, success bool) error {
	ct, err := s.GetTransition(transitionID)
	if err != nil {
		return err
	}
	if ct == nil {
		return errors.New("transition not found")
	}

	ct.TotalHandoffs++
	if success {
		ct.SuccessfulHandoffs++
		// Update rolling average transit time
		if ct.AvgTransitTime == 0 {
			ct.AvgTransitTime = transitTime
		} else {
			ct.AvgTransitTime = (ct.AvgTransitTime*float64(ct.SuccessfulHandoffs-1) + transitTime) / float64(ct.SuccessfulHandoffs)
		}
	}
	ct.SuccessRate = float64(ct.SuccessfulHandoffs) / float64(ct.TotalHandoffs)

	return s.UpdateTransition(ct)
}

// DeleteTransition deletes a camera transition
func (s *Store) DeleteTransition(id string) error {
	result, err := s.db.Exec(`DELETE FROM camera_transitions WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errors.New("transition not found")
	}

	return nil
}

// ============================================================================
// Global Track CRUD
// ============================================================================

// CreateTrack creates a new global track
func (s *Store) CreateTrack(gt *GlobalTrack) error {
	if gt.ID == "" {
		gt.ID = uuid.New().String()
	}
	gt.CreatedAt = time.Now()
	gt.UpdatedAt = gt.CreatedAt

	colorsJSON, _ := json.Marshal(gt.DominantColors)

	var predictedArrival *time.Time
	if gt.PredictedArrival != nil {
		predictedArrival = gt.PredictedArrival
	}

	_, err := s.db.Exec(`
		INSERT INTO global_tracks (id, first_seen, last_seen, current_camera_id, current_local_track, object_type, embedding, embedding_confidence, dominant_colors_json, estimated_height, state, predicted_next_camera, predicted_arrival, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, gt.ID, gt.FirstSeen, gt.LastSeen, gt.CurrentCameraID, gt.CurrentLocalTrack, gt.ObjectType, gt.Embedding, gt.EmbeddingConf, string(colorsJSON), gt.EstimatedHeight, gt.State, gt.PredictedNext, predictedArrival, gt.CreatedAt, gt.UpdatedAt)

	return err
}

// GetTrack retrieves a global track by ID
func (s *Store) GetTrack(id string) (*GlobalTrack, error) {
	var gt GlobalTrack
	var colorsJSON string
	var currentCameraID, currentLocalTrack, predictedNext sql.NullString
	var embeddingConf, estimatedHeight sql.NullFloat64
	var predictedArrival sql.NullTime
	var embedding []byte

	err := s.db.QueryRow(`
		SELECT id, first_seen, last_seen, current_camera_id, current_local_track, object_type, embedding, embedding_confidence, dominant_colors_json, estimated_height, state, predicted_next_camera, predicted_arrival, created_at, updated_at
		FROM global_tracks WHERE id = ?
	`, id).Scan(&gt.ID, &gt.FirstSeen, &gt.LastSeen, &currentCameraID, &currentLocalTrack, &gt.ObjectType, &embedding, &embeddingConf, &colorsJSON, &estimatedHeight, &gt.State, &predictedNext, &predictedArrival, &gt.CreatedAt, &gt.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	gt.Embedding = embedding
	if currentCameraID.Valid {
		gt.CurrentCameraID = currentCameraID.String
	}
	if currentLocalTrack.Valid {
		gt.CurrentLocalTrack = currentLocalTrack.String
	}
	if embeddingConf.Valid {
		gt.EmbeddingConf = embeddingConf.Float64
	}
	if colorsJSON != "" {
		_ = json.Unmarshal([]byte(colorsJSON), &gt.DominantColors)
	}
	if estimatedHeight.Valid {
		gt.EstimatedHeight = estimatedHeight.Float64
	}
	if predictedNext.Valid {
		gt.PredictedNext = predictedNext.String
	}
	if predictedArrival.Valid {
		gt.PredictedArrival = &predictedArrival.Time
	}

	// Load track segments
	segments, err := s.ListSegmentsByTrack(gt.ID)
	if err != nil {
		return nil, err
	}
	gt.Path = segments

	return &gt, nil
}

// ListActiveTracks returns all active tracks
func (s *Store) ListActiveTracks() ([]GlobalTrack, error) {
	return s.listTracksByState("active")
}

// ListTracksByState returns tracks in a specific state
func (s *Store) listTracksByState(state string) ([]GlobalTrack, error) {
	rows, err := s.db.Query(`
		SELECT id, first_seen, last_seen, current_camera_id, current_local_track, object_type, embedding, embedding_confidence, dominant_colors_json, estimated_height, state, predicted_next_camera, predicted_arrival, created_at, updated_at
		FROM global_tracks WHERE state = ? ORDER BY last_seen DESC
	`, state)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var tracks []GlobalTrack
	for rows.Next() {
		var gt GlobalTrack
		var colorsJSON string
		var currentCameraID, currentLocalTrack, predictedNext sql.NullString
		var embeddingConf, estimatedHeight sql.NullFloat64
		var predictedArrival sql.NullTime
		var embedding []byte

		if err := rows.Scan(&gt.ID, &gt.FirstSeen, &gt.LastSeen, &currentCameraID, &currentLocalTrack, &gt.ObjectType, &embedding, &embeddingConf, &colorsJSON, &estimatedHeight, &gt.State, &predictedNext, &predictedArrival, &gt.CreatedAt, &gt.UpdatedAt); err != nil {
			return nil, err
		}

		gt.Embedding = embedding
		if currentCameraID.Valid {
			gt.CurrentCameraID = currentCameraID.String
		}
		if currentLocalTrack.Valid {
			gt.CurrentLocalTrack = currentLocalTrack.String
		}
		if embeddingConf.Valid {
			gt.EmbeddingConf = embeddingConf.Float64
		}
		if colorsJSON != "" {
			_ = json.Unmarshal([]byte(colorsJSON), &gt.DominantColors)
		}
		if estimatedHeight.Valid {
			gt.EstimatedHeight = estimatedHeight.Float64
		}
		if predictedNext.Valid {
			gt.PredictedNext = predictedNext.String
		}
		if predictedArrival.Valid {
			gt.PredictedArrival = &predictedArrival.Time
		}

		tracks = append(tracks, gt)
	}

	return tracks, rows.Err()
}

// UpdateTrack updates a global track
func (s *Store) UpdateTrack(gt *GlobalTrack) error {
	gt.UpdatedAt = time.Now()

	colorsJSON, _ := json.Marshal(gt.DominantColors)

	var predictedArrival *time.Time
	if gt.PredictedArrival != nil {
		predictedArrival = gt.PredictedArrival
	}

	result, err := s.db.Exec(`
		UPDATE global_tracks SET first_seen = ?, last_seen = ?, current_camera_id = ?, current_local_track = ?, object_type = ?, embedding = ?, embedding_confidence = ?, dominant_colors_json = ?, estimated_height = ?, state = ?, predicted_next_camera = ?, predicted_arrival = ?, updated_at = ?
		WHERE id = ?
	`, gt.FirstSeen, gt.LastSeen, gt.CurrentCameraID, gt.CurrentLocalTrack, gt.ObjectType, gt.Embedding, gt.EmbeddingConf, string(colorsJSON), gt.EstimatedHeight, gt.State, gt.PredictedNext, predictedArrival, gt.UpdatedAt, gt.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errors.New("track not found")
	}

	return nil
}

// DeleteTrack deletes a global track and its segments
func (s *Store) DeleteTrack(id string) error {
	result, err := s.db.Exec(`DELETE FROM global_tracks WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errors.New("track not found")
	}

	return nil
}

// ============================================================================
// Track Segment CRUD
// ============================================================================

// CreateSegment creates a new track segment
func (s *Store) CreateSegment(ts *TrackSegment) error {
	if ts.ID == "" {
		ts.ID = uuid.New().String()
	}

	bboxJSON, _ := json.Marshal(ts.BoundingBoxes)

	var exitPosX, exitPosY *float64
	if ts.ExitPosition != nil {
		exitPosX = &ts.ExitPosition.X
		exitPosY = &ts.ExitPosition.Y
	}

	_, err := s.db.Exec(`
		INSERT INTO track_segments (id, global_track_id, camera_id, local_track_id, entered_at, exited_at, exit_direction, exit_position_x, exit_position_y, bounding_boxes_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, ts.ID, ts.GlobalTrackID, ts.CameraID, ts.LocalTrackID, ts.EnteredAt, ts.ExitedAt, ts.ExitDirection, exitPosX, exitPosY, string(bboxJSON), time.Now())

	return err
}

// ListSegmentsByTrack returns all segments for a track
func (s *Store) ListSegmentsByTrack(trackID string) ([]TrackSegment, error) {
	rows, err := s.db.Query(`
		SELECT id, global_track_id, camera_id, local_track_id, entered_at, exited_at, exit_direction, exit_position_x, exit_position_y, bounding_boxes_json
		FROM track_segments WHERE global_track_id = ? ORDER BY entered_at
	`, trackID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var segments []TrackSegment
	for rows.Next() {
		var ts TrackSegment
		var exitedAt sql.NullTime
		var exitDir sql.NullString
		var exitPosX, exitPosY sql.NullFloat64
		var bboxJSON string

		if err := rows.Scan(&ts.ID, &ts.GlobalTrackID, &ts.CameraID, &ts.LocalTrackID, &ts.EnteredAt, &exitedAt, &exitDir, &exitPosX, &exitPosY, &bboxJSON); err != nil {
			return nil, err
		}

		if exitedAt.Valid {
			ts.ExitedAt = &exitedAt.Time
		}
		if exitDir.Valid {
			ts.ExitDirection = EdgeDirection(exitDir.String)
		}
		if exitPosX.Valid && exitPosY.Valid {
			ts.ExitPosition = &Point{X: exitPosX.Float64, Y: exitPosY.Float64}
		}
		if bboxJSON != "" {
			_ = json.Unmarshal([]byte(bboxJSON), &ts.BoundingBoxes)
		}

		segments = append(segments, ts)
	}

	return segments, rows.Err()
}

// UpdateSegment updates a track segment
func (s *Store) UpdateSegment(ts *TrackSegment) error {
	bboxJSON, _ := json.Marshal(ts.BoundingBoxes)

	var exitPosX, exitPosY *float64
	if ts.ExitPosition != nil {
		exitPosX = &ts.ExitPosition.X
		exitPosY = &ts.ExitPosition.Y
	}

	result, err := s.db.Exec(`
		UPDATE track_segments SET exited_at = ?, exit_direction = ?, exit_position_x = ?, exit_position_y = ?, bounding_boxes_json = ?
		WHERE id = ?
	`, ts.ExitedAt, ts.ExitDirection, exitPosX, exitPosY, string(bboxJSON), ts.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errors.New("segment not found")
	}

	return nil
}

// ============================================================================
// Pending Handoff CRUD
// ============================================================================

// CreatePendingHandoff creates a new pending handoff
func (s *Store) CreatePendingHandoff(ph *PendingHandoff) error {
	if ph.ID == "" {
		ph.ID = uuid.New().String()
	}

	toCamerasJSON, _ := json.Marshal(ph.ToCameraIDs)
	colorsJSON, _ := json.Marshal(ph.DominantColors)

	_, err := s.db.Exec(`
		INSERT INTO pending_handoffs (id, global_track_id, from_camera_id, to_camera_ids_json, transition_type, exited_at, expected_by, exit_direction, exit_position_x, exit_position_y, embedding, dominant_colors_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, ph.ID, ph.GlobalTrackID, ph.FromCameraID, string(toCamerasJSON), ph.TransitionType, ph.ExitedAt, ph.ExpectedBy, ph.ExitDirection, ph.ExitPosition.X, ph.ExitPosition.Y, ph.Embedding, string(colorsJSON), time.Now())

	return err
}

// GetPendingHandoff retrieves a pending handoff by ID
func (s *Store) GetPendingHandoff(id string) (*PendingHandoff, error) {
	var ph PendingHandoff
	var toCamerasJSON, colorsJSON string

	err := s.db.QueryRow(`
		SELECT id, global_track_id, from_camera_id, to_camera_ids_json, transition_type, exited_at, expected_by, exit_direction, exit_position_x, exit_position_y, embedding, dominant_colors_json
		FROM pending_handoffs WHERE id = ?
	`, id).Scan(&ph.ID, &ph.GlobalTrackID, &ph.FromCameraID, &toCamerasJSON, &ph.TransitionType, &ph.ExitedAt, &ph.ExpectedBy, &ph.ExitDirection, &ph.ExitPosition.X, &ph.ExitPosition.Y, &ph.Embedding, &colorsJSON)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	_ = json.Unmarshal([]byte(toCamerasJSON), &ph.ToCameraIDs)
	_ = json.Unmarshal([]byte(colorsJSON), &ph.DominantColors)

	return &ph, nil
}

// ListPendingHandoffs returns all pending handoffs
func (s *Store) ListPendingHandoffs() ([]PendingHandoff, error) {
	rows, err := s.db.Query(`
		SELECT id, global_track_id, from_camera_id, to_camera_ids_json, transition_type, exited_at, expected_by, exit_direction, exit_position_x, exit_position_y, embedding, dominant_colors_json
		FROM pending_handoffs WHERE expected_by > ? ORDER BY expected_by
	`, time.Now())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var handoffs []PendingHandoff
	for rows.Next() {
		var ph PendingHandoff
		var toCamerasJSON, colorsJSON string

		if err := rows.Scan(&ph.ID, &ph.GlobalTrackID, &ph.FromCameraID, &toCamerasJSON, &ph.TransitionType, &ph.ExitedAt, &ph.ExpectedBy, &ph.ExitDirection, &ph.ExitPosition.X, &ph.ExitPosition.Y, &ph.Embedding, &colorsJSON); err != nil {
			return nil, err
		}

		_ = json.Unmarshal([]byte(toCamerasJSON), &ph.ToCameraIDs)
		_ = json.Unmarshal([]byte(colorsJSON), &ph.DominantColors)

		handoffs = append(handoffs, ph)
	}

	return handoffs, rows.Err()
}

// ListPendingHandoffsForCamera returns pending handoffs expecting arrival at a camera
func (s *Store) ListPendingHandoffsForCamera(cameraID string) ([]PendingHandoff, error) {
	rows, err := s.db.Query(`
		SELECT id, global_track_id, from_camera_id, to_camera_ids_json, transition_type, exited_at, expected_by, exit_direction, exit_position_x, exit_position_y, embedding, dominant_colors_json
		FROM pending_handoffs WHERE expected_by > ? ORDER BY expected_by
	`, time.Now())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var handoffs []PendingHandoff
	for rows.Next() {
		var ph PendingHandoff
		var toCamerasJSON, colorsJSON string

		if err := rows.Scan(&ph.ID, &ph.GlobalTrackID, &ph.FromCameraID, &toCamerasJSON, &ph.TransitionType, &ph.ExitedAt, &ph.ExpectedBy, &ph.ExitDirection, &ph.ExitPosition.X, &ph.ExitPosition.Y, &ph.Embedding, &colorsJSON); err != nil {
			return nil, err
		}

		_ = json.Unmarshal([]byte(toCamerasJSON), &ph.ToCameraIDs)
		_ = json.Unmarshal([]byte(colorsJSON), &ph.DominantColors)

		// Filter by camera ID
		for _, camID := range ph.ToCameraIDs {
			if camID == cameraID {
				handoffs = append(handoffs, ph)
				break
			}
		}
	}

	return handoffs, rows.Err()
}

// DeletePendingHandoff deletes a pending handoff
func (s *Store) DeletePendingHandoff(id string) error {
	_, err := s.db.Exec(`DELETE FROM pending_handoffs WHERE id = ?`, id)
	return err
}

// CleanupExpiredHandoffs removes expired pending handoffs
func (s *Store) CleanupExpiredHandoffs() (int64, error) {
	result, err := s.db.Exec(`DELETE FROM pending_handoffs WHERE expected_by < ?`, time.Now())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ============================================================================
// Analytics
// ============================================================================

// GetAnalytics returns aggregate tracking statistics
func (s *Store) GetAnalytics() (*Analytics, error) {
	var analytics Analytics

	// Total and active tracks
	err := s.db.QueryRow(`SELECT COUNT(*) FROM global_tracks`).Scan(&analytics.TotalTracks)
	if err != nil {
		return nil, err
	}

	err = s.db.QueryRow(`SELECT COUNT(*) FROM global_tracks WHERE state = 'active'`).Scan(&analytics.ActiveTracks)
	if err != nil {
		return nil, err
	}

	// Handoff stats
	err = s.db.QueryRow(`SELECT COALESCE(SUM(total_handoffs), 0), COALESCE(SUM(successful_handoffs), 0) FROM camera_transitions`).Scan(&analytics.TotalHandoffs, &analytics.SuccessfulHandoffs)
	if err != nil {
		return nil, err
	}

	if analytics.TotalHandoffs > 0 {
		analytics.OverallSuccessRate = float64(analytics.SuccessfulHandoffs) / float64(analytics.TotalHandoffs)
	}

	// Per-transition stats
	rows, err := s.db.Query(`
		SELECT id, from_camera_id, to_camera_id, transition_type, total_handoffs, success_rate, avg_transit_time
		FROM camera_transitions WHERE total_handoffs > 0
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var stat TransitionStat
		var avgTime sql.NullFloat64
		if err := rows.Scan(&stat.TransitionID, &stat.FromCameraID, &stat.ToCameraID, &stat.Type, &stat.TotalHandoffs, &stat.SuccessRate, &avgTime); err != nil {
			return nil, err
		}
		if avgTime.Valid {
			stat.AvgTransitTime = avgTime.Float64
		}
		analytics.TransitionStats = append(analytics.TransitionStats, stat)
	}

	// Hourly activity (last 24 hours)
	hourlyRows, err := s.db.Query(`
		SELECT strftime('%H', first_seen) as hour, COUNT(*) as count
		FROM global_tracks
		WHERE first_seen > datetime('now', '-24 hours')
		GROUP BY hour
		ORDER BY hour
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = hourlyRows.Close() }()

	hourlyMap := make(map[int]int)
	for hourlyRows.Next() {
		var hourStr string
		var count int
		if err := hourlyRows.Scan(&hourStr, &count); err != nil {
			return nil, err
		}
		var hour int
		_, _ = fmt.Sscanf(hourStr, "%d", &hour)
		hourlyMap[hour] = count
	}

	for hour := 0; hour < 24; hour++ {
		analytics.HourlyActivity = append(analytics.HourlyActivity, HourlyActivityStat{
			Hour:       hour,
			TrackCount: hourlyMap[hour],
		})
	}

	return &analytics, nil
}

// ============================================================================
// Context-aware wrapper methods (for API compatibility)
// ============================================================================

// ListMaps returns all spatial maps (context-aware wrapper)
func (s *Store) ListMapsCtx(ctx context.Context) ([]SpatialMap, error) {
	return s.ListMaps()
}

// CreateMapCtx creates a new spatial map (context-aware wrapper)
func (s *Store) CreateMapCtx(ctx context.Context, m *SpatialMap) error {
	return s.CreateMap(m)
}

// GetMapCtx retrieves a spatial map by ID (context-aware wrapper)
func (s *Store) GetMapCtx(ctx context.Context, id string) (*SpatialMap, error) {
	return s.GetMap(id)
}

// UpdateMapCtx updates a spatial map (context-aware wrapper)
func (s *Store) UpdateMapCtx(ctx context.Context, m *SpatialMap) error {
	return s.UpdateMap(m)
}

// DeleteMapCtx deletes a spatial map (context-aware wrapper)
func (s *Store) DeleteMapCtx(ctx context.Context, id string) error {
	return s.DeleteMap(id)
}

// ListPlacementsCtx returns camera placements for a map (context-aware wrapper)
func (s *Store) ListPlacementsCtx(ctx context.Context, mapID string) ([]CameraPlacement, error) {
	return s.ListPlacementsByMap(mapID)
}

// CreatePlacementCtx creates a new camera placement (context-aware wrapper)
func (s *Store) CreatePlacementCtx(ctx context.Context, cp *CameraPlacement) error {
	return s.CreatePlacement(cp)
}

// GetPlacementCtx retrieves a camera placement by ID (context-aware wrapper)
func (s *Store) GetPlacementCtx(ctx context.Context, id string) (*CameraPlacement, error) {
	return s.GetPlacement(id)
}

// UpdatePlacementCtx updates a camera placement (context-aware wrapper)
func (s *Store) UpdatePlacementCtx(ctx context.Context, cp *CameraPlacement) error {
	return s.UpdatePlacement(cp)
}

// DeletePlacementCtx deletes a camera placement (context-aware wrapper)
func (s *Store) DeletePlacementCtx(ctx context.Context, id string) error {
	return s.DeletePlacement(id)
}

// ListTransitionsCtx returns all camera transitions (context-aware wrapper)
func (s *Store) ListTransitionsCtx(ctx context.Context) ([]CameraTransition, error) {
	return s.ListTransitions()
}

// CreateTransitionCtx creates a new camera transition (context-aware wrapper)
func (s *Store) CreateTransitionCtx(ctx context.Context, ct *CameraTransition) error {
	return s.CreateTransition(ct)
}

// GetTransitionCtx retrieves a camera transition by ID (context-aware wrapper)
func (s *Store) GetTransitionCtx(ctx context.Context, id string) (*CameraTransition, error) {
	return s.GetTransition(id)
}

// UpdateTransitionCtx updates a camera transition (context-aware wrapper)
func (s *Store) UpdateTransitionCtx(ctx context.Context, ct *CameraTransition) error {
	return s.UpdateTransition(ct)
}

// DeleteTransitionCtx deletes a camera transition (context-aware wrapper)
func (s *Store) DeleteTransitionCtx(ctx context.Context, id string) error {
	return s.DeleteTransition(id)
}

// GetAnalyticsCtx returns aggregate tracking statistics (context-aware wrapper)
func (s *Store) GetAnalyticsCtx(ctx context.Context) (*Analytics, error) {
	return s.GetAnalytics()
}

// GetMapAnalytics returns analytics for a specific map
func (s *Store) GetMapAnalytics(mapID string) (*MapAnalytics, error) {
	var analytics MapAnalytics

	// Get placements in this map
	placements, err := s.ListPlacementsByMap(mapID)
	if err != nil {
		return nil, err
	}
	cameraIDs := make([]string, 0, len(placements))
	for _, p := range placements {
		cameraIDs = append(cameraIDs, p.CameraID)
	}

	// Active tracks for cameras in this map
	if len(cameraIDs) > 0 {
		query := `SELECT COUNT(*) FROM global_tracks WHERE state = 'active' AND current_camera_id IN (?` + strings.Repeat(",?", len(cameraIDs)-1) + `)`
		args := make([]interface{}, len(cameraIDs))
		for i, id := range cameraIDs {
			args[i] = id
		}
		err = s.db.QueryRow(query, args...).Scan(&analytics.ActiveTracks)
		if err != nil && err != sql.ErrNoRows {
			analytics.ActiveTracks = 0
		}
	}

	// Total tracks for cameras in this map
	if len(cameraIDs) > 0 {
		query := `SELECT COUNT(DISTINCT id) FROM global_tracks WHERE current_camera_id IN (?` + strings.Repeat(",?", len(cameraIDs)-1) + `)`
		args := make([]interface{}, len(cameraIDs))
		for i, id := range cameraIDs {
			args[i] = id
		}
		err = s.db.QueryRow(query, args...).Scan(&analytics.TotalTracks)
		if err != nil && err != sql.ErrNoRows {
			analytics.TotalTracks = 0
		}
	}

	// Handoff stats for transitions in this map
	if len(cameraIDs) > 0 {
		query := `SELECT COALESCE(SUM(total_handoffs), 0), COALESCE(SUM(successful_handoffs), 0), COALESCE(AVG(avg_transit_time), 0)
		          FROM camera_transitions WHERE map_id = ?`
		var avgTransit sql.NullFloat64
		err = s.db.QueryRow(query, mapID).Scan(&analytics.TotalHandoffs, &analytics.SuccessfulHandoffs, &avgTransit)
		if err != nil && err != sql.ErrNoRows {
			analytics.TotalHandoffs = 0
			analytics.SuccessfulHandoffs = 0
		}
		if avgTransit.Valid {
			analytics.AverageTransitTime = avgTransit.Float64
		}
	}

	analytics.FailedHandoffs = analytics.TotalHandoffs - analytics.SuccessfulHandoffs
	if analytics.FailedHandoffs < 0 {
		analytics.FailedHandoffs = 0
	}

	// Coverage gaps (cameras without any transitions)
	if len(cameraIDs) > 0 {
		for _, camID := range cameraIDs {
			var count int
			_ = s.db.QueryRow(`SELECT COUNT(*) FROM camera_transitions WHERE from_camera_id = ? OR to_camera_id = ?`, camID, camID).Scan(&count)
			if count == 0 {
				analytics.CoverageGaps = append(analytics.CoverageGaps, camID)
			}
		}
	}

	return &analytics, nil
}

// ============================================================================
// Image Storage
// ============================================================================

// SaveMapImage saves a map image file and returns its URL
func (s *Store) SaveMapImage(ctx context.Context, mapID string, file io.Reader, filename string) (string, error) {
	// Create directory if it doesn't exist
	imageDir := filepath.Join(s.dataPath, "maps", mapID)
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create image directory: %w", err)
	}

	// Generate filename with timestamp to avoid conflicts
	ext := filepath.Ext(filename)
	newFilename := fmt.Sprintf("map_%d%s", time.Now().Unix(), ext)
	imagePath := filepath.Join(imageDir, newFilename)

	// Create file
	dst, err := os.Create(imagePath)
	if err != nil {
		return "", fmt.Errorf("failed to create image file: %w", err)
	}
	defer func() { _ = dst.Close() }()

	// Copy contents
	if _, err := io.Copy(dst, file); err != nil {
		return "", fmt.Errorf("failed to save image: %w", err)
	}

	// Return relative URL path
	imageURL := fmt.Sprintf("/api/v1/plugins/nvr-spatial-tracking/maps/%s/image/%s", mapID, newFilename)

	// Update map with image URL
	m, err := s.GetMap(mapID)
	if err != nil {
		return "", err
	}
	if m != nil {
		m.ImageURL = imageURL
		if err := s.UpdateMap(m); err != nil {
			return "", err
		}
	}

	return imageURL, nil
}

// ============================================================================
// Auto-Detection
// ============================================================================

// AutoDetectTransitions analyzes camera placements and detects transitions
func (s *Store) AutoDetectTransitions(ctx context.Context, mapID string) ([]CameraTransition, error) {
	placements, err := s.ListPlacementsByMap(mapID)
	if err != nil {
		return nil, err
	}

	if len(placements) < 2 {
		return nil, errors.New("need at least 2 camera placements to detect transitions")
	}

	var detectedTransitions []CameraTransition

	// Compare each pair of cameras
	for i := 0; i < len(placements); i++ {
		for j := i + 1; j < len(placements); j++ {
			p1 := &placements[i]
			p2 := &placements[j]

			// Calculate FOV polygons
			fov1 := p1.CalculateFOVPolygon()
			fov2 := p2.CalculateFOVPolygon()

			var transitionType TransitionType
			var overlapZone Polygon

			if fov1.Intersects(fov2) {
				// Cameras overlap
				transitionType = TransitionOverlap
				overlapZone = s.calculateOverlapZone(fov1, fov2)
			} else {
				// Calculate distance between FOV edges
				distance := s.calculateFOVDistance(fov1, fov2)

				if distance < 10 { // Adjacent threshold (units)
					transitionType = TransitionAdjacent
				} else {
					transitionType = TransitionGap
				}
			}

			// Estimate transit time based on distance
			centerDist := p1.Position.Distance(p2.Position)
			walkingSpeed := 1.4 // meters per second (average walking speed)
			expectedTime := centerDist / walkingSpeed

			transition := CameraTransition{
				FromCameraID:        p1.CameraID,
				ToCameraID:          p2.CameraID,
				Type:                transitionType,
				Bidirectional:       true,
				OverlapZone:         overlapZone,
				ExpectedTransitTime: expectedTime,
				TransitTimeVariance: expectedTime * 0.5, // 50% variance
			}

			// Check if transition already exists
			existing, _ := s.GetTransitionByCameras(p1.CameraID, p2.CameraID)
			if existing == nil {
				// Create new transition
				if err := s.CreateTransition(&transition); err != nil {
					return nil, err
				}
			} else {
				transition.ID = existing.ID
			}

			detectedTransitions = append(detectedTransitions, transition)
		}
	}

	return detectedTransitions, nil
}

// calculateOverlapZone finds the intersection of two polygons (simplified)
func (s *Store) calculateOverlapZone(p1, p2 Polygon) Polygon {
	// Simplified: return points that are inside both polygons
	var overlap Polygon

	for _, pt := range p1 {
		if p2.ContainsPoint(pt) {
			overlap = append(overlap, pt)
		}
	}
	for _, pt := range p2 {
		if p1.ContainsPoint(pt) {
			overlap = append(overlap, pt)
		}
	}

	return overlap
}

// calculateFOVDistance finds minimum distance between two FOV polygons
func (s *Store) calculateFOVDistance(p1, p2 Polygon) float64 {
	if len(p1) == 0 || len(p2) == 0 {
		return 1000 // Large default distance
	}

	minDist := p1[0].Distance(p2[0])

	for _, pt1 := range p1 {
		for _, pt2 := range p2 {
			dist := pt1.Distance(pt2)
			if dist < minDist {
				minDist = dist
			}
		}
	}

	return minDist
}
