package recording

import (
	"context"
	"sort"
	"time"
)

// TimelineBuilder builds timeline data from segments
type TimelineBuilder struct {
	repository Repository
}

// NewTimelineBuilder creates a new timeline builder
func NewTimelineBuilder(repository Repository) *TimelineBuilder {
	return &TimelineBuilder{
		repository: repository,
	}
}

// BuildTimeline creates a timeline for a camera within a time range
func (b *TimelineBuilder) BuildTimeline(ctx context.Context, cameraID string, start, end time.Time) (*Timeline, error) {
	// Get all segments in the range
	segments, err := b.repository.GetByTimeRange(ctx, cameraID, start, end)
	if err != nil {
		return nil, err
	}

	timeline := &Timeline{
		CameraID:  cameraID,
		StartTime: start,
		EndTime:   end,
		Segments:  make([]TimelineSegment, 0),
	}

	if len(segments) == 0 {
		// Return timeline with a single gap
		timeline.Segments = append(timeline.Segments, TimelineSegment{
			StartTime: start,
			EndTime:   end,
			Type:      "gap",
		})
		return timeline, nil
	}

	// Sort segments by start time
	sort.Slice(segments, func(i, j int) bool {
		return segments[i].StartTime.Before(segments[j].StartTime)
	})

	// Build timeline segments with gaps
	currentTime := start
	var totalSize int64
	var totalDuration float64

	for _, seg := range segments {
		// Clip segment to requested range
		segStart := seg.StartTime
		segEnd := seg.EndTime
		if segStart.Before(start) {
			segStart = start
		}
		if segEnd.After(end) {
			segEnd = end
		}

		// Add gap if there's time between current position and segment start
		if currentTime.Before(segStart) {
			timeline.Segments = append(timeline.Segments, TimelineSegment{
				StartTime: currentTime,
				EndTime:   segStart,
				Type:      "gap",
			})
		}

		// Find or extend existing recording segment
		if len(timeline.Segments) > 0 {
			lastSeg := &timeline.Segments[len(timeline.Segments)-1]
			if lastSeg.Type == "recording" && !lastSeg.EndTime.Before(segStart) {
				// Extend existing segment
				if segEnd.After(lastSeg.EndTime) {
					lastSeg.EndTime = segEnd
				}
				lastSeg.SegmentIDs = append(lastSeg.SegmentIDs, seg.ID)
				if seg.HasEvents {
					lastSeg.HasEvents = true
					lastSeg.EventCount += seg.EventCount
				}
				currentTime = segEnd
				totalSize += seg.FileSize
				totalDuration += seg.Duration
				continue
			}
		}

		// Add new recording segment
		timeline.Segments = append(timeline.Segments, TimelineSegment{
			StartTime:  segStart,
			EndTime:    segEnd,
			Type:       "recording",
			HasEvents:  seg.HasEvents,
			EventCount: seg.EventCount,
			SegmentIDs: []string{seg.ID},
		})

		currentTime = segEnd
		totalSize += seg.FileSize
		totalDuration += seg.Duration
	}

	// Add trailing gap if needed
	if currentTime.Before(end) {
		timeline.Segments = append(timeline.Segments, TimelineSegment{
			StartTime: currentTime,
			EndTime:   end,
			Type:      "gap",
		})
	}

	timeline.TotalSize = totalSize
	timeline.TotalHours = totalDuration / 3600

	return timeline, nil
}

// GetTimelineSegments returns simplified timeline segments for UI display
func (b *TimelineBuilder) GetTimelineSegments(ctx context.Context, cameraID string, start, end time.Time) ([]TimelineSegment, error) {
	timeline, err := b.BuildTimeline(ctx, cameraID, start, end)
	if err != nil {
		return nil, err
	}
	return timeline.Segments, nil
}

// GetCoverage calculates the recording coverage percentage for a time range
func (b *TimelineBuilder) GetCoverage(ctx context.Context, cameraID string, start, end time.Time) (float64, error) {
	timeline, err := b.BuildTimeline(ctx, cameraID, start, end)
	if err != nil {
		return 0, err
	}

	var recordingDuration time.Duration
	for _, seg := range timeline.Segments {
		if seg.Type == "recording" {
			recordingDuration += seg.EndTime.Sub(seg.StartTime)
		}
	}

	totalDuration := end.Sub(start)
	if totalDuration == 0 {
		return 0, nil
	}

	return float64(recordingDuration) / float64(totalDuration) * 100, nil
}

// GetEventTimeline returns timeline segments that contain events
func (b *TimelineBuilder) GetEventTimeline(ctx context.Context, cameraID string, start, end time.Time) ([]TimelineSegment, error) {
	timeline, err := b.BuildTimeline(ctx, cameraID, start, end)
	if err != nil {
		return nil, err
	}

	var eventSegments []TimelineSegment
	for _, seg := range timeline.Segments {
		if seg.Type == "recording" && seg.HasEvents {
			eventSegments = append(eventSegments, seg)
		}
	}

	return eventSegments, nil
}

// GetDailyStats returns daily recording statistics
func (b *TimelineBuilder) GetDailyStats(ctx context.Context, cameraID string, date time.Time) (*DailyStats, error) {
	// Get start and end of day
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	end := start.Add(24 * time.Hour)

	segments, err := b.repository.GetByTimeRange(ctx, cameraID, start, end)
	if err != nil {
		return nil, err
	}

	stats := &DailyStats{
		Date:     start,
		CameraID: cameraID,
	}

	for _, seg := range segments {
		stats.TotalDuration += seg.Duration
		stats.TotalSize += seg.FileSize
		stats.SegmentCount++
		if seg.HasEvents {
			stats.EventCount += seg.EventCount
		}
	}

	stats.Coverage = (stats.TotalDuration / (24 * 3600)) * 100

	return stats, nil
}

