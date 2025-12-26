// Package video provides video processing utilities including hardware acceleration detection
package video

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// HWAccelType represents a hardware acceleration type
type HWAccelType string

const (
	HWAccelNone        HWAccelType = ""
	HWAccelCUDA        HWAccelType = "cuda"        // NVIDIA GPU
	HWAccelVideoToolbox HWAccelType = "videotoolbox" // macOS
	HWAccelVAAPI       HWAccelType = "vaapi"       // Linux VA-API
	HWAccelQSV         HWAccelType = "qsv"         // Intel Quick Sync
	HWAccelD3D11VA     HWAccelType = "d3d11va"     // Windows DirectX 11
	HWAccelDXVA2       HWAccelType = "dxva2"       // Windows DirectX 9
	HWAccelVulkan      HWAccelType = "vulkan"      // Vulkan (cross-platform)
)

// HWAccelCapabilities describes available hardware acceleration
type HWAccelCapabilities struct {
	Available    []HWAccelType `json:"available"`
	Recommended  HWAccelType   `json:"recommended"`
	DecodeH264   bool          `json:"decode_h264"`
	DecodeH265   bool          `json:"decode_h265"`
	EncodeH264   bool          `json:"encode_h264"`
	EncodeH265   bool          `json:"encode_h265"`
	GPUName      string        `json:"gpu_name,omitempty"`
	DetectedAt   time.Time     `json:"detected_at"`
}

// HWAccelDetector detects and tests hardware acceleration capabilities
type HWAccelDetector struct {
	mu           sync.RWMutex
	capabilities *HWAccelCapabilities
	logger       *slog.Logger
}

// NewHWAccelDetector creates a new hardware acceleration detector
func NewHWAccelDetector() *HWAccelDetector {
	return &HWAccelDetector{
		logger: slog.Default().With("component", "hwaccel"),
	}
}

// Detect detects available hardware acceleration
func (d *HWAccelDetector) Detect(ctx context.Context) (*HWAccelCapabilities, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.logger.Info("Detecting hardware acceleration capabilities")

	caps := &HWAccelCapabilities{
		Available:  make([]HWAccelType, 0),
		DetectedAt: time.Now(),
	}

	// Check FFmpeg is available
	if !d.checkFFmpeg() {
		d.logger.Warn("FFmpeg not found, hardware acceleration unavailable")
		d.capabilities = caps
		return caps, nil
	}

	// Detect based on OS
	switch runtime.GOOS {
	case "darwin":
		d.detectMacOS(ctx, caps)
	case "linux":
		d.detectLinux(ctx, caps)
	case "windows":
		d.detectWindows(ctx, caps)
	}

	// Determine recommended acceleration
	caps.Recommended = d.selectRecommended(caps.Available)

	d.capabilities = caps
	d.logger.Info("Hardware acceleration detection complete",
		"available", caps.Available,
		"recommended", caps.Recommended,
		"gpu", caps.GPUName,
	)

	return caps, nil
}

// GetCapabilities returns cached capabilities or detects if not cached
func (d *HWAccelDetector) GetCapabilities(ctx context.Context) (*HWAccelCapabilities, error) {
	d.mu.RLock()
	if d.capabilities != nil {
		caps := d.capabilities
		d.mu.RUnlock()
		return caps, nil
	}
	d.mu.RUnlock()

	return d.Detect(ctx)
}

// GetRecommended returns the recommended hardware acceleration type
func (d *HWAccelDetector) GetRecommended(ctx context.Context) HWAccelType {
	caps, err := d.GetCapabilities(ctx)
	if err != nil || caps == nil {
		return HWAccelNone
	}
	return caps.Recommended
}

// GetFFmpegArgs returns FFmpeg arguments for the recommended hardware acceleration
func (d *HWAccelDetector) GetFFmpegArgs(ctx context.Context) []string {
	accel := d.GetRecommended(ctx)
	return GetFFmpegHWAccelArgs(accel)
}

// GetFFmpegHWAccelArgs returns FFmpeg arguments for a specific acceleration type
func GetFFmpegHWAccelArgs(accel HWAccelType) []string {
	switch accel {
	case HWAccelCUDA:
		return []string{"-hwaccel", "cuda", "-hwaccel_output_format", "cuda"}
	case HWAccelVideoToolbox:
		return []string{"-hwaccel", "videotoolbox"}
	case HWAccelVAAPI:
		return []string{"-hwaccel", "vaapi", "-hwaccel_device", "/dev/dri/renderD128"}
	case HWAccelQSV:
		return []string{"-hwaccel", "qsv"}
	case HWAccelD3D11VA:
		return []string{"-hwaccel", "d3d11va"}
	case HWAccelDXVA2:
		return []string{"-hwaccel", "dxva2"}
	case HWAccelVulkan:
		return []string{"-hwaccel", "vulkan"}
	default:
		return nil
	}
}

