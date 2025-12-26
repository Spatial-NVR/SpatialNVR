package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/Spatial-NVR/SpatialNVR/internal/events"
)

// ZoneHandler handles motion zone API requests
type ZoneHandler struct {
	service *events.Service
}

// NewZoneHandler creates a new zone handler
func NewZoneHandler(service *events.Service) *ZoneHandler {
	return &ZoneHandler{service: service}
}

// Routes returns the zone routes
func (h *ZoneHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Get("/{id}", h.Get)
	r.Put("/{id}", h.Update)
	r.Delete("/{id}", h.Delete)

	return r
}

// ZoneRequest represents a zone creation/update request
type ZoneRequest struct {
	CameraID      string          `json:"camera_id"`
	Name          string          `json:"name"`
	Enabled       bool            `json:"enabled"`
	Points        []events.Point  `json:"points"`
	ObjectTypes   []string        `json:"object_types,omitempty"`
	MinConfidence float64         `json:"min_confidence"`
	MinSize       float64         `json:"min_size,omitempty"`
	Sensitivity   int             `json:"sensitivity"`
	Cooldown      int             `json:"cooldown_seconds"`
	Notifications bool            `json:"notifications"`
	Recording     bool            `json:"recording"`
	Color         string          `json:"color,omitempty"`
}

// List lists all zones, optionally filtered by camera
func (h *ZoneHandler) List(w http.ResponseWriter, r *http.Request) {
	cameraID := r.URL.Query().Get("camera_id")

	zones, err := h.service.ListZones(r.Context(), cameraID)
	if err != nil {
		InternalError(w, err.Error())
		return
	}

	OK(w, zones)
}

// Create creates a new zone
func (h *ZoneHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req ZoneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Invalid request body")
		return
	}

	// Validate required fields
	if req.CameraID == "" {
		BadRequest(w, "camera_id is required")
		return
	}
	if req.Name == "" {
		BadRequest(w, "name is required")
		return
	}
	if len(req.Points) < 3 {
		BadRequest(w, "at least 3 points are required for a zone polygon")
		return
	}

	// Set defaults
	if req.Sensitivity == 0 {
		req.Sensitivity = 5
	}
	if req.Sensitivity < 1 || req.Sensitivity > 10 {
		BadRequest(w, "sensitivity must be between 1 and 10")
		return
	}
	if req.Cooldown == 0 {
		req.Cooldown = 30
	}
	if req.MinConfidence == 0 {
		req.MinConfidence = 0.5
	}

	zone := &events.MotionZone{
		CameraID:      req.CameraID,
		Name:          req.Name,
		Enabled:       req.Enabled,
		Points:        req.Points,
		ObjectTypes:   req.ObjectTypes,
		MinConfidence: req.MinConfidence,
		MinSize:       req.MinSize,
		Sensitivity:   req.Sensitivity,
		Cooldown:      req.Cooldown,
		Notifications: req.Notifications,
		Recording:     req.Recording,
		Color:         req.Color,
	}

	if err := h.service.CreateZone(r.Context(), zone); err != nil {
		InternalError(w, err.Error())
		return
	}

	Created(w, zone)
}

// Get retrieves a zone by ID
func (h *ZoneHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	zone, err := h.service.GetZone(r.Context(), id)
	if err != nil {
		NotFound(w, "Zone not found")
		return
	}

	OK(w, zone)
}

// Update updates a zone
func (h *ZoneHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Get existing zone first
	existing, err := h.service.GetZone(r.Context(), id)
	if err != nil {
		NotFound(w, "Zone not found")
		return
	}

	var req ZoneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "Invalid request body")
		return
	}

	// Validate
	if req.Name == "" {
		BadRequest(w, "name is required")
		return
	}
	if len(req.Points) < 3 {
		BadRequest(w, "at least 3 points are required for a zone polygon")
		return
	}
	if req.Sensitivity < 1 || req.Sensitivity > 10 {
		BadRequest(w, "sensitivity must be between 1 and 10")
		return
	}

	// Update fields
	existing.Name = req.Name
	existing.Enabled = req.Enabled
	existing.Points = req.Points
	existing.ObjectTypes = req.ObjectTypes
	existing.MinConfidence = req.MinConfidence
	existing.MinSize = req.MinSize
	existing.Sensitivity = req.Sensitivity
	existing.Cooldown = req.Cooldown
	existing.Notifications = req.Notifications
	existing.Recording = req.Recording
	existing.Color = req.Color

	if err := h.service.UpdateZone(r.Context(), existing); err != nil {
		InternalError(w, err.Error())
		return
	}

	OK(w, existing)
}

// Delete deletes a zone
func (h *ZoneHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.service.DeleteZone(r.Context(), id); err != nil {
		NotFound(w, "Zone not found")
		return
	}

	NoContent(w)
}
