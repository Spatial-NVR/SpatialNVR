package detection

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Go2RTCFrameGrabber grabs frames from go2rtc
type Go2RTCFrameGrabber struct {
	mu         sync.RWMutex
	baseURL    string
	httpClient *http.Client
	streams    map[string]chan struct{}
	logger     *slog.Logger
}

// NewGo2RTCFrameGrabber creates a new frame grabber for go2rtc
func NewGo2RTCFrameGrabber(baseURL string) *Go2RTCFrameGrabber {
	return &Go2RTCFrameGrabber{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		streams: make(map[string]chan struct{}),
		logger:  slog.Default().With("component", "frame_grabber"),
	}
}

// GrabFrame grabs a single frame from a camera
func (g *Go2RTCFrameGrabber) GrabFrame(ctx context.Context, cameraID string) (*Frame, error) {
	// go2rtc uses lowercase stream names
	streamName := strings.ToLower(strings.ReplaceAll(cameraID, " ", "_"))
	url := fmt.Sprintf("%s/api/frame.jpeg?src=%s", g.baseURL, streamName)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch frame: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read image data
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read frame data: %w", err)
	}

	// Decode image
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	bounds := img.Bounds()

	return &Frame{
		CameraID:  cameraID,
		Timestamp: time.Now(),
		Image:     img,
		Data:      data,
		Width:     bounds.Dx(),
		Height:    bounds.Dy(),
		Format:    "jpeg",
	}, nil
}

// StartStream starts a continuous frame stream
func (g *Go2RTCFrameGrabber) StartStream(ctx context.Context, cameraID string, fps int) (<-chan *Frame, error) {
	g.mu.Lock()
	if _, exists := g.streams[cameraID]; exists {
		g.mu.Unlock()
		return nil, fmt.Errorf("stream already running for camera: %s", cameraID)
	}

	stopCh := make(chan struct{})
	g.streams[cameraID] = stopCh
	g.mu.Unlock()

	// Default to 5 FPS if not specified or invalid
	if fps <= 0 {
		fps = 5
	}

	frameCh := make(chan *Frame, 10)
	interval := time.Second / time.Duration(fps)

	go func() {
		defer close(frameCh)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		var frameID int64

		for {
			select {
			case <-ctx.Done():
				return
			case <-stopCh:
				return
			case <-ticker.C:
				frame, err := g.GrabFrame(ctx, cameraID)
				if err != nil {
					g.logger.Warn("Failed to grab frame", "camera", cameraID, "error", err)
					continue
				}

				frameID++
				frame.FrameID = frameID

				select {
				case frameCh <- frame:
				default:
					// Drop frame if channel is full
					g.logger.Debug("Dropped frame", "camera", cameraID, "frame", frameID)
				}
			}
		}
	}()

	return frameCh, nil
}

// StopStream stops a frame stream
func (g *Go2RTCFrameGrabber) StopStream(cameraID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	stopCh, exists := g.streams[cameraID]
	if !exists {
		return nil
	}

	close(stopCh)
	delete(g.streams, cameraID)
	return nil
}

// Close closes the frame grabber
func (g *Go2RTCFrameGrabber) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	for cameraID, stopCh := range g.streams {
		close(stopCh)
		delete(g.streams, cameraID)
	}

	return nil
}

// RTSPFrameGrabber grabs frames directly from RTSP streams
type RTSPFrameGrabber struct {
	mu      sync.RWMutex
	streams map[string]*rtspStream
	logger  *slog.Logger
}

type rtspStream struct {
	stopCh chan struct{}
}

// NewRTSPFrameGrabber creates a new RTSP frame grabber
func NewRTSPFrameGrabber() *RTSPFrameGrabber {
	return &RTSPFrameGrabber{
		streams: make(map[string]*rtspStream),
		logger:  slog.Default().With("component", "rtsp_frame_grabber"),
	}
}

// GrabFrame grabs a frame using FFmpeg
func (g *RTSPFrameGrabber) GrabFrame(ctx context.Context, cameraID string) (*Frame, error) {
	// This would use FFmpeg to grab a single frame
	// ffmpeg -rtsp_transport tcp -i rtsp://... -frames:v 1 -f image2pipe -
	return nil, fmt.Errorf("not implemented - use Go2RTCFrameGrabber")
}

// StartStream starts an RTSP stream capture
func (g *RTSPFrameGrabber) StartStream(ctx context.Context, cameraID string, fps int) (<-chan *Frame, error) {
	// This would start an FFmpeg process to decode the RTSP stream
	return nil, fmt.Errorf("not implemented - use Go2RTCFrameGrabber")
}

// StopStream stops an RTSP stream
func (g *RTSPFrameGrabber) StopStream(cameraID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	stream, exists := g.streams[cameraID]
	if !exists {
		return nil
	}

	close(stream.stopCh)
	delete(g.streams, cameraID)
	return nil
}

// Close closes all streams
func (g *RTSPFrameGrabber) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	for _, stream := range g.streams {
		close(stream.stopCh)
	}
	g.streams = make(map[string]*rtspStream)

	return nil
}

// ImageToBytes converts an image to JPEG bytes
func ImageToBytes(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ImageToRGB converts an image to RGB byte array
func ImageToRGB(img image.Image) []byte {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	rgb := make([]byte, width*height*3)

	idx := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			rgb[idx] = byte(r >> 8)
			rgb[idx+1] = byte(g >> 8)
			rgb[idx+2] = byte(b >> 8)
			idx += 3
		}
	}

	return rgb
}
