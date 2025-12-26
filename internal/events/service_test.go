package events

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Spatial-NVR/SpatialNVR/internal/database"
)

func setupTestDB(t *testing.T) *database.DB {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := database.Open(&database.Config{Path: dbPath})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Create events table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS events (
			id TEXT PRIMARY KEY,
			camera_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			label TEXT,
			timestamp INTEGER NOT NULL,
			end_timestamp INTEGER,
			confidence REAL,
			thumbnail_path TEXT,
			video_segment_id TEXT,
			metadata TEXT,
			acknowledged INTEGER DEFAULT 0,
			tags TEXT,
			notes TEXT,
			created_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create events table: %v", err)
	}

	return db
}

func TestNewService(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	service := NewService(db)
	if service == nil {
		t.Fatal("NewService returned nil")
	}

	if service.db != db {
		t.Error("Service db not set correctly")
	}
}

func TestCreate(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	service := NewService(db)

	event := &Event{
		CameraID:   "cam1",
		EventType:  EventPerson,
		Label:      "person",
		Timestamp:  time.Now(),
		Confidence: 0.95,
	}

	err := service.Create(context.Background(), event)
	if err != nil {
		t.Fatalf("Failed to create event: %v", err)
	}

	if event.ID == "" {
		t.Error("Event ID should be generated")
	}

	if event.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestGet(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	service := NewService(db)

	// Create an event first
	event := &Event{
		CameraID:   "cam1",
		EventType:  EventMotion,
		Label:      "motion",
		Timestamp:  time.Now(),
		Confidence: 0.8,
	}
	service.Create(context.Background(), event)

	// Get the event
	retrieved, err := service.Get(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("Failed to get event: %v", err)
	}

	if retrieved.ID != event.ID {
		t.Errorf("Expected ID %s, got %s", event.ID, retrieved.ID)
	}

	if retrieved.CameraID != event.CameraID {
		t.Errorf("Expected camera_id %s, got %s", event.CameraID, retrieved.CameraID)
	}

	if retrieved.EventType != event.EventType {
		t.Errorf("Expected event_type %s, got %s", event.EventType, retrieved.EventType)
	}
}

func TestGetNotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	service := NewService(db)

	_, err := service.Get(context.Background(), "nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent event")
	}
}

