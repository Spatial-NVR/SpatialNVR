package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	spatial "github.com/Spatial-NVR/SpatialNVR/plugins/nvr-spatial-tracking"
)

// Maps handlers
func handleListMaps(store *spatial.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		maps, err := store.ListMaps()
		if err != nil {
			http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
			return
		}
		if maps == nil {
			maps = []spatial.SpatialMap{}
		}
		jsonResponse(w, maps)
	}
}

func handleCreateMap(store *spatial.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req spatial.SpatialMap
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, jsonError("Invalid request body"), http.StatusBadRequest)
			return
		}

		if err := store.CreateMap(&req); err != nil {
			http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		jsonResponse(w, req)
	}
}

func handleGetMap(store *spatial.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mapID := chi.URLParam(r, "mapId")
		m, err := store.GetMap(mapID)
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
}

func handleUpdateMap(store *spatial.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mapID := chi.URLParam(r, "mapId")
		var req spatial.SpatialMap
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, jsonError("Invalid request body"), http.StatusBadRequest)
			return
		}
		req.ID = mapID

		if err := store.UpdateMap(&req); err != nil {
			http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, req)
	}
}

func handleDeleteMap(store *spatial.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mapID := chi.URLParam(r, "mapId")
		if err := store.DeleteMap(mapID); err != nil {
			http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// Placement handlers
func handleListPlacements(store *spatial.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mapID := chi.URLParam(r, "mapId")
		placements, err := store.ListPlacementsByMap(mapID)
		if err != nil {
			http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
			return
		}
		if placements == nil {
			placements = []spatial.CameraPlacement{}
		}
		jsonResponse(w, placements)
	}
}

func handleCreatePlacement(store *spatial.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mapID := chi.URLParam(r, "mapId")
		var req spatial.CameraPlacement
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, jsonError("Invalid request body"), http.StatusBadRequest)
			return
		}
		req.MapID = mapID

		if err := store.CreatePlacement(&req); err != nil {
			http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		jsonResponse(w, req)
	}
}

func handleUpdatePlacement(store *spatial.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		placementID := chi.URLParam(r, "placementId")
		var req spatial.CameraPlacement
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, jsonError("Invalid request body"), http.StatusBadRequest)
			return
		}
		req.ID = placementID

		if err := store.UpdatePlacement(&req); err != nil {
			http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, req)
	}
}

func handleDeletePlacement(store *spatial.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		placementID := chi.URLParam(r, "placementId")
		if err := store.DeletePlacement(placementID); err != nil {
			http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// Transition handlers
func handleListTransitions(store *spatial.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		transitions, err := store.ListTransitions()
		if err != nil {
			http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
			return
		}
		if transitions == nil {
			transitions = []spatial.CameraTransition{}
		}
		jsonResponse(w, transitions)
	}
}

func handleCreateTransition(store *spatial.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req spatial.CameraTransition
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, jsonError("Invalid request body"), http.StatusBadRequest)
			return
		}

		if err := store.CreateTransition(&req); err != nil {
			http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		jsonResponse(w, req)
	}
}

func handleDeleteTransition(store *spatial.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		transitionID := chi.URLParam(r, "transitionId")
		if err := store.DeleteTransition(transitionID); err != nil {
			http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleAutoDetectTransitions(store *spatial.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mapID := chi.URLParam(r, "mapId")
		transitions, err := store.AutoDetectTransitions(r.Context(), mapID)
		if err != nil {
			http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
			return
		}
		if transitions == nil {
			transitions = []spatial.CameraTransition{}
		}
		jsonResponse(w, transitions)
	}
}

// Track handlers
func handleListTracks(trackManager *spatial.TrackManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tracks := trackManager.ListActiveTracks()
		if tracks == nil {
			tracks = []spatial.GlobalTrack{}
		}
		jsonResponse(w, tracks)
	}
}

// Analytics handlers
func handleGetAnalytics(store *spatial.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		analytics, err := store.GetAnalytics()
		if err != nil {
			http.Error(w, jsonError(err.Error()), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, analytics)
	}
}

// Helper functions
func jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func jsonError(message string) string {
	return fmt.Sprintf(`{"error":"%s"}`, message)
}
