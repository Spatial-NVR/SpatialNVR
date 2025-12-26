# Scaling Quick Reference

Do not use emojis in documentation.

## Do I Need to Scale?

Run this diagnostic command:

```bash
curl -s http://localhost:12000/api/v1/system/health && \
curl -s http://localhost:12000/api/v1/plugins/nvr-detection/status | jq '{
  detection_queue: .queue_size,
  detection_latency_ms: .avg_latency_ms,
  detection_errors: .error_count
}' && \
curl -s http://localhost:12000/api/v1/spatial/stats | jq '{
  active_tracks: .active_tracks,
  spatial_latency_ms: .avg_processing_ms
}'
```

## Decision Matrix

| Your Situation | Action |
|----------------|--------|
| Everything running, latency < 200ms | No scaling needed |
| Detection latency > 200ms, have GPU | Enable GPU acceleration |
| Detection latency > 200ms, no GPU | Add GPU or offload to GPU server |
| Detection queue > 50 frames | Reduce FPS or add detection workers |
| Spatial tracking > 500 tracks | Increase memory or offload |
| More than 16 cameras | Consider dedicated detection server |
| More than 50 cameras | Deploy detection worker cluster |

## Quick Fixes (No Hardware Changes)

### Reduce Detection Load

```yaml
# data/config.yaml - reduce FPS
cameras:
  - id: your_camera
    detection:
      fps: 2  # Down from 5
      min_confidence: 0.6  # Fewer false positives
```

### Reduce Spatial Load

```yaml
# data/config.yaml
spatial:
  track_ttl_seconds: 120  # Down from 300
  reid_enabled: false  # Disable if not needed
  max_tracks: 500
```

## Enable GPU (Same Machine)

### NVIDIA GPU

```bash
# 1. Add to docker-compose.yaml under nvr service:
deploy:
  resources:
    reservations:
      devices:
        - driver: nvidia
          count: 1
          capabilities: [gpu]

# 2. Set environment:
DETECTION_BACKEND=cuda

# 3. Restart
docker-compose up -d
```

### Apple Silicon

```bash
# Automatic - just set:
DETECTION_BACKEND=coreml
```

## Offload to Separate Machine

### Detection Server (on GPU machine)

```bash
# On GPU machine (e.g., 192.168.1.100)
docker run -d --gpus all -p 50051:50051 nvr/detection-server:latest
```

### Point NVR to External Server

```bash
# On NVR machine
export DETECTION_ADDR=192.168.1.100:50051
export DETECTION_EMBEDDED=false
docker-compose up -d
```

## Verify After Scaling

```bash
# Check detection is connected and faster
curl -s http://localhost:12000/api/v1/plugins/nvr-detection/status | jq '{
  connected,
  backend,
  avg_latency_ms,
  queue_size
}'

# Expected: connected=true, lower latency, queue_size near 0
```

## Common Port Reference

| Service | Default Port | Purpose |
|---------|--------------|---------|
| API | 12000 | Main NVR API |
| NATS | 4222 | Event bus |
| go2rtc API | 1984 | Streaming API |
| go2rtc RTSP | 8554 | RTSP streams |
| go2rtc WebRTC | 8555 | WebRTC |
| Spatial | 12020 | Spatial tracking |
| Detection | 12021 | Detection service |

## Troubleshooting One-Liners

```bash
# Is detection connected?
curl -s http://localhost:12000/api/v1/plugins | jq '.[] | select(.id=="nvr-detection") | {state, health}'

# What backend is detection using?
curl -s http://localhost:50051/backends

# Is the external detection server reachable?
nc -zv detection-server.local 50051

# Check all service health at once
curl -s http://localhost:12000/api/v1/system/health | jq .
```

## See Also

- [Full Scaling Documentation](./SCALING.md) - Complete guide with all options
- [Hardware Recommendations](./SCALING.md#hardware-recommendations) - What to buy
- [Monitoring Guide](./SCALING.md#monitoring-and-troubleshooting) - Ongoing monitoring
