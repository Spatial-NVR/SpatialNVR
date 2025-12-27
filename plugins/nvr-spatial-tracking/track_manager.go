package nvrspatialtracking

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/Spatial-NVR/SpatialNVR/sdk"
)

// TrackManager handles active cross-camera tracking
type TrackManager struct {
	store  *Store
	logger *slog.Logger

	// Active tracks in memory for fast lookup
	tracks   map[string]*GlobalTrack
	tracksMu sync.RWMutex

	// Camera to track mapping
	cameraToTrack   map[string]string // cameraID -> globalTrackID
	cameraToTrackMu sync.RWMutex

	// Pending handoffs waiting for matches
	pendingHandoffs   map[string]*PendingHandoff
	pendingHandoffsMu sync.RWMutex

	// Configuration
	maxGapSeconds   float64
	trackTTLSeconds float64
	reidEnabled     bool
}

// NewTrackManager creates a new track manager
func NewTrackManager(store *Store, logger *slog.Logger) *TrackManager {
	return &TrackManager{
		store:           store,
		logger:          logger,
		tracks:          make(map[string]*GlobalTrack),
		cameraToTrack:   make(map[string]string),
		pendingHandoffs: make(map[string]*PendingHandoff),
		maxGapSeconds:   30,
		trackTTLSeconds: 300,
		reidEnabled:     true,
	}
}

// Run starts background processing
func (tm *TrackManager) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tm.cleanupExpiredHandoffs()
			tm.cleanupStaleTransitTracks()
		}
	}
}

// ActiveTrackCount returns the number of active tracks
func (tm *TrackManager) ActiveTrackCount() int {
	tm.tracksMu.RLock()
	defer tm.tracksMu.RUnlock()

	count := 0
	for _, track := range tm.tracks {
		if track.State == TrackStateActive || track.State == TrackStateTransit {
			count++
		}
	}
	return count
}

// ProcessDetection handles an incoming detection event
func (tm *TrackManager) ProcessDetection(ctx context.Context, event *sdk.Event) {
	if event == nil || event.CameraID == "" || event.TrackID == "" {
		return
	}

	// Check if this local track is already associated with a global track
	tm.cameraToTrackMu.RLock()
	globalTrackID, exists := tm.cameraToTrack[event.CameraID+":"+event.TrackID]
	tm.cameraToTrackMu.RUnlock()

	if exists {
		// Update existing track
		tm.updateTrack(ctx, globalTrackID, event)
		return
	}

	// Check if there's a pending handoff expecting this detection
	matchedHandoff := tm.matchPendingHandoff(ctx, event)
	if matchedHandoff != nil {
		// Continue existing global track
		tm.continueTrack(ctx, matchedHandoff, event)
		return
	}

	// Create new global track
	tm.createNewTrack(ctx, event)
}

// updateTrack updates an existing track with new detection data
func (tm *TrackManager) updateTrack(ctx context.Context, globalTrackID string, event *sdk.Event) {
	tm.tracksMu.Lock()
	track, exists := tm.tracks[globalTrackID]
	if !exists {
		tm.tracksMu.Unlock()
		return
	}

	track.LastSeen = event.Timestamp
	track.State = TrackStateActive

	// Add bounding box sample to current segment
	if len(track.Path) > 0 && event.BoundingBox != nil {
		segment := &track.Path[len(track.Path)-1]
		segment.BoundingBoxes = append(segment.BoundingBoxes, BoundingBoxSample{
			Timestamp: event.Timestamp,
			X:         event.BoundingBox.X,
			Y:         event.BoundingBox.Y,
			Width:     event.BoundingBox.Width,
			Height:    event.BoundingBox.Height,
		})

		// Keep only last 100 samples per segment
		if len(segment.BoundingBoxes) > 100 {
			segment.BoundingBoxes = segment.BoundingBoxes[len(segment.BoundingBoxes)-100:]
		}
	}

	tm.tracksMu.Unlock()

	// Persist update periodically (every 10 seconds)
	if event.Timestamp.Second()%10 == 0 {
		_ = tm.store.UpdateTrack(track)
	}
}

