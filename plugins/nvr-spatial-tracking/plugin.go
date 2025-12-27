// Package nvrspatialtracking provides multi-camera spatial awareness and cross-camera tracking
package nvrspatialtracking

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/Spatial-NVR/SpatialNVR/sdk"
)

// Plugin implements the spatial tracking plugin
type Plugin struct {
	runtime *sdk.PluginRuntime
	db      *sql.DB
	store   *Store
	router  chi.Router

	// Track manager for active global tracks
	trackManager *TrackManager

	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a new spatial tracking plugin instance
func New() *Plugin {
	return &Plugin{}
}

// Manifest returns the plugin manifest
func (p *Plugin) Manifest() sdk.PluginManifest {
	return sdk.PluginManifest{
		ID:          "nvr-spatial-tracking",
		Name:        "Spatial Tracking",
		Version:     "0.1.0",
		Description: "Multi-camera spatial awareness with cross-camera object tracking",
		Category:    "tracking",
		Critical:    false,
		Dependencies: []string{
			"nvr-core-events",
			"nvr-detection",
		},
		Capabilities: []string{
			"spatial-mapping",
			"cross-camera-tracking",
			"trajectory-prediction",
		},
		ConfigSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"redis_url": map[string]interface{}{
					"type":        "string",
					"description": "Redis URL for distributed state (optional for single-node)",
				},
				"reid_enabled": map[string]interface{}{
					"type":        "boolean",
					"description": "Enable Re-ID for cross-camera matching",
					"default":     true,
				},
				"max_gap_seconds": map[string]interface{}{
					"type":        "number",
					"description": "Maximum seconds to wait for gap transitions",
					"default":     30,
				},
				"track_ttl_seconds": map[string]interface{}{
					"type":        "number",
					"description": "How long to keep inactive tracks",
					"default":     300,
				},
			},
		},
	}
}

// Initialize prepares the plugin
func (p *Plugin) Initialize(ctx context.Context, runtime *sdk.PluginRuntime) error {
	p.runtime = runtime
	p.db = runtime.DB()
	p.ctx, p.cancel = context.WithCancel(ctx)

	// Initialize store
	p.store = NewStore(p.db)

	// Run migrations
	if err := p.store.Migrate(ctx); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Initialize track manager
	p.trackManager = NewTrackManager(p.store, runtime.Logger())

	// Setup HTTP router
	p.router = p.setupRoutes()

	runtime.Logger().Info("Spatial tracking plugin initialized")
	return nil
}

// Start begins plugin operation
func (p *Plugin) Start(ctx context.Context) error {
	// Subscribe to detection events
	if err := p.runtime.SubscribeEvents(p.handleDetectionEvent, sdk.EventTypeDetection); err != nil {
		return fmt.Errorf("failed to subscribe to detection events: %w", err)
	}

	// Start track manager background tasks
	go p.trackManager.Run(p.ctx)

	p.runtime.Logger().Info("Spatial tracking plugin started")
	return nil
}

// Stop gracefully shuts down the plugin
func (p *Plugin) Stop(ctx context.Context) error {
	if p.cancel != nil {
		p.cancel()
	}
	p.runtime.Logger().Info("Spatial tracking plugin stopped")
	return nil
}

// Health returns the plugin health status
func (p *Plugin) Health() sdk.HealthStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Check database connectivity
	if err := p.db.PingContext(context.Background()); err != nil {
		return sdk.HealthStatus{
			State:       sdk.HealthStateUnhealthy,
			Message:     "Database connection failed",
			LastChecked: time.Now(),
		}
	}

	activeCount := 0
	if p.trackManager != nil {
		activeCount = p.trackManager.ActiveTrackCount()
	}

	return sdk.HealthStatus{
		State:   sdk.HealthStateHealthy,
		Message: fmt.Sprintf("Tracking %d active objects", activeCount),
		LastChecked: time.Now(),
		Details: map[string]string{
			"active_tracks": fmt.Sprintf("%d", activeCount),
		},
	}
}

// Routes returns the HTTP handler for plugin routes
func (p *Plugin) Routes() http.Handler {
	return p.router
}

// handleDetectionEvent processes incoming detection events
func (p *Plugin) handleDetectionEvent(event *sdk.Event) {
	if event == nil || event.CameraID == "" {
		return
	}

	p.trackManager.ProcessDetection(p.ctx, event)
}

