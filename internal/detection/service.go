package detection

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/Spatial-NVR/SpatialNVR/internal/config"
	"github.com/Spatial-NVR/SpatialNVR/internal/events"
)

// Service manages object detection for cameras
type Service struct {
	mu            sync.RWMutex
	config        *config.Config
	client        *Client
	frameGrabber  FrameGrabber
	eventService  *events.Service

	// Running streams
	streams       map[string]context.CancelFunc
	running       bool
	ctx           context.Context
	cancel        context.CancelFunc
	logger        *slog.Logger
}

// ServiceConfig holds detection service configuration
type ServiceConfig struct {
	DetectionAddr string        // gRPC address of detection service
	Go2RTCAddr    string        // go2rtc API address
	DefaultFPS    int           // Default detection FPS
	MinConfidence float64       // Default minimum confidence
}

// NewService creates a new detection service
func NewService(cfg *config.Config, eventService *events.Service, svcCfg ServiceConfig) (*Service, error) {
	// Set defaults
	if svcCfg.DetectionAddr == "" {
		svcCfg.DetectionAddr = "localhost:50051"
	}
	if svcCfg.Go2RTCAddr == "" {
		svcCfg.Go2RTCAddr = "http://localhost:1984"
	}
	if svcCfg.DefaultFPS == 0 {
		svcCfg.DefaultFPS = 5
	}
	if svcCfg.MinConfidence == 0 {
		svcCfg.MinConfidence = 0.5
	}

	// Create detection client
	client, err := NewClient(ClientConfig{
		Address: svcCfg.DetectionAddr,
		Timeout: 30 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	// Create frame grabber
	frameGrabber := NewGo2RTCFrameGrabber(svcCfg.Go2RTCAddr)

	return &Service{
		config:       cfg,
		client:       client,
		frameGrabber: frameGrabber,
		eventService: eventService,
		streams:      make(map[string]context.CancelFunc),
		logger:       slog.Default().With("component", "detection_service"),
	}, nil
}

// Start starts the detection service
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	s.ctx, s.cancel = context.WithCancel(ctx)

	// Start detection for enabled cameras
	for _, camera := range s.config.Cameras {
		if camera.Enabled && camera.Detection.Enabled {
			if err := s.startCamera(camera.ID, camera.Detection.FPS); err != nil {
				s.logger.Error("Failed to start detection", "camera", camera.ID, "error", err)
			}
		}
	}

	s.running = true
	s.logger.Info("Detection service started")

	return nil
}

// Stop stops the detection service
func (s *Service) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	// Stop all camera streams
	for cameraID, cancel := range s.streams {
		cancel()
		s.frameGrabber.StopStream(cameraID)
	}
	s.streams = make(map[string]context.CancelFunc)

	if s.cancel != nil {
		s.cancel()
	}

	// Close client
	if s.client != nil {
		s.client.Close()
	}

	// Close frame grabber
	s.frameGrabber.Close()

	s.running = false
	s.logger.Info("Detection service stopped")

	return nil
}

// StartCamera starts detection for a camera
func (s *Service) StartCamera(cameraID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	camCfg := s.config.GetCamera(cameraID)
	if camCfg == nil {
		return &DetectionError{Message: "camera not found: " + cameraID}
	}

	return s.startCamera(cameraID, camCfg.Detection.FPS)
}

// startCamera starts detection (must hold lock)
func (s *Service) startCamera(cameraID string, fps int) error {
	if _, exists := s.streams[cameraID]; exists {
		return nil // Already running
	}

	ctx, cancel := context.WithCancel(s.ctx)
	s.streams[cameraID] = cancel

	// Start frame stream
	frameCh, err := s.frameGrabber.StartStream(ctx, cameraID, fps)
	if err != nil {
		cancel()
		delete(s.streams, cameraID)
		return err
	}

	// Process frames in background
	go s.processFrames(ctx, cameraID, frameCh)

	s.logger.Info("Started detection", "camera", cameraID, "fps", fps)
	return nil
}

// StopCamera stops detection for a camera
func (s *Service) StopCamera(cameraID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cancel, exists := s.streams[cameraID]
	if !exists {
		return nil
	}

	cancel()
	s.frameGrabber.StopStream(cameraID)
	delete(s.streams, cameraID)

	s.logger.Info("Stopped detection", "camera", cameraID)
	return nil
}

// processFrames processes frames from a camera
func (s *Service) processFrames(ctx context.Context, cameraID string, frameCh <-chan *Frame) {
	camCfg := s.config.GetCamera(cameraID)
	if camCfg == nil {
		s.logger.Error("Camera config not found", "camera", cameraID)
		return
	}

	minConfidence := 0.5
	// Check for zone-level confidence settings
	for _, zone := range camCfg.Detection.Zones {
		if zone.MinConfidence > 0 {
			minConfidence = zone.MinConfidence
			break
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-frameCh:
			if !ok {
				return
			}

			s.processFrame(ctx, cameraID, frame, float64(minConfidence))
		}
	}
}

// processFrame processes a single frame
func (s *Service) processFrame(ctx context.Context, cameraID string, frame *Frame, minConfidence float64) {
	// Send to detection service
	resp, err := s.client.Detect(ctx, &DetectRequest{
		CameraID:      cameraID,
		Frame:         frame,
		MinConfidence: minConfidence,
	})
	if err != nil {
		s.logger.Debug("Detection failed", "camera", cameraID, "error", err)
		return
	}

	// Process detections
	for _, det := range resp.Detections {
		// Apply zone filtering
		if !s.inConfiguredZone(cameraID, det) {
			continue
		}

		// Create event
		s.createDetectionEvent(ctx, cameraID, det)
	}
}