func TestList(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	service := NewService(db)

	// Create multiple events
	for i := 0; i < 5; i++ {
		event := &Event{
			CameraID:   "cam1",
			EventType:  EventPerson,
			Timestamp:  time.Now().Add(-time.Duration(i) * time.Minute),
			Confidence: 0.9,
		}
		service.Create(context.Background(), event)
	}

	// List events
	events, total, err := service.List(context.Background(), ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("Failed to list events: %v", err)
	}

	if len(events) != 5 {
		t.Errorf("Expected 5 events, got %d", len(events))
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
}

func TestListWithFilters(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	service := NewService(db)

	// Create events with different cameras and types
	events := []*Event{
		{CameraID: "cam1", EventType: EventPerson, Timestamp: time.Now()},
		{CameraID: "cam1", EventType: EventVehicle, Timestamp: time.Now()},
		{CameraID: "cam2", EventType: EventPerson, Timestamp: time.Now()},
		{CameraID: "cam2", EventType: EventMotion, Timestamp: time.Now()},
	}
	for _, e := range events {
		e.Confidence = 0.9
		service.Create(context.Background(), e)
	}

	// Filter by camera
	result, _, _ := service.List(context.Background(), ListOptions{
		CameraID: "cam1",
		Limit:    10,
	})
	if len(result) != 2 {
		t.Errorf("Expected 2 events for cam1, got %d", len(result))
	}

	// Filter by event type
	result, _, _ = service.List(context.Background(), ListOptions{
		EventType: EventPerson,
		Limit:     10,
	})
	if len(result) != 2 {
		t.Errorf("Expected 2 person events, got %d", len(result))
	}

	// Filter by both
	result, _, _ = service.List(context.Background(), ListOptions{
		CameraID:  "cam1",
		EventType: EventPerson,
		Limit:     10,
	})
	if len(result) != 1 {
		t.Errorf("Expected 1 event for cam1 + person, got %d", len(result))
	}
}

func TestListPagination(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	service := NewService(db)

	// Create 20 events
	for i := 0; i < 20; i++ {
		event := &Event{
			CameraID:   "cam1",
			EventType:  EventMotion,
			Timestamp:  time.Now().Add(-time.Duration(i) * time.Minute),
			Confidence: 0.9,
		}
		service.Create(context.Background(), event)
	}

	// Get first page
	page1, total, _ := service.List(context.Background(), ListOptions{
		Limit:  5,
		Offset: 0,
	})

	if len(page1) != 5 {
		t.Errorf("Expected 5 events on page 1, got %d", len(page1))
	}

	if total != 20 {
		t.Errorf("Expected total 20, got %d", total)
	}

	// Get second page
	page2, _, _ := service.List(context.Background(), ListOptions{
		Limit:  5,
		Offset: 5,
	})

	if len(page2) != 5 {
		t.Errorf("Expected 5 events on page 2, got %d", len(page2))
	}

	// Verify pages are different (only if both have content)
	if len(page1) > 0 && len(page2) > 0 {
		if page1[0].ID == page2[0].ID {
			t.Error("Page 1 and 2 should have different events")
		}
	}
}

func TestAcknowledge(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	service := NewService(db)

	event := &Event{
		CameraID:   "cam1",
		EventType:  EventPerson,
		Timestamp:  time.Now(),
		Confidence: 0.9,
	}
	service.Create(context.Background(), event)

	// Verify not acknowledged initially
	retrieved, _ := service.Get(context.Background(), event.ID)
	if retrieved.Acknowledged {
		t.Error("Event should not be acknowledged initially")
	}

	// Acknowledge
	err := service.Acknowledge(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("Failed to acknowledge: %v", err)
	}

	// Verify acknowledged
	retrieved, _ = service.Get(context.Background(), event.ID)
	if !retrieved.Acknowledged {
		t.Error("Event should be acknowledged")
	}
}

func TestSubscribe(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	service := NewService(db)

	// Subscribe
	ch := service.Subscribe()
	if ch == nil {
		t.Fatal("Subscribe returned nil channel")
	}

	// Create an event
	event := &Event{
		CameraID:   "cam1",
		EventType:  EventPerson,
		Timestamp:  time.Now(),
		Confidence: 0.9,
	}

	// Start goroutine to receive
	received := make(chan *Event, 1)
	go func() {
		select {
		case e := <-ch:
			received <- e
		case <-time.After(time.Second):
			received <- nil
		}
	}()

	// Create event
	service.Create(context.Background(), event)

	// Wait for notification
	result := <-received
	if result == nil {
		t.Error("Did not receive event notification")
	} else if result.ID != event.ID {
		t.Errorf("Received wrong event: expected %s, got %s", event.ID, result.ID)
	}

	// Unsubscribe
	service.Unsubscribe(ch)
}

func TestGetStats(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	service := NewService(db)

	// Create events
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// Today's events
	for i := 0; i < 3; i++ {
		event := &Event{
			CameraID:     "cam1",
			EventType:    EventPerson,
			Timestamp:    todayStart.Add(time.Duration(i) * time.Hour),
			Confidence:   0.9,
			Acknowledged: i%2 == 0, // Some acknowledged, some not
		}
		service.Create(context.Background(), event)
	}

	// Yesterday's event
	event := &Event{
		CameraID:   "cam1",
		EventType:  EventPerson,
		Timestamp:  todayStart.Add(-24 * time.Hour),
		Confidence: 0.9,
	}
	service.Create(context.Background(), event)

	stats, err := service.GetStats(context.Background(), "")
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}

	today := stats["today"].(int)
	if today != 3 {
		t.Errorf("Expected 3 events today, got %d", today)
	}

	unack := stats["unacknowledged"].(int)
	if unack < 1 {
		t.Errorf("Expected at least 1 unacknowledged event, got %d", unack)
	}
}

