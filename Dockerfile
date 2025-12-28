# SpatialNVR - Standalone Dockerfile
# Multi-stage build for optimal image size
#
# Architecture Strategy:
# - amd64 (latest): Default image, full plugin compatibility including Wyze
# - arm64: Native ARM performance for Raspberry Pi, AWS Graviton, etc.
#
# The Wyze plugin requires the TUTK library which only provides Linux x86_64 binaries.
# Users who need Wyze support must use the amd64 image (works on Apple Silicon via Rosetta 2).
# Users on ARM devices who don't need Wyze can use the arm64 image for better performance.
#
# Self-Updating Architecture:
# - /app/bin/nvr: Main NVR binary (updatable via /data/bin/nvr)
# - /app/web: Web UI assets (updatable via /data/web)
# - /data/plugins: External plugins (always updatable)
# - /data/updates: Downloaded updates staging area
#
# The container ships with initial versions, but the system checks GitHub releases
# and can update components without rebuilding the container.

# =============================================================================
# Stage 0: Extract TUTK library from docker-wyze-bridge (x86_64 only)
# =============================================================================
FROM --platform=linux/amd64 mrlt8/wyze-bridge:latest AS wyze-libs

# =============================================================================
# Stage 1: Build the Go backend (using Debian for glibc compatibility)
# =============================================================================
FROM golang:1.24-bookworm AS builder

WORKDIR /build

# Install build dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    git ca-certificates tzdata gcc libc6-dev \
    && rm -rf /var/lib/apt/lists/*

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary with CGO enabled for SQLite support
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o nvr ./cmd/nvr

# =============================================================================
# Stage 2: Build the web UI
# =============================================================================
FROM node:20-alpine AS ui-builder

WORKDIR /build

# Copy package files
COPY web-ui/package*.json ./

# Install dependencies
RUN npm ci

# Copy source
COPY web-ui/ .

# Build for production - API is on same host in Docker
ENV VITE_API_URL=""
RUN npm run build

# =============================================================================
# Stage 3: Final runtime image (Debian for full glibc support)
# =============================================================================
FROM debian:bookworm-slim

# Labels
LABEL org.opencontainers.image.title="NVR System"
LABEL org.opencontainers.image.description="Network Video Recorder with AI detection"
LABEL org.opencontainers.image.source="https://github.com/nvr-system/nvr"

# Install runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    tzdata \
    curl \
    git \
    # FFmpeg with all codecs for stream processing
    ffmpeg \
    # Python for wyze-bridge and other plugin dependencies
    python3 \
    python3-pip \
    python3-venv \
    # Additional utilities
    bash \
    procps \
    # passwd for usermod/groupmod (PUID/PGID support)
    passwd \
    # gosu for dropping privileges securely
    gosu \
    && rm -rf /var/lib/apt/lists/* \
    # Configure pip
    && mkdir -p /etc/pip.conf.d \
    && echo "[global]" > /etc/pip.conf \
    && echo "break-system-packages = true" >> /etc/pip.conf

# Create non-root user
RUN groupadd -g 1000 nvr && \
    useradd -u 1000 -g nvr -s /bin/bash -m nvr

# Create directories
# /app contains the shipped versions (read-only after build)
# /data/bin and /data/web can contain updated versions (writable, checked first at runtime)
# /tokens and /img are required by wyze-bridge plugin
RUN mkdir -p /app /app/bin /app/web \
    /config \
    /data /data/bin /data/web /data/plugins /data/updates \
    /data/recordings /data/thumbnails /data/snapshots /data/exports \
    /tokens /img \
    && chown -R nvr:nvr /app /config /data /tokens /img

WORKDIR /app

# Copy go2rtc binary (download at build time)
ARG GO2RTC_VERSION=1.9.13
ARG TARGETARCH=amd64
ADD --chmod=755 https://github.com/AlexxIT/go2rtc/releases/download/v${GO2RTC_VERSION}/go2rtc_linux_${TARGETARCH} /app/bin/go2rtc

# Download MediaMTX for wyze-bridge plugin (required for Wyze camera support)
ARG MTX_VERSION=1.9.1
RUN MTX_ARCH=$(case ${TARGETARCH} in arm64) echo "arm64v8" ;; armv7) echo "armv7" ;; *) echo "amd64" ;; esac) && \
    curl -SL https://github.com/bluenviron/mediamtx/releases/download/v${MTX_VERSION}/mediamtx_v${MTX_VERSION}_linux_${MTX_ARCH}.tar.gz \
    | tar -xzf - -C /app mediamtx mediamtx.yml && \
    chmod +x /app/mediamtx

# Copy TUTK library from wyze-bridge for Wyze P2P connections (x86_64 only)
# This is a proprietary library required for direct camera connections
COPY --from=wyze-libs /usr/local/lib/libIOTCAPIs_ALL.so /usr/local/lib/libIOTCAPIs_ALL.so
RUN ldconfig

# Copy the backend binary to /app/bin (new location for self-updating)
COPY --from=builder /build/nvr /app/bin/nvr

# Copy the web UI
COPY --from=ui-builder /build/dist /app/web

# Copy default config template
COPY config/config.example.yaml /app/config.example.yaml

# Copy the entrypoint script
COPY docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh

# Set ownership
RUN chown -R nvr:nvr /app

# NOTE: We do NOT switch to nvr user here anymore.
# The entrypoint script handles PUID/PGID and drops privileges.
# This allows dynamic user ID mapping for systems like Unraid.

# Environment variables
ENV DEPLOYMENT_TYPE=standalone \
    CONFIG_PATH=/config/config.yaml \
    DATA_PATH=/data \
    WEB_PATH=/app/web \
    LOG_LEVEL=info \
    TZ=UTC \
    # FFmpeg settings
    FFMPEG_BIN=/usr/bin/ffmpeg \
    FFPROBE_BIN=/usr/bin/ffprobe \
    # go2rtc binary path (entrypoint may override)
    GO2RTC_PATH=/app/bin/go2rtc

# Expose ports
# 8080: Main API and Web UI (standard web port)
# 1984: go2rtc API (internal, for streaming control)
# 8554: RTSP streams
# 8555: WebRTC (TCP and UDP)
EXPOSE 8080 1984 8554 8555/tcp 8555/udp

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# Use entrypoint script to check for updates before starting
ENTRYPOINT ["/app/docker-entrypoint.sh"]
