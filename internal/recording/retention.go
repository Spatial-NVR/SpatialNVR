package recording

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/Spatial-NVR/SpatialNVR/internal/config"
)

// RetentionPolicy manages retention and cleanup of recordings
type RetentionPolicy struct {
	mu            sync.RWMutex
	config        *config.Config
	repository    Repository
	segmentHandler SegmentHandler
	storagePath   string
	maxStorageGB  int
	running       bool
	stopCh        chan struct{}
	logger        *slog.Logger
}

// RetentionConfig holds retention policy configuration
type RetentionPolicyConfig struct {
	DefaultDays    int   // Default retention period
	EventsDays     int   // Retention for event-related recordings
	MaxStorageGB   int   // Maximum storage usage
	CleanupInterval time.Duration
}

// NewRetentionPolicy creates a new retention policy manager
func NewRetentionPolicy(
	cfg *config.Config,
	repository Repository,
	segmentHandler SegmentHandler,
	storagePath string,
) *RetentionPolicy {
	return &RetentionPolicy{
		config:         cfg,
		repository:     repository,
		segmentHandler: segmentHandler,
		storagePath:    storagePath,
		maxStorageGB:   cfg.System.MaxStorageGB,
		stopCh:         make(chan struct{}),
		logger:         slog.Default().With("component", "retention"),
	}
}

// Start starts the retention policy enforcement
func (p *RetentionPolicy) Start(ctx context.Context, interval time.Duration) error {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return nil
	}
	p.running = true
	p.stopCh = make(chan struct{})
	p.mu.Unlock()

	go p.runCleanupLoop(ctx, interval)
	return nil
}

// Stop stops the retention policy enforcement
func (p *RetentionPolicy) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return
	}

	close(p.stopCh)
	p.running = false
}

// runCleanupLoop runs the periodic cleanup
func (p *RetentionPolicy) runCleanupLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run initial cleanup
	if _, err := p.RunCleanup(ctx); err != nil {
		p.logger.Error("Initial retention cleanup failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stopCh:
			return
		case <-ticker.C:
			if _, err := p.RunCleanup(ctx); err != nil {
				p.logger.Error("Retention cleanup failed", "error", err)
			}
		}
	}
}

// RunCleanup executes a cleanup cycle
func (p *RetentionPolicy) RunCleanup(ctx context.Context) (*RetentionStats, error) {
	p.logger.Info("Starting retention cleanup")
	stats := &RetentionStats{}

	// Get all cameras from config
	for _, camera := range p.config.Cameras {
		if !camera.Recording.Enabled {
			continue
		}

		cameraStats, err := p.cleanupCamera(ctx, camera)
		if err != nil {
			p.logger.Error("Failed to cleanup camera", "camera", camera.ID, "error", err)
			continue
		}

		stats.SegmentsDeleted += cameraStats.SegmentsDeleted
		stats.BytesFreed += cameraStats.BytesFreed
	}

	// Check overall storage usage
	if p.maxStorageGB > 0 {
		storageStats, err := p.enforceStorageLimit(ctx)
		if err != nil {
			p.logger.Error("Failed to enforce storage limit", "error", err)
		} else {
			stats.SegmentsDeleted += storageStats.SegmentsDeleted
			stats.BytesFreed += storageStats.BytesFreed
		}
	}

	p.logger.Info("Retention cleanup completed",
		"segments_deleted", stats.SegmentsDeleted,
		"bytes_freed", stats.BytesFreed,
	)

	return stats, nil
}