func TestEventTypes(t *testing.T) {
	eventTypes := []EventType{
		EventMotion,
		EventPerson,
		EventVehicle,
		EventAnimal,
		EventFace,
		EventLPR,
		EventAudio,
		EventStateChange,
		EventDoorbell,
	}

	for _, et := range eventTypes {
		if et == "" {
			t.Error("Event type should not be empty")
		}
	}
}

// ====================
// Motion Zone Tests
// ====================

func setupTestDBWithZones(t *testing.T) *database.DB {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := database.Open(&database.Config{Path: dbPath})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Create events table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS events (
			id TEXT PRIMARY KEY,
			camera_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			label TEXT,
			timestamp INTEGER NOT NULL,
			end_timestamp INTEGER,
			confidence REAL,
			thumbnail_path TEXT,
			video_segment_id TEXT,
			metadata TEXT,
			acknowledged INTEGER DEFAULT 0,
			tags TEXT,
			notes TEXT,
			created_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create events table: %v", err)
	}

	// Create motion_zones table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS motion_zones (
			id TEXT PRIMARY KEY,
			camera_id TEXT NOT NULL,
			name TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			points TEXT NOT NULL,
			object_types TEXT,
			min_confidence REAL NOT NULL DEFAULT 0.5,
			min_size REAL,
			sensitivity INTEGER NOT NULL DEFAULT 5,
			cooldown_seconds INTEGER NOT NULL DEFAULT 30,
			notifications INTEGER NOT NULL DEFAULT 1,
			recording INTEGER NOT NULL DEFAULT 1,
			color TEXT,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create motion_zones table: %v", err)
	}

	return db
}

func TestCreateZone(t *testing.T) {
	db := setupTestDBWithZones(t)
	defer db.Close()

	service := NewService(db)

	zone := &MotionZone{
		CameraID:      "cam1",
		Name:          "Front Door",
		Enabled:       true,
		Points:        []Point{{X: 0.1, Y: 0.1}, {X: 0.9, Y: 0.1}, {X: 0.9, Y: 0.9}, {X: 0.1, Y: 0.9}},
		ObjectTypes:   []string{"person", "vehicle"},
		MinConfidence: 0.7,
		Sensitivity:   5,
		Cooldown:      30,
		Notifications: true,
		Recording:     true,
		Color:         "#FF0000",
	}

	err := service.CreateZone(context.Background(), zone)
	if err != nil {
		t.Fatalf("Failed to create zone: %v", err)
	}

	if zone.ID == "" {
		t.Error("Zone ID should be generated")
	}

	if zone.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestGetZone(t *testing.T) {
	db := setupTestDBWithZones(t)
	defer db.Close()

	service := NewService(db)

	// Create a zone first
	zone := &MotionZone{
		CameraID:      "cam1",
		Name:          "Backyard",
		Enabled:       true,
		Points:        []Point{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}},
		MinConfidence: 0.6,
		Sensitivity:   7,
		Cooldown:      60,
	}
	service.CreateZone(context.Background(), zone)

	// Get the zone
	retrieved, err := service.GetZone(context.Background(), zone.ID)
	if err != nil {
		t.Fatalf("Failed to get zone: %v", err)
	}

	if retrieved.ID != zone.ID {
		t.Errorf("Expected ID %s, got %s", zone.ID, retrieved.ID)
	}

	if retrieved.Name != zone.Name {
		t.Errorf("Expected name %s, got %s", zone.Name, retrieved.Name)
	}

	if len(retrieved.Points) != len(zone.Points) {
		t.Errorf("Expected %d points, got %d", len(zone.Points), len(retrieved.Points))
	}

	if retrieved.Sensitivity != zone.Sensitivity {
		t.Errorf("Expected sensitivity %d, got %d", zone.Sensitivity, retrieved.Sensitivity)
	}
}

