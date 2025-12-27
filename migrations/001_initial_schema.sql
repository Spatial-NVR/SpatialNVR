-- NVR System Initial Schema
-- Version: 001
-- Description: Initial database schema for NVR system

-- Enable features
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA cache_size = -64000;
PRAGMA temp_store = MEMORY;

-- ====================
-- CAMERAS
-- ====================
-- Runtime state only - configuration lives in YAML
CREATE TABLE IF NOT EXISTS cameras (
    id TEXT PRIMARY KEY,
    status TEXT CHECK(status IN ('online', 'offline', 'error', 'starting')) NOT NULL DEFAULT 'offline',
    last_seen INTEGER NOT NULL DEFAULT 0,
    fps_current REAL,
    bitrate_current INTEGER,
    resolution_current TEXT,
    stats JSON,
    error_message TEXT,
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at INTEGER NOT NULL DEFAULT (unixepoch())
) STRICT;

CREATE INDEX IF NOT EXISTS idx_cameras_status ON cameras(status);
CREATE INDEX IF NOT EXISTS idx_cameras_last_seen ON cameras(last_seen DESC);

-- ====================
-- EVENTS
-- ====================
CREATE TABLE IF NOT EXISTS events (
    id TEXT PRIMARY KEY,
    camera_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    label TEXT,
    timestamp INTEGER NOT NULL,
    end_timestamp INTEGER,
    confidence REAL,
    thumbnail_path TEXT,
    video_segment_id TEXT,
    metadata JSON,
    acknowledged INTEGER NOT NULL DEFAULT 0 CHECK(acknowledged IN (0, 1)),
    tags JSON,
    notes TEXT,
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
) STRICT;

CREATE INDEX IF NOT EXISTS idx_events_camera_time ON events(camera_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_events_type ON events(event_type);
CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_events_camera_type_time ON events(camera_id, event_type, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_events_ack ON events(acknowledged) WHERE acknowledged = 0;

-- Full-text search on notes
CREATE VIRTUAL TABLE IF NOT EXISTS events_fts USING fts5(
    event_id UNINDEXED,
    notes,
    content=events,
    content_rowid=rowid
);

-- ====================
-- DETECTIONS
-- ====================
CREATE TABLE IF NOT EXISTS detections (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL,
    object_type TEXT NOT NULL,
    label TEXT,
    confidence REAL NOT NULL,
    bbox JSON NOT NULL,
    attributes JSON,
    track_id TEXT,
    track_confidence REAL,
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    FOREIGN KEY (event_id) REFERENCES events(id) ON DELETE CASCADE
) STRICT;

CREATE INDEX IF NOT EXISTS idx_detections_event ON detections(event_id);
CREATE INDEX IF NOT EXISTS idx_detections_object ON detections(object_type);
CREATE INDEX IF NOT EXISTS idx_detections_track ON detections(track_id) WHERE track_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_detections_label ON detections(label) WHERE label IS NOT NULL;

-- ====================
-- PERSONS (Facial Recognition)
-- ====================
CREATE TABLE IF NOT EXISTS persons (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    notes TEXT,
    thumbnail_path TEXT,
    reference_embeddings JSON,
    metadata JSON,
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at INTEGER NOT NULL DEFAULT (unixepoch())
) STRICT;

CREATE INDEX IF NOT EXISTS idx_persons_name ON persons(name);

-- ====================
-- FACES
-- ====================
CREATE TABLE IF NOT EXISTS faces (
    id TEXT PRIMARY KEY,
    person_id TEXT,
    event_id TEXT NOT NULL,
    embedding BLOB NOT NULL,
    confidence REAL NOT NULL,
    match_confidence REAL,
    bbox JSON NOT NULL,
    age INTEGER,
    gender TEXT CHECK(gender IN ('M', 'F', 'unknown')),
    attributes JSON,
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    FOREIGN KEY (person_id) REFERENCES persons(id) ON DELETE SET NULL,
    FOREIGN KEY (event_id) REFERENCES events(id) ON DELETE CASCADE
) STRICT;

CREATE INDEX IF NOT EXISTS idx_faces_person ON faces(person_id);
CREATE INDEX IF NOT EXISTS idx_faces_event ON faces(event_id);
CREATE INDEX IF NOT EXISTS idx_faces_unknown ON faces(person_id) WHERE person_id IS NULL;

-- ====================
-- VEHICLES (LPR)
-- ====================
CREATE TABLE IF NOT EXISTS vehicles (
    id TEXT PRIMARY KEY,
    license_plate TEXT NOT NULL UNIQUE,
    make TEXT,
    model TEXT,
    year INTEGER,
    color TEXT,
    owner_name TEXT,
    relationship TEXT,
    notes TEXT,
    thumbnail_path TEXT,
    allowed INTEGER NOT NULL DEFAULT 1 CHECK(allowed IN (0, 1)),
    allowed_zones JSON,
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at INTEGER NOT NULL DEFAULT (unixepoch())
) STRICT;

CREATE INDEX IF NOT EXISTS idx_vehicles_plate ON vehicles(license_plate);
CREATE INDEX IF NOT EXISTS idx_vehicles_allowed ON vehicles(allowed);

-- ====================
-- LPR DETECTIONS
-- ====================
CREATE TABLE IF NOT EXISTS lpr_detections (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL,
    vehicle_id TEXT,
    plate_text TEXT NOT NULL,
    confidence REAL NOT NULL,
    bbox JSON NOT NULL,
    attributes JSON,
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    FOREIGN KEY (event_id) REFERENCES events(id) ON DELETE CASCADE,
    FOREIGN KEY (vehicle_id) REFERENCES vehicles(id) ON DELETE SET NULL
) STRICT;

CREATE INDEX IF NOT EXISTS idx_lpr_event ON lpr_detections(event_id);
CREATE INDEX IF NOT EXISTS idx_lpr_vehicle ON lpr_detections(vehicle_id);
CREATE INDEX IF NOT EXISTS idx_lpr_plate ON lpr_detections(plate_text);

-- ====================
-- RECORDINGS
-- ====================
CREATE TABLE IF NOT EXISTS recordings (
    id TEXT PRIMARY KEY,
    camera_id TEXT NOT NULL,
    start_time INTEGER NOT NULL,
    end_time INTEGER NOT NULL,
    duration INTEGER NOT NULL,
    file_path TEXT NOT NULL,
    file_size INTEGER NOT NULL,
    storage_tier TEXT NOT NULL DEFAULT 'hot' CHECK(storage_tier IN ('hot', 'warm', 'cold')),
    has_events INTEGER NOT NULL DEFAULT 0 CHECK(has_events IN (0, 1)),
    event_count INTEGER NOT NULL DEFAULT 0,
    codec TEXT,
    resolution TEXT,
    fps REAL,
    bitrate INTEGER,
    checksum TEXT,
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
) STRICT;

CREATE INDEX IF NOT EXISTS idx_recordings_camera_time ON recordings(camera_id, start_time DESC);
CREATE INDEX IF NOT EXISTS idx_recordings_tier ON recordings(storage_tier);
CREATE INDEX IF NOT EXISTS idx_recordings_events ON recordings(has_events) WHERE has_events = 1;
CREATE INDEX IF NOT EXISTS idx_recordings_cleanup ON recordings(storage_tier, end_time);

-- ====================
-- USERS
-- ====================
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    email TEXT UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'user' CHECK(role IN ('admin', 'user', 'guest', 'api')),
    permissions JSON,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0, 1)),
    totp_secret TEXT,
    totp_enabled INTEGER NOT NULL DEFAULT 0 CHECK(totp_enabled IN (0, 1)),
    last_login INTEGER,
    last_ip TEXT,
    preferences JSON,
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at INTEGER NOT NULL DEFAULT (unixepoch())
) STRICT;

CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email) WHERE email IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_users_enabled ON users(enabled) WHERE enabled = 1;

-- ====================
-- API KEYS
-- ====================
CREATE TABLE IF NOT EXISTS api_keys (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    key_hash TEXT NOT NULL UNIQUE,
    key_prefix TEXT NOT NULL,
    name TEXT NOT NULL,
    permissions JSON,
    last_used INTEGER,
    expires_at INTEGER,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0, 1)),
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) STRICT;

CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys(user_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash);

-- ====================
-- PLUGINS
-- ====================
CREATE TABLE IF NOT EXISTS plugins (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    version TEXT NOT NULL,
    author TEXT,
    description TEXT,
    manifest JSON NOT NULL,
    install_path TEXT NOT NULL,
    config JSON,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0, 1)),
    status TEXT NOT NULL DEFAULT 'installed' CHECK(status IN ('installed', 'running', 'stopped', 'error')),
    error_message TEXT,
    installed_at INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at INTEGER NOT NULL DEFAULT (unixepoch()),
    last_started INTEGER,
    last_stopped INTEGER
) STRICT;

CREATE INDEX IF NOT EXISTS idx_plugins_enabled ON plugins(enabled) WHERE enabled = 1;
CREATE INDEX IF NOT EXISTS idx_plugins_status ON plugins(status);

-- ====================
-- NOTIFICATIONS
-- ====================
CREATE TABLE IF NOT EXISTS notifications (
    id TEXT PRIMARY KEY,
    event_id TEXT,
    title TEXT NOT NULL,
    message TEXT NOT NULL,
    thumbnail_path TEXT,
    channels JSON NOT NULL,
    priority TEXT NOT NULL DEFAULT 'normal' CHECK(priority IN ('low', 'normal', 'high', 'urgent')),
    status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'sent', 'failed')),
    sent_at INTEGER,
    error_message TEXT,
    metadata JSON,
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    FOREIGN KEY (event_id) REFERENCES events(id) ON DELETE SET NULL
) STRICT;

CREATE INDEX IF NOT EXISTS idx_notifications_status ON notifications(status) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_notifications_created ON notifications(created_at DESC);

-- ====================
-- SYSTEM CONFIG
-- ====================
CREATE TABLE IF NOT EXISTS system_config (
    key TEXT PRIMARY KEY,
    value JSON NOT NULL,
    description TEXT,
    updated_at INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_by TEXT
) STRICT;

-- ====================
-- AUDIT LOG
-- ====================
CREATE TABLE IF NOT EXISTS audit_log (
    id TEXT PRIMARY KEY,
    user_id TEXT,
    ip_address TEXT,
    action TEXT NOT NULL,
    resource_type TEXT,
    resource_id TEXT,
    changes JSON,
    success INTEGER NOT NULL CHECK(success IN (0, 1)),
    error_message TEXT,
    timestamp INTEGER NOT NULL DEFAULT (unixepoch()),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL
) STRICT;

CREATE INDEX IF NOT EXISTS idx_audit_user_time ON audit_log(user_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_log(action);
CREATE INDEX IF NOT EXISTS idx_audit_time ON audit_log(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_resource ON audit_log(resource_type, resource_id);

-- ====================
-- SCHEMA MIGRATIONS
-- ====================
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at INTEGER NOT NULL DEFAULT (unixepoch())
) STRICT;

-- Record this migration
INSERT OR IGNORE INTO schema_migrations (version, name) VALUES (1, 'initial_schema');