// checkFFmpeg verifies FFmpeg is installed
func (d *HWAccelDetector) checkFFmpeg() bool {
	cmd := exec.Command("ffmpeg", "-version")
	return cmd.Run() == nil
}

// detectMacOS detects hardware acceleration on macOS
func (d *HWAccelDetector) detectMacOS(ctx context.Context, caps *HWAccelCapabilities) {
	// VideoToolbox is always available on macOS
	if d.testVideoToolbox(ctx) {
		caps.Available = append(caps.Available, HWAccelVideoToolbox)
		caps.DecodeH264 = true
		caps.DecodeH265 = true
		caps.EncodeH264 = true
		caps.EncodeH265 = true
	}

	// Get GPU info
	caps.GPUName = d.getMacGPUName()
}

// detectLinux detects hardware acceleration on Linux
func (d *HWAccelDetector) detectLinux(ctx context.Context, caps *HWAccelCapabilities) {
	// Check for NVIDIA GPU (CUDA)
	if d.hasNVIDIAGPU() && d.testCUDA(ctx) {
		caps.Available = append(caps.Available, HWAccelCUDA)
		caps.GPUName = d.getNVIDIAGPUName()
		caps.DecodeH264 = true
		caps.DecodeH265 = true
		caps.EncodeH264 = true
		caps.EncodeH265 = true
	}

	// Check for VA-API (Intel/AMD integrated graphics)
	if d.hasVAAPI() && d.testVAAPI(ctx) {
		caps.Available = append(caps.Available, HWAccelVAAPI)
		if caps.GPUName == "" {
			caps.GPUName = d.getVAAPIGPUName()
		}
		caps.DecodeH264 = true
		caps.DecodeH265 = true
		caps.EncodeH264 = true
	}

	// Check for Intel Quick Sync
	if d.hasQSV() && d.testQSV(ctx) {
		caps.Available = append(caps.Available, HWAccelQSV)
		caps.DecodeH264 = true
		caps.DecodeH265 = true
		caps.EncodeH264 = true
	}
}

// detectWindows detects hardware acceleration on Windows
func (d *HWAccelDetector) detectWindows(ctx context.Context, caps *HWAccelCapabilities) {
	// Check for NVIDIA GPU
	if d.hasNVIDIAGPU() && d.testCUDA(ctx) {
		caps.Available = append(caps.Available, HWAccelCUDA)
		caps.GPUName = d.getNVIDIAGPUName()
		caps.DecodeH264 = true
		caps.DecodeH265 = true
		caps.EncodeH264 = true
		caps.EncodeH265 = true
	}

	// Check for D3D11VA (DirectX 11)
	if d.testD3D11VA(ctx) {
		caps.Available = append(caps.Available, HWAccelD3D11VA)
		caps.DecodeH264 = true
		caps.DecodeH265 = true
	}

	// Check for Intel Quick Sync
	if d.hasQSV() && d.testQSV(ctx) {
		caps.Available = append(caps.Available, HWAccelQSV)
		caps.DecodeH264 = true
		caps.DecodeH265 = true
		caps.EncodeH264 = true
	}
}

// selectRecommended selects the best available acceleration
func (d *HWAccelDetector) selectRecommended(available []HWAccelType) HWAccelType {
	// Priority order (best performance first)
	priority := []HWAccelType{
		HWAccelCUDA,        // NVIDIA is generally fastest
		HWAccelVideoToolbox, // macOS native
		HWAccelQSV,         // Intel Quick Sync
		HWAccelVAAPI,       // Linux VA-API
		HWAccelD3D11VA,     // Windows DirectX 11
		HWAccelDXVA2,       // Windows DirectX 9
		HWAccelVulkan,      // Cross-platform Vulkan
	}

	for _, accel := range priority {
		for _, avail := range available {
			if accel == avail {
				return accel
			}
		}
	}

	return HWAccelNone
}

// Test functions for each acceleration type