func TestGetZoneNotFound(t *testing.T) {
	db := setupTestDBWithZones(t)
	defer db.Close()

	service := NewService(db)

	_, err := service.GetZone(context.Background(), "nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent zone")
	}
}

func TestListZones(t *testing.T) {
	db := setupTestDBWithZones(t)
	defer db.Close()

	service := NewService(db)

	// Create zones for multiple cameras
	zones := []*MotionZone{
		{CameraID: "cam1", Name: "Zone 1", Points: []Point{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0.5, Y: 1}}, Sensitivity: 5, MinConfidence: 0.5, Cooldown: 30},
		{CameraID: "cam1", Name: "Zone 2", Points: []Point{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0.5, Y: 1}}, Sensitivity: 5, MinConfidence: 0.5, Cooldown: 30},
		{CameraID: "cam2", Name: "Zone 3", Points: []Point{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0.5, Y: 1}}, Sensitivity: 5, MinConfidence: 0.5, Cooldown: 30},
	}
	for _, z := range zones {
		service.CreateZone(context.Background(), z)
	}

	// List all zones
	allZones, err := service.ListZones(context.Background(), "")
	if err != nil {
		t.Fatalf("Failed to list zones: %v", err)
	}
	if len(allZones) != 3 {
		t.Errorf("Expected 3 zones, got %d", len(allZones))
	}

	// List zones for cam1 only
	cam1Zones, err := service.ListZones(context.Background(), "cam1")
	if err != nil {
		t.Fatalf("Failed to list zones for cam1: %v", err)
	}
	if len(cam1Zones) != 2 {
		t.Errorf("Expected 2 zones for cam1, got %d", len(cam1Zones))
	}
}

func TestUpdateZone(t *testing.T) {
	db := setupTestDBWithZones(t)
	defer db.Close()

	service := NewService(db)

	// Create a zone
	zone := &MotionZone{
		CameraID:      "cam1",
		Name:          "Original Name",
		Enabled:       true,
		Points:        []Point{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0.5, Y: 1}},
		MinConfidence: 0.5,
		Sensitivity:   5,
		Cooldown:      30,
	}
	service.CreateZone(context.Background(), zone)

	// Update the zone
	zone.Name = "Updated Name"
	zone.Sensitivity = 8
	zone.Enabled = false

	err := service.UpdateZone(context.Background(), zone)
	if err != nil {
		t.Fatalf("Failed to update zone: %v", err)
	}

	// Verify updates
	retrieved, _ := service.GetZone(context.Background(), zone.ID)
	if retrieved.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", retrieved.Name)
	}
	if retrieved.Sensitivity != 8 {
		t.Errorf("Expected sensitivity 8, got %d", retrieved.Sensitivity)
	}
	if retrieved.Enabled {
		t.Error("Expected zone to be disabled")
	}
}

func TestDeleteZone(t *testing.T) {
	db := setupTestDBWithZones(t)
	defer db.Close()

	service := NewService(db)

	// Create a zone
	zone := &MotionZone{
		CameraID:      "cam1",
		Name:          "To Delete",
		Points:        []Point{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0.5, Y: 1}},
		MinConfidence: 0.5,
		Sensitivity:   5,
		Cooldown:      30,
	}
	service.CreateZone(context.Background(), zone)

	// Delete the zone
	err := service.DeleteZone(context.Background(), zone.ID)
	if err != nil {
		t.Fatalf("Failed to delete zone: %v", err)
	}

	// Verify deletion
	_, err = service.GetZone(context.Background(), zone.ID)
	if err == nil {
		t.Error("Zone should not exist after deletion")
	}
}

