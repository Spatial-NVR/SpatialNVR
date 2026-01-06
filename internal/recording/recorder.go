package recording

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Spatial-NVR/SpatialNVR/internal/config"
	"github.com/Spatial-NVR/SpatialNVR/internal/video"
)

// Recorder manages FFmpeg recording for a single camera
type Recorder struct {
	cameraID    string
	config      *config.CameraConfig
	storagePath string

	mu             sync.RWMutex
	state          RecorderState
	cmd            *exec.Cmd
	cancel         context.CancelFunc
	currentSegment string
	segmentStart   time.Time
	bytesWritten   int64
	segmentsCount  int
	startTime      time.Time
	lastError      string
	lastErrorTime  time.Time

	onSegmentComplete func(segment *Segment)
	segmentHandler    SegmentHandler
	logger            *slog.Logger
}

// NewRecorder creates a new camera recorder
// Records directly from the camera's native stream URL (RTSP, RTMP, HLS, HTTP, etc.)
func NewRecorder(
	cameraID string,
	cfg *config.CameraConfig,
	storagePath string,
	segmentHandler SegmentHandler,
	onSegmentComplete func(segment *Segment),
) *Recorder {
	return &Recorder{
		cameraID:          cameraID,
		config:            cfg,
		storagePath:       storagePath,
		state:             RecorderStateIdle,
		segmentHandler:    segmentHandler,
		onSegmentComplete: onSegmentComplete,
		logger:            slog.Default().With("component", "recorder", "camera", cameraID),
	}
}

// Start begins recording for this camera
func (r *Recorder) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.state == RecorderStateRunning {
		r.mu.Unlock()
		return nil
	}
	r.state = RecorderStateStarting
	r.mu.Unlock()

	// Create camera recording directory
	cameraDir := filepath.Join(r.storagePath, r.cameraID)
	if err := os.MkdirAll(cameraDir, 0755); err != nil {
		r.setError(fmt.Errorf("failed to create recording directory: %w", err))
		return err
	}

	// Start FFmpeg in a goroutine
	go r.runFFmpeg(ctx)

	return nil
}

// Stop stops recording for this camera
func (r *Recorder) Stop() error {
	r.mu.Lock()
	if r.state != RecorderStateRunning && r.state != RecorderStateStarting {
		r.mu.Unlock()
		return nil
	}
	r.state = RecorderStateStopping
	cancel := r.cancel
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	// Wait for FFmpeg to stop (with timeout)
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			r.mu.Lock()
			if r.cmd != nil && r.cmd.Process != nil {
				_ = r.cmd.Process.Kill()
			}
			r.state = RecorderStateIdle
			r.mu.Unlock()
			return nil
		case <-ticker.C:
			r.mu.RLock()
			state := r.state
			r.mu.RUnlock()
			if state == RecorderStateIdle {
				return nil
			}
		}
	}
}

// Status returns the current recorder status
func (r *Recorder) Status() *RecorderStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	status := &RecorderStatus{
		CameraID:        r.cameraID,
		State:           r.state,
		CurrentSegment:  r.currentSegment,
		BytesWritten:    r.bytesWritten,
		SegmentsCreated: r.segmentsCount,
		LastError:       r.lastError,
	}

	if !r.segmentStart.IsZero() {
		t := r.segmentStart
		status.SegmentStart = &t
	}
	if !r.lastErrorTime.IsZero() {
		t := r.lastErrorTime
		status.LastErrorTime = &t
	}
	if !r.startTime.IsZero() {
		status.Uptime = time.Since(r.startTime).Seconds()
	}

	return status
}

