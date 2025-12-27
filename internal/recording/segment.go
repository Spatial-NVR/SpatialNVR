package recording

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DefaultSegmentHandler implements SegmentHandler using FFprobe
type DefaultSegmentHandler struct {
	storagePath   string
	thumbnailPath string
}

// NewDefaultSegmentHandler creates a new segment handler
func NewDefaultSegmentHandler(storagePath, thumbnailPath string) *DefaultSegmentHandler {
	return &DefaultSegmentHandler{
		storagePath:   storagePath,
		thumbnailPath: thumbnailPath,
	}
}

// CreatePath creates a new segment file path
func (h *DefaultSegmentHandler) CreatePath(cameraID string, startTime time.Time) string {
	dir := filepath.Join(h.storagePath, cameraID)
	filename := startTime.Format("2006-01-02_15-04-05") + ".mp4"
	return filepath.Join(dir, filename)
}

// ExtractMetadata extracts metadata from a segment file using ffprobe
func (h *DefaultSegmentHandler) ExtractMetadata(filePath string) (*SegmentMetadata, error) {
	// Check if file exists
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}

	// Run ffprobe
	args := []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		filePath,
	}

	cmd := exec.Command("ffprobe", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	// Parse ffprobe output
	var probeData struct {
		Format struct {
			Duration   string `json:"duration"`
			BitRate    string `json:"bit_rate"`
			FormatName string `json:"format_name"`
		} `json:"format"`
		Streams []struct {
			CodecType  string `json:"codec_type"`
			CodecName  string `json:"codec_name"`
			Width      int    `json:"width"`
			Height     int    `json:"height"`
			RFrameRate string `json:"r_frame_rate"`
		} `json:"streams"`
	}

	if err := json.Unmarshal(output, &probeData); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}

	// Extract metadata
	metadata := &SegmentMetadata{
		FileSize: info.Size(),
	}

	// Parse duration
	if probeData.Format.Duration != "" {
		if duration, err := strconv.ParseFloat(probeData.Format.Duration, 64); err == nil {
			metadata.Duration = duration
		}
	}

	// Parse bitrate
	if probeData.Format.BitRate != "" {
		if bitrate, err := strconv.Atoi(probeData.Format.BitRate); err == nil {
			metadata.Bitrate = bitrate
		}
	}

	// Find video stream
	for _, stream := range probeData.Streams {
		if stream.CodecType == "video" {
			metadata.Codec = stream.CodecName
			metadata.Resolution = fmt.Sprintf("%dx%d", stream.Width, stream.Height)
			break
		}
	}

	// Calculate start/end times from file mod time and duration
	metadata.EndTime = info.ModTime()
	metadata.StartTime = metadata.EndTime.Add(-time.Duration(metadata.Duration * float64(time.Second)))

	return metadata, nil
}

// GenerateThumbnail generates a thumbnail from a segment at the specified offset
func (h *DefaultSegmentHandler) GenerateThumbnail(segmentPath, thumbnailPath string, offsetSeconds float64) error {
	// Ensure thumbnail directory exists
	if err := os.MkdirAll(filepath.Dir(thumbnailPath), 0755); err != nil {
		return fmt.Errorf("failed to create thumbnail directory: %w", err)
	}

	// Use ffmpeg to extract a frame
	args := []string{
		"-ss", fmt.Sprintf("%.2f", offsetSeconds),
		"-i", segmentPath,
		"-vframes", "1",
		"-q:v", "2",
		"-y",
		thumbnailPath,
	}

	cmd := exec.Command("ffmpeg", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg failed: %s: %w", string(output), err)
	}

	return nil
}

// GenerateThumbnailAuto generates a thumbnail from the middle of a segment
func (h *DefaultSegmentHandler) GenerateThumbnailAuto(segmentPath string) (string, error) {
	// Extract metadata to get duration
	metadata, err := h.ExtractMetadata(segmentPath)
	if err != nil {
		return "", err
	}

	// Calculate thumbnail path
	baseName := strings.TrimSuffix(filepath.Base(segmentPath), filepath.Ext(segmentPath))
	cameraDir := filepath.Base(filepath.Dir(segmentPath))
	thumbnailPath := filepath.Join(h.thumbnailPath, cameraDir, baseName+".jpg")

	// Generate at middle of segment
	offset := metadata.Duration / 2
	if err := h.GenerateThumbnail(segmentPath, thumbnailPath, offset); err != nil {
		return "", err
	}

	return thumbnailPath, nil
}