func TestDeleteZoneNotFound(t *testing.T) {
	db := setupTestDBWithZones(t)
	defer db.Close()

	service := NewService(db)

	err := service.DeleteZone(context.Background(), "nonexistent")
	if err == nil {
		t.Error("Expected error when deleting non-existent zone")
	}
}

func TestGetEnabledZonesForCamera(t *testing.T) {
	db := setupTestDBWithZones(t)
	defer db.Close()

	service := NewService(db)

	// Create zones with different enabled states
	zones := []*MotionZone{
		{CameraID: "cam1", Name: "Enabled 1", Enabled: true, Points: []Point{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0.5, Y: 1}}, MinConfidence: 0.5, Sensitivity: 5, Cooldown: 30},
		{CameraID: "cam1", Name: "Disabled", Enabled: false, Points: []Point{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0.5, Y: 1}}, MinConfidence: 0.5, Sensitivity: 5, Cooldown: 30},
		{CameraID: "cam1", Name: "Enabled 2", Enabled: true, Points: []Point{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0.5, Y: 1}}, MinConfidence: 0.5, Sensitivity: 5, Cooldown: 30},
	}
	for _, z := range zones {
		service.CreateZone(context.Background(), z)
	}

	// Get enabled zones only
	enabledZones, err := service.GetEnabledZonesForCamera(context.Background(), "cam1")
	if err != nil {
		t.Fatalf("Failed to get enabled zones: %v", err)
	}

	if len(enabledZones) != 2 {
		t.Errorf("Expected 2 enabled zones, got %d", len(enabledZones))
	}

	for _, z := range enabledZones {
		if !z.Enabled {
			t.Error("GetEnabledZonesForCamera returned a disabled zone")
		}
	}
}

func TestDelete(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	service := NewService(db)

	event := &Event{
		CameraID:   "cam1",
		EventType:  EventPerson,
		Timestamp:  time.Now(),
		Confidence: 0.9,
	}
	service.Create(context.Background(), event)

	// Delete the event
	err := service.Delete(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("Failed to delete event: %v", err)
	}

	// Verify deletion
	_, err = service.Get(context.Background(), event.ID)
	if err == nil {
		t.Error("Event should not exist after deletion")
	}
}

func TestCreateMotionEvent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	service := NewService(db)

	event, err := service.CreateMotionEvent(context.Background(), "cam1", 0.85, "/thumbnails/motion1.jpg")
	if err != nil {
		t.Fatalf("Failed to create motion event: %v", err)
	}

	if event.EventType != EventMotion {
		t.Errorf("Expected EventMotion, got %s", event.EventType)
	}
	if event.CameraID != "cam1" {
		t.Errorf("Expected camera_id 'cam1', got %s", event.CameraID)
	}
	if event.Confidence != 0.85 {
		t.Errorf("Expected confidence 0.85, got %f", event.Confidence)
	}
}

func TestCreateDoorbellEvent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	service := NewService(db)

	event, err := service.CreateDoorbellEvent(context.Background(), "cam1", "/thumbnails/doorbell.jpg")
	if err != nil {
		t.Fatalf("Failed to create doorbell event: %v", err)
	}

	if event.EventType != EventDoorbell {
		t.Errorf("Expected EventDoorbell, got %s", event.EventType)
	}
	if event.Confidence != 1.0 {
		t.Errorf("Expected confidence 1.0, got %f", event.Confidence)
	}
}

func TestCreateAudioEvent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	service := NewService(db)

	event, err := service.CreateAudioEvent(context.Background(), "cam1", "glass_break", 0.92)
	if err != nil {
		t.Fatalf("Failed to create audio event: %v", err)
	}

	if event.EventType != EventAudio {
		t.Errorf("Expected EventAudio, got %s", event.EventType)
	}
	if event.Label != "glass_break" {
		t.Errorf("Expected label 'glass_break', got %s", event.Label)
	}
}

