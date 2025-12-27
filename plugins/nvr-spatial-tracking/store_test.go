package nvrspatialtracking

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) (*Store, func()) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	store := NewStore(db)
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	cleanup := func() {
		_ = db.Close()
	}

	return store, cleanup
}

func TestStoreMigration(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Verify tables exist by running queries
	tables := []string{
		"spatial_maps",
		"camera_placements",
		"camera_transitions",
		"global_tracks",
		"track_segments",
		"pending_handoffs",
	}

	for _, table := range tables {
		var name string
		err := store.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("Table %s not found: %v", table, err)
		}
	}
}

func TestSpatialMapCRUD(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Create
	m := &SpatialMap{
		Name:   "Test Map",
		Width:  100,
		Height: 80,
		Scale:  1.0,
		Metadata: MapMeta{
			Building: "Main",
			Floor:    "1",
		},
	}

	err := store.CreateMap(m)
	if err != nil {
		t.Fatalf("CreateMap failed: %v", err)
	}
	if m.ID == "" {
		t.Error("CreateMap should set ID")
	}

	// Read
	retrieved, err := store.GetMap(m.ID)
	if err != nil {
		t.Fatalf("GetMap failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("GetMap returned nil")
	}
	if retrieved.Name != m.Name {
		t.Errorf("Name = %s, want %s", retrieved.Name, m.Name)
	}
	if retrieved.Metadata.Building != m.Metadata.Building {
		t.Errorf("Metadata.Building = %s, want %s", retrieved.Metadata.Building, m.Metadata.Building)
	}

	// Update
	m.Name = "Updated Map"
	err = store.UpdateMap(m)
	if err != nil {
		t.Fatalf("UpdateMap failed: %v", err)
	}

	retrieved, _ = store.GetMap(m.ID)
	if retrieved.Name != "Updated Map" {
		t.Errorf("Updated Name = %s, want 'Updated Map'", retrieved.Name)
	}

	// List
	maps, err := store.ListMaps()
	if err != nil {
		t.Fatalf("ListMaps failed: %v", err)
	}
	if len(maps) != 1 {
		t.Errorf("ListMaps returned %d maps, want 1", len(maps))
	}

	// Delete
	err = store.DeleteMap(m.ID)
	if err != nil {
		t.Fatalf("DeleteMap failed: %v", err)
	}

	retrieved, _ = store.GetMap(m.ID)
	if retrieved != nil {
		t.Error("GetMap should return nil after delete")
	}
}

func TestCameraPlacementCRUD(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Create a map first
	m := &SpatialMap{Name: "Test Map", Width: 100, Height: 100, Scale: 1.0}
	_ = store.CreateMap(m)

	// Create placement
	cp := &CameraPlacement{
		CameraID: "cam-1",
		MapID:    m.ID,
		Position: Point{X: 50, Y: 50},
		Rotation: 45,
		FOVAngle: 90,
		FOVDepth: 100,
	}

	err := store.CreatePlacement(cp)
	if err != nil {
		t.Fatalf("CreatePlacement failed: %v", err)
	}
	if cp.ID == "" {
		t.Error("CreatePlacement should set ID")
	}

	// Read
	retrieved, err := store.GetPlacement(cp.ID)
	if err != nil {
		t.Fatalf("GetPlacement failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("GetPlacement returned nil")
	}
	if retrieved.CameraID != cp.CameraID {
		t.Errorf("CameraID = %s, want %s", retrieved.CameraID, cp.CameraID)
	}
	if retrieved.Position.X != 50 {
		t.Errorf("Position.X = %f, want 50", retrieved.Position.X)
	}

	// Get by camera ID
	byCameraID, err := store.GetPlacementByCameraID("cam-1")
	if err != nil {
		t.Fatalf("GetPlacementByCameraID failed: %v", err)
	}
	if byCameraID == nil || byCameraID.ID != cp.ID {
		t.Error("GetPlacementByCameraID returned wrong placement")
	}

	// List by map
	placements, err := store.ListPlacementsByMap(m.ID)
	if err != nil {
		t.Fatalf("ListPlacementsByMap failed: %v", err)
	}
	if len(placements) != 1 {
		t.Errorf("ListPlacementsByMap returned %d, want 1", len(placements))
	}

	// Update
	cp.Rotation = 90
	err = store.UpdatePlacement(cp)
	if err != nil {
		t.Fatalf("UpdatePlacement failed: %v", err)
	}

	retrieved, _ = store.GetPlacement(cp.ID)
	if retrieved.Rotation != 90 {
		t.Errorf("Updated Rotation = %f, want 90", retrieved.Rotation)
	}

	// Delete
	err = store.DeletePlacement(cp.ID)
	if err != nil {
		t.Fatalf("DeletePlacement failed: %v", err)
	}
}

func TestCameraTransitionCRUD(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Create transition
	ct := &CameraTransition{
		FromCameraID:        "cam-1",
		ToCameraID:          "cam-2",
		Type:                TransitionGap,
		Bidirectional:       true,
		ExpectedTransitTime: 5.0,
		TransitTimeVariance: 2.0,
	}

	err := store.CreateTransition(ct)
	if err != nil {
		t.Fatalf("CreateTransition failed: %v", err)
	}
	if ct.ID == "" {
		t.Error("CreateTransition should set ID")
	}

	// Read
	retrieved, err := store.GetTransition(ct.ID)
	if err != nil {
		t.Fatalf("GetTransition failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("GetTransition returned nil")
	}
	if retrieved.Type != TransitionGap {
		t.Errorf("Type = %s, want gap", retrieved.Type)
	}

	// Get by cameras
	byCameras, err := store.GetTransitionByCameras("cam-1", "cam-2")
	if err != nil {
		t.Fatalf("GetTransitionByCameras failed: %v", err)
	}
	if byCameras == nil || byCameras.ID != ct.ID {
		t.Error("GetTransitionByCameras returned wrong transition")
	}

	// Get by cameras (bidirectional)
	byCameras, _ = store.GetTransitionByCameras("cam-2", "cam-1")
	if byCameras == nil || byCameras.ID != ct.ID {
		t.Error("GetTransitionByCameras should work bidirectionally")
	}

	// List
	transitions, err := store.ListTransitions()
	if err != nil {
		t.Fatalf("ListTransitions failed: %v", err)
	}
	if len(transitions) != 1 {
		t.Errorf("ListTransitions returned %d, want 1", len(transitions))
	}

	// Record handoff
	err = store.RecordHandoff(ct.ID, 4.5, true)
	if err != nil {
		t.Fatalf("RecordHandoff failed: %v", err)
	}

	retrieved, _ = store.GetTransition(ct.ID)
	if retrieved.TotalHandoffs != 1 {
		t.Errorf("TotalHandoffs = %d, want 1", retrieved.TotalHandoffs)
	}
	if retrieved.SuccessfulHandoffs != 1 {
		t.Errorf("SuccessfulHandoffs = %d, want 1", retrieved.SuccessfulHandoffs)
	}
	if retrieved.AvgTransitTime != 4.5 {
		t.Errorf("AvgTransitTime = %f, want 4.5", retrieved.AvgTransitTime)
	}

	// Delete
	err = store.DeleteTransition(ct.ID)
	if err != nil {
		t.Fatalf("DeleteTransition failed: %v", err)
	}
}

func TestGlobalTrackCRUD(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now()
	gt := &GlobalTrack{
		FirstSeen:         now,
		LastSeen:          now,
		CurrentCameraID:   "cam-1",
		CurrentLocalTrack: "track-1",
		ObjectType:        "person",
		State:             TrackStateActive,
		DominantColors:    []string{"red", "blue"},
	}

	err := store.CreateTrack(gt)
	if err != nil {
		t.Fatalf("CreateTrack failed: %v", err)
	}
	if gt.ID == "" {
		t.Error("CreateTrack should set ID")
	}

	// Read
	retrieved, err := store.GetTrack(gt.ID)
	if err != nil {
		t.Fatalf("GetTrack failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("GetTrack returned nil")
	}
	if retrieved.ObjectType != "person" {
		t.Errorf("ObjectType = %s, want person", retrieved.ObjectType)
	}
	if len(retrieved.DominantColors) != 2 {
		t.Errorf("DominantColors length = %d, want 2", len(retrieved.DominantColors))
	}

	// List active
	active, err := store.ListActiveTracks()
	if err != nil {
		t.Fatalf("ListActiveTracks failed: %v", err)
	}
	if len(active) != 1 {
		t.Errorf("ListActiveTracks returned %d, want 1", len(active))
	}

	// Update
	gt.State = TrackStateTransit
	err = store.UpdateTrack(gt)
	if err != nil {
		t.Fatalf("UpdateTrack failed: %v", err)
	}

	// Delete
	err = store.DeleteTrack(gt.ID)
	if err != nil {
		t.Fatalf("DeleteTrack failed: %v", err)
	}
}

func TestTrackSegmentCRUD(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Create track first
	now := time.Now()
	gt := &GlobalTrack{
		FirstSeen:  now,
		LastSeen:   now,
		ObjectType: "person",
		State:      TrackStateActive,
	}
	_ = store.CreateTrack(gt)

	// Create segment
	ts := &TrackSegment{
		GlobalTrackID: gt.ID,
		CameraID:      "cam-1",
		LocalTrackID:  "local-1",
		EnteredAt:     now,
		BoundingBoxes: []BoundingBoxSample{
			{Timestamp: now, X: 0.5, Y: 0.5, Width: 0.1, Height: 0.2},
		},
	}

	err := store.CreateSegment(ts)
	if err != nil {
		t.Fatalf("CreateSegment failed: %v", err)
	}

	// List by track
	segments, err := store.ListSegmentsByTrack(gt.ID)
	if err != nil {
		t.Fatalf("ListSegmentsByTrack failed: %v", err)
	}
	if len(segments) != 1 {
		t.Errorf("ListSegmentsByTrack returned %d, want 1", len(segments))
	}
	if len(segments[0].BoundingBoxes) != 1 {
		t.Errorf("BoundingBoxes length = %d, want 1", len(segments[0].BoundingBoxes))
	}

	// Update segment
	exitTime := now.Add(10 * time.Second)
	ts.ExitedAt = &exitTime
	ts.ExitDirection = EdgeRight
	ts.ExitPosition = &Point{X: 1.0, Y: 0.5}

	err = store.UpdateSegment(ts)
	if err != nil {
		t.Fatalf("UpdateSegment failed: %v", err)
	}

	segments, _ = store.ListSegmentsByTrack(gt.ID)
	if segments[0].ExitDirection != EdgeRight {
		t.Errorf("ExitDirection = %s, want right", segments[0].ExitDirection)
	}
}

func TestPendingHandoffCRUD(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Create track first
	now := time.Now()
	gt := &GlobalTrack{
		FirstSeen:  now,
		LastSeen:   now,
		ObjectType: "person",
		State:      TrackStateTransit,
	}
	_ = store.CreateTrack(gt)

	// Create pending handoff
	ph := &PendingHandoff{
		GlobalTrackID:  gt.ID,
		FromCameraID:   "cam-1",
		ToCameraIDs:    []string{"cam-2", "cam-3"},
		TransitionType: TransitionGap,
		ExitedAt:       now,
		ExpectedBy:     now.Add(10 * time.Second),
		ExitDirection:  EdgeRight,
		ExitPosition:   Point{X: 1.0, Y: 0.5},
		DominantColors: []string{"red"},
	}

	err := store.CreatePendingHandoff(ph)
	if err != nil {
		t.Fatalf("CreatePendingHandoff failed: %v", err)
	}

	// Get
	retrieved, err := store.GetPendingHandoff(ph.ID)
	if err != nil {
		t.Fatalf("GetPendingHandoff failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("GetPendingHandoff returned nil")
	}
	if len(retrieved.ToCameraIDs) != 2 {
		t.Errorf("ToCameraIDs length = %d, want 2", len(retrieved.ToCameraIDs))
	}

	// List pending
	pending, err := store.ListPendingHandoffs()
	if err != nil {
		t.Fatalf("ListPendingHandoffs failed: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("ListPendingHandoffs returned %d, want 1", len(pending))
	}

	// List for camera
	forCamera, err := store.ListPendingHandoffsForCamera("cam-2")
	if err != nil {
		t.Fatalf("ListPendingHandoffsForCamera failed: %v", err)
	}
	if len(forCamera) != 1 {
		t.Errorf("ListPendingHandoffsForCamera returned %d, want 1", len(forCamera))
	}

	// Delete
	err = store.DeletePendingHandoff(ph.ID)
	if err != nil {
		t.Fatalf("DeletePendingHandoff failed: %v", err)
	}
}

func TestAutoDetectTransitions(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Create a map
	m := &SpatialMap{Name: "Test Map", Width: 200, Height: 200, Scale: 1.0}
	_ = store.CreateMap(m)

	// Create two camera placements that overlap
	cp1 := &CameraPlacement{
		CameraID: "cam-1",
		MapID:    m.ID,
		Position: Point{X: 50, Y: 100},
		Rotation: 0, // Pointing right
		FOVAngle: 90,
		FOVDepth: 80,
	}
	_ = store.CreatePlacement(cp1)

	cp2 := &CameraPlacement{
		CameraID: "cam-2",
		MapID:    m.ID,
		Position: Point{X: 100, Y: 100},
		Rotation: 180, // Pointing left
		FOVAngle: 90,
		FOVDepth: 80,
	}
	_ = store.CreatePlacement(cp2)

	// Auto-detect transitions
	transitions, err := store.AutoDetectTransitions(context.Background(), m.ID)
	if err != nil {
		t.Fatalf("AutoDetectTransitions failed: %v", err)
	}
	if len(transitions) != 1 {
		t.Errorf("AutoDetectTransitions returned %d transitions, want 1", len(transitions))
	}

	// These cameras should overlap
	if transitions[0].Type != TransitionOverlap {
		t.Errorf("Expected overlap transition, got %s", transitions[0].Type)
	}
}

func TestGetAnalytics(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Create some data
	now := time.Now()
	gt := &GlobalTrack{
		FirstSeen:  now,
		LastSeen:   now,
		ObjectType: "person",
		State:      TrackStateActive,
	}
	_ = store.CreateTrack(gt)

	ct := &CameraTransition{
		FromCameraID:  "cam-1",
		ToCameraID:    "cam-2",
		Type:          TransitionGap,
		Bidirectional: true,
	}
	_ = store.CreateTransition(ct)
	_ = store.RecordHandoff(ct.ID, 5.0, true)

	// Get analytics
	analytics, err := store.GetAnalytics()
	if err != nil {
		t.Fatalf("GetAnalytics failed: %v", err)
	}

	if analytics.TotalTracks != 1 {
		t.Errorf("TotalTracks = %d, want 1", analytics.TotalTracks)
	}
	if analytics.ActiveTracks != 1 {
		t.Errorf("ActiveTracks = %d, want 1", analytics.ActiveTracks)
	}
	if analytics.TotalHandoffs != 1 {
		t.Errorf("TotalHandoffs = %d, want 1", analytics.TotalHandoffs)
	}
	if analytics.OverallSuccessRate != 1.0 {
		t.Errorf("OverallSuccessRate = %f, want 1.0", analytics.OverallSuccessRate)
	}
}
