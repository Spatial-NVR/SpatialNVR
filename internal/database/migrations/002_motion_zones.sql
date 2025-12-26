-- Motion Zones Migration
-- Version: 002
-- Description: Add motion zones table and enhance camera audio settings

-- ====================
-- MOTION ZONES
-- ====================
CREATE TABLE IF NOT EXISTS motion_zones (
    id TEXT PRIMARY KEY,
    camera_id TEXT NOT NULL,
    name TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0, 1)),
    points TEXT NOT NULL, -- JSON array of {x, y} points
    object_types TEXT, -- JSON array of object types to detect
    min_confidence REAL NOT NULL DEFAULT 0.5,
    min_size REAL,
    sensitivity INTEGER NOT NULL DEFAULT 5 CHECK(sensitivity BETWEEN 1 AND 10),
    cooldown_seconds INTEGER NOT NULL DEFAULT 30,
    notifications INTEGER NOT NULL DEFAULT 1 CHECK(notifications IN (0, 1)),
    recording INTEGER NOT NULL DEFAULT 1 CHECK(recording IN (0, 1)),
    color TEXT,
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at INTEGER NOT NULL DEFAULT (unixepoch()),
    FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_zones_camera ON motion_zones(camera_id);
CREATE INDEX IF NOT EXISTS idx_zones_enabled ON motion_zones(enabled) WHERE enabled = 1;

-- ====================
-- DOORBELL EVENTS
-- ====================
CREATE TABLE IF NOT EXISTS doorbell_events (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL,
    camera_id TEXT NOT NULL,
    answered INTEGER NOT NULL DEFAULT 0 CHECK(answered IN (0, 1)),
    answered_at INTEGER,
    answered_by TEXT,
    duration_seconds INTEGER,
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    FOREIGN KEY (event_id) REFERENCES events(id) ON DELETE CASCADE,
    FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_doorbell_camera ON doorbell_events(camera_id);
CREATE INDEX IF NOT EXISTS idx_doorbell_event ON doorbell_events(event_id);

-- ====================
-- AUDIO SESSIONS
-- ====================
CREATE TABLE IF NOT EXISTS audio_sessions (
    id TEXT PRIMARY KEY,
    camera_id TEXT NOT NULL,
    user_id TEXT,
    started_at INTEGER NOT NULL,
    ended_at INTEGER,
    duration_seconds INTEGER,
    session_type TEXT NOT NULL DEFAULT 'two_way' CHECK(session_type IN ('two_way', 'listen', 'speak')),
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_audio_camera ON audio_sessions(camera_id);
CREATE INDEX IF NOT EXISTS idx_audio_started ON audio_sessions(started_at DESC);

-- ====================
-- CAMERA CAPABILITIES (cached from camera discovery)
-- ====================
CREATE TABLE IF NOT EXISTS camera_capabilities (
    camera_id TEXT PRIMARY KEY,
    has_audio INTEGER NOT NULL DEFAULT 0 CHECK(has_audio IN (0, 1)),
    has_two_way_audio INTEGER NOT NULL DEFAULT 0 CHECK(has_two_way_audio IN (0, 1)),
    has_ptz INTEGER NOT NULL DEFAULT 0 CHECK(has_ptz IN (0, 1)),
    has_ir INTEGER NOT NULL DEFAULT 0 CHECK(has_ir IN (0, 1)),
    has_motion_detection INTEGER NOT NULL DEFAULT 0 CHECK(has_motion_detection IN (0, 1)),
    is_doorbell INTEGER NOT NULL DEFAULT 0 CHECK(is_doorbell IN (0, 1)),
    supported_codecs TEXT, -- JSON array
    max_resolution TEXT,
    max_fps INTEGER,
    discovered_at INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at INTEGER NOT NULL DEFAULT (unixepoch()),
    FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
);
