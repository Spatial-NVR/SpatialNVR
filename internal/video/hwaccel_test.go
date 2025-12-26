package video

import (
	"context"
	"testing"
)

func TestHWAccelType_String(t *testing.T) {
	tests := []struct {
		accel    HWAccelType
		expected string
	}{
		{HWAccelNone, ""},
		{HWAccelCUDA, "cuda"},
		{HWAccelVideoToolbox, "videotoolbox"},
		{HWAccelVAAPI, "vaapi"},
		{HWAccelQSV, "qsv"},
		{HWAccelD3D11VA, "d3d11va"},
		{HWAccelDXVA2, "dxva2"},
		{HWAccelVulkan, "vulkan"},
	}

	for _, tt := range tests {
		if string(tt.accel) != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, string(tt.accel))
		}
	}
}

func TestNewHWAccelDetector(t *testing.T) {
	detector := NewHWAccelDetector()
	if detector == nil {
		t.Fatal("NewHWAccelDetector returned nil")
	}
	if detector.logger == nil {
		t.Error("logger should be initialized")
	}
}

func TestGetFFmpegHWAccelArgs(t *testing.T) {
	tests := []struct {
		accel    HWAccelType
		expected []string
	}{
		{HWAccelNone, nil},
		{HWAccelCUDA, []string{"-hwaccel", "cuda", "-hwaccel_output_format", "cuda"}},
		{HWAccelVideoToolbox, []string{"-hwaccel", "videotoolbox"}},
		{HWAccelVAAPI, []string{"-hwaccel", "vaapi", "-hwaccel_device", "/dev/dri/renderD128"}},
		{HWAccelQSV, []string{"-hwaccel", "qsv"}},
		{HWAccelD3D11VA, []string{"-hwaccel", "d3d11va"}},
		{HWAccelDXVA2, []string{"-hwaccel", "dxva2"}},
		{HWAccelVulkan, []string{"-hwaccel", "vulkan"}},
	}

	for _, tt := range tests {
		result := GetFFmpegHWAccelArgs(tt.accel)
		if tt.expected == nil {
			if result != nil {
				t.Errorf("Expected nil for %s, got %v", tt.accel, result)
			}
		} else {
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d args for %s, got %d", len(tt.expected), tt.accel, len(result))
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("Expected arg %d to be %s, got %s", i, tt.expected[i], v)
				}
			}
		}
	}
}

func TestHWAccelDetector_selectRecommended(t *testing.T) {
	detector := NewHWAccelDetector()

	tests := []struct {
		available []HWAccelType
		expected  HWAccelType
	}{
		{[]HWAccelType{}, HWAccelNone},
		{[]HWAccelType{HWAccelCUDA}, HWAccelCUDA},
		{[]HWAccelType{HWAccelVAAPI, HWAccelCUDA}, HWAccelCUDA},
		{[]HWAccelType{HWAccelVideoToolbox}, HWAccelVideoToolbox},
		{[]HWAccelType{HWAccelVAAPI, HWAccelQSV}, HWAccelQSV},
		{[]HWAccelType{HWAccelD3D11VA, HWAccelDXVA2}, HWAccelD3D11VA},
	}

	for _, tt := range tests {
		result := detector.selectRecommended(tt.available)
		if result != tt.expected {
			t.Errorf("For available %v, expected %s, got %s", tt.available, tt.expected, result)
		}
	}
}

func TestHWAccelCapabilities_FormatCapabilities(t *testing.T) {
	// Test empty capabilities
	emptyCaps := &HWAccelCapabilities{
		Available: []HWAccelType{},
	}
	output := emptyCaps.FormatCapabilities()
	if output != "No hardware acceleration available (using software encoding)" {
		t.Errorf("Unexpected output for empty capabilities: %s", output)
	}

	// Test with capabilities
	caps := &HWAccelCapabilities{
		Available:   []HWAccelType{HWAccelCUDA, HWAccelVAAPI},
		Recommended: HWAccelCUDA,
		DecodeH264:  true,
		DecodeH265:  true,
		EncodeH264:  true,
		EncodeH265:  false,
		GPUName:     "NVIDIA GTX 1080",
	}
	output = caps.FormatCapabilities()
	if output == "" {
		t.Error("Expected non-empty output")
	}
}