// matchPendingHandoff tries to match a new detection with a pending handoff
func (tm *TrackManager) matchPendingHandoff(ctx context.Context, event *sdk.Event) *PendingHandoff {
	tm.pendingHandoffsMu.Lock()
	defer tm.pendingHandoffsMu.Unlock()

	now := time.Now()

	for id, handoff := range tm.pendingHandoffs {
		// Check if this camera is in the expected destinations
		isExpectedCamera := false
		for _, camID := range handoff.ToCameraIDs {
			if camID == event.CameraID {
				isExpectedCamera = true
				break
			}
		}
		if !isExpectedCamera {
			continue
		}

		// Check if not expired
		if now.After(handoff.ExpectedBy) {
			continue
		}

		// Check object type matches
		if handoff.TransitionType == TransitionOverlap || handoff.TransitionType == TransitionAdjacent {
			// For overlap/adjacent, require same object type
			tm.tracksMu.RLock()
			track, exists := tm.tracks[handoff.GlobalTrackID]
			tm.tracksMu.RUnlock()

			if exists && track.ObjectType != event.ObjectType {
				continue
			}
		}

		// TODO: Add Re-ID embedding comparison for better matching
		// For now, use spatial proximity and timing

		// Match found!
		delete(tm.pendingHandoffs, id)

		// Record successful handoff
		transition, _ := tm.store.GetTransitionByCameras(handoff.FromCameraID, event.CameraID)
		if transition != nil {
			transitTime := event.Timestamp.Sub(handoff.ExitedAt).Seconds()
			_ = tm.store.RecordHandoff(transition.ID, transitTime, true)
		}

		tm.logger.Info("Handoff matched",
			"global_track_id", handoff.GlobalTrackID,
			"from_camera", handoff.FromCameraID,
			"to_camera", event.CameraID,
		)

		return handoff
	}

	return nil
}

// continueTrack continues an existing track after a camera transition
func (tm *TrackManager) continueTrack(ctx context.Context, handoff *PendingHandoff, event *sdk.Event) {
	tm.tracksMu.Lock()
	track, exists := tm.tracks[handoff.GlobalTrackID]
	if !exists {
		tm.tracksMu.Unlock()
		return
	}

	// Close previous segment
	if len(track.Path) > 0 {
		lastSegment := &track.Path[len(track.Path)-1]
		if lastSegment.ExitedAt == nil {
			exitTime := handoff.ExitedAt
			lastSegment.ExitedAt = &exitTime
			lastSegment.ExitDirection = handoff.ExitDirection
			_ = tm.store.UpdateSegment(lastSegment)
		}
	}

	// Create new segment
	segment := TrackSegment{
		ID:            uuid.New().String(),
		GlobalTrackID: track.ID,
		CameraID:      event.CameraID,
		LocalTrackID:  event.TrackID,
		EnteredAt:     event.Timestamp,
	}
	track.Path = append(track.Path, segment)
	_ = tm.store.CreateSegment(&segment)

	// Update track state
	track.CurrentCameraID = event.CameraID
	track.CurrentLocalTrack = event.TrackID
	track.LastSeen = event.Timestamp
	track.State = TrackStateActive
	track.PredictedNext = ""
	track.PredictedArrival = nil

	tm.tracksMu.Unlock()

	// Update camera mapping
	tm.cameraToTrackMu.Lock()
	tm.cameraToTrack[event.CameraID+":"+event.TrackID] = track.ID
	tm.cameraToTrackMu.Unlock()

	_ = tm.store.UpdateTrack(track)
}

