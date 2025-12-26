package nvrspatialtracking

import (
	"encoding/json"
	"math"
	"testing"
)

func TestPointDistance(t *testing.T) {
	tests := []struct {
		name     string
		p1       Point
		p2       Point
		expected float64
	}{
		{
			name:     "same point",
			p1:       Point{X: 0, Y: 0},
			p2:       Point{X: 0, Y: 0},
			expected: 0,
		},
		{
			name:     "horizontal distance",
			p1:       Point{X: 0, Y: 0},
			p2:       Point{X: 3, Y: 0},
			expected: 3,
		},
		{
			name:     "vertical distance",
			p1:       Point{X: 0, Y: 0},
			p2:       Point{X: 0, Y: 4},
			expected: 4,
		},
		{
			name:     "diagonal 3-4-5 triangle",
			p1:       Point{X: 0, Y: 0},
			p2:       Point{X: 3, Y: 4},
			expected: 5,
		},
		{
			name:     "negative coordinates",
			p1:       Point{X: -1, Y: -1},
			p2:       Point{X: 2, Y: 3},
			expected: 5,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.p1.Distance(tc.p2)
			if math.Abs(result-tc.expected) > 0.0001 {
				t.Errorf("Distance(%v, %v) = %f, want %f", tc.p1, tc.p2, result, tc.expected)
			}
		})
	}
}

func TestPolygonContainsPoint(t *testing.T) {
	// Simple square: (0,0), (4,0), (4,4), (0,4)
	square := Polygon{
		{X: 0, Y: 0},
		{X: 4, Y: 0},
		{X: 4, Y: 4},
		{X: 0, Y: 4},
	}

	// Triangle
	triangle := Polygon{
		{X: 0, Y: 0},
		{X: 4, Y: 0},
		{X: 2, Y: 4},
	}

	tests := []struct {
		name     string
		polygon  Polygon
		point    Point
		expected bool
	}{
		{
			name:     "point inside square",
			polygon:  square,
			point:    Point{X: 2, Y: 2},
			expected: true,
		},
		{
			name:     "point outside square",
			polygon:  square,
			point:    Point{X: 5, Y: 2},
			expected: false,
		},
		{
			name:     "point on edge (may vary)",
			polygon:  square,
			point:    Point{X: 0, Y: 2},
			expected: true, // This implementation includes points on left edge
		},
		{
			name:     "point inside triangle",
			polygon:  triangle,
			point:    Point{X: 2, Y: 1},
			expected: true,
		},
		{
			name:     "point outside triangle",
			polygon:  triangle,
			point:    Point{X: 2, Y: 5},
			expected: false,
		},
		{
			name:     "empty polygon",
			polygon:  Polygon{},
			point:    Point{X: 0, Y: 0},
			expected: false,
		},
		{
			name:     "polygon with 2 points (line)",
			polygon:  Polygon{{X: 0, Y: 0}, {X: 1, Y: 1}},
			point:    Point{X: 0.5, Y: 0.5},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.polygon.ContainsPoint(tc.point)
			if result != tc.expected {
				t.Errorf("Polygon.ContainsPoint(%v) = %v, want %v", tc.point, result, tc.expected)
			}
		})
	}
}

func TestPolygonIntersects(t *testing.T) {
	square1 := Polygon{
		{X: 0, Y: 0},
		{X: 4, Y: 0},
		{X: 4, Y: 4},
		{X: 0, Y: 4},
	}

	square2 := Polygon{ // Overlaps with square1
		{X: 2, Y: 2},
		{X: 6, Y: 2},
		{X: 6, Y: 6},
		{X: 2, Y: 6},
	}

	square3 := Polygon{ // Completely separate
		{X: 10, Y: 10},
		{X: 14, Y: 10},
		{X: 14, Y: 14},
		{X: 10, Y: 14},
	}

	tests := []struct {
		name     string
		p1       Polygon
		p2       Polygon
		expected bool
	}{
		{
			name:     "overlapping squares",
			p1:       square1,
			p2:       square2,
			expected: true,
		},
		{
			name:     "non-overlapping squares",
			p1:       square1,
			p2:       square3,
			expected: false,
		},
		{
			name:     "same polygon",
			p1:       square1,
			p2:       square1,
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.p1.Intersects(tc.p2)
			if result != tc.expected {
				t.Errorf("Polygon.Intersects() = %v, want %v", result, tc.expected)
			}
		})
	}
}

func TestPolygonMarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		polygon  Polygon
		expected string
	}{
		{
			name:     "empty polygon",
			polygon:  nil,
			expected: "[]",
		},
		{
			name:     "polygon with points",
			polygon:  Polygon{{X: 0, Y: 0}, {X: 1, Y: 1}},
			expected: `[{"x":0,"y":0},{"x":1,"y":1}]`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := json.Marshal(tc.polygon)
			if err != nil {
				t.Fatalf("Failed to marshal: %v", err)
			}
			if string(result) != tc.expected {
				t.Errorf("Marshal() = %s, want %s", string(result), tc.expected)
			}
		})
	}
}

