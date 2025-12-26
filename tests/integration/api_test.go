// Package integration provides integration tests for the NVR system
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/Spatial-NVR/SpatialNVR/internal/api"
	"github.com/Spatial-NVR/SpatialNVR/internal/camera"
	"github.com/Spatial-NVR/SpatialNVR/internal/config"
	"github.com/Spatial-NVR/SpatialNVR/internal/database"
	"github.com/Spatial-NVR/SpatialNVR/internal/events"
)

// TestEnv holds the test environment
type TestEnv struct {
	DB            *database.DB
	Config        *config.Config
	CameraService *camera.Service
	EventService  *events.Service
	Router        chi.Router
	Server        *httptest.Server
	TmpDir        string
}

// SetupTestEnv creates a test environment
func SetupTestEnv(t *testing.T) *TestEnv {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	configPath := filepath.Join(tmpDir, "config.yaml")
	storagePath := filepath.Join(tmpDir, "storage")

	// Create storage directory
	if err := os.MkdirAll(storagePath, 0755); err != nil {
		t.Fatalf("Failed to create storage directory: %v", err)
	}

	// Open database
	db, err := database.Open(&database.Config{Path: dbPath})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Create tables
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS cameras (
			id TEXT PRIMARY KEY,
			status TEXT NOT NULL DEFAULT 'offline',
			last_seen INTEGER,
			fps_current REAL,
			bitrate_current INTEGER,
			resolution_current TEXT,
			stats TEXT,
			error_message TEXT,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create cameras table: %v", err)
	}

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

	// Create config
	cfg := &config.Config{
		Version: "1.0",
		System: config.SystemConfig{
			Name:        "Test NVR",
			Timezone:    "UTC",
			StoragePath: storagePath,
		},
		Cameras: []config.CameraConfig{},
	}
	cfg.SetPath(configPath)

	// Create services
	cameraService := camera.NewService(db, cfg, nil)
	eventService := events.NewService(db)

	// Create router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Setup routes
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		api.OK(w, map[string]string{"status": "healthy"})
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/cameras", func(r chi.Router) {
			r.Get("/", handleListCameras(cameraService))
			r.Post("/", handleCreateCamera(cameraService))
			r.Get("/{id}", handleGetCamera(cameraService))
			r.Delete("/{id}", handleDeleteCamera(cameraService))
		})

		r.Route("/events", func(r chi.Router) {
			r.Get("/", handleListEvents(eventService))
			r.Get("/{id}", handleGetEvent(eventService))
			r.Put("/{id}/acknowledge", handleAcknowledgeEvent(eventService))
		})
	})

	// Create test server
	server := httptest.NewServer(r)

	return &TestEnv{
		DB:            db,
		Config:        cfg,
		CameraService: cameraService,
		EventService:  eventService,
		Router:        r,
		Server:        server,
		TmpDir:        tmpDir,
	}
}

// Cleanup cleans up the test environment
func (e *TestEnv) Cleanup() {
	e.Server.Close()
	e.DB.Close()
}

// Handler factories

func handleListCameras(svc *camera.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cameras, err := svc.List(r.Context())
		if err != nil {
			api.InternalError(w, err.Error())
			return
		}
		api.OK(w, cameras)
	}
}

func handleCreateCamera(svc *camera.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var camCfg config.CameraConfig
		if err := json.NewDecoder(r.Body).Decode(&camCfg); err != nil {
			api.BadRequest(w, "Invalid request body")
			return
		}

		validator := api.NewCameraValidator()
		if errors := validator.Validate(camCfg); errors.HasErrors() {
			api.ValidationErrorResponse(w, errors)
			return
		}

		cam, err := svc.Create(r.Context(), camCfg)
		if err != nil {
			api.InternalError(w, err.Error())
			return
		}

		api.Created(w, cam)
	}
}

func handleGetCamera(svc *camera.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		cam, err := svc.Get(r.Context(), id)
		if err != nil {
			api.NotFound(w, "Camera not found")
			return
		}
		api.OK(w, cam)
	}
}

func handleDeleteCamera(svc *camera.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.Delete(r.Context(), id); err != nil {
			api.NotFound(w, "Camera not found")
			return
		}
		api.NoContent(w)
	}
}

func handleListEvents(svc *events.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		events, total, err := svc.List(r.Context(), events.ListOptions{Limit: 50})
		if err != nil {
			api.InternalError(w, err.Error())
			return
		}
		api.List(w, events, total, 1, 50)
	}
}

func handleGetEvent(svc *events.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		event, err := svc.Get(r.Context(), id)
		if err != nil {
			api.NotFound(w, "Event not found")
			return
		}
		api.OK(w, event)
	}
}

func handleAcknowledgeEvent(svc *events.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.Acknowledge(r.Context(), id); err != nil {
			api.NotFound(w, "Event not found")
			return
		}
		api.OK(w, map[string]bool{"acknowledged": true})
	}
}

// Tests

