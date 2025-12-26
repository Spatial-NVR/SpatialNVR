package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/Spatial-NVR/SpatialNVR/internal/database"
	"github.com/Spatial-NVR/SpatialNVR/internal/events"
)

// setupZoneTest creates a test environment for zone tests
func setupZoneTest(t *testing.T) (*ZoneHandler, *events.Service, func()) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	// Create database
	db, err := database.Open(&database.Config{Path: dbPath})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
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

	// Create events service
	eventService := events.NewService(db)

	// Create handler
	handler := NewZoneHandler(eventService)

	cleanup := func() {
		db.Close()
	}

	return handler, eventService, cleanup
}

func TestZoneHandler_Create(t *testing.T) {
	handler, _, cleanup := setupZoneTest(t)
	defer cleanup()

	tests := []struct {
		name       string
		body       map[string]interface{}
		wantStatus int
		wantError  bool
	}{
		{
			name: "valid zone",
			body: map[string]interface{}{
				"camera_id": "cam_123",
				"name":      "Front Porch",
				"enabled":   true,
				"points": []map[string]float64{
					{"x": 0.1, "y": 0.1},
					{"x": 0.9, "y": 0.1},
					{"x": 0.9, "y": 0.9},
					{"x": 0.1, "y": 0.9},
				},
				"sensitivity":      5,
				"min_confidence":   0.5,
				"cooldown_seconds": 30,
				"notifications":    true,
				"recording":        true,
				"color":            "#ef4444",
			},
			wantStatus: http.StatusCreated,
			wantError:  false,
		},
		{
			name: "missing camera_id",
			body: map[string]interface{}{
				"name":    "Test Zone",
				"enabled": true,
				"points": []map[string]float64{
					{"x": 0.1, "y": 0.1},
					{"x": 0.9, "y": 0.1},
					{"x": 0.9, "y": 0.9},
				},
			},
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name: "missing name",
			body: map[string]interface{}{
				"camera_id": "cam_123",
				"enabled":   true,
				"points": []map[string]float64{
					{"x": 0.1, "y": 0.1},
					{"x": 0.9, "y": 0.1},
					{"x": 0.9, "y": 0.9},
				},
			},
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name: "too few points",
			body: map[string]interface{}{
				"camera_id": "cam_123",
				"name":      "Test Zone",
				"enabled":   true,
				"points": []map[string]float64{
					{"x": 0.1, "y": 0.1},
					{"x": 0.9, "y": 0.1},
				},
			},
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name: "invalid sensitivity too high",
			body: map[string]interface{}{
				"camera_id":   "cam_123",
				"name":        "Test Zone",
				"enabled":     true,
				"sensitivity": 15,
				"points": []map[string]float64{
					{"x": 0.1, "y": 0.1},
					{"x": 0.9, "y": 0.1},
					{"x": 0.9, "y": 0.9},
				},
			},
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name: "invalid sensitivity too low",
			body: map[string]interface{}{
				"camera_id":   "cam_123",
				"name":        "Test Zone",
				"enabled":     true,
				"sensitivity": 0,
				"points": []map[string]float64{
					{"x": 0.1, "y": 0.1},
					{"x": 0.9, "y": 0.1},
					{"x": 0.9, "y": 0.9},
				},
			},
			// Sensitivity 0 gets default of 5, so this should succeed
			wantStatus: http.StatusCreated,
			wantError:  false,
		},
		{
			name: "zone with object types",
			body: map[string]interface{}{
				"camera_id":    "cam_123",
				"name":         "Driveway",
				"enabled":      true,
				"object_types": []string{"person", "car"},
				"points": []map[string]float64{
					{"x": 0.0, "y": 0.5},
					{"x": 0.5, "y": 0.0},
					{"x": 1.0, "y": 0.5},
					{"x": 0.5, "y": 1.0},
				},
				"sensitivity":      7,
				"min_confidence":   0.7,
				"cooldown_seconds": 60,
			},
			wantStatus: http.StatusCreated,
			wantError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest("POST", "/zones", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.Create(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.wantStatus, w.Code, w.Body.String())
			}

			var resp Response
			json.Unmarshal(w.Body.Bytes(), &resp)

			if tt.wantError && resp.Success {
				t.Error("Expected error response, got success")
			}
			if !tt.wantError && !resp.Success {
				t.Errorf("Expected success, got error: %v", resp.Error)
			}
		})
	}
}