// inConfiguredZone checks if a detection is in a configured zone
func (s *Service) inConfiguredZone(cameraID string, det Detection) bool {
	camCfg := s.config.GetCamera(cameraID)

	// Build combined zones from config and database
	var allZones []Zone

	// Add zones from config
	if camCfg != nil {
		for _, zoneCfg := range camCfg.Detection.Zones {
			if zoneCfg.Enabled {
				allZones = append(allZones, Zone{
					ID:            zoneCfg.ID,
					Points:        zoneCfg.Points,
					Objects:       zoneCfg.Objects,
					MinConfidence: zoneCfg.MinConfidence,
					Enabled:       true,
				})
			}
		}
	}

	// Add zones from database
	dbZones, err := s.eventService.GetEnabledZonesForCamera(context.Background(), cameraID)
	if err == nil && len(dbZones) > 0 {
		for _, dbZone := range dbZones {
			// Convert events.Point to [][]float64 for Zone.Points
			points := make([][]float64, len(dbZone.Points))
			for i, p := range dbZone.Points {
				points[i] = []float64{p.X, p.Y}
			}
			allZones = append(allZones, Zone{
				ID:            dbZone.ID,
				Name:          dbZone.Name,
				CameraID:      dbZone.CameraID,
				Points:        points,
				Objects:       dbZone.ObjectTypes,
				MinConfidence: dbZone.MinConfidence,
				MinSize:       dbZone.MinSize,
				Enabled:       true,
			})
		}
	}

	// If no zones configured anywhere, allow all detections
	if len(allZones) == 0 {
		return true
	}

	// Check each zone
	cx, cy := det.BoundingBox.Center()
	for _, zone := range allZones {
		// Check if object type is allowed
		if len(zone.Objects) > 0 {
			found := false
			for _, obj := range zone.Objects {
				if obj == string(det.ObjectType) || obj == det.Label {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		// Check confidence
		if zone.MinConfidence > 0 && det.Confidence < zone.MinConfidence {
			continue
		}

		// Check if detection is in zone
		if zone.ContainsPoint(cx, cy) {
			return true
		}
	}

	return false
}

// createDetectionEvent creates an event for a detection
func (s *Service) createDetectionEvent(ctx context.Context, cameraID string, det Detection) {
	eventType := events.EventType(det.ObjectType)

	// Build metadata as JSON
	metadata := map[string]interface{}{
		"bbox": map[string]float64{
			"x":      det.BoundingBox.X,
			"y":      det.BoundingBox.Y,
			"width":  det.BoundingBox.Width,
			"height": det.BoundingBox.Height,
		},
		"track_id":   det.TrackID,
		"model_id":   det.ModelID,
		"backend":    string(det.Backend),
		"attributes": det.Attributes,
	}
	metadataBytes, _ := json.Marshal(metadata)

	event := &events.Event{
		CameraID:   cameraID,
		EventType:  eventType,
		Label:      det.Label,
		Timestamp:  det.Timestamp,
		Confidence: det.Confidence,
		Metadata:   metadataBytes,
	}

	if err := s.eventService.Create(ctx, event); err != nil {
		s.logger.Error("Failed to create detection event", "error", err)
	}
}

// Detect performs a single detection on an image
func (s *Service) Detect(ctx context.Context, req *DetectRequest) (*DetectResponse, error) {
	return s.client.Detect(ctx, req)
}

// GetStatus returns detection service status
func (s *Service) GetStatus(ctx context.Context) (*ServiceStatus, error) {
	return s.client.GetStatus(ctx)
}

// LoadModel loads a detection model
func (s *Service) LoadModel(ctx context.Context, path string, modelType ModelType, backend BackendType) (string, error) {
	return s.client.LoadModel(ctx, path, modelType, backend)
}

// UnloadModel unloads a detection model
func (s *Service) UnloadModel(ctx context.Context, modelID string) error {
	return s.client.UnloadModel(ctx, modelID)
}

// GetBackends returns available detection backends
func (s *Service) GetBackends(ctx context.Context) ([]BackendInfo, error) {
	return s.client.GetBackends(ctx)
}

// GetModels returns loaded detection models
func (s *Service) GetModels(ctx context.Context) ([]ModelInfo, error) {
	return s.client.GetModels(ctx)
}

// OnConfigChange handles configuration changes
func (s *Service) OnConfigChange(cfg *config.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = cfg

	// Update detection streams based on new config
	for _, camera := range cfg.Cameras {
		_, running := s.streams[camera.ID]
		shouldRun := camera.Enabled && camera.Detection.Enabled

		if shouldRun && !running {
			if err := s.startCamera(camera.ID, camera.Detection.FPS); err != nil {
				s.logger.Error("Failed to start detection on config change", "camera", camera.ID, "error", err)
			}
		} else if !shouldRun && running {
			if cancel, exists := s.streams[camera.ID]; exists {
				cancel()
				s.frameGrabber.StopStream(camera.ID)
				delete(s.streams, camera.ID)
			}
		}
	}
}

// DetectionError represents a detection error
type DetectionError struct {
	Message string
}

func (e *DetectionError) Error() string {
	return e.Message
}
