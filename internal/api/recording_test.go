package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/Spatial-NVR/SpatialNVR/internal/recording"
)

func TestRecordingHandler_Routes(t *testing.T) {
	handler := &RecordingHandler{}
	router := handler.Routes()

	if router == nil {
		t.Error("Routes() returned nil")
	}
}

func TestNewRecordingHandler(t *testing.T) {
	handler := NewRecordingHandler(nil)
	if handler == nil {
		t.Error("NewRecordingHandler returned nil")
	}
}

func TestParseTimeRange(t *testing.T) {
	tests := []struct {
		name       string
		queryStart string
		queryEnd   string
		wantStart  bool
		wantEnd    bool
		wantErr    bool
	}{
		{"no params", "", "", true, true, false},
		{"valid start only", "2024-01-01T00:00:00Z", "", true, true, false},
		{"valid both", "2024-01-01T00:00:00Z", "2024-01-02T00:00:00Z", true, true, false},
		{"invalid start", "not-a-date", "", false, false, true},
		{"invalid end", "2024-01-01T00:00:00Z", "not-a-date", true, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/test?"
			if tt.queryStart != "" {
				url += "start=" + tt.queryStart + "&"
			}
			if tt.queryEnd != "" {
				url += "end=" + tt.queryEnd
			}

			req := httptest.NewRequest("GET", url, nil)
			start, end, err := parseTimeRange(req)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if tt.wantStart && start.IsZero() {
					t.Error("expected non-zero start time")
				}
				if tt.wantEnd && end.IsZero() {
					t.Error("expected non-zero end time")
				}
			}
		})
	}
}

// MockRecordingService is a mock implementation of the recording service
type MockRecordingService struct {
	ListSegmentsFunc         func(ctx context.Context, opts recording.ListOptions) ([]recording.Segment, int, error)
	GetSegmentFunc           func(ctx context.Context, id string) (*recording.Segment, error)
	DeleteSegmentFunc        func(ctx context.Context, id string) error
	GetTimelineFunc          func(ctx context.Context, cameraID string, start, end time.Time) (*recording.Timeline, error)
	GetTimelineSegmentsFunc  func(ctx context.Context, cameraID string, start, end time.Time) ([]*recording.TimelineSegment, error)
	StartCameraFunc          func(cameraID string) error
	StopCameraFunc           func(cameraID string) error
	RestartCameraFunc        func(cameraID string) error
	GetRecorderStatusFunc    func(cameraID string) (*recording.RecorderStatus, error)
	GetAllRecorderStatusFunc func() map[string]*recording.RecorderStatus
	GetStorageStatsFunc      func(ctx context.Context) (*recording.StorageStats, error)
	GetPlaybackInfoFunc      func(ctx context.Context, cameraID string, timestamp time.Time) (string, float64, error)
	ExportSegmentsFunc       func(ctx context.Context, cameraID string, start, end time.Time, outputPath string) error
	RunRetentionFunc         func(ctx context.Context) (*recording.RetentionStats, error)
	GenerateThumbnailFunc    func(ctx context.Context, segmentID string) (string, error)
}

func (m *MockRecordingService) ListSegments(ctx context.Context, opts recording.ListOptions) ([]recording.Segment, int, error) {
	if m.ListSegmentsFunc != nil {
		return m.ListSegmentsFunc(ctx, opts)
	}
	return nil, 0, nil
}

func (m *MockRecordingService) GetSegment(ctx context.Context, id string) (*recording.Segment, error) {
	if m.GetSegmentFunc != nil {
		return m.GetSegmentFunc(ctx, id)
	}
	return nil, errors.New("not found")
}

func (m *MockRecordingService) DeleteSegment(ctx context.Context, id string) error {
	if m.DeleteSegmentFunc != nil {
		return m.DeleteSegmentFunc(ctx, id)
	}
	return nil
}

func (m *MockRecordingService) GetTimeline(ctx context.Context, cameraID string, start, end time.Time) (*recording.Timeline, error) {
	if m.GetTimelineFunc != nil {
		return m.GetTimelineFunc(ctx, cameraID, start, end)
	}
	return nil, nil
}

func (m *MockRecordingService) GetTimelineSegments(ctx context.Context, cameraID string, start, end time.Time) ([]*recording.TimelineSegment, error) {
	if m.GetTimelineSegmentsFunc != nil {
		return m.GetTimelineSegmentsFunc(ctx, cameraID, start, end)
	}
	return nil, nil
}

