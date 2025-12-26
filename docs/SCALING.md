# Scaling Detection and Spatial Tracking Services

This document explains when and how to scale the detection and spatial tracking services horizontally. The NVR system is designed to run these services in-process by default, with the option to offload them to separate machines when performance demands exceed what a single system can provide.

Do not use emojis in this documentation or any related configuration files.

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [When to Scale](#when-to-scale)
3. [Detection Service Scaling](#detection-service-scaling)
4. [Spatial Tracking Scaling](#spatial-tracking-scaling)
5. [Hardware Recommendations](#hardware-recommendations)
6. [Monitoring and Troubleshooting](#monitoring-and-troubleshooting)

---

## Architecture Overview

### Default Configuration (Single Node)

By default, both services run in-process within the main NVR container:

```
+--------------------------------------------------+
|                   NVR Container                   |
|                                                  |
|  +------------+  +-------------+  +------------+ |
|  | Streaming  |  | Detection   |  | Spatial    | |
|  | Service    |  | Service     |  | Tracking   | |
|  | (go2rtc)   |  | (embedded)  |  | (embedded) | |
|  +------------+  +-------------+  +------------+ |
|        |               |               |         |
|        v               v               v         |
|  +------------------------------------------+   |
|  |              Event Bus (NATS)            |   |
|  +------------------------------------------+   |
+--------------------------------------------------+
```

### Scaled Configuration (Multi-Node)

When scaling, services can be offloaded to dedicated machines:

```
+-------------------+     +----------------------+
|   NVR Container   |     | Detection Worker(s)  |
|                   |     |                      |
| +---------------+ |     | +------------------+ |
| | Streaming     | | --> | | Detection Server | |
| | Service       | |     | | (GPU/NPU)        | |
| +---------------+ |     | +------------------+ |
|                   |     +----------------------+
| +---------------+ |
| | Event Bus     | |     +----------------------+
| | (NATS)        | | --> | Spatial Worker(s)    |
| +---------------+ |     |                      |
+-------------------+     | +------------------+ |
                          | | Spatial Tracking | |
                          | | Server           | |
                          | +------------------+ |
                          +----------------------+
```

---

## When to Scale

### Detection Service

Scale the detection service when you observe ANY of the following:

| Indicator | Threshold | How to Check |
|-----------|-----------|--------------|
| Detection latency | > 200ms average | Health page or `/api/v1/plugins/nvr-detection/status` |
| Frame drop rate | > 10% | Backend logs showing "frame skipped" messages |
| CPU usage | > 80% sustained | Health page system metrics |
| GPU/NPU usage | > 90% sustained | Health page or `nvidia-smi` / system monitor |
| Camera count | > 8 cameras at 5 FPS | Configuration review |
| Detection queue | > 50 frames backlog | `/api/v1/plugins/nvr-detection/status` queue_size |

#### Quick Assessment

Run this command to check detection performance:

```bash
curl -s http://localhost:12000/api/v1/plugins/nvr-detection/status | jq '{
  queue_size: .queue_size,
  avg_latency_ms: .avg_latency_ms,
  error_rate: (.error_count / (.processed_count + 1) * 100)
}'
```

If `avg_latency_ms` exceeds 200 or `queue_size` exceeds 50, consider scaling.

### Spatial Tracking Service

Scale the spatial tracking service when you observe ANY of the following:

| Indicator | Threshold | How to Check |
|-----------|-----------|--------------|
| Track processing latency | > 100ms | `/api/v1/spatial/stats` |
| Active tracks | > 500 concurrent | `/api/v1/spatial/stats` |
| Re-ID matching time | > 50ms | Backend logs |
| Memory usage growth | > 100MB/hour | System monitoring |
| Cross-camera handoff failures | > 5% | Event logs |

#### Quick Assessment

```bash
curl -s http://localhost:12000/api/v1/spatial/stats | jq '{
  active_tracks: .active_tracks,
  avg_processing_ms: .avg_processing_ms,
  memory_mb: .memory_usage_mb
}'
```

---

## Detection Service Scaling

### Option 1: GPU Offload (Same Machine)

If your machine has a GPU but detection is running on CPU, enable GPU inference:

1. Install GPU dependencies:

```bash
# NVIDIA GPU
apt-get install nvidia-cuda-toolkit
pip install onnxruntime-gpu

# Apple Silicon (automatic via CoreML)
# No additional installation needed

# AMD GPU
pip install onnxruntime-rocm
```

2. Update detection configuration:

```yaml
# data/config.yaml
detection:
  backend: cuda  # or 'coreml' for Apple, 'rocm' for AMD
  device_id: 0
```

3. Restart the NVR:

```bash
docker-compose restart nvr
```

### Option 2: External Detection Server (Separate Machine)

For maximum performance, run detection on a dedicated GPU machine.

#### Step 1: Deploy Detection Server

On the GPU machine, run the detection server container:

```bash
docker run -d \
  --name nvr-detection \
  --gpus all \
  -p 50051:50051 \
  -v /path/to/models:/models \
  nvr/detection-server:latest \
  --models-path=/models \
  --backend=cuda \
  --device-id=0
```

Or with docker-compose:

```yaml
# docker-compose.detection.yaml
version: '3.8'
services:
  detection:
    image: nvr/detection-server:latest
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu]
    ports:
      - "50051:50051"
    volumes:
      - ./models:/models
    environment:
      - DETECTION_BACKEND=cuda
      - DETECTION_DEVICE_ID=0
      - LOG_LEVEL=info
```

#### Step 2: Configure NVR to Use External Server

Update the NVR configuration to point to the external detection server:

```yaml
# data/config.yaml
detection:
  # Point to external detection server
  address: "gpu-server.local:50051"

  # Disable embedded server
  embedded: false

  # Connection settings
  timeout_seconds: 30
  max_retries: 3
  retry_delay_ms: 1000
```

Or via environment variables:

```bash
DETECTION_ADDR=gpu-server.local:50051 \
DETECTION_EMBEDDED=false \
docker-compose up -d nvr
```

#### Step 3: Verify Connection

```bash
# Check detection service status
curl -s http://localhost:12000/api/v1/plugins/nvr-detection/status

# Expected output should show:
# "connected": true
# "backend": "cuda" (or your configured backend)
```

### Option 3: Multiple Detection Workers (Load Balanced)

For very high camera counts (50+ cameras), deploy multiple detection workers:

```yaml
# docker-compose.detection-cluster.yaml
version: '3.8'
services:
  detection-lb:
    image: haproxy:latest
    ports:
      - "50051:50051"
    volumes:
      - ./haproxy.cfg:/usr/local/etc/haproxy/haproxy.cfg

  detection-1:
    image: nvr/detection-server:latest
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              device_ids: ['0']
              capabilities: [gpu]

  detection-2:
    image: nvr/detection-server:latest
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              device_ids: ['1']
              capabilities: [gpu]
```

HAProxy configuration for load balancing:

```
# haproxy.cfg
frontend detection_frontend
    bind *:50051
    default_backend detection_backend

backend detection_backend
    balance roundrobin
    option httpchk GET /health
    server detection1 detection-1:50051 check
    server detection2 detection-2:50051 check
```

---

## Spatial Tracking Scaling

### Option 1: Increase Resources (Same Machine)

Spatial tracking is CPU and memory intensive. Before offloading, try:

1. Increase memory allocation:

```yaml
# docker-compose.yaml
services:
  nvr:
    deploy:
      resources:
        limits:
          memory: 8G
```

2. Tune tracking parameters:

```yaml
# data/config.yaml
spatial:
  # Reduce memory usage
  max_tracks: 1000
  track_ttl_seconds: 180  # Shorter TTL

  # Reduce CPU usage
  reid_enabled: false  # Disable re-identification if not needed
  matching_interval_ms: 500  # Less frequent matching
```

### Option 2: External Spatial Server

For large deployments with many cameras and complex floor plans:

#### Step 1: Deploy Spatial Server

```bash
docker run -d \
  --name nvr-spatial \
  -p 12020:12020 \
  -v /path/to/floor-plans:/floor-plans \
  nvr/spatial-server:latest \
  --nats-url=nats://nvr-host:4222 \
  --floor-plans=/floor-plans
```

#### Step 2: Configure NVR

```yaml
# data/config.yaml
spatial:
  # Point to external spatial server
  address: "spatial-server.local:12020"

  # Disable embedded tracking
  embedded: false
```

#### Step 3: Verify Connection

```bash
curl -s http://localhost:12000/api/v1/spatial/stats
```

### Option 3: Sharded Spatial Tracking

For very large deployments, shard by floor or building:

```yaml
# Floor 1 spatial server
docker run -d \
  --name nvr-spatial-floor1 \
  -e SPATIAL_SHARD=floor1 \
  -e SPATIAL_CAMERAS=cam1,cam2,cam3,cam4 \
  nvr/spatial-server:latest

# Floor 2 spatial server
docker run -d \
  --name nvr-spatial-floor2 \
  -e SPATIAL_SHARD=floor2 \
  -e SPATIAL_CAMERAS=cam5,cam6,cam7,cam8 \
  nvr/spatial-server:latest
```

---

## Hardware Recommendations

### Detection Service

| Deployment Size | Cameras | Recommended Hardware |
|-----------------|---------|---------------------|
| Small | 1-4 | CPU only (embedded) |
| Medium | 5-16 | Single GPU (RTX 3060 or better) |
| Large | 17-50 | Dual GPU or dedicated detection server |
| Enterprise | 50+ | Multiple detection workers with load balancing |

GPU recommendations by brand:

| Brand | Model | Cameras Supported | Notes |
|-------|-------|-------------------|-------|
| NVIDIA | RTX 3060 | 8-12 | Good price/performance |
| NVIDIA | RTX 4080 | 20-30 | High throughput |
| NVIDIA | A4000 | 15-25 | Datacenter, no display needed |
| Apple | M1 Pro | 6-10 | Uses Neural Engine |
| Apple | M2 Max | 12-20 | Best Apple Silicon option |
| Intel | Arc A770 | 8-12 | OpenVINO backend |

### Spatial Tracking Service

| Deployment Size | Active Tracks | Recommended Hardware |
|-----------------|---------------|---------------------|
| Small | < 100 | Embedded (2GB RAM) |
| Medium | 100-500 | Embedded (4GB RAM) |
| Large | 500-2000 | Dedicated server (8GB RAM, 4 cores) |
| Enterprise | 2000+ | Sharded deployment |

---

## Monitoring and Troubleshooting

### Health Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /health` | Overall system health |
| `GET /api/v1/system/health` | Detailed service status |
| `GET /api/v1/plugins/nvr-detection/status` | Detection service metrics |
| `GET /api/v1/spatial/stats` | Spatial tracking metrics |

### Key Metrics to Monitor

Detection service:

```bash
# Monitor detection performance
watch -n 5 'curl -s http://localhost:12000/api/v1/plugins/nvr-detection/status | jq "{
  connected,
  queue_size,
  processed_count,
  error_count,
  avg_latency_ms
}"'
```

Spatial tracking:

```bash
# Monitor spatial tracking
watch -n 5 'curl -s http://localhost:12000/api/v1/spatial/stats | jq "{
  active_tracks,
  total_tracks,
  avg_processing_ms,
  handoffs_completed
}"'
```

### Common Issues

#### Detection Queue Growing

Symptom: `queue_size` continuously increasing

Solutions:
1. Reduce detection FPS per camera
2. Add GPU acceleration
3. Scale to external detection server

```yaml
# Reduce FPS to decrease load
cameras:
  - id: front_door
    detection:
      fps: 2  # Reduced from 5
```

#### High Detection Latency

Symptom: `avg_latency_ms` > 500ms

Solutions:
1. Use smaller detection model
2. Enable GPU/NPU acceleration
3. Reduce image resolution for detection

```yaml
detection:
  model: yolov8n  # Nano model, faster
  input_size: 320  # Smaller input, faster processing
```

#### Spatial Tracking Memory Growth

Symptom: Memory usage growing over time

Solutions:
1. Reduce track TTL
2. Limit maximum active tracks
3. Disable re-identification if not needed

```yaml
spatial:
  track_ttl_seconds: 120
  max_tracks: 500
  reid_enabled: false
```

#### Connection Failures to External Services

Symptom: Services showing as disconnected

Checklist:
1. Verify network connectivity: `ping detection-server.local`
2. Check port accessibility: `nc -zv detection-server.local 50051`
3. Verify service is running: `curl http://detection-server.local:50051/health`
4. Check firewall rules
5. Verify NATS connectivity for event bus

---

## Configuration Reference

### Detection Service Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DETECTION_ADDR` | `localhost:12021` | Detection server address |
| `DETECTION_EMBEDDED` | `true` | Use embedded detection server |
| `DETECTION_BACKEND` | `cpu` | Inference backend (cpu, cuda, coreml, rocm) |
| `DETECTION_DEVICE_ID` | `0` | GPU device index |
| `DETECTION_DEFAULT_FPS` | `5` | Default detection FPS |
| `DETECTION_MIN_CONFIDENCE` | `0.5` | Minimum detection confidence |
| `DETECTION_MODELS_PATH` | `/data/models` | Path to detection models |

### Spatial Tracking Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SPATIAL_ADDR` | `localhost:12020` | Spatial server address |
| `SPATIAL_EMBEDDED` | `true` | Use embedded spatial tracking |
| `SPATIAL_REID_ENABLED` | `true` | Enable re-identification |
| `SPATIAL_MAX_GAP_SECONDS` | `30` | Max gap for track continuity |
| `SPATIAL_TRACK_TTL_SECONDS` | `300` | Track time-to-live |
| `SPATIAL_MAX_TRACKS` | `10000` | Maximum concurrent tracks |

---

## Quick Start Examples

### Example 1: Add GPU to Existing Installation

```bash
# 1. Stop NVR
docker-compose down

# 2. Update docker-compose.yaml to include GPU
# Add under nvr service:
#   deploy:
#     resources:
#       reservations:
#         devices:
#           - driver: nvidia
#             count: 1
#             capabilities: [gpu]

# 3. Update config
echo "detection:
  backend: cuda
  device_id: 0" >> data/config.yaml

# 4. Restart
docker-compose up -d
```

### Example 2: Offload Detection to Separate Machine

```bash
# On GPU machine (192.168.1.100):
docker run -d --gpus all -p 50051:50051 nvr/detection-server:latest

# On NVR machine:
export DETECTION_ADDR=192.168.1.100:50051
export DETECTION_EMBEDDED=false
docker-compose up -d
```

### Example 3: Monitor Before and After Scaling

```bash
# Before scaling - capture baseline
curl -s http://localhost:12000/api/v1/plugins/nvr-detection/status > before.json

# After scaling - compare
curl -s http://localhost:12000/api/v1/plugins/nvr-detection/status > after.json

# Compare latency improvement
jq -s '.[0].avg_latency_ms as $before | .[1].avg_latency_ms as $after |
  "Latency improved from \($before)ms to \($after)ms (\((($before - $after) / $before * 100) | floor)% reduction)"' \
  before.json after.json
```
