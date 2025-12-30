#!/bin/bash
# SpatialNVR Docker Entrypoint with Hot-Restart Support
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
# Hot-Restart Support:
# Send SIGHUP to the entrypoint (PID 1) to trigger a hot restart of NVR.
# This allows updating the binary and restarting without container recreation.
# Example: docker exec <container> kill -HUP 1
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
    # Recursively chown directories that need write access
    chown -R nvr:nvr "$DATA_DIR" \
                     /config \
                     /tokens \
                     /img 2>/dev/null || true

    # Ensure config file is writable if it exists (0600 for security - contains secrets)
    if [ -f "/config/config.yaml" ]; then
        chown nvr:nvr /config/config.yaml 2>/dev/null || true
        chmod 600 /config/config.yaml 2>/dev/null || true
    fi

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

# ============================================================================
# Hot-Restart Loop
# This allows the NVR to be restarted without recreating the container.
# Send SIGHUP to trigger a restart, SIGTERM/SIGINT to stop.
# ============================================================================

# Track restart state
RESTART_REQUESTED=0
NVR_PID=0

# Graceful shutdown timeout (seconds) - configurable via environment
SHUTDOWN_TIMEOUT=${SHUTDOWN_TIMEOUT:-30}

# Graceful shutdown function with timeout
# Ensures the NVR process has time to cleanup before being killed
graceful_shutdown() {
    local signal=$1
    local pid=$2

    if [ $pid -eq 0 ]; then
        return 0
    fi

    # Check if process is still running
    if ! kill -0 $pid 2>/dev/null; then
        echo "[entrypoint] NVR process already exited"
        return 0
    fi

    echo "[entrypoint] Sending TERM signal to NVR (PID $pid)..."
    kill -TERM $pid 2>/dev/null || true

    # Wait for graceful shutdown with timeout
    local elapsed=0
    while [ $elapsed -lt $SHUTDOWN_TIMEOUT ]; do
        if ! kill -0 $pid 2>/dev/null; then
            echo "[entrypoint] NVR shut down gracefully after ${elapsed}s"
            return 0
        fi
        sleep 1
        elapsed=$((elapsed + 1))

        # Show progress every 5 seconds
        if [ $((elapsed % 5)) -eq 0 ]; then
            echo "[entrypoint] Waiting for graceful shutdown... ${elapsed}/${SHUTDOWN_TIMEOUT}s"
        fi
    done

    # Force kill if still running
    echo "[entrypoint] NVR did not shutdown gracefully after ${SHUTDOWN_TIMEOUT}s, sending KILL..."
    kill -KILL $pid 2>/dev/null || true
    sleep 1
    return 0
}

# Signal handlers
handle_sighup() {
    echo "[entrypoint] SIGHUP received - hot restart requested"
    RESTART_REQUESTED=1
    graceful_shutdown "SIGHUP" $NVR_PID
}

handle_sigterm() {
    echo "[entrypoint] SIGTERM received - shutting down"
    RESTART_REQUESTED=0
    graceful_shutdown "SIGTERM" $NVR_PID
}

handle_sigint() {
    echo "[entrypoint] SIGINT received - shutting down"
    RESTART_REQUESTED=0
    graceful_shutdown "SIGINT" $NVR_PID
}

# Setup signal handlers
trap handle_sighup SIGHUP
trap handle_sigterm SIGTERM
trap handle_sigint SIGINT

# Determine how to run (with or without privilege drop)
run_nvr() {
    # Refresh binary selection (in case an update was installed)
    if [ -x "/data/bin/nvr" ]; then
        NVR_BIN="/data/bin/nvr"
        echo "[entrypoint] Using updated NVR binary from /data/bin/nvr"
    else
        NVR_BIN="/app/bin/nvr"
        if [ ! -x "$NVR_BIN" ] && [ -x "/app/nvr" ]; then
            NVR_BIN="/app/nvr"
        fi
        echo "[entrypoint] Using NVR binary from $NVR_BIN"
    fi

    # Refresh web path selection
    if [ -d "/data/web" ] && [ -f "/data/web/index.html" ]; then
        export WEB_PATH="/data/web"
    else
        export WEB_PATH="/app/web"
    fi

    # Refresh go2rtc selection
    if [ -x "/data/bin/go2rtc" ]; then
        export GO2RTC_PATH="/data/bin/go2rtc"
    else
        export GO2RTC_PATH="/app/bin/go2rtc"
    fi

    if [ "$(id -u)" = "0" ]; then
        gosu nvr "$NVR_BIN" "$@" &
    else
        "$NVR_BIN" "$@" &
    fi
    NVR_PID=$!
}

# Health check function - verifies NVR is responding after start
# This prevents the container from being considered healthy when NVR is stuck
HEALTH_CHECK_URL=${HEALTH_CHECK_URL:-"http://localhost:8080/health"}
HEALTH_CHECK_TIMEOUT=${HEALTH_CHECK_TIMEOUT:-60}

wait_for_healthy() {
    echo "[entrypoint] Waiting for NVR to become healthy..."
    local elapsed=0

    while [ $elapsed -lt $HEALTH_CHECK_TIMEOUT ]; do
        # Check if process is still running
        if ! kill -0 $NVR_PID 2>/dev/null; then
            echo "[entrypoint] NVR process died during startup"
            return 1
        fi

        # Try health check
        if curl -sf --connect-timeout 2 "$HEALTH_CHECK_URL" >/dev/null 2>&1; then
            echo "[entrypoint] NVR is healthy after ${elapsed}s"
            return 0
        fi

        sleep 2
        elapsed=$((elapsed + 2))

        # Show progress every 10 seconds
        if [ $((elapsed % 10)) -eq 0 ]; then
            echo "[entrypoint] Waiting for health check... ${elapsed}/${HEALTH_CHECK_TIMEOUT}s"
        fi
    done

    echo "[entrypoint] Health check timed out after ${HEALTH_CHECK_TIMEOUT}s"
    return 1
}

# Main restart loop
while true; do
    RESTART_REQUESTED=0

    echo "[entrypoint] Starting NVR process..."
    run_nvr "$@"

    # Give the process a moment to start
    sleep 2

    # Wait for health check on hot restarts (after updates)
    if [ -f "/data/updates/.health_check_pending" ]; then
        rm -f "/data/updates/.health_check_pending"
        if ! wait_for_healthy; then
            echo "[entrypoint] Post-update health check failed!"
            # Check if there's a rollback marker
            if [ -f "/data/updates/.rollback_on_failure" ]; then
                echo "[entrypoint] Rollback marker found, but rollback must be done manually via API"
                rm -f "/data/updates/.rollback_on_failure"
            fi
        fi
    fi

    # Wait for NVR to exit
    wait $NVR_PID
    EXIT_CODE=$?

    echo "[entrypoint] NVR process exited with code $EXIT_CODE"

    # Check if restart was requested
    if [ $RESTART_REQUESTED -eq 1 ]; then
        echo "[entrypoint] Hot restart in progress..."
        # Create health check pending marker for next start
        touch "/data/updates/.health_check_pending" 2>/dev/null || true
        sleep 1
        continue
    fi

    # Check if we should auto-restart on crash
    if [ $EXIT_CODE -ne 0 ]; then
        echo "[entrypoint] NVR crashed (exit code $EXIT_CODE), restarting in 5 seconds..."
        sleep 5
        continue
    fi

    # Clean exit - stop the loop
    echo "[entrypoint] Clean shutdown"
    break
done

exit $EXIT_CODE