func TestCameraPlacementCalculateFOVPolygon(t *testing.T) {
	tests := []struct {
		name      string
		placement CameraPlacement
		checkFn   func(Polygon) bool
	}{
		{
			name: "simple FOV pointing right",
			placement: CameraPlacement{
				Position: Point{X: 0, Y: 0},
				Rotation: 0, // Pointing right
				FOVAngle: 90,
				FOVDepth: 100,
			},
			checkFn: func(p Polygon) bool {
				// Should have 3 points: origin and two FOV corners
				if len(p) != 3 {
					return false
				}
				// Origin should be first
				if p[0].X != 0 || p[0].Y != 0 {
					return false
				}
				// Both corners should be roughly 100 units away
				d1 := p[0].Distance(p[1])
				d2 := p[0].Distance(p[2])
				return math.Abs(d1-100) < 0.01 && math.Abs(d2-100) < 0.01
			},
		},
		{
			name: "manual coverage polygon override",
			placement: CameraPlacement{
				Position:        Point{X: 0, Y: 0},
				Rotation:        0,
				FOVAngle:        90,
				FOVDepth:        100,
				CoveragePolygon: Polygon{{X: 0, Y: 0}, {X: 50, Y: 0}, {X: 50, Y: 50}},
			},
			checkFn: func(p Polygon) bool {
				// Should return the manual override
				return len(p) == 3 && p[1].X == 50
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.placement.CalculateFOVPolygon()
			if !tc.checkFn(result) {
				t.Errorf("CalculateFOVPolygon() returned unexpected polygon: %v", result)
			}
		})
	}
}

func TestTransitionTypes(t *testing.T) {
	// Verify constant values
	if TransitionOverlap != "overlap" {
		t.Errorf("TransitionOverlap = %s, want 'overlap'", TransitionOverlap)
	}
	if TransitionAdjacent != "adjacent" {
		t.Errorf("TransitionAdjacent = %s, want 'adjacent'", TransitionAdjacent)
	}
	if TransitionGap != "gap" {
		t.Errorf("TransitionGap = %s, want 'gap'", TransitionGap)
	}
}

func TestTrackStates(t *testing.T) {
	// Verify constant values
	states := []struct {
		state    TrackState
		expected string
	}{
		{TrackStateActive, "active"},
		{TrackStateTransit, "transit"},
		{TrackStatePending, "pending"},
		{TrackStateLost, "lost"},
		{TrackStateCompleted, "completed"},
	}

	for _, tc := range states {
		if string(tc.state) != tc.expected {
			t.Errorf("TrackState %s = %s, want %s", tc.state, string(tc.state), tc.expected)
		}
	}
}

func TestEdgeDirections(t *testing.T) {
	edges := []struct {
		edge     EdgeDirection
		expected string
	}{
		{EdgeTop, "top"},
		{EdgeBottom, "bottom"},
		{EdgeLeft, "left"},
		{EdgeRight, "right"},
	}

	for _, tc := range edges {
		if string(tc.edge) != tc.expected {
			t.Errorf("EdgeDirection %s = %s, want %s", tc.edge, string(tc.edge), tc.expected)
		}
	}
}

func TestSpatialMapJSON(t *testing.T) {
	m := SpatialMap{
		ID:     "test-map",
		Name:   "Test Map",
		Width:  100,
		Height: 80,
		Scale:  1.0,
		Metadata: MapMeta{
			Building: "Main",
			Floor:    "1",
			Area:     "Lobby",
		},
	}

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Failed to marshal SpatialMap: %v", err)
	}

	var decoded SpatialMap
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal SpatialMap: %v", err)
	}

	if decoded.ID != m.ID || decoded.Name != m.Name {
		t.Errorf("Roundtrip failed: got %+v, want %+v", decoded, m)
	}
}

func TestCameraTransitionJSON(t *testing.T) {
	ct := CameraTransition{
		ID:                  "trans-1",
		FromCameraID:        "cam-1",
		ToCameraID:          "cam-2",
		Type:                TransitionGap,
		Bidirectional:       true,
		ExpectedTransitTime: 5.0,
		TransitTimeVariance: 2.5,
		ExitZone: &ZoneDefinition{
			Edge:  EdgeRight,
			Start: 0.3,
			End:   0.7,
		},
	}

	data, err := json.Marshal(ct)
	if err != nil {
		t.Fatalf("Failed to marshal CameraTransition: %v", err)
	}

	var decoded CameraTransition
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal CameraTransition: %v", err)
	}

	if decoded.Type != ct.Type || decoded.ExpectedTransitTime != ct.ExpectedTransitTime {
		t.Errorf("Roundtrip failed: got %+v, want %+v", decoded, ct)
	}

	if decoded.ExitZone == nil || decoded.ExitZone.Edge != EdgeRight {
		t.Errorf("ExitZone not preserved: got %+v", decoded.ExitZone)
	}
}
