#!/bin/sh
# SpatialNVR Docker Entrypoint
#
# This script checks for updated components in /data before running the
# shipped versions in /app. This enables self-updating without container rebuilds.
#
# Update priority:
# 1. /data/bin/nvr (user-installed update)
# 2. /app/bin/nvr (container-shipped version)
#
# Web UI priority:
# 1. /data/web (user-installed update)
# 2. /app/web (container-shipped version)
#
# PUID/PGID Support:
# Set PUID and PGID environment variables to run the container as a specific user.
# This is useful for NAS systems like Unraid where volume permissions matter.
# Example: -e PUID=99 -e PGID=100

set -e

# ============================================================================
# PUID/PGID handling - modify nvr user to match host user
# ============================================================================
PUID=${PUID:-1000}
PGID=${PGID:-1000}

# Only modify if running as root and PUID/PGID differ from defaults
if [ "$(id -u)" = "0" ]; then
    echo "[entrypoint] Setting up user with PUID=$PUID and PGID=$PGID"

    # Modify group ID if different
    if [ "$(id -g nvr)" != "$PGID" ]; then
        groupmod -o -g "$PGID" nvr 2>/dev/null || true
    fi

    # Modify user ID if different
    if [ "$(id -u nvr)" != "$PUID" ]; then
        usermod -o -u "$PUID" nvr 2>/dev/null || true
    fi
fi

# Ensure required directories exist with correct permissions
# This is needed when volumes are mounted on a fresh system
DATA_DIR="${DATA_PATH:-/data}"
mkdir -p "$DATA_DIR" \
         "$DATA_DIR/bin" \
         "$DATA_DIR/web" \
         "$DATA_DIR/plugins" \
         "$DATA_DIR/updates" \
         "$DATA_DIR/recordings" \
         "$DATA_DIR/thumbnails" \
         "$DATA_DIR/snapshots" \
         "$DATA_DIR/exports" \
         "$DATA_DIR/models" \
         /config \
         /tokens \
         /img 2>/dev/null || true

# Set ownership of directories if running as root
if [ "$(id -u)" = "0" ]; then
    chown nvr:nvr "$DATA_DIR" \
                  "$DATA_DIR/bin" \
                  "$DATA_DIR/web" \
                  "$DATA_DIR/plugins" \
                  "$DATA_DIR/updates" \
                  "$DATA_DIR/recordings" \
                  "$DATA_DIR/thumbnails" \
                  "$DATA_DIR/snapshots" \
                  "$DATA_DIR/exports" \
                  "$DATA_DIR/models" \
                  /config \
                  /tokens \
                  /img 2>/dev/null || true
fi

# Determine which NVR binary to use
if [ -x "/data/bin/nvr" ]; then
    NVR_BIN="/data/bin/nvr"
    echo "[entrypoint] Using updated NVR binary from /data/bin/nvr"
else
    NVR_BIN="/app/bin/nvr"
    # Fall back to legacy location if new location doesn't exist
    if [ ! -x "$NVR_BIN" ] && [ -x "/app/nvr" ]; then
        NVR_BIN="/app/nvr"
    fi
    echo "[entrypoint] Using shipped NVR binary from $NVR_BIN"
fi

# Determine which web UI to use
if [ -d "/data/web" ] && [ -f "/data/web/index.html" ]; then
    export WEB_PATH="/data/web"
    echo "[entrypoint] Using updated Web UI from /data/web"
else
    export WEB_PATH="/app/web"
    echo "[entrypoint] Using shipped Web UI from /app/web"
fi

# Determine which go2rtc to use
if [ -x "/data/bin/go2rtc" ]; then
    export GO2RTC_PATH="/data/bin/go2rtc"
    echo "[entrypoint] Using updated go2rtc from /data/bin/go2rtc"
else
    export GO2RTC_PATH="/app/bin/go2rtc"
    echo "[entrypoint] Using shipped go2rtc from /app/bin/go2rtc"
fi

# Print version info
echo "[entrypoint] Starting SpatialNVR..."
echo "[entrypoint] NVR Binary: $NVR_BIN"
echo "[entrypoint] Web Path: $WEB_PATH"
echo "[entrypoint] go2rtc Path: $GO2RTC_PATH"
echo "[entrypoint] Data Path: ${DATA_PATH:-/data}"
echo "[entrypoint] Config Path: ${CONFIG_PATH:-/config/config.yaml}"

# Execute the NVR binary with any passed arguments
# If running as root, drop privileges to nvr user
if [ "$(id -u)" = "0" ]; then
    echo "[entrypoint] Dropping privileges to nvr user (uid=$PUID, gid=$PGID)"
    exec gosu nvr "$NVR_BIN" "$@"
else
    exec "$NVR_BIN" "$@"
fi