// createNewTrack creates a new global track for a new detection
func (tm *TrackManager) createNewTrack(ctx context.Context, event *sdk.Event) {
	track := &GlobalTrack{
		ID:                uuid.New().String(),
		FirstSeen:         event.Timestamp,
		LastSeen:          event.Timestamp,
		CurrentCameraID:   event.CameraID,
		CurrentLocalTrack: event.TrackID,
		ObjectType:        event.ObjectType,
		State:             TrackStateActive,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	// Create initial segment
	segment := TrackSegment{
		ID:            uuid.New().String(),
		GlobalTrackID: track.ID,
		CameraID:      event.CameraID,
		LocalTrackID:  event.TrackID,
		EnteredAt:     event.Timestamp,
	}
	track.Path = []TrackSegment{segment}

	// Store
	if err := tm.store.CreateTrack(track); err != nil {
		tm.logger.Error("Failed to create track", "error", err)
		return
	}
	if err := tm.store.CreateSegment(&segment); err != nil {
		tm.logger.Error("Failed to create segment", "error", err)
	}

	// Add to memory
	tm.tracksMu.Lock()
	tm.tracks[track.ID] = track
	tm.tracksMu.Unlock()

	tm.cameraToTrackMu.Lock()
	tm.cameraToTrack[event.CameraID+":"+event.TrackID] = track.ID
	tm.cameraToTrackMu.Unlock()

	tm.logger.Debug("Created new global track",
		"track_id", track.ID,
		"camera_id", event.CameraID,
		"object_type", event.ObjectType,
	)
}

// HandleTrackExit processes when a track exits a camera's view
func (tm *TrackManager) HandleTrackExit(ctx context.Context, cameraID, localTrackID string, exitDirection EdgeDirection, exitPosition Point) {
	tm.cameraToTrackMu.RLock()
	globalTrackID, exists := tm.cameraToTrack[cameraID+":"+localTrackID]
	tm.cameraToTrackMu.RUnlock()

	if !exists {
		return
	}

	tm.tracksMu.Lock()
	track, exists := tm.tracks[globalTrackID]
	if !exists {
		tm.tracksMu.Unlock()
		return
	}

	now := time.Now()

	// Update current segment
	if len(track.Path) > 0 {
		segment := &track.Path[len(track.Path)-1]
		segment.ExitedAt = &now
		segment.ExitDirection = exitDirection
		segment.ExitPosition = &exitPosition
		_ = tm.store.UpdateSegment(segment)
	}

	// Find possible next cameras
	transitions, _ := tm.store.ListTransitionsFromCamera(cameraID)
	var possibleNextCameras []string
	var expectedArrival time.Time
	var transitionType TransitionType

	for _, t := range transitions {
		var nextCam string
		if t.FromCameraID == cameraID {
			nextCam = t.ToCameraID
		} else if t.Bidirectional && t.ToCameraID == cameraID {
			nextCam = t.FromCameraID
		} else {
			continue
		}

		possibleNextCameras = append(possibleNextCameras, nextCam)
		transitionType = t.Type

		// Use avg transit time if available, otherwise expected
		transitTime := t.ExpectedTransitTime
		if t.AvgTransitTime > 0 {
			transitTime = t.AvgTransitTime
		}
		variance := t.TransitTimeVariance
		if variance == 0 {
			variance = transitTime * 0.5
		}

		arrival := now.Add(time.Duration((transitTime + variance) * float64(time.Second)))
		if expectedArrival.IsZero() || arrival.After(expectedArrival) {
			expectedArrival = arrival
		}
	}

	if len(possibleNextCameras) > 0 {
		// Create pending handoff
		track.State = TrackStateTransit
		track.PredictedNext = possibleNextCameras[0]
		track.PredictedArrival = &expectedArrival

		handoff := &PendingHandoff{
			ID:             uuid.New().String(),
			GlobalTrackID:  track.ID,
			FromCameraID:   cameraID,
			ToCameraIDs:    possibleNextCameras,
			TransitionType: transitionType,
			ExitedAt:       now,
			ExpectedBy:     expectedArrival,
			ExitDirection:  exitDirection,
			ExitPosition:   exitPosition,
			Embedding:      track.Embedding,
			DominantColors: track.DominantColors,
		}

		tm.pendingHandoffsMu.Lock()
		tm.pendingHandoffs[handoff.ID] = handoff
		tm.pendingHandoffsMu.Unlock()

		_ = tm.store.CreatePendingHandoff(handoff)

		tm.logger.Debug("Created pending handoff",
			"track_id", track.ID,
			"from_camera", cameraID,
			"to_cameras", possibleNextCameras,
			"expected_by", expectedArrival,
		)
	} else {
		// No known transitions - mark as lost
		track.State = TrackStateLost
	}

	tm.tracksMu.Unlock()

	// Remove camera mapping
	tm.cameraToTrackMu.Lock()
	delete(tm.cameraToTrack, cameraID+":"+localTrackID)
	tm.cameraToTrackMu.Unlock()

	_ = tm.store.UpdateTrack(track)
}

// cleanupExpiredHandoffs removes handoffs that have timed out
func (tm *TrackManager) cleanupExpiredHandoffs() {
	tm.pendingHandoffsMu.Lock()
	defer tm.pendingHandoffsMu.Unlock()

	now := time.Now()
	for id, handoff := range tm.pendingHandoffs {
		if now.After(handoff.ExpectedBy) {
			delete(tm.pendingHandoffs, id)

			// Mark track as lost
			tm.tracksMu.Lock()
			if track, exists := tm.tracks[handoff.GlobalTrackID]; exists {
				track.State = TrackStateLost
				_ = tm.store.UpdateTrack(track)
			}
			tm.tracksMu.Unlock()

			// Record failed handoff
			transition, _ := tm.store.GetTransitionByCameras(handoff.FromCameraID, handoff.ToCameraIDs[0])
			if transition != nil {
				_ = tm.store.RecordHandoff(transition.ID, 0, false)
			}

			tm.logger.Debug("Handoff expired",
				"track_id", handoff.GlobalTrackID,
				"from_camera", handoff.FromCameraID,
			)
		}
	}

	// Also cleanup from database
	_, _ = tm.store.CleanupExpiredHandoffs()
}

// cleanupStaleTransitTracks marks old transit tracks as completed
func (tm *TrackManager) cleanupStaleTransitTracks() {
	tm.tracksMu.Lock()
	defer tm.tracksMu.Unlock()

	now := time.Now()
	ttl := time.Duration(tm.trackTTLSeconds) * time.Second

	for id, track := range tm.tracks {
		if track.State == TrackStateLost && now.Sub(track.LastSeen) > ttl {
			track.State = TrackStateCompleted
			_ = tm.store.UpdateTrack(track)
			delete(tm.tracks, id)
		}
	}
}

// ListActiveTracks returns all currently active tracks
func (tm *TrackManager) ListActiveTracks() []GlobalTrack {
	tm.tracksMu.RLock()
	defer tm.tracksMu.RUnlock()

	var active []GlobalTrack
	for _, track := range tm.tracks {
		if track.State == TrackStateActive || track.State == TrackStateTransit || track.State == TrackStatePending {
			active = append(active, *track)
		}
	}
	return active
}

// GetTrack returns a specific track by ID
func (tm *TrackManager) GetTrack(id string) (*GlobalTrack, bool) {
	tm.tracksMu.RLock()
	track, exists := tm.tracks[id]
	tm.tracksMu.RUnlock()

	if exists {
		return track, true
	}

	// Try loading from database
	dbTrack, err := tm.store.GetTrack(id)
	if err != nil || dbTrack == nil {
		return nil, false
	}

	return dbTrack, true
}

// GetTrackPath returns the spatial path of a track on the map
func (tm *TrackManager) GetTrackPath(ctx context.Context, trackID string) (*TrackPath, error) {
	track, ok := tm.GetTrack(trackID)
	if !ok {
		return nil, fmt.Errorf("track not found")
	}

	var waypoints []Waypoint

	for _, segment := range track.Path {
		// Get camera placement
		placement, err := tm.store.GetPlacementByCameraID(segment.CameraID)
		if err != nil || placement == nil {
			continue
		}

		// Add entry waypoint
		waypoints = append(waypoints, Waypoint{
			Timestamp:  segment.EnteredAt,
			CameraID:   segment.CameraID,
			Position:   placement.Position,
			Confidence: 1.0,
		})

		// Add exit waypoint if available
		if segment.ExitedAt != nil && segment.ExitPosition != nil {
			waypoints = append(waypoints, Waypoint{
				Timestamp:  *segment.ExitedAt,
				CameraID:   segment.CameraID,
				Position:   *segment.ExitPosition,
				Confidence: 0.8,
			})
		}
	}

	// Determine map ID from first placement
	mapID := ""
	if len(track.Path) > 0 {
		placement, _ := tm.store.GetPlacementByCameraID(track.Path[0].CameraID)
		if placement != nil {
			mapID = placement.MapID
		}
	}

	return &TrackPath{
		TrackID:   trackID,
		MapID:     mapID,
		Waypoints: waypoints,
	}, nil
}

// StartCalibration begins a calibration session for a camera
func (tm *TrackManager) StartCalibration(ctx context.Context, cameraID string) (*CalibrationSession, error) {
	session := &CalibrationSession{
		ID:           uuid.New().String(),
		CameraID:     cameraID,
		StartedAt:    time.Now(),
		Status:       "pending",
		Instructions: "Walk through the camera's field of view from edge to edge. The system will analyze your movement to determine exit zones and transitions.",
	}

	tm.logger.Info("Started calibration session",
		"session_id", session.ID,
		"camera_id", cameraID,
	)

	return session, nil
}

// TestHandoff tests if a handoff between two cameras is configured correctly
func (tm *TrackManager) TestHandoff(ctx context.Context, fromCameraID, toCameraID string) (*HandoffTestResult, error) {
	result := &HandoffTestResult{
		FromCameraID: fromCameraID,
		ToCameraID:   toCameraID,
	}

	// Check if transition exists
	transition, err := tm.store.GetTransitionByCameras(fromCameraID, toCameraID)
	if err != nil {
		return nil, err
	}
	if transition == nil {
		result.Status = "error"
		result.Message = "No transition configured between these cameras"
		return result, nil
	}

	result.TransitionType = transition.Type
	result.ExpectedTime = transition.ExpectedTransitTime

	// Check camera placements
	fromPlacement, _ := tm.store.GetPlacementByCameraID(fromCameraID)
	toPlacement, _ := tm.store.GetPlacementByCameraID(toCameraID)

	if fromPlacement == nil || toPlacement == nil {
		result.Status = "warning"
		result.Message = "Camera placements not fully configured"
		return result, nil
	}

	// Verify transition type matches spatial configuration
	fromFOV := fromPlacement.CalculateFOVPolygon()
	toFOV := toPlacement.CalculateFOVPolygon()

	if fromFOV.Intersects(toFOV) {
		if transition.Type != TransitionOverlap {
			result.Status = "warning"
			result.Message = fmt.Sprintf("Cameras overlap but transition is marked as '%s'", transition.Type)
			return result, nil
		}
	} else if transition.Type == TransitionOverlap {
		result.Status = "warning"
		result.Message = "Transition marked as overlap but cameras don't overlap"
		return result, nil
	}

	result.Status = "ok"
	result.Message = fmt.Sprintf("Transition configured correctly. Type: %s, Expected time: %.1fs", transition.Type, transition.ExpectedTransitTime)

	return result, nil
}