// CalculateChecksum calculates SHA256 checksum of a file
func (h *DefaultSegmentHandler) CalculateChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// Delete deletes a segment file and its associated thumbnail
func (h *DefaultSegmentHandler) Delete(segment *Segment) error {
	// Delete main segment file
	if err := os.Remove(segment.FilePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete segment file: %w", err)
	}

	// Delete thumbnail if exists
	if segment.Thumbnail != "" {
		// Ignore error - thumbnail deletion is best effort
		_ = os.Remove(segment.Thumbnail)
	}

	return nil
}

// GetStreamInfo extracts detailed stream information from a file
func (h *DefaultSegmentHandler) GetStreamInfo(filePath string) (*StreamInfo, error) {
	args := []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		filePath,
	}

	cmd := exec.Command("ffprobe", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	var probeData struct {
		Format struct {
			Duration string `json:"duration"`
			BitRate  string `json:"bit_rate"`
		} `json:"format"`
		Streams []struct {
			CodecType  string `json:"codec_type"`
			CodecName  string `json:"codec_name"`
			Width      int    `json:"width"`
			Height     int    `json:"height"`
			RFrameRate string `json:"r_frame_rate"`
			BitRate    string `json:"bit_rate"`
		} `json:"streams"`
	}

	if err := json.Unmarshal(output, &probeData); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}

	info := &StreamInfo{}

	// Parse duration
	if probeData.Format.Duration != "" {
		if duration, err := strconv.ParseFloat(probeData.Format.Duration, 64); err == nil {
			info.Duration = duration
		}
	}

	// Parse format bitrate
	if probeData.Format.BitRate != "" {
		if bitrate, err := strconv.Atoi(probeData.Format.BitRate); err == nil {
			info.Bitrate = bitrate
		}
	}

	// Process streams
	for _, stream := range probeData.Streams {
		switch stream.CodecType {
		case "video":
			info.Codec = stream.CodecName
			info.Width = stream.Width
			info.Height = stream.Height

			// Parse frame rate (format: "30000/1001" or "30/1")
			if stream.RFrameRate != "" {
				parts := strings.Split(stream.RFrameRate, "/")
				if len(parts) == 2 {
					num, _ := strconv.ParseFloat(parts[0], 64)
					den, _ := strconv.ParseFloat(parts[1], 64)
					if den > 0 {
						info.FPS = num / den
					}
				}
			}

		case "audio":
			info.HasAudio = true
			info.AudioCodec = stream.CodecName
		}
	}

	return info, nil
}

// ValidateSegment checks if a segment file is valid and playable
func (h *DefaultSegmentHandler) ValidateSegment(filePath string) error {
	// Check file exists and has size
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("file not accessible: %w", err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("file is empty")
	}

	// Use ffprobe to validate
	args := []string{
		"-v", "error",
		"-i", filePath,
		"-f", "null",
		"-",
	}

	cmd := exec.Command("ffprobe", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("validation failed: %s", string(output))
	}

	return nil
}

// MergeSegments merges multiple segments into a single file
func (h *DefaultSegmentHandler) MergeSegments(segments []string, outputPath string) error {
	if len(segments) == 0 {
		return fmt.Errorf("no segments to merge")
	}

	// Create concat file
	concatFile, err := os.CreateTemp("", "concat_*.txt")
	if err != nil {
		return fmt.Errorf("failed to create concat file: %w", err)
	}
	defer func() { _ = os.Remove(concatFile.Name()) }()

	// Write segment paths
	for _, seg := range segments {
		absPath, _ := filepath.Abs(seg)
		_, _ = fmt.Fprintf(concatFile, "file '%s'\n", absPath)
	}
	_ = concatFile.Close()

	// Run ffmpeg concat
	args := []string{
		"-f", "concat",
		"-safe", "0",
		"-i", concatFile.Name(),
		"-c", "copy",
		"-y",
		outputPath,
	}

	cmd := exec.Command("ffmpeg", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("merge failed: %s: %w", string(output), err)
	}

	return nil
}