// cleanupCamera cleans up old segments for a single camera
func (p *RetentionPolicy) cleanupCamera(ctx context.Context, camera config.CameraConfig) (*RetentionStats, error) {
	stats := &RetentionStats{}

	// Get retention settings
	defaultDays := camera.Recording.Retention.DefaultDays
	if defaultDays <= 0 {
		defaultDays = 30 // Default to 30 days
	}
	eventsDays := camera.Recording.Retention.EventsDays
	if eventsDays <= 0 {
		eventsDays = defaultDays * 2 // Events kept longer by default
	}

	now := time.Now()

	// Delete non-event segments older than defaultDays
	defaultCutoff := now.AddDate(0, 0, -defaultDays)
	noEvents := false
	segments, _, err := p.repository.List(ctx, ListOptions{
		CameraID:  camera.ID,
		EndTime:   &defaultCutoff,
		HasEvents: &noEvents,
		Limit:     1000,
	})
	if err != nil {
		return stats, fmt.Errorf("failed to list old segments: %w", err)
	}

	for _, segment := range segments {
		if err := p.deleteSegment(ctx, &segment); err != nil {
			p.logger.Error("Failed to delete segment", "id", segment.ID, "error", err)
			continue
		}
		stats.SegmentsDeleted++
		stats.BytesFreed += segment.FileSize
	}

	// Delete event segments older than eventsDays
	eventsCutoff := now.AddDate(0, 0, -eventsDays)
	hasEvents := true
	eventSegments, _, err := p.repository.List(ctx, ListOptions{
		CameraID:  camera.ID,
		EndTime:   &eventsCutoff,
		HasEvents: &hasEvents,
		Limit:     1000,
	})
	if err != nil {
		return stats, fmt.Errorf("failed to list old event segments: %w", err)
	}

	for _, segment := range eventSegments {
		if err := p.deleteSegment(ctx, &segment); err != nil {
			p.logger.Error("Failed to delete event segment", "id", segment.ID, "error", err)
			continue
		}
		stats.SegmentsDeleted++
		stats.BytesFreed += segment.FileSize
	}

	return stats, nil
}

// enforceStorageLimit ensures storage usage stays within limits
func (p *RetentionPolicy) enforceStorageLimit(ctx context.Context) (*RetentionStats, error) {
	stats := &RetentionStats{}

	maxBytes := int64(p.maxStorageGB) * 1024 * 1024 * 1024

	// Get current usage
	usage, err := p.getStorageUsage()
	if err != nil {
		return stats, fmt.Errorf("failed to get storage usage: %w", err)
	}

	if usage <= maxBytes {
		return stats, nil
	}

	p.logger.Warn("Storage limit exceeded",
		"used_gb", float64(usage)/1024/1024/1024,
		"max_gb", p.maxStorageGB,
	)

	// Delete oldest segments until we're under the limit
	targetBytes := int64(float64(maxBytes) * 0.9) // Target 90% of max
	bytesToFree := usage - targetBytes

	// Get storage by camera to balance deletion
	byCamera, err := p.repository.GetStorageByCamera(ctx)
	if err != nil {
		return stats, fmt.Errorf("failed to get storage by camera: %w", err)
	}

	// Delete proportionally from each camera
	for cameraID, cameraBytes := range byCamera {
		if bytesToFree <= 0 {
			break
		}

		// Calculate proportion to delete from this camera
		proportion := float64(cameraBytes) / float64(usage)
		cameraToFree := int64(float64(bytesToFree) * proportion)

		cameraStats, err := p.freeSpaceForCamera(ctx, cameraID, cameraToFree)
		if err != nil {
			p.logger.Error("Failed to free space for camera", "camera", cameraID, "error", err)
			continue
		}

		stats.SegmentsDeleted += cameraStats.SegmentsDeleted
		stats.BytesFreed += cameraStats.BytesFreed
		bytesToFree -= cameraStats.BytesFreed
	}

	return stats, nil
}