// setupRoutes configures the HTTP API routes
func (p *Plugin) setupRoutes() chi.Router {
	r := chi.NewRouter()

	// Spatial Maps
	r.Route("/maps", func(r chi.Router) {
		r.Get("/", p.handleListMaps)
		r.Post("/", p.handleCreateMap)
		r.Get("/{mapId}", p.handleGetMap)
		r.Put("/{mapId}", p.handleUpdateMap)
		r.Delete("/{mapId}", p.handleDeleteMap)
		r.Post("/{mapId}/image", p.handleUploadMapImage)

		// Camera placements within a map
		r.Get("/{mapId}/cameras", p.handleListPlacements)
		r.Post("/{mapId}/cameras", p.handleCreatePlacement)
		r.Put("/{mapId}/cameras/{placementId}", p.handleUpdatePlacement)
		r.Delete("/{mapId}/cameras/{placementId}", p.handleDeletePlacement)

		// Auto-detect transitions for a map
		r.Post("/{mapId}/auto-detect-transitions", p.handleAutoDetectTransitionsForMap)

		// Analytics for a map
		r.Get("/{mapId}/analytics", p.handleGetMapAnalytics)
	})

	// Camera placements (direct access)
	r.Route("/cameras", func(r chi.Router) {
		r.Get("/{placementId}", p.handleGetPlacement)
		r.Put("/{placementId}", p.handleUpdatePlacement)
		r.Delete("/{placementId}", p.handleDeletePlacement)
	})

	// Transitions
	r.Route("/transitions", func(r chi.Router) {
		r.Get("/", p.handleListTransitions)
		r.Post("/", p.handleCreateTransition)
		r.Get("/{transitionId}", p.handleGetTransition)
		r.Put("/{transitionId}", p.handleUpdateTransition)
		r.Delete("/{transitionId}", p.handleDeleteTransition)
		r.Post("/auto-detect", p.handleAutoDetectTransitions)
	})

	// Active tracking
	r.Route("/tracks", func(r chi.Router) {
		r.Get("/", p.handleListTracks)
		r.Get("/{trackId}", p.handleGetTrack)
		r.Get("/{trackId}/path", p.handleGetTrackPath)
	})

	// Calibration & analytics
	r.Post("/calibrate/{cameraId}", p.handleStartCalibration)
	r.Post("/test-handoff", p.handleTestHandoff)
	r.Get("/analytics", p.handleGetAnalytics)

	return r
}

// API Handlers - Maps

func (p *Plugin) handleListMaps(w http.ResponseWriter, r *http.Request) {
	maps, err := p.store.ListMaps()
	if err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, maps)
}

func (p *Plugin) handleCreateMap(w http.ResponseWriter, r *http.Request) {
	var req SpatialMap
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, jsonError("Invalid request body"), http.StatusBadRequest)
		return
	}

	if err := p.store.CreateMap(&req); err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	jsonResponse(w, req)
}

func (p *Plugin) handleGetMap(w http.ResponseWriter, r *http.Request) {
	mapID := chi.URLParam(r, "mapId")
	m, err := p.store.GetMap(mapID)
	if err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
		return
	}
	if m == nil {
		http.Error(w, jsonError("Map not found"), http.StatusNotFound)
		return
	}
	jsonResponse(w, m)
}

func (p *Plugin) handleUpdateMap(w http.ResponseWriter, r *http.Request) {
	mapID := chi.URLParam(r, "mapId")
	var req SpatialMap
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, jsonError("Invalid request body"), http.StatusBadRequest)
		return
	}
	req.ID = mapID

	if err := p.store.UpdateMap(&req); err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, req)
}

func (p *Plugin) handleDeleteMap(w http.ResponseWriter, r *http.Request) {
	mapID := chi.URLParam(r, "mapId")
	if err := p.store.DeleteMap(mapID); err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (p *Plugin) handleUploadMapImage(w http.ResponseWriter, r *http.Request) {
	mapID := chi.URLParam(r, "mapId")

	// Parse multipart form (max 10MB)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, jsonError("File too large"), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, jsonError("No image file provided"), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Store image and get URL
	imageURL, err := p.store.SaveMapImage(r.Context(), mapID, file, header.Filename)
	if err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, map[string]string{"image_url": imageURL})
}

// API Handlers - Camera Placements

func (p *Plugin) handleListPlacements(w http.ResponseWriter, r *http.Request) {
	mapID := chi.URLParam(r, "mapId")
	placements, err := p.store.ListPlacementsByMap(mapID)
	if err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, placements)
}

func (p *Plugin) handleCreatePlacement(w http.ResponseWriter, r *http.Request) {
	mapID := chi.URLParam(r, "mapId")
	var req CameraPlacement
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, jsonError("Invalid request body"), http.StatusBadRequest)
		return
	}
	req.MapID = mapID

	if err := p.store.CreatePlacement(&req); err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	jsonResponse(w, req)
}

func (p *Plugin) handleGetPlacement(w http.ResponseWriter, r *http.Request) {
	placementID := chi.URLParam(r, "placementId")
	placement, err := p.store.GetPlacement(placementID)
	if err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
		return
	}
	if placement == nil {
		http.Error(w, jsonError("Placement not found"), http.StatusNotFound)
		return
	}
	jsonResponse(w, placement)
}

func (p *Plugin) handleUpdatePlacement(w http.ResponseWriter, r *http.Request) {
	placementID := chi.URLParam(r, "placementId")
	var req CameraPlacement
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, jsonError("Invalid request body"), http.StatusBadRequest)
		return
	}
	req.ID = placementID

	if err := p.store.UpdatePlacement(&req); err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, req)
}

