-- Add extended recording columns
-- Version: 003
-- Description: Add thumbnail, recording_mode, trigger_event_id, and updated_at columns to recordings table

-- Add thumbnail column for preview images
ALTER TABLE recordings ADD COLUMN thumbnail TEXT;

-- Add recording_mode column (continuous, motion, events)
ALTER TABLE recordings ADD COLUMN recording_mode TEXT;

-- Add trigger_event_id for event-triggered recordings
ALTER TABLE recordings ADD COLUMN trigger_event_id TEXT;

-- Add updated_at column for tracking modifications
ALTER TABLE recordings ADD COLUMN updated_at INTEGER NOT NULL DEFAULT (unixepoch());

-- Add index on recordings start_time for timeline queries
CREATE INDEX IF NOT EXISTS idx_recordings_start_time ON recordings(start_time);

-- Add index on storage_tier for cleanup queries
CREATE INDEX IF NOT EXISTS idx_recordings_storage_tier ON recordings(storage_tier);

-- Add index on has_events for event filtering
CREATE INDEX IF NOT EXISTS idx_recordings_has_events ON recordings(has_events);
