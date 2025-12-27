# SpatialNVR

A modern, plugin-based Network Video Recorder system with AI detection and spatial tracking.

## Features

- **Modern UI** - Responsive web interface built with React
- **AI Detection** - Object detection with support for GPU/NPU acceleration
- **Spatial Tracking** - Track objects across multiple cameras
- **Plugin Architecture** - Extensible system for camera integrations
- **go2rtc Integration** - WebRTC, RTSP, and HLS streaming
- **Standalone Deployment** - Single Docker container for easy setup

## Docker Images

| Tag | Architecture | Description |
|-----|-------------|-------------|
| `latest` | amd64 | **Recommended.** Full compatibility, supports all plugins including Wyze |
| `arm64` | arm64 | Native ARM performance for Raspberry Pi and ARM servers |
| `vX.X.X` | amd64 | Versioned release (amd64) |
| `vX.X.X-arm64` | arm64 | Versioned release (arm64) |

### Which image should I use?

- **Most users**: Use `latest` (amd64). Works everywhere including Apple Silicon Macs via Rosetta 2
- **ARM devices without Wyze**: Use `arm64` for native performance on Raspberry Pi, AWS Graviton, etc.
- **Wyze camera users**: Must use `latest` (amd64) - the Wyze plugin requires the TUTK library which only supports x86_64

## Quick Start with Docker

```bash
# Pull and run
docker run -d \
  --name spatialnvr \
  -p 12000:12000 \
  -p 12010:12010 \
  -p 12011:12011 \
  -p 12012:12012/udp \
  -v ~/spatialnvr/config:/config \
  -v ~/spatialnvr/data:/data \
  -e TZ=America/New_York \
  ghcr.io/spatial-nvr/spatialnvr:latest

# Access the web UI
open http://localhost:12000
```

### Docker Compose

```yaml
services:
  spatialnvr:
    image: ghcr.io/spatial-nvr/spatialnvr:latest
    container_name: spatialnvr
    restart: unless-stopped
    ports:
      - "12000:12000"   # Web UI and API
      - "12010:12010"   # go2rtc API
      - "12011:12011"   # RTSP streams
      - "12012:12012/udp"  # WebRTC
    volumes:
      - ./config:/config
      - ./data:/data
    environment:
      - TZ=America/New_York
```

Save as `docker-compose.yml` and run:

```bash
docker compose up -d
```

## Ports

| Port | Protocol | Description |
|------|----------|-------------|
| 12000 | TCP | Web UI and REST API |
| 12010 | TCP | go2rtc API |
| 12011 | TCP | RTSP stream output |
| 12012 | UDP | WebRTC |

## Volumes

| Path | Description |
|------|-------------|
| `/config` | Configuration files (config.yaml) |
| `/data` | Database, recordings, thumbnails, snapshots |

## Configuration

On first run, a default `config.yaml` is created in `/config`. You can manage everything through the web UI at `http://localhost:12000`.

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TZ` | `UTC` | Timezone |
| `LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `DEPLOYMENT_TYPE` | `standalone` | Deployment type |

## Resource Usage

Idle system (no active streams):
- **Memory**: ~40 MB
- **CPU**: < 1%
- **Image Size**: ~176 MB

## Architecture

```
+--------------------------------------------------+
|                 Web UI (React)                    |
+--------------------------------------------------+
|              REST API (Go/Chi)                    |
+--------------------------------------------------+
|  Streaming  |  Recording  |  Detection  | Events |
|  (go2rtc)   |  (FFmpeg)   |  (Embedded) |        |
+--------------------------------------------------+
|              Plugin System (SDK)                  |
+--------------------------------------------------+
|         SQLite Database  |  File Storage          |
+--------------------------------------------------+
```

## Plugins

Built-in core plugins:
- **nvr-streaming** - go2rtc video streaming
- **nvr-recording** - Continuous and event recording
- **nvr-detection** - Object detection service
- **nvr-spatial-tracking** - Cross-camera object tracking
- **nvr-core-api** - Camera management API
- **nvr-core-config** - Configuration management
- **nvr-core-events** - Event bus and notifications

External plugins (separate repos):
- [reolink-plugin](https://github.com/Spatial-NVR/reolink-plugin) - Reolink camera support
- [wyze-plugin](https://github.com/Spatial-NVR/wyze-plugin) - Wyze camera support

## Development

### Prerequisites

- Go 1.24+
- Node.js 20+
- Docker (for building)

### Building

```bash
# Clone
git clone https://github.com/Spatial-NVR/SpatialNVR.git
cd SpatialNVR

# Build Go backend
go build -o nvr ./cmd/nvr

# Build web UI
cd web-ui && npm install && npm run build && cd ..

# Build Docker image
docker build -t spatialnvr:local .
```

### Running Locally

```bash
# Start the server
./nvr

# Access at http://localhost:12000
```

## Scaling

The system runs all services in-process by default. For larger deployments with many cameras, detection and spatial tracking can be offloaded to external servers.

See [docs/SCALING.md](docs/SCALING.md) for details.

## API

REST API available at `http://localhost:12000/api/v1/`

Key endpoints:
- `GET /api/v1/cameras` - List cameras
- `POST /api/v1/cameras` - Add camera
- `GET /api/v1/system/health` - System health
- `GET /api/v1/plugins` - Plugin status

## License

MIT License - see [LICENSE](LICENSE) for details.