// runFFmpeg runs the FFmpeg process
func (r *Recorder) runFFmpeg(parentCtx context.Context) {
	ctx, cancel := context.WithCancel(parentCtx)
	r.mu.Lock()
	r.cancel = cancel
	r.startTime = time.Now()
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		r.state = RecorderStateIdle
		r.cmd = nil
		r.cancel = nil
		r.mu.Unlock()
		cancel()
	}()

	// Build FFmpeg command
	args := r.buildFFmpegArgs()
	r.logger.Info("Starting FFmpeg", "args", strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)

	// Capture stderr for logging and segment detection
	stderr, err := cmd.StderrPipe()
	if err != nil {
		r.setError(fmt.Errorf("failed to create stderr pipe: %w", err))
		return
	}

	if err := cmd.Start(); err != nil {
		r.setError(fmt.Errorf("failed to start FFmpeg: %w", err))
		return
	}

	r.mu.Lock()
	r.cmd = cmd
	r.state = RecorderStateRunning
	r.mu.Unlock()

	r.logger.Info("FFmpeg started", "pid", cmd.Process.Pid)

	// Parse FFmpeg output for segment information
	go r.parseFFmpegOutput(bufio.NewReader(stderr))

	// Wait for FFmpeg to exit
	err = cmd.Wait()
	if err != nil && ctx.Err() == nil {
		r.setError(fmt.Errorf("FFmpeg exited with error: %w", err))
		r.logger.Error("FFmpeg exited with error", "error", err)
	} else {
		r.logger.Info("FFmpeg stopped")
	}
}

// sanitizeStreamName ensures the stream name is valid for go2rtc (matches streaming package)
func sanitizeStreamName(name string) string {
	replacer := strings.NewReplacer(
		" ", "_",
		"-", "_",
		".", "_",
		"/", "_",
		"\\", "_",
	)
	return strings.ToLower(replacer.Replace(name))
}

// buildStreamURL constructs the stream URL with authentication if needed
// Uses substream if configured via Stream.Roles.Record = "sub"
func (r *Recorder) buildStreamURL() string {
	streamURL := r.config.Stream.URL

	// Check if we should use substream for recording
	// This allows using H.265 substream for storage efficiency while keeping H.264 main for live
	if r.config.Stream.Roles != nil && r.config.Stream.Roles.Record == "sub" {
		if r.config.Stream.SubURL != "" {
			streamURL = r.config.Stream.SubURL
			r.logger.Debug("Using substream for recording", "camera", r.cameraID)
		} else {
			r.logger.Warn("Record role set to 'sub' but no SubURL configured, falling back to main stream", "camera", r.cameraID)
		}
	}

	if streamURL == "" {
		r.logger.Error("No stream URL configured for camera", "camera", r.cameraID)
		return ""
	}

	// Add authentication to URL if configured AND URL doesn't already have credentials
	// Check for @ symbol after the protocol:// to detect existing credentials
	if r.config.Stream.Username != "" && r.config.Stream.Password != "" && !urlHasCredentials(streamURL) {
		// Parse the URL to inject credentials
		if strings.HasPrefix(streamURL, "rtsp://") {
			streamURL = fmt.Sprintf("rtsp://%s:%s@%s",
				r.config.Stream.Username,
				r.config.Stream.Password,
				strings.TrimPrefix(streamURL, "rtsp://"))
		} else if strings.HasPrefix(streamURL, "http://") {
			streamURL = fmt.Sprintf("http://%s:%s@%s",
				r.config.Stream.Username,
				r.config.Stream.Password,
				strings.TrimPrefix(streamURL, "http://"))
		} else if strings.HasPrefix(streamURL, "https://") {
			streamURL = fmt.Sprintf("https://%s:%s@%s",
				r.config.Stream.Username,
				r.config.Stream.Password,
				strings.TrimPrefix(streamURL, "https://"))
		}
	}

	return streamURL
}