func TestZoneHandler_Get(t *testing.T) {
	handler, eventService, cleanup := setupZoneTest(t)
	defer cleanup()

	// Create a zone first
	zone := &events.MotionZone{
		CameraID:      "cam_123",
		Name:          "Test Zone",
		Enabled:       true,
		Points:        []events.Point{{X: 0.1, Y: 0.1}, {X: 0.9, Y: 0.1}, {X: 0.5, Y: 0.9}},
		Sensitivity:   5,
		MinConfidence: 0.5,
		Cooldown:      30,
	}
	if err := eventService.CreateZone(context.Background(), zone); err != nil {
		t.Fatalf("Failed to create test zone: %v", err)
	}

	// Test getting the zone
	r := chi.NewRouter()
	r.Get("/{id}", handler.Get)

	req := httptest.NewRequest("GET", "/"+zone.ID, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var resp Response
	json.Unmarshal(w.Body.Bytes(), &resp)

	if !resp.Success {
		t.Errorf("Expected success, got error: %v", resp.Error)
	}

	// Test getting non-existent zone
	req = httptest.NewRequest("GET", "/nonexistent", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestZoneHandler_List(t *testing.T) {
	handler, eventService, cleanup := setupZoneTest(t)
	defer cleanup()

	// Create zones for different cameras
	zones := []*events.MotionZone{
		{
			CameraID:    "cam_1",
			Name:        "Zone A",
			Enabled:     true,
			Points:      []events.Point{{X: 0.1, Y: 0.1}, {X: 0.9, Y: 0.1}, {X: 0.5, Y: 0.9}},
			Sensitivity: 5,
		},
		{
			CameraID:    "cam_1",
			Name:        "Zone B",
			Enabled:     true,
			Points:      []events.Point{{X: 0.2, Y: 0.2}, {X: 0.8, Y: 0.2}, {X: 0.5, Y: 0.8}},
			Sensitivity: 5,
		},
		{
			CameraID:    "cam_2",
			Name:        "Zone C",
			Enabled:     true,
			Points:      []events.Point{{X: 0.0, Y: 0.0}, {X: 1.0, Y: 0.0}, {X: 0.5, Y: 1.0}},
			Sensitivity: 5,
		},
	}

	for _, z := range zones {
		if err := eventService.CreateZone(context.Background(), z); err != nil {
			t.Fatalf("Failed to create test zone: %v", err)
		}
	}

	// Test listing all zones
	req := httptest.NewRequest("GET", "/zones", nil)
	w := httptest.NewRecorder()
	handler.List(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var resp Response
	json.Unmarshal(w.Body.Bytes(), &resp)

	data := resp.Data.([]interface{})
	if len(data) != 3 {
		t.Errorf("Expected 3 zones, got %d", len(data))
	}

	// Test filtering by camera_id
	req = httptest.NewRequest("GET", "/zones?camera_id=cam_1", nil)
	w = httptest.NewRecorder()
	handler.List(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	json.Unmarshal(w.Body.Bytes(), &resp)
	data = resp.Data.([]interface{})
	if len(data) != 2 {
		t.Errorf("Expected 2 zones for cam_1, got %d", len(data))
	}
}

func TestZoneHandler_Update(t *testing.T) {
	handler, eventService, cleanup := setupZoneTest(t)
	defer cleanup()

	// Create a zone first
	zone := &events.MotionZone{
		CameraID:      "cam_123",
		Name:          "Original Name",
		Enabled:       true,
		Points:        []events.Point{{X: 0.1, Y: 0.1}, {X: 0.9, Y: 0.1}, {X: 0.5, Y: 0.9}},
		Sensitivity:   5,
		MinConfidence: 0.5,
		Cooldown:      30,
		Notifications: false,
		Recording:     false,
	}
	if err := eventService.CreateZone(context.Background(), zone); err != nil {
		t.Fatalf("Failed to create test zone: %v", err)
	}

	r := chi.NewRouter()
	r.Put("/{id}", handler.Update)

	// Test updating the zone
	updateBody := map[string]interface{}{
		"name":    "Updated Name",
		"enabled": false,
		"points": []map[string]float64{
			{"x": 0.2, "y": 0.2},
			{"x": 0.8, "y": 0.2},
			{"x": 0.8, "y": 0.8},
			{"x": 0.2, "y": 0.8},
		},
		"sensitivity":      8,
		"min_confidence":   0.7,
		"cooldown_seconds": 60,
		"notifications":    true,
		"recording":        true,
	}
	body, _ := json.Marshal(updateBody)
	req := httptest.NewRequest("PUT", "/"+zone.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Verify the update
	updated, err := eventService.GetZone(context.Background(), zone.ID)
	if err != nil {
		t.Fatalf("Failed to get updated zone: %v", err)
	}

	if updated.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", updated.Name)
	}
	if updated.Enabled {
		t.Error("Expected zone to be disabled")
	}
	if updated.Sensitivity != 8 {
		t.Errorf("Expected sensitivity 8, got %d", updated.Sensitivity)
	}
	if !updated.Notifications {
		t.Error("Expected notifications to be enabled")
	}
	if len(updated.Points) != 4 {
		t.Errorf("Expected 4 points, got %d", len(updated.Points))
	}
}

func TestZoneHandler_Delete(t *testing.T) {
	handler, eventService, cleanup := setupZoneTest(t)
	defer cleanup()

	// Create a zone first
	zone := &events.MotionZone{
		CameraID:    "cam_123",
		Name:        "To Delete",
		Enabled:     true,
		Points:      []events.Point{{X: 0.1, Y: 0.1}, {X: 0.9, Y: 0.1}, {X: 0.5, Y: 0.9}},
		Sensitivity: 5,
	}
	if err := eventService.CreateZone(context.Background(), zone); err != nil {
		t.Fatalf("Failed to create test zone: %v", err)
	}

	r := chi.NewRouter()
	r.Delete("/{id}", handler.Delete)

	// Test deleting the zone
	req := httptest.NewRequest("DELETE", "/"+zone.ID, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", w.Code)
	}

	// Verify it's gone
	_, err := eventService.GetZone(context.Background(), zone.ID)
	if err == nil {
		t.Error("Expected error getting deleted zone, got none")
	}

	// Test deleting non-existent zone
	req = httptest.NewRequest("DELETE", "/nonexistent", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestZoneHandler_InvalidJSON(t *testing.T) {
	handler, _, cleanup := setupZoneTest(t)
	defer cleanup()

	req := httptest.NewRequest("POST", "/zones", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestNewZoneHandler(t *testing.T) {
	handler := NewZoneHandler(nil)
	if handler == nil {
		t.Error("NewZoneHandler returned nil")
	}
}

func TestZoneHandler_Routes(t *testing.T) {
	handler := &ZoneHandler{}
	router := handler.Routes()

	if router == nil {
		t.Error("Routes() returned nil")
	}
}

func TestZoneHandler_Update_InvalidJSON(t *testing.T) {
	handler, eventService, cleanup := setupZoneTest(t)
	defer cleanup()

	// Create a zone first
	zone := &events.MotionZone{
		CameraID:    "cam_123",
		Name:        "Test Zone",
		Enabled:     true,
		Points:      []events.Point{{X: 0.1, Y: 0.1}, {X: 0.9, Y: 0.1}, {X: 0.5, Y: 0.9}},
		Sensitivity: 5,
	}
	if err := eventService.CreateZone(context.Background(), zone); err != nil {
		t.Fatalf("Failed to create test zone: %v", err)
	}

	r := chi.NewRouter()
	r.Put("/{id}", handler.Update)

	// Test with invalid JSON
	req := httptest.NewRequest("PUT", "/"+zone.ID, bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestZoneHandler_Update_NotFound(t *testing.T) {
	handler, _, cleanup := setupZoneTest(t)
	defer cleanup()

	r := chi.NewRouter()
	r.Put("/{id}", handler.Update)

	updateBody := map[string]interface{}{
		"name":        "Updated Name",
		"sensitivity": 5,
		"points": []map[string]float64{
			{"x": 0.1, "y": 0.1},
			{"x": 0.9, "y": 0.1},
			{"x": 0.5, "y": 0.9},
		},
	}
	body, _ := json.Marshal(updateBody)
	req := httptest.NewRequest("PUT", "/nonexistent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestZoneHandler_Update_MissingName(t *testing.T) {
	handler, eventService, cleanup := setupZoneTest(t)
	defer cleanup()

	zone := &events.MotionZone{
		CameraID:    "cam_123",
		Name:        "Test Zone",
		Enabled:     true,
		Points:      []events.Point{{X: 0.1, Y: 0.1}, {X: 0.9, Y: 0.1}, {X: 0.5, Y: 0.9}},
		Sensitivity: 5,
	}
	if err := eventService.CreateZone(context.Background(), zone); err != nil {
		t.Fatalf("Failed to create test zone: %v", err)
	}

	r := chi.NewRouter()
	r.Put("/{id}", handler.Update)

	updateBody := map[string]interface{}{
		"sensitivity": 5,
		"points": []map[string]float64{
			{"x": 0.1, "y": 0.1},
			{"x": 0.9, "y": 0.1},
			{"x": 0.5, "y": 0.9},
		},
	}
	body, _ := json.Marshal(updateBody)
	req := httptest.NewRequest("PUT", "/"+zone.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestZoneHandler_Update_TooFewPoints(t *testing.T) {
	handler, eventService, cleanup := setupZoneTest(t)
	defer cleanup()

	zone := &events.MotionZone{
		CameraID:    "cam_123",
		Name:        "Test Zone",
		Enabled:     true,
		Points:      []events.Point{{X: 0.1, Y: 0.1}, {X: 0.9, Y: 0.1}, {X: 0.5, Y: 0.9}},
		Sensitivity: 5,
	}
	if err := eventService.CreateZone(context.Background(), zone); err != nil {
		t.Fatalf("Failed to create test zone: %v", err)
	}

	r := chi.NewRouter()
	r.Put("/{id}", handler.Update)

	updateBody := map[string]interface{}{
		"name":        "Updated",
		"sensitivity": 5,
		"points": []map[string]float64{
			{"x": 0.1, "y": 0.1},
			{"x": 0.9, "y": 0.1},
		},
	}
	body, _ := json.Marshal(updateBody)
	req := httptest.NewRequest("PUT", "/"+zone.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestZoneHandler_Update_InvalidSensitivity(t *testing.T) {
	handler, eventService, cleanup := setupZoneTest(t)
	defer cleanup()

	zone := &events.MotionZone{
		CameraID:    "cam_123",
		Name:        "Test Zone",
		Enabled:     true,
		Points:      []events.Point{{X: 0.1, Y: 0.1}, {X: 0.9, Y: 0.1}, {X: 0.5, Y: 0.9}},
		Sensitivity: 5,
	}
	if err := eventService.CreateZone(context.Background(), zone); err != nil {
		t.Fatalf("Failed to create test zone: %v", err)
	}

	r := chi.NewRouter()
	r.Put("/{id}", handler.Update)

	updateBody := map[string]interface{}{
		"name":        "Updated",
		"sensitivity": 15, // Out of range
		"points": []map[string]float64{
			{"x": 0.1, "y": 0.1},
			{"x": 0.9, "y": 0.1},
			{"x": 0.5, "y": 0.9},
		},
	}
	body, _ := json.Marshal(updateBody)
	req := httptest.NewRequest("PUT", "/"+zone.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}