// freeSpaceForCamera deletes oldest segments for a camera to free space
func (p *RetentionPolicy) freeSpaceForCamera(ctx context.Context, cameraID string, bytesToFree int64) (*RetentionStats, error) {
	stats := &RetentionStats{}

	var freed int64
	offset := 0
	batchSize := 100

	for freed < bytesToFree {
		// Get oldest non-event segments first
		noEvents := false
		segments, _, err := p.repository.List(ctx, ListOptions{
			CameraID:  cameraID,
			HasEvents: &noEvents,
			OrderBy:   "start_time",
			OrderDesc: false,
			Limit:     batchSize,
			Offset:    offset,
		})
		if err != nil {
			return stats, err
		}

		if len(segments) == 0 {
			// Fall back to event segments
			segments, _, err = p.repository.List(ctx, ListOptions{
				CameraID:  cameraID,
				OrderBy:   "start_time",
				OrderDesc: false,
				Limit:     batchSize,
				Offset:    offset,
			})
			if err != nil {
				return stats, err
			}
		}

		if len(segments) == 0 {
			break // No more segments to delete
		}

		for _, segment := range segments {
			if freed >= bytesToFree {
				break
			}

			if err := p.deleteSegment(ctx, &segment); err != nil {
				p.logger.Error("Failed to delete segment", "id", segment.ID, "error", err)
				offset++
				continue
			}

			stats.SegmentsDeleted++
			stats.BytesFreed += segment.FileSize
			freed += segment.FileSize
		}
	}

	return stats, nil
}

// deleteSegment deletes a segment from disk and database
func (p *RetentionPolicy) deleteSegment(ctx context.Context, segment *Segment) error {
	// Delete file
	if err := p.segmentHandler.Delete(segment); err != nil {
		// Log but continue if file is already gone
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to delete file: %w", err)
		}
	}

	// Delete from database
	if err := p.repository.Delete(ctx, segment.ID); err != nil {
		return fmt.Errorf("failed to delete from database: %w", err)
	}

	return nil
}

// getStorageUsage returns total storage usage in bytes
func (p *RetentionPolicy) getStorageUsage() (int64, error) {
	var total int64

	err := walkDir(p.storagePath, func(path string, info os.FileInfo) error {
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})

	return total, err
}

// walkDir walks a directory tree
func walkDir(root string, fn func(path string, info os.FileInfo) error) error {
	return walkDirRecursive(root, fn)
}

func walkDirRecursive(path string, fn func(path string, info os.FileInfo) error) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		fullPath := path + "/" + entry.Name()
		info, err := entry.Info()
		if err != nil {
			continue
		}

		if err := fn(fullPath, info); err != nil {
			return err
		}

		if entry.IsDir() {
			if err := walkDirRecursive(fullPath, fn); err != nil {
				return err
			}
		}
	}

	return nil
}

// TierMigration handles storage tier migration
type TierMigration struct {
	repository     Repository
	segmentHandler SegmentHandler
	hotPath        string
	warmPath       string
	coldPath       string
	logger         *slog.Logger
}

// NewTierMigration creates a new tier migration manager
func NewTierMigration(
	repository Repository,
	segmentHandler SegmentHandler,
	hotPath, warmPath, coldPath string,
) *TierMigration {
	return &TierMigration{
		repository:     repository,
		segmentHandler: segmentHandler,
		hotPath:        hotPath,
		warmPath:       warmPath,
		coldPath:       coldPath,
		logger:         slog.Default().With("component", "tier_migration"),
	}
}

// MigrateToWarm moves segments from hot to warm storage
func (m *TierMigration) MigrateToWarm(ctx context.Context, olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan)
	tier := StorageTierHot

	segments, _, err := m.repository.List(ctx, ListOptions{
		EndTime: &cutoff,
		Tier:    &tier,
		Limit:   100,
	})
	if err != nil {
		return err
	}

	for _, segment := range segments {
		if err := m.migrateSegment(ctx, &segment, StorageTierWarm, m.warmPath); err != nil {
			m.logger.Error("Failed to migrate segment to warm", "id", segment.ID, "error", err)
		}
	}

	return nil
}

// migrateSegment moves a segment to a different tier
func (m *TierMigration) migrateSegment(ctx context.Context, segment *Segment, newTier StorageTier, newBasePath string) error {
	// Implementation would:
	// 1. Copy file to new location
	// 2. Update database record
	// 3. Delete original file
	// For now, just update the tier in the database
	segment.StorageTier = newTier
	return m.repository.Update(ctx, segment)
}