func (p *Plugin) handleDeletePlacement(w http.ResponseWriter, r *http.Request) {
	placementID := chi.URLParam(r, "placementId")
	if err := p.store.DeletePlacement(placementID); err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// API Handlers - Transitions

func (p *Plugin) handleListTransitions(w http.ResponseWriter, r *http.Request) {
	transitions, err := p.store.ListTransitions()
	if err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, transitions)
}

func (p *Plugin) handleCreateTransition(w http.ResponseWriter, r *http.Request) {
	var req CameraTransition
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, jsonError("Invalid request body"), http.StatusBadRequest)
		return
	}

	if err := p.store.CreateTransition(&req); err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	jsonResponse(w, req)
}

func (p *Plugin) handleGetTransition(w http.ResponseWriter, r *http.Request) {
	transitionID := chi.URLParam(r, "transitionId")
	transition, err := p.store.GetTransition(transitionID)
	if err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
		return
	}
	if transition == nil {
		http.Error(w, jsonError("Transition not found"), http.StatusNotFound)
		return
	}
	jsonResponse(w, transition)
}

func (p *Plugin) handleUpdateTransition(w http.ResponseWriter, r *http.Request) {
	transitionID := chi.URLParam(r, "transitionId")
	var req CameraTransition
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, jsonError("Invalid request body"), http.StatusBadRequest)
		return
	}
	req.ID = transitionID

	if err := p.store.UpdateTransition(&req); err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, req)
}

func (p *Plugin) handleDeleteTransition(w http.ResponseWriter, r *http.Request) {
	transitionID := chi.URLParam(r, "transitionId")
	if err := p.store.DeleteTransition(transitionID); err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (p *Plugin) handleAutoDetectTransitions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MapID string `json:"map_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, jsonError("Invalid request body"), http.StatusBadRequest)
		return
	}

	transitions, err := p.store.AutoDetectTransitions(r.Context(), req.MapID)
	if err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, transitions)
}

// API Handlers - Tracks

func (p *Plugin) handleListTracks(w http.ResponseWriter, r *http.Request) {
	tracks := p.trackManager.ListActiveTracks()
	if tracks == nil {
		tracks = []GlobalTrack{}
	}
	jsonResponse(w, tracks)
}

func (p *Plugin) handleGetTrack(w http.ResponseWriter, r *http.Request) {
	trackID := chi.URLParam(r, "trackId")
	track, ok := p.trackManager.GetTrack(trackID)
	if !ok {
		http.Error(w, jsonError("Track not found"), http.StatusNotFound)
		return
	}
	jsonResponse(w, track)
}

func (p *Plugin) handleGetTrackPath(w http.ResponseWriter, r *http.Request) {
	trackID := chi.URLParam(r, "trackId")
	path, err := p.trackManager.GetTrackPath(r.Context(), trackID)
	if err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusNotFound)
		return
	}
	jsonResponse(w, path)
}

// API Handlers - Calibration & Analytics

func (p *Plugin) handleStartCalibration(w http.ResponseWriter, r *http.Request) {
	cameraID := chi.URLParam(r, "cameraId")

	// Start calibration mode for this camera
	session, err := p.trackManager.StartCalibration(r.Context(), cameraID)
	if err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, session)
}

func (p *Plugin) handleTestHandoff(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FromCameraID string `json:"from_camera_id"`
		ToCameraID   string `json:"to_camera_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, jsonError("Invalid request body"), http.StatusBadRequest)
		return
	}

	result, err := p.trackManager.TestHandoff(r.Context(), req.FromCameraID, req.ToCameraID)
	if err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, result)
}

func (p *Plugin) handleGetAnalytics(w http.ResponseWriter, r *http.Request) {
	analytics, err := p.store.GetAnalytics()
	if err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, analytics)
}

// handleAutoDetectTransitionsForMap auto-detects transitions for a specific map
func (p *Plugin) handleAutoDetectTransitionsForMap(w http.ResponseWriter, r *http.Request) {
	mapID := chi.URLParam(r, "mapId")
	if mapID == "" {
		http.Error(w, jsonError("Map ID required"), http.StatusBadRequest)
		return
	}

	transitions, err := p.store.AutoDetectTransitions(r.Context(), mapID)
	if err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, transitions)
}

// handleGetMapAnalytics returns analytics for a specific map
func (p *Plugin) handleGetMapAnalytics(w http.ResponseWriter, r *http.Request) {
	mapID := chi.URLParam(r, "mapId")
	if mapID == "" {
		http.Error(w, jsonError("Map ID required"), http.StatusBadRequest)
		return
	}

	analytics, err := p.store.GetMapAnalytics(mapID)
	if err != nil {
		http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, analytics)
}

// Helper functions

func jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func jsonError(message string) string {
	return fmt.Sprintf(`{"error":"%s"}`, message)
}