// urlHasCredentials checks if a URL already contains embedded credentials
func urlHasCredentials(urlStr string) bool {
	// Remove protocol prefix and check for @ before first /
	for _, proto := range []string{"rtsp://", "http://", "https://", "rtmp://"} {
		if strings.HasPrefix(urlStr, proto) {
			rest := strings.TrimPrefix(urlStr, proto)
			// Find the host part (before first /)
			slashIdx := strings.Index(rest, "/")
			hostPart := rest
			if slashIdx != -1 {
				hostPart = rest[:slashIdx]
			}
			// If there's an @ in the host part, credentials are present
			return strings.Contains(hostPart, "@")
		}
	}
	return false
}

// buildFFmpegArgs constructs the FFmpeg command arguments
func (r *Recorder) buildFFmpegArgs() []string {
	// Use the camera's native stream URL directly
	streamURL := r.buildStreamURL()
	if streamURL == "" {
		return nil
	}

	r.logger.Info("Recording from stream", "url", sanitizeURLForLog(streamURL))

	// Output path pattern
	outputPattern := filepath.Join(r.storagePath, r.cameraID, "%Y-%m-%d_%H-%M-%S.mp4")

	// Segment duration (default 10 seconds for quicker event isolation)
	segmentDuration := r.config.Recording.SegmentDuration
	if segmentDuration <= 0 {
		segmentDuration = 10
	}

	// Start with hardware acceleration args
	var hwAccelArgs []string

	// Use configured hardware acceleration, or auto-detect
	if r.config.Advanced.HWAccel != "" {
		hwAccelArgs = video.GetFFmpegHWAccelArgs(video.HWAccelType(r.config.Advanced.HWAccel))
		r.logger.Info("Using configured hardware acceleration", "type", r.config.Advanced.HWAccel)
	} else {
		// Auto-detect hardware acceleration
		detector := video.GetGlobalDetector()
		recommended := detector.GetRecommended(context.Background())
		if recommended != video.HWAccelNone {
			hwAccelArgs = video.GetFFmpegHWAccelArgs(recommended)
			r.logger.Info("Using auto-detected hardware acceleration", "type", recommended)
		}
	}

	args := []string{"-hide_banner", "-loglevel", "info"}

	// Add hardware acceleration args if available
	if len(hwAccelArgs) > 0 {
		args = append(args, hwAccelArgs...)
	}

	// Input processing flags for reliability (must come BEFORE -i)
	args = append(args,
		"-fflags", "+genpts+discardcorrupt", // Generate PTS if missing, discard corrupt frames
		"-avoid_negative_ts", "make_zero", // Ensure timestamps start at 0
		"-max_delay", "500000", // 500ms max demux delay for low latency
	)

	// Add protocol-specific input options with optimizations
	if strings.HasPrefix(streamURL, "rtsp://") {
		args = append(args,
			"-rtsp_transport", "tcp",
			"-buffer_size", "1024000", // 1MB buffer for network jitter
			"-stimeout", "5000000", // 5 second socket timeout (microseconds)
		)
	} else if strings.HasPrefix(streamURL, "rtmp://") {
		args = append(args, "-live_start_index", "-1")
	}
	// HLS, HTTP, and other protocols don't need special options

	// Add input
	args = append(args, "-i", streamURL)

	// Output args - stream copy (no transcoding)
	args = append(args,
		"-c:v", "copy",
		"-c:a", "copy",
		"-f", "segment",
		"-segment_time", strconv.Itoa(segmentDuration),
		"-segment_format", "mp4",
		"-segment_atclocktime", "1",
		"-strftime", "1",
		"-movflags", "+frag_keyframe+empty_moov+default_base_moof",
		"-reset_timestamps", "1",
		outputPattern,
	)

	return args
}

// sanitizeURLForLog removes credentials from URL for safe logging
func sanitizeURLForLog(url string) string {
	// Simple pattern to remove credentials from URL
	// Matches protocol://user:pass@host...
	for _, proto := range []string{"rtsp://", "http://", "https://", "rtmp://"} {
		if strings.HasPrefix(url, proto) {
			remainder := strings.TrimPrefix(url, proto)
			if atIdx := strings.Index(remainder, "@"); atIdx != -1 {
				return proto + "***:***@" + remainder[atIdx+1:]
			}
		}
	}
	return url
}