func (m *MockRecordingService) StartCamera(cameraID string) error {
	if m.StartCameraFunc != nil {
		return m.StartCameraFunc(cameraID)
	}
	return nil
}

func (m *MockRecordingService) StopCamera(cameraID string) error {
	if m.StopCameraFunc != nil {
		return m.StopCameraFunc(cameraID)
	}
	return nil
}

func (m *MockRecordingService) RestartCamera(cameraID string) error {
	if m.RestartCameraFunc != nil {
		return m.RestartCameraFunc(cameraID)
	}
	return nil
}

func (m *MockRecordingService) GetRecorderStatus(cameraID string) (*recording.RecorderStatus, error) {
	if m.GetRecorderStatusFunc != nil {
		return m.GetRecorderStatusFunc(cameraID)
	}
	return nil, errors.New("not found")
}

func (m *MockRecordingService) GetAllRecorderStatus() map[string]*recording.RecorderStatus {
	if m.GetAllRecorderStatusFunc != nil {
		return m.GetAllRecorderStatusFunc()
	}
	return map[string]*recording.RecorderStatus{}
}

func (m *MockRecordingService) GetStorageStats(ctx context.Context) (*recording.StorageStats, error) {
	if m.GetStorageStatsFunc != nil {
		return m.GetStorageStatsFunc(ctx)
	}
	return nil, nil
}

func (m *MockRecordingService) GetPlaybackInfo(ctx context.Context, cameraID string, timestamp time.Time) (string, float64, error) {
	if m.GetPlaybackInfoFunc != nil {
		return m.GetPlaybackInfoFunc(ctx, cameraID, timestamp)
	}
	return "", 0, errors.New("not found")
}

func (m *MockRecordingService) ExportSegments(ctx context.Context, cameraID string, start, end time.Time, outputPath string) error {
	if m.ExportSegmentsFunc != nil {
		return m.ExportSegmentsFunc(ctx, cameraID, start, end, outputPath)
	}
	return nil
}

func (m *MockRecordingService) RunRetention(ctx context.Context) (*recording.RetentionStats, error) {
	if m.RunRetentionFunc != nil {
		return m.RunRetentionFunc(ctx)
	}
	return nil, nil
}

func (m *MockRecordingService) GenerateThumbnail(ctx context.Context, segmentID string) (string, error) {
	if m.GenerateThumbnailFunc != nil {
		return m.GenerateThumbnailFunc(ctx, segmentID)
	}
	return "", errors.New("not implemented")
}