// DailyStats holds daily recording statistics
type DailyStats struct {
	Date          time.Time `json:"date"`
	CameraID      string    `json:"camera_id"`
	TotalDuration float64   `json:"total_duration"` // seconds
	TotalSize     int64     `json:"total_size"`     // bytes
	SegmentCount  int       `json:"segment_count"`
	EventCount    int       `json:"event_count"`
	Coverage      float64   `json:"coverage"` // percentage
}

// GetWeeklyStats returns weekly recording statistics
func (b *TimelineBuilder) GetWeeklyStats(ctx context.Context, cameraID string, weekStart time.Time) ([]*DailyStats, error) {
	var stats []*DailyStats

	for i := 0; i < 7; i++ {
		date := weekStart.AddDate(0, 0, i)
		dayStats, err := b.GetDailyStats(ctx, cameraID, date)
		if err != nil {
			return nil, err
		}
		stats = append(stats, dayStats)
	}

	return stats, nil
}

// FindSegmentsContaining finds segments that contain a specific timestamp
func (b *TimelineBuilder) FindSegmentsContaining(ctx context.Context, cameraID string, timestamp time.Time) ([]Segment, error) {
	// Search a small window around the timestamp
	start := timestamp.Add(-time.Minute)
	end := timestamp.Add(time.Minute)

	segments, err := b.repository.GetByTimeRange(ctx, cameraID, start, end)
	if err != nil {
		return nil, err
	}

	// Filter to segments that actually contain the timestamp
	var containing []Segment
	for _, seg := range segments {
		if !seg.StartTime.After(timestamp) && !seg.EndTime.Before(timestamp) {
			containing = append(containing, seg)
		}
	}

	return containing, nil
}

// GetPlaybackURL returns the URL/path for playback at a specific timestamp
func (b *TimelineBuilder) GetPlaybackURL(ctx context.Context, cameraID string, timestamp time.Time) (string, float64, error) {
	segments, err := b.FindSegmentsContaining(ctx, cameraID, timestamp)
	if err != nil {
		return "", 0, err
	}

	if len(segments) == 0 {
		return "", 0, ErrNoSegmentFound
	}

	// Use the first matching segment
	segment := segments[0]
	offset := timestamp.Sub(segment.StartTime).Seconds()

	return segment.FilePath, offset, nil
}

// TimelineError represents a timeline-related error
type TimelineError string

func (e TimelineError) Error() string { return string(e) }

// ErrNoSegmentFound is returned when no segment is found for a timestamp
const ErrNoSegmentFound = TimelineError("no segment found for timestamp")

// MergeTimelines merges timelines from multiple cameras
func MergeTimelines(timelines []*Timeline) *Timeline {
	if len(timelines) == 0 {
		return nil
	}

	merged := &Timeline{
		CameraID:  "all",
		StartTime: timelines[0].StartTime,
		EndTime:   timelines[0].EndTime,
		Segments:  make([]TimelineSegment, 0),
	}

	// Find overall time range
	for _, t := range timelines {
		if t.StartTime.Before(merged.StartTime) {
			merged.StartTime = t.StartTime
		}
		if t.EndTime.After(merged.EndTime) {
			merged.EndTime = t.EndTime
		}
		merged.TotalSize += t.TotalSize
		merged.TotalHours += t.TotalHours
	}

	// Collect all segment boundaries
	type boundary struct {
		time     time.Time
		isStart  bool
		hasEvent bool
	}

	var boundaries []boundary
	for _, t := range timelines {
		for _, seg := range t.Segments {
			if seg.Type == "recording" {
				boundaries = append(boundaries, boundary{seg.StartTime, true, seg.HasEvents})
				boundaries = append(boundaries, boundary{seg.EndTime, false, false})
			}
		}
	}

	// Sort boundaries
	sort.Slice(boundaries, func(i, j int) bool {
		if boundaries[i].time.Equal(boundaries[j].time) {
			return boundaries[i].isStart
		}
		return boundaries[i].time.Before(boundaries[j].time)
	})

	// Build merged timeline
	activeCount := 0
	hasEvents := false
	currentTime := merged.StartTime

	for _, b := range boundaries {
		if b.time.Before(merged.StartTime) || b.time.After(merged.EndTime) {
			continue
		}

		if activeCount == 0 && currentTime.Before(b.time) {
			// Add gap
			merged.Segments = append(merged.Segments, TimelineSegment{
				StartTime: currentTime,
				EndTime:   b.time,
				Type:      "gap",
			})
		}

		if b.isStart {
			if activeCount == 0 {
				currentTime = b.time
			}
			activeCount++
			if b.hasEvent {
				hasEvents = true
			}
		} else {
			activeCount--
			if activeCount == 0 {
				// Add recording segment
				merged.Segments = append(merged.Segments, TimelineSegment{
					StartTime: currentTime,
					EndTime:   b.time,
					Type:      "recording",
					HasEvents: hasEvents,
				})
				currentTime = b.time
				hasEvents = false
			}
		}
	}

	// Add trailing gap if needed
	if currentTime.Before(merged.EndTime) {
		merged.Segments = append(merged.Segments, TimelineSegment{
			StartTime: currentTime,
			EndTime:   merged.EndTime,
			Type:      "gap",
		})
	}

	return merged
}