func (d *HWAccelDetector) testVideoToolbox(ctx context.Context) bool {
	// On macOS, check if FFmpeg has VideoToolbox support by listing hwaccels
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-hwaccels")
	output, err := cmd.CombinedOutput()
	if err != nil {
		d.logger.Debug("Failed to list hwaccels", "error", err)
		return false
	}

	// Check if videotoolbox is in the list
	if strings.Contains(string(output), "videotoolbox") {
		d.logger.Debug("VideoToolbox is available")
		return true
	}

	d.logger.Debug("VideoToolbox not found in hwaccels list")
	return false
}

func (d *HWAccelDetector) testCUDA(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-hwaccel", "cuda",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=1",
		"-f", "null", "-",
	)
	err := cmd.Run()
	if err != nil {
		d.logger.Debug("CUDA test failed", "error", err)
		return false
	}
	return true
}

func (d *HWAccelDetector) testVAAPI(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-hwaccel", "vaapi",
		"-hwaccel_device", "/dev/dri/renderD128",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=1",
		"-f", "null", "-",
	)
	err := cmd.Run()
	if err != nil {
		d.logger.Debug("VAAPI test failed", "error", err)
		return false
	}
	return true
}

func (d *HWAccelDetector) testQSV(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-hwaccel", "qsv",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=1",
		"-f", "null", "-",
	)
	err := cmd.Run()
	if err != nil {
		d.logger.Debug("QSV test failed", "error", err)
		return false
	}
	return true
}

func (d *HWAccelDetector) testD3D11VA(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-hwaccel", "d3d11va",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=1",
		"-f", "null", "-",
	)
	err := cmd.Run()
	if err != nil {
		d.logger.Debug("D3D11VA test failed", "error", err)
		return false
	}
	return true
}

// Hardware detection helpers

func (d *HWAccelDetector) hasNVIDIAGPU() bool {
	// Check for nvidia-smi
	cmd := exec.Command("nvidia-smi", "-L")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), "GPU")
}

func (d *HWAccelDetector) getNVIDIAGPUName() string {
	cmd := exec.Command("nvidia-smi", "--query-gpu=name", "--format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func (d *HWAccelDetector) hasVAAPI() bool {
	// Check for VA-API device
	cmd := exec.Command("ls", "/dev/dri/renderD128")
	return cmd.Run() == nil
}

func (d *HWAccelDetector) getVAAPIGPUName() string {
	// Try to get GPU info from vainfo
	cmd := exec.Command("vainfo")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Driver version") {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

func (d *HWAccelDetector) hasQSV() bool {
	// Check for Intel GPU (QSV support)
	if runtime.GOOS == "linux" {
		// Check for Intel render device
		cmd := exec.Command("ls", "/dev/dri/renderD128")
		if cmd.Run() != nil {
			return false
		}
		// Check if it's Intel
		cmd = exec.Command("lspci")
		output, err := cmd.Output()
		if err != nil {
			return false
		}
		return strings.Contains(strings.ToLower(string(output)), "intel") &&
			strings.Contains(strings.ToLower(string(output)), "vga")
	}
	return false
}

func (d *HWAccelDetector) getMacGPUName() string {
	cmd := exec.Command("system_profiler", "SPDisplaysDataType")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Chipset Model:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

// Global detector instance
var (
	globalDetector     *HWAccelDetector
	globalDetectorOnce sync.Once
)

// GetGlobalDetector returns the global hardware acceleration detector
func GetGlobalDetector() *HWAccelDetector {
	globalDetectorOnce.Do(func() {
		globalDetector = NewHWAccelDetector()
	})
	return globalDetector
}

// DetectHWAccel is a convenience function to detect hardware acceleration
func DetectHWAccel(ctx context.Context) (*HWAccelCapabilities, error) {
	return GetGlobalDetector().Detect(ctx)
}

// GetRecommendedHWAccel returns the recommended hardware acceleration type
func GetRecommendedHWAccel(ctx context.Context) HWAccelType {
	return GetGlobalDetector().GetRecommended(ctx)
}

// FormatCapabilities returns a human-readable string of capabilities
func (c *HWAccelCapabilities) FormatCapabilities() string {
	if len(c.Available) == 0 {
		return "No hardware acceleration available (using software encoding)"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Recommended: %s\n", c.Recommended))
	sb.WriteString(fmt.Sprintf("Available: %v\n", c.Available))
	if c.GPUName != "" {
		sb.WriteString(fmt.Sprintf("GPU: %s\n", c.GPUName))
	}
	sb.WriteString(fmt.Sprintf("Decode H.264: %v, H.265: %v\n", c.DecodeH264, c.DecodeH265))
	sb.WriteString(fmt.Sprintf("Encode H.264: %v, H.265: %v\n", c.EncodeH264, c.EncodeH265))
	return sb.String()
}