// parseFFmpegOutput parses FFmpeg stderr for segment completion
func (r *Recorder) parseFFmpegOutput(stderr *bufio.Reader) {
	scanner := bufio.NewScanner(stderr)

	// Pattern to detect new segment being opened
	segmentPattern := regexp.MustCompile(`Opening '(.+\.mp4)' for writing`)

	var previousSegment string

	for scanner.Scan() {
		line := scanner.Text()

		// Log FFmpeg output
		if strings.Contains(line, "error") || strings.Contains(line, "Error") {
			r.logger.Warn("FFmpeg output", "line", line)
		}

		// Check for new segment
		if matches := segmentPattern.FindStringSubmatch(line); len(matches) > 1 {
			newSegment := matches[1]

			// Process the previous segment if it exists
			if previousSegment != "" {
				go r.processCompletedSegment(previousSegment)
			}

			r.mu.Lock()
			r.currentSegment = newSegment
			r.segmentStart = time.Now()
			r.segmentsCount++
			r.mu.Unlock()

			previousSegment = newSegment
			r.logger.Debug("New segment started", "path", newSegment)
		}
	}

	// Process the last segment when FFmpeg stops
	if previousSegment != "" {
		r.processCompletedSegment(previousSegment)
	}
}

// processCompletedSegment handles a completed segment
func (r *Recorder) processCompletedSegment(filePath string) {
	// Wait a moment for file to be fully written
	time.Sleep(500 * time.Millisecond)

	// Check if file exists
	info, err := os.Stat(filePath)
	if err != nil {
		r.logger.Error("Failed to stat segment file", "path", filePath, "error", err)
		return
	}

	// Extract metadata
	metadata, err := r.segmentHandler.ExtractMetadata(filePath)
	if err != nil {
		r.logger.Warn("Failed to extract segment metadata", "path", filePath, "error", err)
		// Use defaults if extraction fails
		metadata = &SegmentMetadata{
			FileSize: info.Size(),
		}
	}

	// Parse start time from filename
	startTime := r.parseTimeFromFilename(filePath)
	if startTime.IsZero() {
		startTime = info.ModTime().Add(-time.Duration(metadata.Duration) * time.Second)
	}

	// Create segment record
	segment := &Segment{
		CameraID:      r.cameraID,
		StartTime:     startTime,
		EndTime:       startTime.Add(time.Duration(metadata.Duration * float64(time.Second))),
		Duration:      metadata.Duration,
		FilePath:      filePath,
		FileSize:      metadata.FileSize,
		StorageTier:   StorageTierHot,
		Codec:         metadata.Codec,
		Resolution:    metadata.Resolution,
		Bitrate:       metadata.Bitrate,
		RecordingMode: string(RecordingModeContinuous),
	}

	r.mu.Lock()
	r.bytesWritten += segment.FileSize
	r.mu.Unlock()

	// Notify about completed segment
	if r.onSegmentComplete != nil {
		r.onSegmentComplete(segment)
	}

	r.logger.Info("Segment completed",
		"path", filePath,
		"duration", segment.Duration,
		"size", segment.FileSize,
	)
}

// parseTimeFromFilename extracts timestamp from segment filename
func (r *Recorder) parseTimeFromFilename(filePath string) time.Time {
	// Expected format: /path/to/camera_id/2024-01-15_10-30-00.mp4
	base := filepath.Base(filePath)
	base = strings.TrimSuffix(base, filepath.Ext(base))

	t, err := time.ParseInLocation("2006-01-02_15-04-05", base, time.Local)
	if err != nil {
		return time.Time{}
	}
	return t
}

// setError sets the error state
func (r *Recorder) setError(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = RecorderStateError
	r.lastError = err.Error()
	r.lastErrorTime = time.Now()
	r.logger.Error("Recorder error", "error", err)
}