func TestListWithTimeFilters(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	service := NewService(db)

	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)
	twoDaysAgo := now.Add(-48 * time.Hour)

	// Create events at different times
	events := []*Event{
		{CameraID: "cam1", EventType: EventPerson, Timestamp: twoDaysAgo, Confidence: 0.9},
		{CameraID: "cam1", EventType: EventPerson, Timestamp: yesterday, Confidence: 0.9},
		{CameraID: "cam1", EventType: EventPerson, Timestamp: now, Confidence: 0.9},
	}
	for _, e := range events {
		service.Create(context.Background(), e)
	}

	// Filter by start time
	result, total, _ := service.List(context.Background(), ListOptions{
		StartTime: yesterday.Add(-time.Hour),
		Limit:     10,
	})
	if len(result) != 2 {
		t.Errorf("Expected 2 events after yesterday, got %d", len(result))
	}
	if total != 2 {
		t.Errorf("Expected total 2, got %d", total)
	}
}

func TestEventWithTags(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	service := NewService(db)

	event := &Event{
		CameraID:   "cam1",
		EventType:  EventPerson,
		Timestamp:  time.Now(),
		Confidence: 0.9,
		Tags:       []string{"important", "front_door"},
	}
	service.Create(context.Background(), event)

	retrieved, _ := service.Get(context.Background(), event.ID)
	if len(retrieved.Tags) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(retrieved.Tags))
	}
}

func TestEventWithEndTimestamp(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	service := NewService(db)

	now := time.Now()
	endTime := now.Add(5 * time.Minute)

	event := &Event{
		CameraID:     "cam1",
		EventType:    EventMotion,
		Timestamp:    now,
		EndTimestamp: &endTime,
		Confidence:   0.9,
	}
	service.Create(context.Background(), event)

	retrieved, _ := service.Get(context.Background(), event.ID)
	if retrieved.EndTimestamp == nil {
		t.Error("Expected EndTimestamp to be set")
	}
}

func TestEventWithOptionalFields(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	service := NewService(db)

	event := &Event{
		CameraID:       "cam1",
		EventType:      EventPerson,
		Label:          "person",
		Timestamp:      time.Now(),
		Confidence:     0.95,
		ThumbnailPath:  "/thumb/1.jpg",
		VideoSegmentID: "seg_123",
		Notes:          "Test notes",
	}
	service.Create(context.Background(), event)

	retrieved, _ := service.Get(context.Background(), event.ID)
	if retrieved.ThumbnailPath != "/thumb/1.jpg" {
		t.Errorf("Expected thumbnail path, got %s", retrieved.ThumbnailPath)
	}
	if retrieved.VideoSegmentID != "seg_123" {
		t.Errorf("Expected video segment ID, got %s", retrieved.VideoSegmentID)
	}
	if retrieved.Notes != "Test notes" {
		t.Errorf("Expected notes, got %s", retrieved.Notes)
	}
}

func TestMultipleSubscribers(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	service := NewService(db)

	ch1 := service.Subscribe()
	ch2 := service.Subscribe()

	// Create an event
	event := &Event{
		CameraID:   "cam1",
		EventType:  EventPerson,
		Timestamp:  time.Now(),
		Confidence: 0.9,
	}

	// Track received events
	received := make(chan int, 2)
	go func() {
		select {
		case <-ch1:
			received <- 1
		case <-time.After(time.Second):
		}
	}()
	go func() {
		select {
		case <-ch2:
			received <- 2
		case <-time.After(time.Second):
		}
	}()

	service.Create(context.Background(), event)

	// Wait for all to receive
	count := 0
	for i := 0; i < 2; i++ {
		select {
		case <-received:
			count++
		case <-time.After(2 * time.Second):
		}
	}

	if count != 2 {
		t.Errorf("Expected 2 subscribers to receive, got %d", count)
	}

	service.Unsubscribe(ch1)
	service.Unsubscribe(ch2)
}