func TestHealthEndpoint(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	resp, err := http.Get(env.Server.URL + "/health")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestCameraWorkflow(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	// 1. List cameras (should be empty)
	resp, err := http.Get(env.Server.URL + "/api/v1/cameras")
	if err != nil {
		t.Fatalf("Failed to list cameras: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// 2. Create a camera
	camData := map[string]interface{}{
		"name": "Test Camera",
		"stream": map[string]string{
			"url": "rtsp://192.168.1.100:554/stream",
		},
	}
	body, _ := json.Marshal(camData)

	resp, err = http.Post(env.Server.URL+"/api/v1/cameras", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to create camera: %v", err)
	}

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", resp.StatusCode)
	}

	var createResp api.Response
	json.NewDecoder(resp.Body).Decode(&createResp)
	resp.Body.Close()

	if !createResp.Success {
		t.Error("Create camera should succeed")
	}

	// Extract camera ID from response
	dataMap := createResp.Data.(map[string]interface{})
	cameraID := dataMap["id"].(string)

	// 3. Get the camera
	resp, err = http.Get(env.Server.URL + "/api/v1/cameras/" + cameraID)
	if err != nil {
		t.Fatalf("Failed to get camera: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// 4. List cameras (should have 1)
	resp, err = http.Get(env.Server.URL + "/api/v1/cameras")
	if err != nil {
		t.Fatalf("Failed to list cameras: %v", err)
	}

	var listResp api.Response
	json.NewDecoder(resp.Body).Decode(&listResp)
	resp.Body.Close()

	// 5. Delete the camera
	req, _ := http.NewRequest(http.MethodDelete, env.Server.URL+"/api/v1/cameras/"+cameraID, nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to delete camera: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", resp.StatusCode)
	}

	// 6. Verify camera is gone
	resp, err = http.Get(env.Server.URL + "/api/v1/cameras/" + cameraID)
	if err != nil {
		t.Fatalf("Failed to get camera: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", resp.StatusCode)
	}
}

func TestEventWorkflow(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	// Create an event directly
	event := &events.Event{
		CameraID:   "test_cam",
		EventType:  events.EventPerson,
		Timestamp:  time.Now(),
		Confidence: 0.95,
	}
	err := env.EventService.Create(context.Background(), event)
	if err != nil {
		t.Fatalf("Failed to create event: %v", err)
	}

	// 1. List events
	resp, err := http.Get(env.Server.URL + "/api/v1/events")
	if err != nil {
		t.Fatalf("Failed to list events: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var listResp api.Response
	json.NewDecoder(resp.Body).Decode(&listResp)
	resp.Body.Close()

	if !listResp.Success {
		t.Error("List events should succeed")
	}

	// 2. Get specific event
	resp, err = http.Get(env.Server.URL + "/api/v1/events/" + event.ID)
	if err != nil {
		t.Fatalf("Failed to get event: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 3. Acknowledge event
	req, _ := http.NewRequest(http.MethodPut, env.Server.URL+"/api/v1/events/"+event.ID+"/acknowledge", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to acknowledge event: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 4. Verify event is acknowledged
	retrieved, err := env.EventService.Get(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("Failed to get event: %v", err)
	}

	if !retrieved.Acknowledged {
		t.Error("Event should be acknowledged")
	}
}

func TestValidationErrors(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	// Try to create a camera with missing name
	camData := map[string]interface{}{
		"name": "",
		"stream": map[string]string{
			"url": "rtsp://192.168.1.100:554/stream",
		},
	}
	body, _ := json.Marshal(camData)

	resp, err := http.Post(env.Server.URL+"/api/v1/cameras", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to create camera: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}

	var errResp api.Response
	json.NewDecoder(resp.Body).Decode(&errResp)

	if errResp.Success {
		t.Error("Request should fail")
	}

	if errResp.Error == nil {
		t.Error("Response should have error")
	}
}

func TestNotFoundErrors(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	// Try to get non-existent camera
	resp, err := http.Get(env.Server.URL + "/api/v1/cameras/nonexistent")
	if err != nil {
		t.Fatalf("Failed to get camera: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", resp.StatusCode)
	}

	// Try to get non-existent event
	resp, err = http.Get(env.Server.URL + "/api/v1/events/nonexistent")
	if err != nil {
		t.Fatalf("Failed to get event: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", resp.StatusCode)
	}
}

func TestConcurrentRequests(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	// Create multiple cameras concurrently
	done := make(chan bool, 5)

	for i := 0; i < 5; i++ {
		go func(idx int) {
			camData := map[string]interface{}{
				"name": "Camera " + string(rune('A'+idx)),
				"stream": map[string]string{
					"url": "rtsp://192.168.1.100:554/stream" + string(rune('0'+idx)),
				},
			}
			body, _ := json.Marshal(camData)

			resp, err := http.Post(env.Server.URL+"/api/v1/cameras", "application/json", bytes.NewReader(body))
			if err != nil {
				t.Errorf("Failed to create camera: %v", err)
				done <- false
				return
			}
			resp.Body.Close()

			done <- resp.StatusCode == http.StatusCreated
		}(i)
	}

	// Wait for all goroutines
	successCount := 0
	for i := 0; i < 5; i++ {
		if <-done {
			successCount++
		}
	}

	if successCount != 5 {
		t.Errorf("Expected 5 successful creates, got %d", successCount)
	}

	// Verify all cameras exist
	cameras, err := env.CameraService.List(context.Background())
	if err != nil {
		t.Fatalf("Failed to list cameras: %v", err)
	}

	if len(cameras) != 5 {
		t.Errorf("Expected 5 cameras, got %d", len(cameras))
	}
}