// Helper to create request with chi URL params
func requestWithParams(method, url string, body []byte, params map[string]string) *http.Request {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, url, bytes.NewReader(body))
	} else {
		req = httptest.NewRequest(method, url, nil)
	}

	// Add chi URL params to context
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestRecordingHandler_ListSegments(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		mockReturn     func(ctx context.Context, opts recording.ListOptions) ([]recording.Segment, int, error)
		expectedStatus int
	}{
		{
			name:  "success with defaults",
			query: "",
			mockReturn: func(ctx context.Context, opts recording.ListOptions) ([]recording.Segment, int, error) {
				return []recording.Segment{{ID: "seg1"}}, 1, nil
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:  "success with query params",
			query: "?camera_id=cam1&limit=10&offset=5&order_by=start_time&order_desc=true",
			mockReturn: func(ctx context.Context, opts recording.ListOptions) ([]recording.Segment, int, error) {
				if opts.CameraID != "cam1" || opts.Limit != 10 || opts.Offset != 5 {
					return nil, 0, errors.New("wrong options")
				}
				return []recording.Segment{{ID: "seg1"}}, 1, nil
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:  "success with time range",
			query: "?start_time=2024-01-01T00:00:00Z&end_time=2024-01-02T00:00:00Z",
			mockReturn: func(ctx context.Context, opts recording.ListOptions) ([]recording.Segment, int, error) {
				return []recording.Segment{}, 0, nil
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:  "success with has_events filter",
			query: "?has_events=true",
			mockReturn: func(ctx context.Context, opts recording.ListOptions) ([]recording.Segment, int, error) {
				if opts.HasEvents == nil || !*opts.HasEvents {
					return nil, 0, errors.New("has_events not set")
				}
				return []recording.Segment{}, 0, nil
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:  "service error",
			query: "",
			mockReturn: func(ctx context.Context, opts recording.ListOptions) ([]recording.Segment, int, error) {
				return nil, 0, errors.New("database error")
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockRecordingService{ListSegmentsFunc: tt.mockReturn}
			handler := &RecordingHandler{service: createServiceFromMock(mock)}

			req := httptest.NewRequest("GET", "/recordings"+tt.query, nil)
			w := httptest.NewRecorder()

			handler.ListSegments(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestRecordingHandler_GetSegment(t *testing.T) {
	tests := []struct {
		name           string
		segmentID      string
		mockReturn     func(ctx context.Context, id string) (*recording.Segment, error)
		expectedStatus int
	}{
		{
			name:      "success",
			segmentID: "seg1",
			mockReturn: func(ctx context.Context, id string) (*recording.Segment, error) {
				return &recording.Segment{ID: id, CameraID: "cam1"}, nil
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:      "not found",
			segmentID: "nonexistent",
			mockReturn: func(ctx context.Context, id string) (*recording.Segment, error) {
				return nil, errors.New("not found")
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockRecordingService{GetSegmentFunc: tt.mockReturn}
			handler := &RecordingHandler{service: createServiceFromMock(mock)}

			req := requestWithParams("GET", "/recordings/"+tt.segmentID, nil, map[string]string{"id": tt.segmentID})
			w := httptest.NewRecorder()

			handler.GetSegment(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestRecordingHandler_DeleteSegment(t *testing.T) {
	tests := []struct {
		name           string
		segmentID      string
		mockReturn     error
		expectedStatus int
	}{
		{
			name:           "success",
			segmentID:      "seg1",
			mockReturn:     nil,
			expectedStatus: http.StatusNoContent,
		},
		{
			name:           "not found",
			segmentID:      "nonexistent",
			mockReturn:     errors.New("not found"),
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockRecordingService{
				DeleteSegmentFunc: func(ctx context.Context, id string) error {
					return tt.mockReturn
				},
			}
			handler := &RecordingHandler{service: createServiceFromMock(mock)}

			req := requestWithParams("DELETE", "/recordings/"+tt.segmentID, nil, map[string]string{"id": tt.segmentID})
			w := httptest.NewRecorder()

			handler.DeleteSegment(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestRecordingHandler_StartRecording(t *testing.T) {
	tests := []struct {
		name           string
		cameraID       string
		mockReturn     error
		expectedStatus int
	}{
		{
			name:           "success",
			cameraID:       "cam1",
			mockReturn:     nil,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "error",
			cameraID:       "cam1",
			mockReturn:     errors.New("camera not configured"),
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockRecordingService{
				StartCameraFunc: func(cameraID string) error {
					return tt.mockReturn
				},
			}
			handler := &RecordingHandler{service: createServiceFromMock(mock)}

			req := requestWithParams("POST", "/recordings/cameras/"+tt.cameraID+"/start", nil, map[string]string{"cameraId": tt.cameraID})
			w := httptest.NewRecorder()

			handler.StartRecording(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestRecordingHandler_StopRecording(t *testing.T) {
	tests := []struct {
		name           string
		cameraID       string
		mockReturn     error
		expectedStatus int
	}{
		{
			name:           "success",
			cameraID:       "cam1",
			mockReturn:     nil,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "error",
			cameraID:       "cam1",
			mockReturn:     errors.New("camera not recording"),
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockRecordingService{
				StopCameraFunc: func(cameraID string) error {
					return tt.mockReturn
				},
			}
			handler := &RecordingHandler{service: createServiceFromMock(mock)}

			req := requestWithParams("POST", "/recordings/cameras/"+tt.cameraID+"/stop", nil, map[string]string{"cameraId": tt.cameraID})
			w := httptest.NewRecorder()

			handler.StopRecording(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestRecordingHandler_RestartRecording(t *testing.T) {
	mock := &MockRecordingService{
		RestartCameraFunc: func(cameraID string) error {
			return nil
		},
	}
	handler := &RecordingHandler{service: createServiceFromMock(mock)}

	req := requestWithParams("POST", "/recordings/cameras/cam1/restart", nil, map[string]string{"cameraId": "cam1"})
	w := httptest.NewRecorder()

	handler.RestartRecording(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestRecordingHandler_GetStatus(t *testing.T) {
	tests := []struct {
		name           string
		cameraID       string
		mockReturn     func(cameraID string) (*recording.RecorderStatus, error)
		expectedStatus int
	}{
		{
			name:     "success",
			cameraID: "cam1",
			mockReturn: func(cameraID string) (*recording.RecorderStatus, error) {
				return &recording.RecorderStatus{CameraID: cameraID, State: recording.RecorderStateRunning}, nil
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:     "not found",
			cameraID: "nonexistent",
			mockReturn: func(cameraID string) (*recording.RecorderStatus, error) {
				return nil, errors.New("not found")
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockRecordingService{GetRecorderStatusFunc: tt.mockReturn}
			handler := &RecordingHandler{service: createServiceFromMock(mock)}

			req := requestWithParams("GET", "/recordings/status/"+tt.cameraID, nil, map[string]string{"cameraId": tt.cameraID})
			w := httptest.NewRecorder()

			handler.GetStatus(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestRecordingHandler_GetAllStatus(t *testing.T) {
	mock := &MockRecordingService{
		GetAllRecorderStatusFunc: func() map[string]*recording.RecorderStatus {
			return map[string]*recording.RecorderStatus{
				"cam1": {CameraID: "cam1", State: recording.RecorderStateRunning},
			}
		},
	}
	handler := &RecordingHandler{service: createServiceFromMock(mock)}

	req := httptest.NewRequest("GET", "/recordings/status", nil)
	w := httptest.NewRecorder()

	handler.GetAllStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestRecordingHandler_GetStorageStats(t *testing.T) {
	tests := []struct {
		name           string
		mockReturn     func(ctx context.Context) (*recording.StorageStats, error)
		expectedStatus int
	}{
		{
			name: "success",
			mockReturn: func(ctx context.Context) (*recording.StorageStats, error) {
				return &recording.StorageStats{UsedBytes: 1000}, nil
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "error",
			mockReturn: func(ctx context.Context) (*recording.StorageStats, error) {
				return nil, errors.New("storage error")
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockRecordingService{GetStorageStatsFunc: tt.mockReturn}
			handler := &RecordingHandler{service: createServiceFromMock(mock)}

			req := httptest.NewRequest("GET", "/recordings/storage", nil)
			w := httptest.NewRecorder()

			handler.GetStorageStats(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestRecordingHandler_GetPlaybackInfo(t *testing.T) {
	tests := []struct {
		name           string
		cameraID       string
		timestamp      string
		mockReturn     func(ctx context.Context, cameraID string, timestamp time.Time) (string, float64, error)
		expectedStatus int
	}{
		{
			name:      "success",
			cameraID:  "cam1",
			timestamp: "2024-01-01T12:00:00Z",
			mockReturn: func(ctx context.Context, cameraID string, timestamp time.Time) (string, float64, error) {
				return "/recordings/segment.mp4", 30.5, nil
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "missing timestamp",
			cameraID:       "cam1",
			timestamp:      "",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "invalid timestamp",
			cameraID:       "cam1",
			timestamp:      "not-a-timestamp",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:      "not found",
			cameraID:  "cam1",
			timestamp: "2024-01-01T12:00:00Z",
			mockReturn: func(ctx context.Context, cameraID string, timestamp time.Time) (string, float64, error) {
				return "", 0, errors.New("not found")
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockRecordingService{GetPlaybackInfoFunc: tt.mockReturn}
			handler := &RecordingHandler{service: createServiceFromMock(mock)}

			url := "/recordings/playback/" + tt.cameraID
			if tt.timestamp != "" {
				url += "?timestamp=" + tt.timestamp
			}
			req := requestWithParams("GET", url, nil, map[string]string{"cameraId": tt.cameraID})
			w := httptest.NewRecorder()

			handler.GetPlaybackInfo(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestRecordingHandler_ExportSegments(t *testing.T) {
	tests := []struct {
		name           string
		body           ExportRequest
		mockReturn     error
		expectedStatus int
	}{
		{
			name: "success",
			body: ExportRequest{
				CameraID:  "cam1",
				StartTime: "2024-01-01T00:00:00Z",
				EndTime:   "2024-01-01T01:00:00Z",
			},
			mockReturn:     nil,
			expectedStatus: http.StatusOK,
		},
		{
			name: "success with custom output path",
			body: ExportRequest{
				CameraID:   "cam1",
				StartTime:  "2024-01-01T00:00:00Z",
				EndTime:    "2024-01-01T01:00:00Z",
				OutputPath: "/custom/path.mp4",
			},
			mockReturn:     nil,
			expectedStatus: http.StatusOK,
		},
		{
			name: "invalid start time",
			body: ExportRequest{
				CameraID:  "cam1",
				StartTime: "invalid",
				EndTime:   "2024-01-01T01:00:00Z",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "invalid end time",
			body: ExportRequest{
				CameraID:  "cam1",
				StartTime: "2024-01-01T00:00:00Z",
				EndTime:   "invalid",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "export error",
			body: ExportRequest{
				CameraID:  "cam1",
				StartTime: "2024-01-01T00:00:00Z",
				EndTime:   "2024-01-01T01:00:00Z",
			},
			mockReturn:     errors.New("export failed"),
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockRecordingService{
				ExportSegmentsFunc: func(ctx context.Context, cameraID string, start, end time.Time, outputPath string) error {
					return tt.mockReturn
				},
			}
			handler := &RecordingHandler{service: createServiceFromMock(mock)}

			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest("POST", "/recordings/export", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.ExportSegments(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestRecordingHandler_ExportSegments_InvalidJSON(t *testing.T) {
	handler := &RecordingHandler{service: createServiceFromMock(&MockRecordingService{})}

	req := httptest.NewRequest("POST", "/recordings/export", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ExportSegments(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestRecordingHandler_RunRetention(t *testing.T) {
	tests := []struct {
		name           string
		mockReturn     func(ctx context.Context) (*recording.RetentionStats, error)
		expectedStatus int
	}{
		{
			name: "success",
			mockReturn: func(ctx context.Context) (*recording.RetentionStats, error) {
				return &recording.RetentionStats{SegmentsDeleted: 5}, nil
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "error",
			mockReturn: func(ctx context.Context) (*recording.RetentionStats, error) {
				return nil, errors.New("retention failed")
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockRecordingService{RunRetentionFunc: tt.mockReturn}
			handler := &RecordingHandler{service: createServiceFromMock(mock)}

			req := httptest.NewRequest("POST", "/recordings/retention/run", nil)
			w := httptest.NewRecorder()

			handler.RunRetention(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestRecordingHandler_GetTimeline(t *testing.T) {
	tests := []struct {
		name           string
		cameraID       string
		query          string
		mockReturn     func(ctx context.Context, cameraID string, start, end time.Time) (*recording.Timeline, error)
		expectedStatus int
	}{
		{
			name:     "success",
			cameraID: "cam1",
			query:    "",
			mockReturn: func(ctx context.Context, cameraID string, start, end time.Time) (*recording.Timeline, error) {
				return &recording.Timeline{CameraID: cameraID}, nil
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid time range",
			cameraID:       "cam1",
			query:          "?start=invalid",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:     "service error",
			cameraID: "cam1",
			query:    "",
			mockReturn: func(ctx context.Context, cameraID string, start, end time.Time) (*recording.Timeline, error) {
				return nil, errors.New("timeline error")
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockRecordingService{GetTimelineFunc: tt.mockReturn}
			handler := &RecordingHandler{service: createServiceFromMock(mock)}

			req := requestWithParams("GET", "/recordings/timeline/"+tt.cameraID+tt.query, nil, map[string]string{"cameraId": tt.cameraID})
			w := httptest.NewRecorder()

			handler.GetTimeline(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestRecordingHandler_GetTimelineSegments(t *testing.T) {
	tests := []struct {
		name           string
		cameraID       string
		query          string
		mockReturn     func(ctx context.Context, cameraID string, start, end time.Time) ([]*recording.TimelineSegment, error)
		expectedStatus int
	}{
		{
			name:     "success",
			cameraID: "cam1",
			query:    "",
			mockReturn: func(ctx context.Context, cameraID string, start, end time.Time) ([]*recording.TimelineSegment, error) {
				return []*recording.TimelineSegment{}, nil
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid time range",
			cameraID:       "cam1",
			query:          "?start=invalid",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:     "service error",
			cameraID: "cam1",
			query:    "",
			mockReturn: func(ctx context.Context, cameraID string, start, end time.Time) ([]*recording.TimelineSegment, error) {
				return nil, errors.New("timeline error")
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockRecordingService{GetTimelineSegmentsFunc: tt.mockReturn}
			handler := &RecordingHandler{service: createServiceFromMock(mock)}

			req := requestWithParams("GET", "/recordings/timeline/"+tt.cameraID+"/segments"+tt.query, nil, map[string]string{"cameraId": tt.cameraID})
			w := httptest.NewRecorder()

			handler.GetTimelineSegments(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

// createServiceFromMock returns the mock directly since RecordingHandler now uses an interface
func createServiceFromMock(mock *MockRecordingService) RecordingService {
	return mock
}