func TestGetStatsWithCameraFilter(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	service := NewService(db)

	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// Create events for different cameras
	for i := 0; i < 3; i++ {
		service.Create(context.Background(), &Event{
			CameraID:   "cam1",
			EventType:  EventPerson,
			Timestamp:  todayStart.Add(time.Duration(i) * time.Hour),
			Confidence: 0.9,
		})
	}
	for i := 0; i < 2; i++ {
		service.Create(context.Background(), &Event{
			CameraID:   "cam2",
			EventType:  EventPerson,
			Timestamp:  todayStart.Add(time.Duration(i) * time.Hour),
			Confidence: 0.9,
		})
	}

	// Stats for cam1 only
	stats, _ := service.GetStats(context.Background(), "cam1")
	if stats["total"].(int) != 3 {
		t.Errorf("Expected 3 total for cam1, got %d", stats["total"].(int))
	}

	// Stats for all
	stats, _ = service.GetStats(context.Background(), "")
	if stats["total"].(int) != 5 {
		t.Errorf("Expected 5 total, got %d", stats["total"].(int))
	}
}

func TestZoneWithObjectTypes(t *testing.T) {
	db := setupTestDBWithZones(t)
	defer db.Close()

	service := NewService(db)

	zone := &MotionZone{
		CameraID:      "cam1",
		Name:          "Zone with types",
		Enabled:       true,
		Points:        []Point{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0.5, Y: 1}},
		ObjectTypes:   []string{"person", "vehicle", "animal"},
		MinConfidence: 0.7,
		Sensitivity:   5,
		Cooldown:      30,
	}
	service.CreateZone(context.Background(), zone)

	retrieved, _ := service.GetZone(context.Background(), zone.ID)
	if len(retrieved.ObjectTypes) != 3 {
		t.Errorf("Expected 3 object types, got %d", len(retrieved.ObjectTypes))
	}
}

func TestZoneWithAllFields(t *testing.T) {
	db := setupTestDBWithZones(t)
	defer db.Close()

	service := NewService(db)

	zone := &MotionZone{
		CameraID:      "cam1",
		Name:          "Full zone",
		Enabled:       true,
		Points:        []Point{{X: 0.1, Y: 0.1}, {X: 0.9, Y: 0.1}, {X: 0.9, Y: 0.9}, {X: 0.1, Y: 0.9}},
		ObjectTypes:   []string{"person"},
		MinConfidence: 0.75,
		MinSize:       0.05,
		Sensitivity:   7,
		Cooldown:      60,
		Notifications: true,
		Recording:     true,
		Color:         "#00FF00",
	}
	service.CreateZone(context.Background(), zone)

	retrieved, _ := service.GetZone(context.Background(), zone.ID)
	if retrieved.MinSize != 0.05 {
		t.Errorf("Expected MinSize 0.05, got %f", retrieved.MinSize)
	}
	if retrieved.Color != "#00FF00" {
		t.Errorf("Expected color '#00FF00', got %s", retrieved.Color)
	}
	if !retrieved.Notifications {
		t.Error("Expected Notifications true")
	}
	if !retrieved.Recording {
		t.Error("Expected Recording true")
	}
}

func TestAllEventTypes(t *testing.T) {
	types := []EventType{
		EventMotion,
		EventPerson,
		EventVehicle,
		EventAnimal,
		EventFace,
		EventLPR,
		EventAudio,
		EventStateChange,
		EventDoorbell,
		EventLineCross,
		EventZoneEnter,
		EventZoneExit,
		EventTamper,
	}

	for _, et := range types {
		if string(et) == "" {
			t.Errorf("Event type %v should not be empty", et)
		}
	}

	if EventMotion != "motion" {
		t.Errorf("Expected 'motion', got '%s'", EventMotion)
	}
}
