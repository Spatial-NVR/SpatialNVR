# NVR System - Standalone Dockerfile
# Multi-stage build for optimal image size

# =============================================================================
# Stage 1: Build the Go backend
# =============================================================================
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Install build dependencies including GCC for CGO (required by go-sqlite3)
RUN apk add --no-cache git ca-certificates tzdata gcc musl-dev

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
# Stage 3: Final runtime image
# =============================================================================
FROM alpine:3.20

# Labels
LABEL org.opencontainers.image.title="NVR System"
LABEL org.opencontainers.image.description="Network Video Recorder with AI detection"
LABEL org.opencontainers.image.source="https://github.com/nvr-system/nvr"

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    curl \
    git \
    # FFmpeg with all codecs for stream processing
    ffmpeg \
    # Intel Quick Sync Video support (x86_64 only)
    # intel-media-driver \
    # libva-intel-driver \
    # Mesa for VA-API
    # mesa-va-gallium \
    # Required for hardware decoding
    # libva \
    # libva-utils \
    # Additional utilities
    bash \
    && rm -rf /var/cache/apk/*

# Create non-root user
RUN addgroup -g 1000 nvr && \
    adduser -u 1000 -G nvr -s /bin/sh -D nvr

# Create directories
RUN mkdir -p /app /app/bin /app/web /config /data /data/recordings /data/thumbnails /data/snapshots /data/exports \
    && chown -R nvr:nvr /app /config /data

WORKDIR /app

# Copy go2rtc binary (download at build time)
ARG GO2RTC_VERSION=1.9.13
ARG TARGETARCH=amd64
ADD --chmod=755 https://github.com/AlexxIT/go2rtc/releases/download/v${GO2RTC_VERSION}/go2rtc_linux_${TARGETARCH} /app/bin/go2rtc

# Copy the backend binary
COPY --from=builder /build/nvr /app/nvr

# Copy the web UI
COPY --from=ui-builder /build/dist /app/web

# Copy default config template
COPY config/config.example.yaml /app/config.example.yaml

# Set ownership
RUN chown -R nvr:nvr /app

# Switch to non-root user
USER nvr

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
    # go2rtc binary path
    GO2RTC_PATH=/app/bin/go2rtc

# Expose ports
# 12000: Main API and Web UI
# 12010: go2rtc API
# 12011: RTSP streams
# 12012: WebRTC (TCP and UDP)
EXPOSE 12000 12010 12011 12012/tcp 12012/udp

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:12000/health || exit 1

# Run the application
ENTRYPOINT ["/app/nvr"]