func TestGetGlobalDetector(t *testing.T) {
	detector1 := GetGlobalDetector()
	if detector1 == nil {
		t.Fatal("GetGlobalDetector returned nil")
	}

	detector2 := GetGlobalDetector()
	if detector1 != detector2 {
		t.Error("GetGlobalDetector should return the same instance")
	}
}

func TestHWAccelDetector_GetCapabilities_Caching(t *testing.T) {
	detector := NewHWAccelDetector()
	ctx := context.Background()

	// First call detects
	caps1, err := detector.GetCapabilities(ctx)
	if err != nil {
		t.Fatalf("GetCapabilities failed: %v", err)
	}

	// Second call should use cache
	caps2, err := detector.GetCapabilities(ctx)
	if err != nil {
		t.Fatalf("GetCapabilities failed: %v", err)
	}

	// Both should return capabilities (possibly empty)
	if caps1 == nil || caps2 == nil {
		t.Error("Expected non-nil capabilities")
	}
}

func TestHWAccelDetector_GetRecommended_NoCapabilities(t *testing.T) {
	detector := NewHWAccelDetector()
	ctx := context.Background()

	// Should return none if no capabilities detected
	recommended := detector.GetRecommended(ctx)
	// This might be HWAccelNone or a valid acceleration depending on the system
	_ = recommended // Just ensure it doesn't panic
}

func TestHWAccelDetector_GetFFmpegArgs(t *testing.T) {
	detector := NewHWAccelDetector()
	ctx := context.Background()

	// Should return args based on recommended acceleration
	args := detector.GetFFmpegArgs(ctx)
	// Args can be nil or contain valid acceleration args
	_ = args // Just ensure it doesn't panic
}

func TestHWAccelDetector_Detect(t *testing.T) {
	detector := NewHWAccelDetector()
	ctx := context.Background()

	caps, err := detector.Detect(ctx)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}

	if caps == nil {
		t.Fatal("Expected non-nil capabilities")
	}

	// DetectedAt should be set
	if caps.DetectedAt.IsZero() {
		t.Error("DetectedAt should be set")
	}

	// Available should be initialized (possibly empty)
	if caps.Available == nil {
		t.Error("Available should not be nil")
	}
}

func TestDetectHWAccel(t *testing.T) {
	ctx := context.Background()

	caps, err := DetectHWAccel(ctx)
	if err != nil {
		t.Fatalf("DetectHWAccel failed: %v", err)
	}

	if caps == nil {
		t.Fatal("Expected non-nil capabilities")
	}
}

func TestGetRecommendedHWAccel(t *testing.T) {
	ctx := context.Background()

	// Should not panic
	recommended := GetRecommendedHWAccel(ctx)
	_ = recommended
}

func TestHWAccelCapabilities_Fields(t *testing.T) {
	caps := &HWAccelCapabilities{
		Available:   []HWAccelType{HWAccelCUDA},
		Recommended: HWAccelCUDA,
		DecodeH264:  true,
		DecodeH265:  true,
		EncodeH264:  true,
		EncodeH265:  true,
		GPUName:     "Test GPU",
	}

	if len(caps.Available) != 1 {
		t.Errorf("Expected 1 available, got %d", len(caps.Available))
	}
	if caps.Recommended != HWAccelCUDA {
		t.Errorf("Expected CUDA, got %s", caps.Recommended)
	}
	if !caps.DecodeH264 {
		t.Error("Expected DecodeH264 true")
	}
	if !caps.DecodeH265 {
		t.Error("Expected DecodeH265 true")
	}
	if !caps.EncodeH264 {
		t.Error("Expected EncodeH264 true")
	}
	if !caps.EncodeH265 {
		t.Error("Expected EncodeH265 true")
	}
	if caps.GPUName != "Test GPU" {
		t.Errorf("Expected 'Test GPU', got '%s'", caps.GPUName)
	}
}
