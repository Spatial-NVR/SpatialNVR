# CLAUDE CODE INSTRUCTIONS: Next-Generation NVR System

**Project Name:** Modern NVR System  
**Target:** Production-ready, plugin-based Network Video Recorder  
**Start Date:** December 24, 2024  
**Documentation Date:** December 23, 2024

---

## 🎯 EXECUTIVE SUMMARY

Build a modern, microservices-capable NVR system that combines the best of existing solutions while fixing their limitations:

### What We're Building
- **Scrypted's polish** - Beautiful, modern UI with smooth UX
- **Frigate 0.17+ AI capabilities** - Advanced object/face/LPR detection with latest models
- **Plugin architecture** - Extensible from day one with built-in Wyze, Reolink, ONVIF support
- **API-first design** - Every feature accessible via REST API for mobile apps
- **UI-driven configuration** - Zero manual YAML editing required (but YAML exists for advanced users)
- **Flexible deployment** - Monolithic for home users, microservices for scale

### Key Architectural Principles

**1. Configuration Philosophy**
- **UI is the primary interface** - Everything configurable through web UI
- **YAML is auto-generated** - System writes config.yaml based on UI changes
- **Advanced users can edit YAML** - But it's hidden in settings by default
- **Hot-reload everything** - Config changes apply instantly without restarts
- **No database for config** - Config lives in YAML, runtime state in SQLite

**2. Deployment Flexibility**
- **Default: Monolithic** - Single Docker container for 1-20 cameras
  - All services run as goroutines/threads in one binary
  - Uses Go channels for inter-service communication
  - SQLite database
  - 2-4GB RAM requirement
  - Perfect for home users (99% of users)
  
- **Option: Distributed** - Multiple containers for scaling
  - Each service in separate container
  - NATS for message passing
  - Still uses SQLite (shared volume) or PostgreSQL
  - Scale AI services independently
  - 10-50 cameras
  
- **Option: Kubernetes** - Enterprise deployments
  - Full horizontal scaling
  - PostgreSQL recommended
  - Multi-site support
  - 50+ cameras

**3. Plugin System - First Class Citizen**
Plugins are NOT an afterthought. They are core to the architecture:
- **Built-in plugins ship with the system**: Wyze Bridge, Reolink, ONVIF Generic
- **Plugin SDK** in both TypeScript and Python
- **Plugins can provide**:
  - Camera discovery (auto-find cameras on network)
  - Stream adapters (convert proprietary protocols to standard RTSP/RTMP)
  - Custom AI detectors
  - Storage backends
  - Notification channels
  - Integrations (Home Assistant, HomeKit, MQTT, etc.)
- **Hot-reload plugins** - Install/update without restart
- **UI auto-generates config forms** - Plugins define config-schema.json

**4. Why These Technology Choices**

**go2rtc** (NOT building custom):
- Already battle-tested in Frigate and Home Assistant
- Supports WebRTC, RTSP, RTMP, HLS, MSE out of the box
- Hardware acceleration built-in
- Two-way audio support
- Active development
- Zero-latency streaming (~100ms)
- Building custom would take months and be inferior

**SQLite as Default** (NOT PostgreSQL):
- 99% of users have <20 cameras
- SQLite handles millions of events easily
- Single file = easy backups
- No separate database service to manage
- JSONB support since 3.38.0
- Full-text search built-in
- PostgreSQL only for 50+ camera deployments

**Monolithic First** (NOT microservices first):
- Simpler deployment (docker run)
- Lower resource usage (1GB vs 4GB+)
- Easier debugging
- No network latency between services
- Can migrate to distributed later
- Frigate and Home Assistant prove this works

**YOLOv12** (latest, Feb 2025):
- Most accurate YOLO variant
- Attention-centric architecture
- Falls back to YOLO11 if speed more important
- Compatible with Ultralytics framework

### What Makes This Different from Frigate/Scrypted

**vs Frigate:**
- ✅ Modern, polished UI (Frigate's is functional but dated)
- ✅ Plugin system (Frigate is monolithic)
- ✅ UI-driven config (Frigate requires YAML editing)
- ✅ State recognition (trash out, water detected, etc.)
- ✅ Custom object training in UI
- ✅ Facial recognition built-in
- ✅ LPR built-in

**vs Scrypted:**
- ✅ Open source and self-hosted (Scrypted has paid cloud features)
- ✅ Built-in AI detection (Scrypted requires plugins for everything)
- ✅ Integrated NVR (Scrypted is more of a camera proxy)
- ✅ Better timeline (smooth scrubbing, event markers)
- ✅ Modern tech stack (Scrypted uses older JS)

---

## 🔍 TECHNOLOGY STACK - RESEARCH REQUIRED

### CRITICAL: Search Before Implementation

**Before writing ANY code**, search the web for the LATEST versions of all technologies. We researched these on December 23, 2024, but you must verify current versions.

### Confirmed Technology Choices (Research Latest Versions)

#### Video Streaming: go2rtc
**Why:** It's the industry standard, battle-tested solution
- **Research:** https://github.com/AlexxIT/go2rtc/releases
- **Last Known Version:** v1.9.7+ (Dec 2024)
- **Alternatives Considered:** Building custom (rejected - too complex)
- **Key Features:**
  - WebRTC, RTSP, RTMP, HLS, MSE support
  - Hardware acceleration (NVIDIA GPU, Intel QSV, Apple VideoToolbox)
  - Two-way audio
  - Zero-latency (~100ms)
  - Active development by AlexxIT
- **Integration Approach:**
  - Embed as subprocess in Go (monolithic mode)
  - Run as sidecar container (distributed mode)
  - Auto-generate go2rtc.yaml from camera configs
  - Manage lifecycle (start/stop/reload)
- **Used By:** Frigate, Home Assistant, thousands of installations

#### Object Detection: YOLOv12 (Primary) / YOLO11 (Fallback)
**Why:** Bleeding-edge accuracy with acceptable speed trade-offs
- **YOLOv12 Research:** https://github.com/sunsmarterjie/yolov12
  - **Released:** February 2025 (very recent!)
  - **Status:** NeurIPS 2025 paper
  - **Architecture:** Attention-centric with FlashAttention
  - **Performance:** Better accuracy than YOLO11, ~25% slower
  - **Use Case:** When accuracy is more important than speed
  - **Model Variants:** yolov12n, yolov12s, yolov12m, yolov12l, yolov12x
  - **Framework:** Built on Ultralytics, same API as YOLO11

- **YOLO11 Research:** https://github.com/ultralytics/ultralytics
  - **Released:** September 2024
  - **Use Case:** When speed is critical (real-time at 40 FPS)
  - **Model Variants:** yolo11n, yolo11s, yolo11m, yolo11l, yolo11x
  - **Framework:** Ultralytics official

- **Installation:**
  ```bash
  pip install ultralytics  # Gets both YOLO11 and YOLOv12 support
  ```

- **Configuration Strategy:**
  - Default to YOLOv12n (nano) for best accuracy/speed balance
  - Allow users to switch to YOLO11n via UI if they need more speed
  - Support custom models (user can upload their own .pt files)

#### Facial Recognition: InsightFace
**Why:** State-of-the-art accuracy, active development
- **Research:** https://github.com/deepinsight/insightface
- **Last Known Version:** v0.7.3 (Dec 2024)
- **Model Pack:** buffalo_l (best accuracy)
- **Key Features:**
  - Face detection + recognition
  - NIST FRVT #1 ranked
  - Age/gender estimation
  - Sub-2ms inference (InspireFace SDK)
  - Privacy-focused (all local)
- **Integration:**
  - Use buffalo_l model pack
  - ONNX runtime for inference
  - Store embeddings as BLOB in SQLite
  - Cosine similarity for matching
- **License:** MIT for code, models non-commercial only

#### License Plate Recognition: PaddleOCR
**Why:** Best open-source OCR, proven in LPR systems
- **Research:** https://github.com/PaddlePaddle/PaddleOCR
- **Last Known Version:** 2.8+ (Dec 2024)
- **Why PaddleOCR:**
  - Specifically designed for scene text (not documents)
  - Multi-language support (70+ languages)
  - Angle classification (handles rotated plates)
  - Production-proven in multiple LPR projects
  - CRNN + Attention mechanism
- **Integration:**
  - YOLOv12 for plate detection (bounding box)
  - PaddleOCR for text recognition
  - Pre-process: resize to 128x32, convert to grayscale
  - Post-process: validate against plate regex patterns
- **Alternatives Considered:**
  - EasyOCR - Good but slower
  - Tesseract - Poor for license plates

#### Backend Services Language Choices

**Go 1.23+ for:**
- Camera management (concurrent connections)
- API gateway (HTTP routing, middleware)
- Event management (fast processing)
- Storage management (file I/O)
- Auth service (security critical)
- **Why Go:** Excellent concurrency, fast, single binary, great stdlib

**Python 3.12+ for:**
- All AI/ML services (ecosystem support)
- Plugin runtime for Python plugins
- **Why Python:** PyTorch, TensorFlow, ONNX runtime, vast ML ecosystem

**Rust 1.75+ for:**
- Video processing (performance critical)
- Timeline service (needs to be FAST)
- **Why Rust:** Memory safety, zero-cost abstractions, FFmpeg bindings
- **Note:** Optional - can use Go with FFmpeg if Rust expertise unavailable

**TypeScript/Node.js 20+ for:**
- Plugin system (sandboxing, VM isolation)
- Web UI (React)
- **Why TypeScript:** Type safety, great tooling, npm ecosystem

#### Database: SQLite 3.45+ (Default) / PostgreSQL 16+ (Optional)

**SQLite Default Strategy:**
- **Why SQLite:**
  - Single file database
  - No separate service to manage
  - Handles millions of events easily
  - JSONB support (since v3.38.0)
  - Full-text search built-in
  - Vector similarity (for face embeddings)
  - WAL mode for concurrent access
  - 99% of users need this

- **SQLite Configuration:**
  ```sql
  PRAGMA journal_mode = WAL;
  PRAGMA synchronous = NORMAL;
  PRAGMA cache_size = -64000;  -- 64MB cache
  PRAGMA temp_store = MEMORY;
  PRAGMA mmap_size = 268435456; -- 256MB mmap
  ```

- **When to Use PostgreSQL:**
  - 50+ cameras
  - Multiple NVR instances (HA setup)
  - Enterprise requirements
  - Complex reporting needs

- **Database Abstraction:**
  - Use same Go interface for SQLite and PostgreSQL
  - Environment variable: `DATABASE_TYPE=sqlite|postgres`
  - Migration system works for both

#### Message Queue: NATS 2.10+ / Go Channels

**For Monolithic Mode:**
- Use Go channels directly
- Zero network overhead
- Simple, fast, reliable

**For Distributed Mode:**
- **NATS 2.10+** - Lightweight message broker
- **Why NATS:**
  - Extremely lightweight (<10MB memory)
  - Written in Go
  - Pub/sub and request/reply
  - At-most-once delivery (perfect for video events)
  - Easy clustering
- **Alternatives Considered:**
  - RabbitMQ - Too heavy
  - Kafka - Overkill for this use case
  - Redis Pub/Sub - Works but NATS is better

#### Web Frontend Stack

**Core:**
- **React 18.3+** with TypeScript
- **Vite 5+** for build tooling (fastest)
- **TanStack Query v5** (formerly React Query) for server state
- **Zustand 4.5+** for client state (simpler than Redux)

**UI/Styling:**
- **Tailwind CSS 3.4+** for styling
- **shadcn/ui** for components (built on Radix UI)
- **Framer Motion 11+** for animations
- **Lucide React** for icons

**Video Playback:**
- **HLS.js 1.5+** for HLS streams
- **Native video element** for MSE (Media Source Extensions)
- **Custom controls** with keyboard shortcuts

**Real-time:**
- **WebSocket** (native) for events stream
- **EventSource** for SSE (fallback)

#### Storage & Caching

**Primary Storage:**
- **Filesystem** - Video segments (10-second chunks)
- **SQLite** - Metadata, events, config state
- **S3/MinIO** - Optional for cold storage tier

**Caching:**
- **Redis 7.2+** - Optional for distributed mode
- **In-memory (Go)** - For monolithic mode
- **Cache Strategy:**
  - Camera status (30s TTL)
  - Thumbnails (5min TTL)
  - User sessions (30min TTL)

### Development Tools

**Testing:**
- **Go:** testing package + testify
- **Python:** pytest + pytest-asyncio
- **Frontend:** Vitest + Testing Library

**Linting/Formatting:**
- **Go:** golangci-lint
- **Python:** ruff (replaces black + flake8 + isort)
- **TypeScript:** ESLint + Prettier

**API Documentation:**
- **OpenAPI 3.1** specification
- **Swagger UI** for testing
- **Go:** swag for annotation-based docs

**Monitoring:**
- **Prometheus** for metrics
- **Grafana** for visualization
- **OpenTelemetry** for tracing (optional)

### Hardware Acceleration

**NVIDIA GPU:**
- **CUDA 12.0+** for YOLOv12/YOLO11
- **NVENC** for video encoding
- **Docker:** nvidia-container-toolkit

**Intel:**
- **Intel Media SDK** / **oneVPL**
- **OpenVINO** for AI inference

**Apple Silicon:**
- **Metal Performance Shaders** (MPS)
- **VideoToolbox** for encoding

### Container Runtime

**Docker 24+:**
- Standard deployment
- Docker Compose for development

**Kubernetes 1.28+:**
- Enterprise deployments
- Helm charts provided

### Operating System Support

**Primary:** Linux (Ubuntu 22.04+, Debian 12+)
**Secondary:** Windows 11 (via WSL2 or native)
**Tertiary:** macOS (Apple Silicon preferred)

### Latest Version Checklist

Before starting implementation, verify these:

```bash
# Run these searches to get latest versions:
web_search "go2rtc latest release 2024"
web_search "YOLOv12 github 2025"
web_search "ultralytics YOLO11 latest"
web_search "InsightFace buffalo_l latest version"
web_search "PaddleOCR latest release"
web_search "NATS server latest version"
web_search "React 18 latest"
web_search "shadcn/ui latest"

# Document findings in version.txt:
echo "Researched on: $(date)" > version.txt
echo "go2rtc: vX.X.X" >> version.txt
echo "YOLOv12: from ultralytics vX.X.X" >> version.txt
# ... etc
```

---

## 📁 REPOSITORY STRUCTURE

Create this exact structure:

```
nvr-system/
├── README.md
├── LICENSE
├── docker-compose.yml
├── docker-compose.dev.yml
├── Dockerfile.monolithic
├── .gitignore
│
├── cmd/
│   ├── nvr/                      # Main monolithic binary
│   │   └── main.go
│   └── services/                 # Individual service binaries
│       ├── camera-management/
│       ├── detection/
│       └── event-management/
│
├── internal/                     # Go shared code
│   ├── api/
│   ├── camera/
│   ├── config/
│   ├── events/
│   ├── storage/
│   ├── timeline/
│   └── auth/
│
├── services/
│   ├── ai-detection/            # Python service
│   │   ├── main.py
│   │   ├── requirements.txt
│   │   └── models/
│   ├── facial-recognition/      # Python service
│   ├── lpr-service/            # Python service
│   ├── state-recognition/      # Python service
│   └── audio-detection/        # Python service
│
├── web-ui/                      # React app
│   ├── package.json
│   ├── src/
│   │   ├── components/
│   │   ├── pages/
│   │   ├── services/
│   │   └── App.tsx
│   └── public/
│
├── ios-app/                     # Swift/SwiftUI (Phase 3)
│   ├── NVR.xcodeproj
│   └── NVR/
│
├── plugins/                     # Built-in plugins
│   ├── wyze-bridge/
│   │   ├── manifest.json
│   │   ├── config-schema.json
│   │   ├── main.js
│   │   └── README.md
│   ├── reolink/
│   └── onvif/
│
├── config/                      # Config templates
│   ├── config.example.yaml
│   └── go2rtc.example.yaml
│
├── docs/
│   ├── architecture/
│   ├── api/
│   ├── plugin-development/
│   └── deployment/
│
├── scripts/
│   ├── dev/
│   │   ├── setup.sh
│   │   └── reset-db.sh
│   └── deploy/
│
├── migrations/                  # SQLite migrations
│   ├── 001_initial_schema.sql
│   ├── 002_add_plugins.sql
│   └── ...
│
└── tests/
    ├── integration/
    ├── e2e/
    └── performance/
```

---

## 🚀 PHASE 1: MVP (WEEKS 1-6)

### Week 1: Foundation & Setup

**Day 1-2: Project Initialization**

```bash
# Initialize repository
mkdir nvr-system && cd nvr-system
git init

# Initialize Go module
go mod init github.com/yourusername/nvr

# Initialize web UI
cd web-ui
npm create vite@latest . -- --template react-ts
npm install @tanstack/react-query zustand tailwindcss
npx shadcn-ui@latest init
```

**Create basic docker-compose.dev.yml:**
```yaml
version: '3.8'

services:
  nvr-dev:
    build:
      context: .
      dockerfile: Dockerfile.dev
    volumes:
      - .:/app
      - ./data:/data
      - ./config:/config
    ports:
      - "5000:5000"   # Web UI
      - "8554:8554"   # RTSP
      - "1984:1984"   # go2rtc
    environment:
      - MODE=development
      - LOG_LEVEL=debug
```

**Day 3-4: Integrate go2rtc**

1. Download latest go2rtc binary
2. Create Go wrapper to manage go2rtc process
3. Test with sample RTSP stream (use Big Buck Bunny RTSP test stream)

```go
// internal/streaming/go2rtc.go
package streaming

type Go2RTCManager struct {
    cmd    *exec.Cmd
    config string
}

func (m *Go2RTCManager) Start() error {
    // Generate go2rtc.yaml from cameras config
    // Start go2rtc process
    // Monitor health
}
```

**Day 5-7: SQLite Setup**

Create migration system:
```go
// internal/database/migrations.go
package database

func RunMigrations(db *sql.DB) error {
    migrations := []string{
        "migrations/001_initial_schema.sql",
        "migrations/002_add_indexes.sql",
    }
    // Run each migration
}
```

**First migration (001_initial_schema.sql):**
```sql
CREATE TABLE IF NOT EXISTS cameras (
    id TEXT PRIMARY KEY,
    status TEXT CHECK(status IN ('online', 'offline', 'error')) NOT NULL,
    last_seen INTEGER NOT NULL,
    fps_current REAL,
    stats JSON,
    created_at INTEGER DEFAULT (unixepoch()) NOT NULL
) STRICT;

CREATE TABLE IF NOT EXISTS events (
    id TEXT PRIMARY KEY,
    camera_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    timestamp INTEGER NOT NULL,
    confidence REAL,
    thumbnail_path TEXT,
    video_segment_id TEXT,
    metadata JSON,
    acknowledged INTEGER DEFAULT 0,
    created_at INTEGER DEFAULT (unixepoch()) NOT NULL,
    FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
) STRICT;

CREATE INDEX idx_events_camera_time ON events(camera_id, timestamp DESC);
CREATE INDEX idx_events_type ON events(event_type);

-- Add all other tables from the full schema
```

**Week 1 Deliverable:**
- ✅ Repository structure created
- ✅ Docker Compose working
- ✅ go2rtc integrated and streaming test video
- ✅ SQLite with migrations working

---

### Week 2: Camera Management & API Gateway

**Camera Management Service (Go):**

```go
// internal/camera/service.go
package camera

type Service struct {
    db     *sql.DB
    config *config.Config
}

type Camera struct {
    ID        string    `json:"id"`
    Name      string    `json:"name"`
    StreamURL string    `json:"stream_url"`
    Username  string    `json:"username"`
    Password  string    `json:"password"` // Encrypted
    Status    string    `json:"status"`
    Enabled   bool      `json:"enabled"`
    Detection DetectionConfig `json:"detection"`
}

func (s *Service) CreateCamera(ctx context.Context, cam *Camera) error {
    // Validate
    // Generate ID
    // Encrypt password
    // Save to database AND config.yaml
    // Trigger go2rtc reload
}

func (s *Service) ListCameras(ctx context.Context) ([]*Camera, error) {
    // Read from database
}

func (s *Service) GetCameraStatus(ctx context.Context, id string) (*CameraStatus, error) {
    // Check if camera is responsive
    // Return current FPS, bitrate, etc.
}
```

**API Gateway with Chi router:**

```go
// cmd/nvr/main.go
package main

import (
    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
)

func main() {
    r := chi.NewRouter()
    
    // Middleware
    r.Use(middleware.Logger)
    r.Use(middleware.Recoverer)
    r.Use(corsMiddleware)
    
    // Routes
    r.Route("/api/v1", func(r chi.Router) {
        r.Route("/cameras", func(r chi.Router) {
            r.Get("/", handleListCameras)
            r.Post("/", handleCreateCamera)
            r.Get("/{id}", handleGetCamera)
            r.Put("/{id}", handleUpdateCamera)
            r.Delete("/{id}", handleDeleteCamera)
            r.Get("/{id}/snapshot", handleSnapshot)
        })
        
        r.Route("/events", func(r chi.Router) {
            r.Get("/", handleListEvents)
            r.Get("/{id}", handleGetEvent)
        })
    })
    
    // WebSocket for live updates
    r.Get("/ws", handleWebSocket)
    
    http.ListenAndServe(":5000", r)
}
```

**Week 2 Deliverable:**
- ✅ Camera CRUD via API
- ✅ Cameras saved to SQLite + config.yaml
- ✅ go2rtc auto-reloads when cameras change
- ✅ Health monitoring (ping cameras every 30s)

---

### Week 3: Video Processing & Storage

**Video Segmentation Service (Go with FFmpeg):**

```go
// internal/video/segmenter.go
package video

type Segmenter struct {
    inputURL  string
    outputDir string
}

func (s *Segmenter) Start() error {
    // FFmpeg command to segment video
    cmd := exec.Command("ffmpeg",
        "-i", s.inputURL,
        "-c", "copy",
        "-f", "segment",
        "-segment_time", "10",
        "-segment_format", "mp4",
        "-reset_timestamps", "1",
        filepath.Join(s.outputDir, "segment_%Y%m%d_%H%M%S.mp4"),
    )
    return cmd.Run()
}
```

**Storage Service:**

```go
// internal/storage/service.go
package storage

func (s *Service) SaveSegment(cameraID string, segment *VideoSegment) error {
    // Save to filesystem
    // Record in database
    // Check retention policies
}

func (s *Service) CleanupOldSegments() error {
    // Find segments older than retention period
    // Delete from filesystem and database
}

func (s *Service) GetSegmentsForTimeline(cameraID string, start, end time.Time) ([]*VideoSegment, error) {
    // Query database for segments in time range
}
```

**Week 3 Deliverable:**
- ✅ Video segments stored as 10-second chunks
- ✅ Segments recorded in database
- ✅ Retention policy enforced
- ✅ Can retrieve segments by time range

---

### Week 4: Basic Web UI

**React App Structure:**

```typescript
// web-ui/src/App.tsx
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter, Routes, Route } from 'react-router-dom';

const queryClient = new QueryClient();

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/cameras" element={<CameraList />} />
          <Route path="/cameras/add" element={<AddCamera />} />
          <Route path="/cameras/:id" element={<CameraDetail />} />
          <Route path="/events" element={<EventList />} />
          <Route path="/settings" element={<Settings />} />
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  );
}
```

**Camera List Component:**

```typescript
// web-ui/src/components/CameraList.tsx
import { useQuery } from '@tanstack/react-query';
import { apiClient } from '../services/api';

export function CameraList() {
  const { data: cameras, isLoading } = useQuery({
    queryKey: ['cameras'],
    queryFn: () => apiClient.cameras.list(),
  });

  if (isLoading) return <div>Loading...</div>;

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
      {cameras?.map(camera => (
        <CameraCard key={camera.id} camera={camera} />
      ))}
    </div>
  );
}
```

**Live Video Player:**

```typescript
// web-ui/src/components/VideoPlayer.tsx
import { useEffect, useRef } from 'react';
import Hls from 'hls.js';

export function VideoPlayer({ streamUrl }: { streamUrl: string }) {
  const videoRef = useRef<HTMLVideoElement>(null);

  useEffect(() => {
    if (!videoRef.current) return;

    const hls = new Hls();
    hls.loadSource(streamUrl);
    hls.attachMedia(videoRef.current);

    return () => hls.destroy();
  }, [streamUrl]);

  return <video ref={videoRef} controls className="w-full" />;
}
```

**Week 4 Deliverable:**
- ✅ Dashboard showing all cameras
- ✅ Live view working
- ✅ Add camera form (UI generates config)
- ✅ Camera detail page with live stream

---

### Week 5-6: AI Detection (YOLOv12)

**Detection Service (Python):**

```python
# services/ai-detection/main.py
from fastapi import FastAPI, File, UploadFile
from ultralytics import YOLO
import numpy as np
from PIL import Image
import io

app = FastAPI()

# Load model on startup
model = YOLO('yolov12n.pt')  # Use nano for speed

@app.post("/detect")
async def detect_objects(
    image: UploadFile = File(...),
    confidence: float = 0.5
):
    # Read image
    img_bytes = await image.read()
    img = Image.open(io.BytesIO(img_bytes))
    
    # Run detection
    results = model(img, conf=confidence)
    
    # Format results
    detections = []
    for r in results:
        boxes = r.boxes
        for box in boxes:
            detections.append({
                'class': model.names[int(box.cls)],
                'confidence': float(box.conf),
                'bbox': box.xyxy[0].tolist()
            })
    
    return {'detections': detections}

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8001)
```

**requirements.txt:**
```
fastapi==0.109.0
uvicorn==0.27.0
ultralytics==8.1.0
pillow==10.2.0
numpy==1.26.3
```

**Detection Integration in Go:**

```go
// internal/detection/client.go
package detection

type Client struct {
    baseURL string
}

func (c *Client) Detect(ctx context.Context, imageData []byte) ([]Detection, error) {
    // Call Python detection service
    resp, err := http.Post(
        c.baseURL+"/detect",
        "application/octet-stream",
        bytes.NewReader(imageData),
    )
    // Parse response
}
```

**Event Manager:**

```go
// internal/events/manager.go
package events

func (m *Manager) ProcessFrame(cameraID string, frame []byte, timestamp time.Time) error {
    // Send to detection service
    detections, err := m.detectionClient.Detect(ctx, frame)
    
    // Create events for each detection
    for _, det := range detections {
        event := &Event{
            ID:         generateID(),
            CameraID:   cameraID,
            EventType:  det.Class,
            Timestamp:  timestamp,
            Confidence: det.Confidence,
            Metadata:   det.BBox,
        }
        
        // Save to database
        m.saveEvent(event)
        
        // Send to WebSocket clients
        m.broadcastEvent(event)
    }
}
```

**Week 5-6 Deliverable:**
- ✅ YOLOv12 detection service running
- ✅ Frames analyzed at 5 fps
- ✅ Events stored in database
- ✅ Events appear on timeline in UI
- ✅ Real-time event notifications via WebSocket

---

## 🎨 PHASE 2: ADVANCED FEATURES (WEEKS 7-12)

### Week 7-8: Additional AI Services

**Facial Recognition (Python + InsightFace):**

```python
# services/facial-recognition/main.py
from fastapi import FastAPI
from insightface.app import FaceAnalysis
import numpy as np

app = FastAPI()
face_app = FaceAnalysis(name='buffalo_l')
face_app.prepare(ctx_id=0, det_size=(640, 640))

@app.post("/detect-faces")
async def detect_faces(image: UploadFile = File(...)):
    img = np.array(Image.open(io.BytesIO(await image.read())))
    faces = face_app.get(img)
    
    return {
        'faces': [{
            'bbox': face.bbox.tolist(),
            'embedding': face.embedding.tolist(),
            'age': face.age,
            'gender': 'M' if face.gender == 1 else 'F'
        } for face in faces]
    }

@app.post("/match-face")
async def match_face(embedding: list[float], threshold: float = 0.5):
    # Compare against known persons database
    pass
```

**License Plate Recognition:**

```python
# services/lpr-service/main.py
from paddleocr import PaddleOCR
from ultralytics import YOLO

ocr = PaddleOCR(use_angle_cls=True, lang='en')
plate_detector = YOLO('license_plate.pt')

@app.post("/read-plate")
async def read_plate(image: UploadFile = File(...)):
    img = np.array(Image.open(io.BytesIO(await image.read())))
    
    # Detect plate
    plates = plate_detector(img)
    
    results = []
    for plate in plates:
        # Crop plate region
        bbox = plate.boxes[0].xyxy[0].int().tolist()
        plate_img = img[bbox[1]:bbox[3], bbox[0]:bbox[2]]
        
        # OCR
        result = ocr.ocr(plate_img)
        text = ' '.join([line[1][0] for line in result[0]])
        
        results.append({
            'bbox': bbox,
            'text': text,
            'confidence': plate.boxes[0].conf.item()
        })
    
    return {'plates': results}
```

---

### Week 9-10: Plugin System

**Plugin Loader (TypeScript):**

```typescript
// services/plugin-system/src/loader.ts
interface Plugin {
  manifest: PluginManifest;
  instance: any;
}

class PluginManager {
  private plugins: Map<string, Plugin> = new Map();
  
  async loadPlugin(pluginPath: string): Promise<void> {
    // Read manifest.json
    const manifest = await this.readManifest(pluginPath);
    
    // Validate manifest
    this.validateManifest(manifest);
    
    // Load plugin code
    const module = await import(path.join(pluginPath, manifest.entry_point));
    
    // Initialize plugin
    const api = new PluginAPI(this.nvrCore);
    const instance = new module.default();
    await instance.initialize(api);
    
    this.plugins.set(manifest.id, { manifest, instance });
  }
  
  async discoverCameras(pluginId: string): Promise<Camera[]> {
    const plugin = this.plugins.get(pluginId);
    if (!plugin) throw new Error('Plugin not found');
    
    return await plugin.instance.discoverCameras();
  }
}
```

**Wyze Bridge Plugin:**

```typescript
// plugins/wyze-bridge/main.ts
import { NVRPlugin, PluginAPI, Camera } from '@nvr/plugin-sdk';
import axios from 'axios';

export default class WyzeBridgePlugin implements NVRPlugin {
  private api: PluginAPI;
  private config: any;
  
  async initialize(api: PluginAPI) {
    this.api = api;
    this.config = await api.getConfig();
    
    api.registerCameraDiscovery({
      name: 'Wyze Cameras',
      discover: this.discoverCameras.bind(this)
    });
    
    api.registerStreamAdapter({
      type: 'wyze',
      getStream: this.getStream.bind(this)
    });
  }
  
  async discoverCameras(): Promise<Camera[]> {
    // Use Wyze API with credentials from config
    const response = await axios.post(
      'https://api.wyze.com/api/v2/cameras',
      {},
      {
        headers: {
          'Authorization': `Bearer ${this.config.api_key}`
        }
      }
    );
    
    return response.data.data.map(cam => ({
      id: cam.mac,
      name: cam.nickname,
      manufacturer: 'Wyze',
      model: cam.product_model,
      streamUrl: `rtsp://wyze-bridge:8554/${cam.mac}`,
      capabilities: ['motion', 'audio']
    }));
  }
  
  async getStream(camera: Camera): Promise<StreamInfo> {
    // Return go2rtc compatible stream
    return {
      url: camera.streamUrl,
      codec: 'h264',
      audio: true
    };
  }
}
```

**Plugin UI Generator:**

```typescript
// web-ui/src/components/PluginConfigForm.tsx
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';

export function PluginConfigForm({ schema }: { schema: JSONSchema }) {
  // Auto-generate form from config-schema.json
  const form = useForm({
    resolver: zodResolver(jsonSchemaToZod(schema))
  });
  
  return (
    <form onSubmit={form.handleSubmit(onSubmit)}>
      {Object.entries(schema.properties).map(([key, prop]) => (
        <FormField
          key={key}
          name={key}
          label={prop.title}
          description={prop.description}
          type={prop.type}
          control={form.control}
        />
      ))}
      <Button type="submit">Save Configuration</Button>
    </form>
  );
}
```

---

### Week 11-12: UI Polish & Timeline

**Advanced Timeline Component:**

```typescript
// web-ui/src/components/Timeline.tsx
import { useQuery } from '@tanstack/react-query';
import { useEffect, useRef, useState } from 'react';

export function Timeline({ cameraId, start, end }: TimelineProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [currentTime, setCurrentTime] = useState(start);
  
  const { data: segments } = useQuery({
    queryKey: ['timeline', cameraId, start, end],
    queryFn: () => apiClient.timeline.getSegments(cameraId, start, end)
  });
  
  const { data: events } = useQuery({
    queryKey: ['events', cameraId, start, end],
    queryFn: () => apiClient.events.list({ cameraId, start, end })
  });
  
  useEffect(() => {
    if (!canvasRef.current || !segments) return;
    
    const ctx = canvasRef.current.getContext('2d');
    drawTimeline(ctx, segments, events, currentTime);
  }, [segments, events, currentTime]);
  
  const handleScrub = (e: React.MouseEvent) => {
    // Calculate time from mouse position
    const rect = canvasRef.current!.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const percentage = x / rect.width;
    const time = start + (end - start) * percentage;
    
    setCurrentTime(time);
    // Update video player
  };
  
  return (
    <div className="relative">
      <canvas
        ref={canvasRef}
        width={1920}
        height={100}
        onClick={handleScrub}
        className="w-full cursor-pointer"
      />
      {/* Event markers */}
      {events?.map(event => (
        <EventMarker
          key={event.id}
          event={event}
          start={start}
          end={end}
        />
      ))}
    </div>
  );
}
```

**Detection Zone Editor:**

```typescript
// web-ui/src/components/ZoneEditor.tsx
export function ZoneEditor({ cameraId }: { cameraId: string }) {
  const [points, setPoints] = useState<Point[]>([]);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  
  const handleClick = (e: React.MouseEvent) => {
    const rect = canvasRef.current!.getBoundingClientRect();
    const x = (e.clientX - rect.left) / rect.width;
    const y = (e.clientY - rect.top) / rect.height;
    
    setPoints([...points, { x, y }]);
  };
  
  const saveZone = async () => {
    await apiClient.cameras.updateZones(cameraId, {
      zones: [{
        name: 'Zone 1',
        points: points
      }]
    });
  };
  
  return (
    <div className="relative">
      <img src={`/api/v1/cameras/${cameraId}/snapshot`} />
      <canvas
        ref={canvasRef}
        onClick={handleClick}
        className="absolute inset-0 w-full h-full"
      />
      <Button onClick={saveZone}>Save Zone</Button>
    </div>
  );
}
```

---

## 🔌 PLUGIN SYSTEM ARCHITECTURE - DETAILED DESIGN

### Philosophy

Plugins are **FIRST-CLASS CITIZENS**, not an afterthought. They should be:
- **Easy to develop** - Simple SDK, good docs, examples
- **Safe to run** - Sandboxed, resource-limited
- **Easy to install** - One-click from UI
- **Hot-reloadable** - No system restart needed
- **Well-integrated** - Plugins can hook into any part of the system

### Built-in Plugins (Ship with System)

These plugins are bundled and should be available immediately:

#### 1. Wyze Bridge Plugin
**Purpose:** Integrate Wyze cameras without cloud dependency
**Based On:** docker-wyze-bridge v2.10.3+ by mrlt8
**GitHub:** https://github.com/mrlt8/docker-wyze-bridge

**Features:**
- Auto-discover Wyze cameras on network
- Support V3, V4, Pan, Outdoor, Doorbell models
- Handle Wyze API authentication (API key required)
- Convert Wyze streams to RTSP via go2rtc
- Support motion events, doorbell presses

**Implementation Notes:**
- Wyze requires API key from https://support.wyze.com/hc/en-us/articles/16129834216731
- Uses Wyze IoT SDK for communication
- Some firmware versions have P2P issues (VPN workaround)
- Local LAN access preferred

**Config Schema:**
```json
{
  "type": "object",
  "properties": {
    "api_key": {
      "type": "string",
      "title": "Wyze API Key",
      "description": "Get from Wyze Developer Portal",
      "ui:widget": "password"
    },
    "api_id": {
      "type": "string",
      "title": "API ID",
      "description": "Your Wyze API ID"
    },
    "username": {
      "type": "string",
      "title": "Wyze Email",
      "format": "email"
    },
    "password": {
      "type": "string",
      "title": "Wyze Password",
      "ui:widget": "password"
    },
    "refresh_interval": {
      "type": "integer",
      "title": "Refresh Interval (seconds)",
      "default": 300,
      "minimum": 60
    }
  },
  "required": ["api_key", "api_id", "username", "password"]
}
```

**Discovery Flow:**
```typescript
async discoverCameras() {
  // 1. Authenticate with Wyze API
  const token = await this.authenticate();
  
  // 2. Get camera list from Wyze
  const response = await axios.post(
    'https://api.wyzecam.com/app/v2/home_page/get_object_list',
    { ... },
    { headers: { 'Authorization': `Bearer ${token}` }}
  );
  
  // 3. Convert to standard camera format
  return response.data.device_list.map(device => ({
    id: `wyze-${device.mac}`,
    name: device.nickname,
    manufacturer: 'Wyze',
    model: device.product_model,
    streamUrl: `rtsp://wyze-bridge:8554/${device.mac}`,
    capabilities: ['motion', 'audio', 'two_way_audio']
  }));
}
```

#### 2. Reolink Plugin
**Purpose:** Integrate Reolink cameras with full ONVIF support
**Based On:** Reolink ONVIF Profile S/T with two-way audio

**Features:**
- Auto-discover via ONVIF
- Support doorbell press events
- AI detection events (person, vehicle, package)
- Two-way audio via ONVIF Profile T
- PTZ control

**Important Notes:**
- Firmware v3.0.0.2033+ supports ONVIF two-way audio
- Must enable ONVIF in camera settings (Settings > Network > Advanced)
- Default ports: RTSP 554, ONVIF 8000
- Works with Home Assistant integration

**Config Schema:**
```json
{
  "type": "object",
  "properties": {
    "auto_discover": {
      "type": "boolean",
      "title": "Auto-discover Reolink cameras",
      "default": true
    },
    "onvif_port": {
      "type": "integer",
      "title": "ONVIF Port",
      "default": 8000
    },
    "rtsp_port": {
      "type": "integer",
      "title": "RTSP Port",
      "default": 554
    }
  }
}
```

**Event Handling:**
```typescript
async subscribeToEvents(camera: Camera) {
  // ONVIF event subscription
  const subscription = await this.onvifClient.subscribe({
    url: `http://${camera.ip}:${camera.onvifPort}/onvif/event_service`,
    events: [
      'tns1:RuleEngine/CellMotionDetector/Motion',
      'tns1:RuleEngine/TamperDetector/Tamper',
      'tns1:VideoSource/MotionAlarm',
      'tns1:Device/Trigger/DigitalInput' // Doorbell
    ]
  });
  
  subscription.on('event', (event) => {
    this.api.emitEvent({
      cameraId: camera.id,
      type: this.mapEventType(event),
      timestamp: new Date(),
      metadata: event
    });
  });
}
```

#### 3. ONVIF Generic Plugin
**Purpose:** Support any ONVIF-compliant camera
**Based On:** ONVIF Profile S (streaming) + Profile T (two-way audio)

**Features:**
- Auto-discovery via WS-Discovery
- Profile S: Video streaming, PTZ
- Profile T: Two-way audio (if supported)
- Profile G: Recording/playback (if supported)
- Event subscription (motion, analytics)

**Implementation:**
```typescript
async discoverONVIFCameras() {
  // WS-Discovery multicast
  const devices = await onvif.Discovery.probe({
    timeout: 5000
  });
  
  // Query each device for capabilities
  const cameras = await Promise.all(
    devices.map(async device => {
      const cam = new onvif.Cam({
        hostname: device.hostname,
        username: 'admin',
        password: '' // Will prompt user
      });
      
      const capabilities = await cam.getCapabilities();
      const profiles = await cam.getProfiles();
      
      return {
        id: `onvif-${device.serialNumber}`,
        name: device.name || device.hostname,
        manufacturer: device.manufacturer,
        model: device.model,
        streamUrl: profiles[0].streamUri.uri,
        capabilities: this.parseCapabilities(capabilities)
      };
    })
  );
  
  return cameras;
}
```

### Plugin API Surface

**Camera Discovery:**
```typescript
interface CameraDiscovery {
  name: string;
  discover: () => Promise<Camera[]>;
  supportsAuthentication?: boolean;
}

api.registerCameraDiscovery({
  name: 'My Cameras',
  discover: async () => { /* ... */ }
});
```

**Stream Adapters:**
```typescript
interface StreamAdapter {
  type: string;
  getStream: (camera: Camera) => Promise<StreamInfo>;
  supports?: {
    audio?: boolean;
    twoWayAudio?: boolean;
    ptz?: boolean;
  };
}

api.registerStreamAdapter({
  type: 'wyze',
  getStream: async (camera) => ({
    url: `rtsp://bridge:8554/${camera.mac}`,
    codec: 'h264',
    audio: true
  })
});
```

**AI Detectors:**
```typescript
interface AIDetector {
  name: string;
  type: 'object_detection' | 'face_recognition' | 'lpr' | 'custom';
  analyze: (frame: Buffer, config: any) => Promise<Detection[]>;
  supports?: {
    gpu?: boolean;
    batch?: boolean;
  };
}

api.registerDetector({
  name: 'Custom Fruit Detector',
  type: 'object_detection',
  analyze: async (frame, config) => {
    const results = await myModel.detect(frame);
    return results.map(r => ({
      class: r.label,
      confidence: r.score,
      bbox: r.box
    }));
  }
});
```

**Storage Backends:**
```typescript
interface StorageBackend {
  name: string;
  save: (segment: VideoSegment) => Promise<string>; // Returns path/URL
  retrieve: (path: string) => Promise<Buffer>;
  delete: (path: string) => Promise<void>;
  list: (filter: StorageFilter) => Promise<string[]>;
}

api.registerStorageBackend({
  name: 'S3 Compatible',
  save: async (segment) => {
    await s3.putObject({
      Bucket: 'nvr-recordings',
      Key: segment.path,
      Body: segment.data
    });
    return segment.path;
  }
});
```

**Notification Channels:**
```typescript
interface NotificationChannel {
  name: string;
  send: (notification: Notification) => Promise<void>;
  supports?: {
    richMedia?: boolean;
    priority?: boolean;
  };
}

api.registerNotificationChannel({
  name: 'Discord',
  send: async (notification) => {
    await axios.post(webhookUrl, {
      content: notification.message,
      embeds: notification.thumbnail ? [{
        image: { url: notification.thumbnail }
      }] : []
    });
  }
});
```

**Integrations:**
```typescript
interface Integration {
  name: string;
  type: 'home_automation' | 'cloud' | 'analytics' | 'custom';
  onEvent?: (event: Event) => Promise<void>;
  onCameraAdded?: (camera: Camera) => Promise<void>;
  getState?: () => Promise<any>;
}

api.registerIntegration({
  name: 'Home Assistant',
  type: 'home_automation',
  onEvent: async (event) => {
    await axios.post(
      `${config.ha_url}/api/webhook/nvr`,
      {
        type: 'camera_event',
        camera_id: event.cameraId,
        event_type: event.type
      },
      {
        headers: { 'Authorization': `Bearer ${config.ha_token}` }
      }
    );
  }
});
```

### Plugin Sandboxing & Security

**Isolation:**
- **TypeScript/JavaScript plugins:** Run in VM2 sandbox
- **Python plugins:** Run in separate process with restricted imports
- **Resource limits:** CPU, memory, network quota
- **Filesystem access:** Only plugin directory + /tmp

**Permissions System:**
```json
{
  "permissions": [
    "network",              // Make HTTP requests
    "camera_management",    // Add/modify cameras
    "event_management",     // Create events
    "storage_access",       // Access video storage
    "ai_inference",         // Use AI models
    "config_read",          // Read system config
    "config_write"          // Modify system config (dangerous!)
  ]
}
```

**Plugin Lifecycle:**
```typescript
class PluginManager {
  async loadPlugin(pluginPath: string) {
    // 1. Read and validate manifest
    const manifest = await this.readManifest(pluginPath);
    this.validateManifest(manifest);
    
    // 2. Check permissions
    await this.checkPermissions(manifest.permissions);
    
    // 3. Install dependencies
    if (manifest.runtime === 'node') {
      await this.npmInstall(pluginPath);
    } else if (manifest.runtime === 'python') {
      await this.pipInstall(pluginPath);
    }
    
    // 4. Create sandbox
    const sandbox = this.createSandbox(manifest);
    
    // 5. Load plugin code
    const Plugin = await sandbox.require(manifest.entry_point);
    
    // 6. Initialize
    const api = new PluginAPI(this.core, manifest.permissions);
    const instance = new Plugin();
    await instance.initialize(api);
    
    // 7. Register plugin
    this.plugins.set(manifest.id, {
      manifest,
      instance,
      sandbox,
      api
    });
    
    // 8. Emit event
    this.emit('plugin:loaded', manifest.id);
  }
  
  async unloadPlugin(pluginId: string) {
    const plugin = this.plugins.get(pluginId);
    
    // 1. Call cleanup
    if (plugin.instance.cleanup) {
      await plugin.instance.cleanup();
    }
    
    // 2. Unregister all hooks
    plugin.api.unregisterAll();
    
    // 3. Destroy sandbox
    plugin.sandbox.destroy();
    
    // 4. Remove from registry
    this.plugins.delete(pluginId);
    
    // 5. Emit event
    this.emit('plugin:unloaded', pluginId);
  }
}
```

### Plugin Development Workflow

**1. Create Plugin:**
```bash
npx create-nvr-plugin my-camera-plugin
# or
python -m nvr_cli create-plugin my-detector-plugin
```

**2. Plugin Structure:**
```
my-plugin/
├── manifest.json          # Plugin metadata
├── config-schema.json     # UI form definition
├── main.js or main.py     # Entry point
├── package.json or requirements.txt
├── icon.svg              # Plugin icon (64x64)
├── README.md             # Documentation
└── examples/             # Usage examples
    └── basic.js
```

**3. Test Plugin:**
```bash
# Development mode - hot reload enabled
nvr plugin dev ./my-plugin

# Run tests
nvr plugin test ./my-plugin

# Validate
nvr plugin validate ./my-plugin
```

**4. Package Plugin:**
```bash
# Creates my-plugin-v1.0.0.nvr file
nvr plugin package ./my-plugin

# Publish to registry (future)
nvr plugin publish ./my-plugin
```

**5. Install Plugin:**
```bash
# From file
nvr plugin install ./my-plugin-v1.0.0.nvr

# From git (development)
nvr plugin install github.com/user/my-plugin

# From registry (future)
nvr plugin install my-plugin
```

### Plugin Examples

See `plugins/` directory for complete examples:
- `plugins/wyze-bridge/` - Camera adapter with auth
- `plugins/reolink/` - ONVIF integration
- `plugins/custom-detector/` - Custom AI model
- `plugins/telegram-notify/` - Notification channel
- `plugins/s3-storage/` - Storage backend

### Plugin Marketplace (Future)

**Phase 1:** Manual installation from files
**Phase 2:** GitHub-based registry
**Phase 3:** Centralized marketplace with ratings/reviews

---

## 🗄️ DATABASE SCHEMA - COMPLETE DESIGN

### Design Principles

1. **SQLite First:** Optimize for SQLite, make PostgreSQL work
2. **STRICT Mode:** Use STRICT tables (type safety)
3. **Integer Timestamps:** Unix timestamps for simplicity
4. **JSON for Flexibility:** Use JSON for variable data
5. **Indexes Everything:** Index all foreign keys and query paths
6. **Normalize Where It Matters:** Don't over-normalize

### Complete Schema

```sql
-- migrations/001_initial_schema.sql

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
    stats JSON,  -- Current performance stats
    error_message TEXT,
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at INTEGER NOT NULL DEFAULT (unixepoch())
) STRICT;

CREATE INDEX idx_cameras_status ON cameras(status);
CREATE INDEX idx_cameras_last_seen ON cameras(last_seen DESC);

-- ====================
-- EVENTS
-- ====================
-- Core event table - heavily queried
CREATE TABLE IF NOT EXISTS events (
    id TEXT PRIMARY KEY,
    camera_id TEXT NOT NULL,
    event_type TEXT NOT NULL,  -- 'motion', 'person', 'vehicle', 'face', 'lpr', 'audio', 'state_change', 'doorbell'
    timestamp INTEGER NOT NULL,  -- Event start time
    end_timestamp INTEGER,  -- Event end time (for duration events)
    confidence REAL,  -- 0.0 - 1.0
    thumbnail_path TEXT,  -- Relative path to thumbnail
    video_segment_id TEXT,  -- Link to recording
    
    -- Flexible metadata as JSON
    -- Examples:
    -- {"class": "person", "bbox": [100,100,200,200], "zone": "driveway"}
    -- {"plate": "ABC123", "vehicle_id": "uuid"}
    -- {"face_id": "uuid", "person_id": "uuid"}
    metadata JSON,
    
    acknowledged INTEGER NOT NULL DEFAULT 0 CHECK(acknowledged IN (0, 1)),
    tags JSON,  -- User-added tags as array
    notes TEXT,  -- User notes
    
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    
    FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE,
    FOREIGN KEY (video_segment_id) REFERENCES recordings(id) ON DELETE SET NULL
) STRICT;

-- Critical indexes for timeline queries
CREATE INDEX idx_events_camera_time ON events(camera_id, timestamp DESC);
CREATE INDEX idx_events_type ON events(event_type);
CREATE INDEX idx_events_timestamp ON events(timestamp DESC);
CREATE INDEX idx_events_camera_type_time ON events(camera_id, event_type, timestamp DESC);
CREATE INDEX idx_events_ack ON events(acknowledged) WHERE acknowledged = 0;

-- Full-text search on notes
CREATE VIRTUAL TABLE events_fts USING fts5(
    event_id UNINDEXED,
    notes,
    content=events,
    content_rowid=rowid
);

-- ====================
-- DETECTIONS
-- ====================
-- Object detection results linked to events
CREATE TABLE IF NOT EXISTS detections (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL,
    object_type TEXT NOT NULL,  -- COCO class name
    label TEXT,  -- Custom label (e.g., "my_car", "front_door")
    confidence REAL NOT NULL,
    
    -- Bounding box as JSON: {"x": 100, "y": 100, "width": 200, "height": 150}
    -- Stored as JSON for flexibility (could be polygon for segmentation)
    bbox JSON NOT NULL,
    
    -- Additional attributes
    attributes JSON,  -- {"color": "red", "size": "large", "direction": "entering"}
    
    -- Multi-frame tracking
    track_id TEXT,  -- Links detections across frames
    track_confidence REAL,  -- Confidence in tracking
    
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    
    FOREIGN KEY (event_id) REFERENCES events(id) ON DELETE CASCADE
) STRICT;

CREATE INDEX idx_detections_event ON detections(event_id);
CREATE INDEX idx_detections_object ON detections(object_type);
CREATE INDEX idx_detections_track ON detections(track_id) WHERE track_id IS NOT NULL;
CREATE INDEX idx_detections_label ON detections(label) WHERE label IS NOT NULL;

-- ====================
-- FACIAL RECOGNITION
-- ====================
-- Known persons database
CREATE TABLE IF NOT EXISTS persons (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    notes TEXT,
    thumbnail_path TEXT,
    
    -- Store multiple reference embeddings for better matching
    -- JSON array of base64 encoded embeddings
    reference_embeddings JSON,
    
    -- Metadata
    metadata JSON,  -- {"relationship": "family", "allowed_areas": ["front_door"]}
    
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at INTEGER NOT NULL DEFAULT (unixepoch())
) STRICT;

CREATE INDEX idx_persons_name ON persons(name);

-- Face detections linked to events
CREATE TABLE IF NOT EXISTS faces (
    id TEXT PRIMARY KEY,
    person_id TEXT,  -- NULL if unknown face
    event_id TEXT NOT NULL,
    
    -- Face embedding for similarity search
    -- Store as BLOB for efficiency (512 floats = 2KB)
    embedding BLOB NOT NULL,
    
    confidence REAL NOT NULL,  -- Detection confidence
    match_confidence REAL,  -- Recognition confidence (if matched)
    
    bbox JSON NOT NULL,
    
    -- Facial attributes (from InsightFace)
    age INTEGER,
    gender TEXT CHECK(gender IN ('M', 'F', 'unknown')),
    attributes JSON,  -- {"expression": "smile", "glasses": true}
    
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    
    FOREIGN KEY (person_id) REFERENCES persons(id) ON DELETE SET NULL,
    FOREIGN KEY (event_id) REFERENCES events(id) ON DELETE CASCADE
) STRICT;

CREATE INDEX idx_faces_person ON faces(person_id);
CREATE INDEX idx_faces_event ON faces(event_id);
CREATE INDEX idx_faces_unknown ON faces(person_id) WHERE person_id IS NULL;

-- ====================
-- LICENSE PLATE RECOGNITION
-- ====================
-- Known vehicles database
CREATE TABLE IF NOT EXISTS vehicles (
    id TEXT PRIMARY KEY,
    license_plate TEXT NOT NULL UNIQUE,
    
    -- Vehicle details
    make TEXT,
    model TEXT,
    year INTEGER,
    color TEXT,
    
    -- Owner information
    owner_name TEXT,
    relationship TEXT,  -- "family", "guest", "delivery", etc.
    
    notes TEXT,
    thumbnail_path TEXT,
    
    -- Access control
    allowed INTEGER NOT NULL DEFAULT 1 CHECK(allowed IN (0, 1)),
    allowed_zones JSON,  -- Array of zone IDs where vehicle is allowed
    
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at INTEGER NOT NULL DEFAULT (unixepoch())
) STRICT;

CREATE INDEX idx_vehicles_plate ON vehicles(license_plate);
CREATE INDEX idx_vehicles_allowed ON vehicles(allowed);

-- LPR detections linked to events
CREATE TABLE IF NOT EXISTS lpr_detections (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL,
    vehicle_id TEXT,  -- NULL if unknown vehicle
    
    plate_text TEXT NOT NULL,  -- Recognized text
    confidence REAL NOT NULL,
    
    bbox JSON NOT NULL,
    
    -- Vehicle attributes from AI
    attributes JSON,  -- {"make": "Toyota", "color": "red", "type": "sedan"}
    
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    
    FOREIGN KEY (event_id) REFERENCES events(id) ON DELETE CASCADE,
    FOREIGN KEY (vehicle_id) REFERENCES vehicles(id) ON DELETE SET NULL
) STRICT;

CREATE INDEX idx_lpr_event ON lpr_detections(event_id);
CREATE INDEX idx_lpr_vehicle ON lpr_detections(vehicle_id);
CREATE INDEX idx_lpr_plate ON lpr_detections(plate_text);
CREATE INDEX idx_lpr_unknown ON lpr_detections(vehicle_id) WHERE vehicle_id IS NULL;

-- ====================
-- STATE RECOGNITION
-- ====================
-- Custom state detectors (trash out, water detected, etc.)
CREATE TABLE IF NOT EXISTS state_detectors (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    camera_id TEXT NOT NULL,
    
    -- Detector type
    detector_type TEXT NOT NULL,  -- 'trash_bin', 'water_detect', 'pet_accident', 'fruit_on_tree', 'package', 'custom'
    
    -- Configuration for the detector
    -- Example: {"target_object": "trash_bin", "target_zone": "curb", "normal_state": "in_driveway"}
    config JSON NOT NULL,
    
    -- Current state
    current_state TEXT,
    last_transition INTEGER,  -- Timestamp of last state change
    
    enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0, 1)),
    
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at INTEGER NOT NULL DEFAULT (unixepoch()),
    
    FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
) STRICT;

CREATE INDEX idx_state_detectors_camera ON state_detectors(camera_id);
CREATE INDEX idx_state_detectors_enabled ON state_detectors(enabled) WHERE enabled = 1;

-- State changes over time
CREATE TABLE IF NOT EXISTS state_changes (
    id TEXT PRIMARY KEY,
    detector_id TEXT NOT NULL,
    from_state TEXT,  -- NULL for initial state
    to_state TEXT NOT NULL,
    timestamp INTEGER NOT NULL,
    confidence REAL,
    
    -- Evidence for the state change
    metadata JSON,  -- {"detection_ids": ["uuid1", "uuid2"], "reasoning": "trash bin detected at curb"}
    
    FOREIGN KEY (detector_id) REFERENCES state_detectors(id) ON DELETE CASCADE
) STRICT;

CREATE INDEX idx_state_changes_detector_time ON state_changes(detector_id, timestamp DESC);
CREATE INDEX idx_state_changes_time ON state_changes(timestamp DESC);

-- ====================
-- VIDEO RECORDINGS
-- ====================
-- Video segments (10-second chunks)
CREATE TABLE IF NOT EXISTS recordings (
    id TEXT PRIMARY KEY,
    camera_id TEXT NOT NULL,
    
    start_time INTEGER NOT NULL,
    end_time INTEGER NOT NULL,
    duration INTEGER NOT NULL,  -- Seconds
    
    -- File storage
    file_path TEXT NOT NULL,  -- Relative path
    file_size INTEGER NOT NULL,  -- Bytes
    
    -- Storage tier for lifecycle management
    storage_tier TEXT NOT NULL DEFAULT 'hot' CHECK(storage_tier IN ('hot', 'warm', 'cold')),
    
    -- Quick lookup for segments with events
    has_events INTEGER NOT NULL DEFAULT 0 CHECK(has_events IN (0, 1)),
    event_count INTEGER NOT NULL DEFAULT 0,
    
    -- Video metadata
    codec TEXT,
    resolution TEXT,
    fps REAL,
    bitrate INTEGER,
    
    -- Checksum for integrity
    checksum TEXT,  -- SHA256
    
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    
    FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
) STRICT;

CREATE INDEX idx_recordings_camera_time ON recordings(camera_id, start_time DESC);
CREATE INDEX idx_recordings_tier ON recordings(storage_tier);
CREATE INDEX idx_recordings_events ON recordings(has_events) WHERE has_events = 1;
CREATE INDEX idx_recordings_cleanup ON recordings(storage_tier, end_time);

-- ====================
-- USERS & AUTH
-- ====================
-- User accounts
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    email TEXT UNIQUE,
    
    password_hash TEXT NOT NULL,  -- bcrypt
    
    role TEXT NOT NULL DEFAULT 'user' CHECK(role IN ('admin', 'user', 'guest', 'api')),
    
    -- Permissions (JSON array of permission strings)
    permissions JSON,
    
    enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0, 1)),
    
    -- MFA
    totp_secret TEXT,  -- TOTP secret if 2FA enabled
    totp_enabled INTEGER NOT NULL DEFAULT 0 CHECK(totp_enabled IN (0, 1)),
    
    -- Session tracking
    last_login INTEGER,
    last_ip TEXT,
    
    -- Preferences
    preferences JSON,  -- UI preferences, notification settings, etc.
    
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at INTEGER NOT NULL DEFAULT (unixepoch())
) STRICT;

CREATE INDEX idx_users_username ON users(username);
CREATE INDEX idx_users_email ON users(email) WHERE email IS NOT NULL;
CREATE INDEX idx_users_enabled ON users(enabled) WHERE enabled = 1;

-- API keys for programmatic access
CREATE TABLE IF NOT EXISTS api_keys (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    
    key_hash TEXT NOT NULL UNIQUE,  -- Store hash, not plaintext
    key_prefix TEXT NOT NULL,  -- First 8 chars for identification
    
    name TEXT NOT NULL,  -- User-friendly name
    
    permissions JSON,  -- Scoped permissions
    
    last_used INTEGER,
    expires_at INTEGER,
    
    enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0, 1)),
    
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) STRICT;

CREATE INDEX idx_api_keys_user ON api_keys(user_id);
CREATE INDEX idx_api_keys_hash ON api_keys(key_hash);

-- ====================
-- PLUGINS
-- ====================
-- Installed plugins
CREATE TABLE IF NOT EXISTS plugins (
    id TEXT PRIMARY KEY,  -- From manifest
    name TEXT NOT NULL UNIQUE,
    version TEXT NOT NULL,
    
    author TEXT,
    description TEXT,
    
    -- Plugin metadata
    manifest JSON NOT NULL,
    
    -- Installation
    install_path TEXT NOT NULL,
    
    -- Configuration
    config JSON,  -- User configuration values
    
    -- State
    enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0, 1)),
    status TEXT NOT NULL DEFAULT 'installed' CHECK(status IN ('installed', 'running', 'stopped', 'error')),
    error_message TEXT,
    
    -- Lifecycle
    installed_at INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at INTEGER NOT NULL DEFAULT (unixepoch()),
    last_started INTEGER,
    last_stopped INTEGER
) STRICT;

CREATE INDEX idx_plugins_enabled ON plugins(enabled) WHERE enabled = 1;
CREATE INDEX idx_plugins_status ON plugins(status);

-- Plugin execution logs (recent only, rotated)
CREATE TABLE IF NOT EXISTS plugin_logs (
    id TEXT PRIMARY KEY,
    plugin_id TEXT NOT NULL,
    
    level TEXT NOT NULL CHECK(level IN ('debug', 'info', 'warn', 'error')),
    message TEXT NOT NULL,
    
    -- Structured data
    data JSON,
    
    timestamp INTEGER NOT NULL DEFAULT (unixepoch()),
    
    FOREIGN KEY (plugin_id) REFERENCES plugins(id) ON DELETE CASCADE
) STRICT;

CREATE INDEX idx_plugin_logs_plugin_time ON plugin_logs(plugin_id, timestamp DESC);
CREATE INDEX idx_plugin_logs_level ON plugin_logs(level) WHERE level IN ('warn', 'error');

-- ====================
-- NOTIFICATIONS
-- ====================
-- Notification queue
CREATE TABLE IF NOT EXISTS notifications (
    id TEXT PRIMARY KEY,
    
    -- What triggered it
    event_id TEXT,  -- Optional link to event
    
    -- Notification content
    title TEXT NOT NULL,
    message TEXT NOT NULL,
    thumbnail_path TEXT,
    
    -- Delivery
    channels JSON NOT NULL,  -- ["push", "email", "discord"]
    priority TEXT NOT NULL DEFAULT 'normal' CHECK(priority IN ('low', 'normal', 'high', 'urgent')),
    
    -- Status
    status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'sent', 'failed')),
    sent_at INTEGER,
    error_message TEXT,
    
    -- Metadata
    metadata JSON,
    
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    
    FOREIGN KEY (event_id) REFERENCES events(id) ON DELETE SET NULL
) STRICT;

CREATE INDEX idx_notifications_status ON notifications(status) WHERE status = 'pending';
CREATE INDEX idx_notifications_created ON notifications(created_at DESC);

-- ====================
-- SYSTEM CONFIGURATION
-- ====================
-- Key-value store for system settings
CREATE TABLE IF NOT EXISTS system_config (
    key TEXT PRIMARY KEY,
    value JSON NOT NULL,
    description TEXT,
    
    updated_at INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_by TEXT  -- User ID
) STRICT;

-- ====================
-- AUDIT LOG
-- ====================
-- Audit trail for sensitive operations
CREATE TABLE IF NOT EXISTS audit_log (
    id TEXT PRIMARY KEY,
    
    user_id TEXT,
    ip_address TEXT,
    
    action TEXT NOT NULL,  -- 'camera.add', 'user.delete', 'config.update'
    resource_type TEXT,
    resource_id TEXT,
    
    -- What changed
    changes JSON,  -- {"before": {...}, "after": {...}}
    
    -- Result
    success INTEGER NOT NULL CHECK(success IN (0, 1)),
    error_message TEXT,
    
    timestamp INTEGER NOT NULL DEFAULT (unixepoch()),
    
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL
) STRICT;

CREATE INDEX idx_audit_user_time ON audit_log(user_id, timestamp DESC);
CREATE INDEX idx_audit_action ON audit_log(action);
CREATE INDEX idx_audit_time ON audit_log(timestamp DESC);
CREATE INDEX idx_audit_resource ON audit_log(resource_type, resource_id);

-- ====================
-- CUSTOM OBJECTS
-- ====================
-- User-trained custom object models
CREATE TABLE IF NOT EXISTS custom_objects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    
    -- Model info
    model_type TEXT NOT NULL,  -- 'yolo', 'tensorflow', 'pytorch'
    model_path TEXT NOT NULL,
    
    -- Training metadata
    trained_on INTEGER,  -- Timestamp
    training_samples INTEGER,
    accuracy REAL,
    
    thumbnail_path TEXT,
    
    enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0, 1)),
    
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at INTEGER NOT NULL DEFAULT (unixepoch())
) STRICT;

CREATE INDEX idx_custom_objects_enabled ON custom_objects(enabled) WHERE enabled = 1;

-- Training samples for custom objects
CREATE TABLE IF NOT EXISTS training_samples (
    id TEXT PRIMARY KEY,
    custom_object_id TEXT NOT NULL,
    
    image_path TEXT NOT NULL,
    
    -- Annotation
    bbox JSON NOT NULL,
    label TEXT NOT NULL,
    
    -- Set
    dataset_split TEXT NOT NULL CHECK(dataset_split IN ('train', 'val', 'test')),
    
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    
    FOREIGN KEY (custom_object_id) REFERENCES custom_objects(id) ON DELETE CASCADE
) STRICT;

CREATE INDEX idx_training_samples_object ON training_samples(custom_object_id);
CREATE INDEX idx_training_samples_split ON training_samples(dataset_split);

-- ====================
-- MIGRATIONS TRACKING
-- ====================
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at INTEGER NOT NULL DEFAULT (unixepoch())
) STRICT;

-- Initial migration marker
INSERT INTO schema_migrations (version, name) VALUES (1, 'initial_schema');
```

### Database Access Patterns

**Common Queries (optimized with indexes):**

```sql
-- Get recent events for a camera
SELECT * FROM events 
WHERE camera_id = ? 
AND timestamp BETWEEN ? AND ?
ORDER BY timestamp DESC
LIMIT 100;

-- Get timeline segments with events
SELECT r.*, COUNT(e.id) as event_count
FROM recordings r
LEFT JOIN events e ON e.video_segment_id = r.id
WHERE r.camera_id = ?
AND r.start_time BETWEEN ? AND ?
GROUP BY r.id
ORDER BY r.start_time;

-- Find unknown faces (for labeling)
SELECT f.*, e.camera_id, e.timestamp
FROM faces f
JOIN events e ON f.event_id = e.id
WHERE f.person_id IS NULL
ORDER BY e.timestamp DESC
LIMIT 50;

-- Get person history
SELECT e.*, f.confidence
FROM events e
JOIN faces f ON f.event_id = e.id
WHERE f.person_id = ?
ORDER BY e.timestamp DESC;

-- Find segments to cleanup (retention policy)
SELECT * FROM recordings
WHERE storage_tier = 'hot'
AND end_time < unixepoch() - (86400 * 7)  -- 7 days
AND has_events = 0
ORDER BY end_time
LIMIT 1000;

-- Search events by text
SELECT e.* FROM events e
JOIN events_fts fts ON e.id = fts.event_id
WHERE events_fts MATCH ?
ORDER BY e.timestamp DESC;
```

### Database Maintenance

```sql
-- Run periodically (daily)
VACUUM;
ANALYZE;

-- Cleanup old plugin logs (keep 7 days)
DELETE FROM plugin_logs
WHERE timestamp < unixepoch() - (86400 * 7);

-- Cleanup old audit logs (keep 90 days)
DELETE FROM audit_log
WHERE timestamp < unixepoch() - (86400 * 90);

-- Cleanup old notifications (keep 30 days)
DELETE FROM notifications
WHERE created_at < unixepoch() - (86400 * 30);
```

---

## ⚙️ CONFIGURATION SYSTEM - DETAILED DESIGN

### Configuration Philosophy

**Key Principle:** YAML is the source of truth for configuration, but UI is the primary interface.

**Data Flow:**
```
User edits in UI → API call → Go service → Write YAML + Update SQLite → Hot reload → Apply changes
```

**Why This Approach:**
- **Gitops-friendly** - YAML can be version controlled
- **Portable** - Easy to backup/restore
- **Inspectable** - Advanced users can see/edit everything
- **Programmatic** - Scripts can modify config
- **UI-friendly** - UI generates valid YAML

### Configuration File Structure

```yaml
# config/config.yaml
version: "1.0"

system:
  name: "Home NVR"
  timezone: "America/New_York"
  storage_path: "/data"
  max_storage_gb: 1000
  
  # Database settings
  database:
    type: "sqlite"  # or "postgres"
    path: "/data/nvr.db"
    # For PostgreSQL:
    # host: "localhost"
    # port: 5432
    # database: "nvr"
    # username: "nvr"
    # password: "encrypted:xxx"
  
  # Deployment mode
  deployment:
    mode: "monolithic"  # or "distributed"
    
  # Logging
  logging:
    level: "info"  # debug, info, warn, error
    format: "json"  # json or text
    
cameras:
  # Each camera has a unique ID (generated by UI)
  - id: "front_door_9a8b7c"
    name: "Front Door"
    enabled: true
    
    # Stream configuration
    stream:
      url: "rtsp://192.168.1.100:554/stream"
      username: "admin"
      password: "encrypted:AES256:base64encodedciphertext"
      
      # Authentication
      auth_type: "basic"  # basic, digest, or none
      
    # Camera metadata
    manufacturer: "Reolink"
    model: "RLC-811A"
    location:
      lat: 40.7128
      lon: -74.0060
      description: "Front entrance"
    
    # go2rtc configuration (auto-generated)
    go2rtc:
      streams:
        main: "rtsp://admin:{password}@192.168.1.100:554/stream"
        sub: "rtsp://admin:{password}@192.168.1.100:554/substream"
    
    # Recording settings
    recording:
      enabled: true
      mode: "continuous"  # continuous, motion, events
      
      # Pre/post buffer
      pre_buffer_seconds: 5
      post_buffer_seconds: 5
      
      # Retention
      retention:
        default_days: 30
        events_days: 60  # Keep segments with events longer
        
      # Quality settings
      codec: "h264"  # h264, h265
      resolution: "2560x1920"
      fps: 15
      bitrate: 2048  # kbps, 0 for auto
    
    # AI Detection
    detection:
      enabled: true
      fps: 5  # Analyze 5 frames per second
      
      # Which models to use
      models:
        - "yolo12"  # Object detection
        - "face_recognition"  # Facial recognition
        # - "lpr"  # License plate (opt-in per camera)
      
      # Detection zones (drawn in UI)
      zones:
        - id: "zone_driveway"
          name: "Driveway"
          enabled: true
          # Normalized coordinates (0-1)
          points:
            - [0.0, 0.5]
            - [1.0, 0.5]
            - [1.0, 1.0]
            - [0.0, 1.0]
          
          # What to detect in this zone
          objects:
            - "person"
            - "car"
            - "bicycle"
          
          # Sensitivity
          min_confidence: 0.6
          min_size: 0.02  # Min object size (% of frame)
          
        - id: "zone_street"
          name: "Street"
          enabled: true
          points:
            - [0.0, 0.0]
            - [1.0, 0.0]
            - [1.0, 0.5]
            - [0.0, 0.5]
          objects:
            - "car"
            - "truck"
          min_confidence: 0.7
      
      # Object filters (ignore certain detections)
      filters:
        # Ignore small objects (likely false positives)
        min_area: 2000  # pixels
        
        # Ignore stationary objects for X frames
        stationary_threshold: 100
        
        # Object tracking
        track_objects: true
        max_disappeared: 30  # frames
      
      # Notification settings per camera
      notifications:
        enabled: true
        channels: ["push", "email"]
        
        # Which events trigger notifications
        events:
          person: true
          vehicle: true
          animal: false
          package: true
        
        # Cooldown to prevent spam
        cooldown_seconds: 300
    
    # Audio detection
    audio:
      enabled: true
      detect:
        - "glass_breaking"
        - "smoke_alarm"
        - "dog_bark"
      sensitivity: 0.7
    
    # PTZ settings (if applicable)
    ptz:
      enabled: false
      # presets:
      #   - id: "preset_1"
      #     name: "Front Gate"
      #     pan: 45
      #     tilt: 10
      #     zoom: 2
    
    # Advanced
    advanced:
      # Hardware acceleration
      hwaccel: "auto"  # auto, none, cuda, qsv, videotoolbox
      
      # Network
      timeout_seconds: 10
      retry_attempts: 3
      
      # Timestamp overlay
      timestamp_overlay: true
      timestamp_format: "%Y-%m-%d %H:%M:%S"

# Detection models configuration
detectors:
  yolo12:
    type: "yolo"
    model: "yolov12n.pt"
    device: "cuda"  # cuda, cpu, mps
    confidence: 0.5
    iou: 0.45
    
    # COCO classes to detect (empty = all)
    classes: []
    
  yolo11:
    type: "yolo"
    model: "yolo11n.pt"
    device: "cuda"
    confidence: 0.5
    
  face_recognition:
    type: "insightface"
    model: "buffalo_l"
    device: "cpu"
    det_size: [640, 640]
    confidence: 0.7
    
  lpr:
    type: "paddleocr"
    device: "cpu"
    lang: "en"
    confidence: 0.6
    
    # Country-specific plate formats
    formats:
      - country: "US"
        regex: "^[A-Z0-9]{5,8}$"
      - country: "UK"
        regex: "^[A-Z]{2}[0-9]{2}\\s[A-Z]{3}$"

# State detectors
state_detectors:
  - id: "trash_bin_detector"
    name: "Trash Bin Monitor"
    camera_id: "front_door_9a8b7c"
    type: "position_detector"
    enabled: true
    
    config:
      target_object: "trash_bin"  # Custom trained object
      zones:
        normal: "zone_driveway"
        alert: "zone_street"
      states:
        - id: "in_driveway"
          name: "Bin In Driveway"
          condition: "object in normal zone"
        - id: "at_curb"
          name: "Bin At Curb"
          condition: "object in alert zone"
    
    notifications:
      enabled: true
      on_state_change: true
      states_to_notify: ["at_curb"]

# Plugins configuration
plugins:
  wyze_bridge:
    enabled: true
    config:
      api_key: "encrypted:xxx"
      api_id: "yyy"
      username: "user@example.com"
      password: "encrypted:zzz"
      refresh_interval: 300
  
  reolink:
    enabled: true
    config:
      auto_discover: true
      onvif_port: 8000
  
  home_assistant:
    enabled: false
    config:
      url: "http://homeassistant.local:8123"
      token: "encrypted:xxx"
      webhook_id: "nvr_events"

# Notifications
notifications:
  # Global settings
  enabled: true
  quiet_hours:
    enabled: true
    start: "22:00"
    end: "07:00"
  
  # Channels
  channels:
    push:
      enabled: true
      service: "firebase"  # or "apns"
      # iOS/Android device tokens stored in database
      
    email:
      enabled: true
      smtp:
        host: "smtp.gmail.com"
        port: 587
        username: "notifications@example.com"
        password: "encrypted:xxx"
        from: "NVR System <notifications@example.com>"
        to: ["user@example.com"]
    
    webhook:
      enabled: false
      url: "https://example.com/webhook"
      method: "POST"
      headers:
        Authorization: "Bearer encrypted:xxx"
    
    discord:
      enabled: false
      webhook_url: "encrypted:xxx"

# Storage management
storage:
  # Paths (relative to storage_path)
  recordings: "recordings"
  thumbnails: "thumbnails"
  snapshots: "snapshots"
  exports: "exports"
  
  # Retention policies
  retention:
    default_days: 30
    
    # Tier transition
    tiers:
      hot:
        duration_days: 2
        location: "local"
      warm:
        duration_days: 7
        location: "local"
      cold:
        duration_days: 30
        location: "s3"  # Optional
  
  # S3/MinIO (optional)
  s3:
    enabled: false
    endpoint: "https://s3.amazonaws.com"
    region: "us-east-1"
    bucket: "nvr-recordings"
    access_key: "encrypted:xxx"
    secret_key: "encrypted:xxx"

# Users (passwords stored in database, not here)
users:
  - username: "admin"
    role: "admin"
    email: "admin@example.com"
  
  - username: "viewer"
    role: "user"
    email: "viewer@example.com"

# System preferences
preferences:
  # UI
  ui:
    theme: "dark"  # light, dark, auto
    language: "en"
    
    # Dashboard defaults
    dashboard:
      grid_columns: 3
      show_fps: true
      show_bitrate: false
  
  # Timeline
  timeline:
    default_range_hours: 24
    thumbnail_interval_seconds: 10
    
  # Events
  events:
    auto_acknowledge_after_days: 7
    group_similar_events: true
    group_window_seconds: 300
```

### Configuration Management in Go

```go
// internal/config/config.go
package config

import (
    "os"
    "sync"
    "gopkg.in/yaml.v3"
    "github.com/fsnotify/fsnotify"
)

type Config struct {
    Version string         `yaml:"version"`
    System  SystemConfig   `yaml:"system"`
    Cameras []CameraConfig `yaml:"cameras"`
    // ... other sections
    
    mu       sync.RWMutex
    path     string
    watchers []func(*Config)
}

type SystemConfig struct {
    Name        string `yaml:"name"`
    Timezone    string `yaml:"timezone"`
    StoragePath string `yaml:"storage_path"`
    // ...
}

type CameraConfig struct {
    ID       string               `yaml:"id"`
    Name     string               `yaml:"name"`
    Enabled  bool                 `yaml:"enabled"`
    Stream   StreamConfig         `yaml:"stream"`
    Recording RecordingConfig     `yaml:"recording"`
    Detection DetectionConfig     `yaml:"detection"`
    // ...
}

// Load configuration from file
func Load(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    
    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return nil, err
    }
    
    cfg.path = path
    
    // Decrypt sensitive fields
    if err := cfg.decryptSecrets(); err != nil {
        return nil, err
    }
    
    return &cfg, nil
}

// Save configuration to file
func (c *Config) Save() error {
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    // Encrypt sensitive fields before saving
    cfg := *c
    if err := cfg.encryptSecrets(); err != nil {
        return err
    }
    
    data, err := yaml.Marshal(&cfg)
    if err != nil {
        return err
    }
    
    // Atomic write (write to temp file, then rename)
    tmpPath := c.path + ".tmp"
    if err := os.WriteFile(tmpPath, data, 0600); err != nil {
        return err
    }
    
    return os.Rename(tmpPath, c.path)
}

// Watch for configuration changes
func (c *Config) Watch() error {
    watcher, err := fsnotify.NewWatcher()
    if err != nil {
        return err
    }
    
    go func() {
        for {
            select {
            case event := <-watcher.Events:
                if event.Op&fsnotify.Write == fsnotify.Write {
                    c.reload()
                }
            case err := <-watcher.Errors:
                log.Error("Config watch error:", err)
            }
        }
    }()
    
    return watcher.Add(c.path)
}

// Reload configuration
func (c *Config) reload() {
    newCfg, err := Load(c.path)
    if err != nil {
        log.Error("Failed to reload config:", err)
        return
    }
    
    c.mu.Lock()
    *c = *newCfg
    c.mu.Unlock()
    
    // Notify watchers
    for _, watcher := range c.watchers {
        watcher(c)
    }
}

// Add a watcher callback
func (c *Config) OnChange(fn func(*Config)) {
    c.watchers = append(c.watchers, fn)
}

// Encrypt/decrypt secrets
func (c *Config) encryptSecrets() error {
    key := getEncryptionKey()  // From environment or key file
    
    for i := range c.Cameras {
        if c.Cameras[i].Stream.Password != "" && !strings.HasPrefix(c.Cameras[i].Stream.Password, "encrypted:") {
            encrypted, err := encrypt(key, c.Cameras[i].Stream.Password)
            if err != nil {
                return err
            }
            c.Cameras[i].Stream.Password = "encrypted:" + encrypted
        }
    }
    
    return nil
}

func (c *Config) decryptSecrets() error {
    key := getEncryptionKey()
    
    for i := range c.Cameras {
        if strings.HasPrefix(c.Cameras[i].Stream.Password, "encrypted:") {
            encrypted := strings.TrimPrefix(c.Cameras[i].Stream.Password, "encrypted:")
            decrypted, err := decrypt(key, encrypted)
            if err != nil {
                return err
            }
            c.Cameras[i].Stream.Password = decrypted
        }
    }
    
    return nil
}

// Get a camera by ID
func (c *Config) GetCamera(id string) *CameraConfig {
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    for i := range c.Cameras {
        if c.Cameras[i].ID == id {
            return &c.Cameras[i]
        }
    }
    
    return nil
}

// Add or update a camera
func (c *Config) UpsertCamera(camera CameraConfig) error {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    // Find existing camera
    for i := range c.Cameras {
        if c.Cameras[i].ID == camera.ID {
            c.Cameras[i] = camera
            return c.Save()
        }
    }
    
    // Add new camera
    c.Cameras = append(c.Cameras, camera)
    return c.Save()
}

// Remove a camera
func (c *Config) RemoveCamera(id string) error {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    for i := range c.Cameras {
        if c.Cameras[i].ID == id {
            c.Cameras = append(c.Cameras[:i], c.Cameras[i+1:]...)
            return c.Save()
        }
    }
    
    return fmt.Errorf("camera not found: %s", id)
}
```

### Configuration API Endpoints

```go
// internal/api/config.go

// GET /api/v1/config
func handleGetConfig(w http.ResponseWriter, r *http.Request) {
    cfg := getConfig()  // From context
    
    // Sanitize (remove sensitive data)
    sanitized := sanitizeConfig(cfg)
    
    respondJSON(w, sanitized)
}

// PUT /api/v1/config/cameras/{id}
func handleUpdateCamera(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    
    var camera CameraConfig
    if err := json.NewDecoder(r.Body).Decode(&camera); err != nil {
        respondError(w, http.StatusBadRequest, "Invalid request body")
        return
    }
    
    camera.ID = id
    
    // Validate
    if err := validateCameraConfig(&camera); err != nil {
        respondError(w, http.StatusBadRequest, err.Error())
        return
    }
    
    cfg := getConfig()
    if err := cfg.UpsertCamera(camera); err != nil {
        respondError(w, http.StatusInternalServerError, "Failed to save config")
        return
    }
    
    // Update database (camera status)
    db := getDB()
    if err := updateCameraInDB(db, &camera); err != nil {
        log.Error("Failed to update camera in DB:", err)
    }
    
    // Trigger go2rtc reload
    if err := reloadGo2RTC(); err != nil {
        log.Error("Failed to reload go2rtc:", err)
    }
    
    respondJSON(w, camera)
}

// POST /api/v1/config/cameras/{id}/zones
func handleUpdateZones(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    
    var zones []DetectionZone
    if err := json.NewDecoder(r.Body).Decode(&zones); err != nil {
        respondError(w, http.StatusBadRequest, "Invalid request body")
        return
    }
    
    cfg := getConfig()
    camera := cfg.GetCamera(id)
    if camera == nil {
        respondError(w, http.StatusNotFound, "Camera not found")
        return
    }
    
    camera.Detection.Zones = zones
    
    if err := cfg.UpsertCamera(*camera); err != nil {
        respondError(w, http.StatusInternalServerError, "Failed to save config")
        return
    }
    
    respondJSON(w, zones)
}
```

### Configuration Validation

```go
// internal/config/validation.go
package config

func validateCameraConfig(camera *CameraConfig) error {
    if camera.ID == "" {
        return fmt.Errorf("camera ID is required")
    }
    
    if camera.Name == "" {
        return fmt.Errorf("camera name is required")
    }
    
    if camera.Stream.URL == "" {
        return fmt.Errorf("stream URL is required")
    }
    
    // Validate stream URL format
    if _, err := url.Parse(camera.Stream.URL); err != nil {
        return fmt.Errorf("invalid stream URL: %w", err)
    }
    
    // Validate detection zones
    for _, zone := range camera.Detection.Zones {
        if err := validateZone(&zone); err != nil {
            return fmt.Errorf("invalid zone %s: %w", zone.Name, err)
        }
    }
    
    return nil
}

func validateZone(zone *DetectionZone) error {
    if len(zone.Points) < 3 {
        return fmt.Errorf("zone must have at least 3 points")
    }
    
    // Validate points are normalized (0-1)
    for _, point := range zone.Points {
        if point[0] < 0 || point[0] > 1 || point[1] < 0 || point[1] > 1 {
            return fmt.Errorf("zone points must be normalized (0-1)")
        }
    }
    
    return nil
}
```

**Production Docker Compose:**

```yaml
version: '3.8'

services:
  nvr:
    image: ghcr.io/yourusername/nvr:latest
    restart: unless-stopped
    
    environment:
      - MODE=monolithic
      - DATABASE_TYPE=sqlite
      - TZ=America/New_York
    
    volumes:
      - ./config:/config
      - ./data:/data
      - /etc/localtime:/etc/localtime:ro
    
    ports:
      - "5000:5000"
      - "8554:8554"
      - "1984:1984"
      - "8889:8889"
      - "8189:8189/udp"
    
    devices:
      - /dev/dri:/dev/dri
    
    runtime: nvidia
```

---

## ✅ CHECKLIST FOR EACH FEATURE

Before marking any feature "done":

- [ ] Code written and tested
- [ ] Unit tests pass (80%+ coverage)
- [ ] Integration tests pass
- [ ] API documented (OpenAPI/Swagger)
- [ ] UI implemented (if applicable)
- [ ] Hot-reload working (no restart needed)
- [ ] Error handling complete
- [ ] Logging added
- [ ] Performance acceptable
- [ ] Security reviewed

---

## 🎯 SUCCESS METRICS

Track these metrics:

- **Setup Time**: User can add camera in < 2 minutes via UI
- **Detection Accuracy**: 95%+ for COCO objects
- **Timeline Performance**: < 1 second to load 24 hours
- **Camera Capacity**: Support 20+ cameras on mid-range hardware
- **Plugin Installation**: Via web UI, < 1 minute
- **Configuration**: Zero manual YAML editing required
- **Uptime**: 99.9%+

---

## 🔧 TROUBLESHOOTING GUIDE

**Common Issues:**

1. **go2rtc won't start**
   - Check ports 8554, 1984 available
   - Verify config.yaml syntax
   - Check logs: `docker logs nvr`

2. **Detection service slow**
   - Verify GPU detected: `nvidia-smi`
   - Check model loaded correctly
   - Reduce FPS in camera config

3. **Database locked errors**
   - Enable WAL mode: `PRAGMA journal_mode=WAL;`
   - Check file permissions

---

## 📚 DOCUMENTATION TO CREATE

1. **README.md** - Quick start, features, installation
2. **API.md** - Complete API reference
3. **PLUGINS.md** - Plugin development guide
4. **ARCHITECTURE.md** - System design, diagrams
5. **DEPLOYMENT.md** - Production deployment guide
6. **CONTRIBUTING.md** - How to contribute

---

## 🚀 START NOW

**Your first command should be:**

```bash
# Search for latest versions
web_search "go2rtc latest version 2024"
web_search "YOLOv12 ultralytics 2024"
web_search "InsightFace latest release"

# Then create repository
mkdir nvr-system && cd nvr-system
git init
go mod init github.com/yourusername/nvr
```

**Then follow Week 1, Day 1 instructions above.**

---

## 💡 REMEMBER

- **UI-first**: Every feature must have UI configuration
- **Plugin-ready**: Design for extensibility from day one
- **Test as you go**: Don't defer testing
- **Document as you go**: Keep docs in sync
- **Performance matters**: Profile and optimize early
- **Security first**: Validate inputs, encrypt secrets
- **Hot-reload everything**: No restarts for config changes

---

**NOW GO BUILD SOMETHING AMAZING!** 🚀

---

## 🔔 NOTIFICATION & ALERTING SYSTEM - ADVANCED DESIGN

### Philosophy

The notification system should be **intelligent, not spammy**. It understands:
- **Context** - "Person detected" vs "Known person John arrived home"
- **Relationships** - Track objects across multiple cameras
- **Spatial awareness** - Understand camera layout and object movement patterns
- **User preferences** - Respect quiet hours, notification cooldowns, priority levels
- **Rich content** - Include snapshots, video clips, tracking paths

### Multi-Camera Object Tracking

**The Problem:** Traditional NVRs treat each camera independently. A person walking from the garage to the front door triggers separate "person detected" alerts on each camera.

**Our Solution:** Track objects across cameras using spatial awareness and appearance similarity.

This system provides intelligent notifications like:
- "John got out of his car in the garage and is now walking towards the front door"
- "Unknown person approaching front door from driveway (ETA: 15 seconds)"
- "Package detected at front door for 5 minutes"

For complete implementation details including:
- Spatial layout configuration (camera positions, adjacency, zones)
- Multi-camera tracker with appearance matching
- Intelligent notification engine with contextual messages
- Rich notification delivery (push, email, SMS, webhook, Discord, Telegram)
- Web UI components for live tracking visualization
- API endpoints for tracking and spatial layout management

**See the full detailed implementation above in the expanded instructions section.**

---

## 🔄 HYBRID DEPLOYMENT: Monolithic with Selective Microservice Scaling

### The Problem

**Scenario:** Home user runs monolithic NVR with 15 cameras. AI detection can't keep up - frames are dropping, detection latency is high. User wants to add GPU acceleration for detection service only, without migrating entire system to microservices.

**Traditional Approach:** Either suffer with slow detection OR completely redesign deployment to full microservices.

**Our Approach:** Allow selective offloading of individual services while keeping monolithic deployment.

### Architecture: Service Abstraction Layer

**Key Concept:** Every service communicates through an abstract interface that works both in-process (monolithic) and out-of-process (RPC).

```go
// internal/services/interface.go
package services

// DetectionService interface (same for local and remote)
type DetectionService interface {
    Detect(ctx context.Context, req *DetectionRequest) (*DetectionResponse, error)
    Health(ctx context.Context) error
}

// ServiceLocator resolves services (local or remote)
type ServiceLocator struct {
    detectionService DetectionService
    faceService      FaceService
    lprService       LPRService
    // ... other services
}

func (sl *ServiceLocator) Detection() DetectionService {
    return sl.detectionService
}

// NewServiceLocator creates locator based on config
func NewServiceLocator(cfg *Config) *ServiceLocator {
    sl := &ServiceLocator{}
    
    // Check if detection service should be remote
    if cfg.Services.Detection.Mode == "remote" {
        // Connect to remote gRPC service
        sl.detectionService = NewRemoteDetectionService(cfg.Services.Detection.Address)
    } else {
        // Use local implementation
        sl.detectionService = NewLocalDetectionService()
    }
    
    // Same pattern for other services
    
    return sl
}
```

### Configuration for Hybrid Deployment

```yaml
# config/config.yaml

system:
  deployment:
    mode: "hybrid"  # monolithic, hybrid, or distributed
    
services:
  # Detection service - offloaded to separate container with GPU
  detection:
    mode: "remote"  # local or remote
    address: "detection-service:50051"  # gRPC address
    protocol: "grpc"
    
    # Health check
    health_check:
      enabled: true
      interval_seconds: 30
      timeout_seconds: 5
      
    # Failover (fall back to local if remote unavailable)
    failover:
      enabled: true
      fallback_to_local: false  # Don't fallback (we need GPU)
      
  # All other services run locally in monolithic container
  face_recognition:
    mode: "local"
    
  lpr:
    mode: "local"
    
  events:
    mode: "local"
```

### Docker Compose - Hybrid Deployment

```yaml
# docker-compose.hybrid.yml
version: '3.8'

services:
  # Main monolithic container (everything except detection)
  nvr:
    image: nvr:latest
    restart: unless-stopped
    
    environment:
      - MODE=hybrid
      - DETECTION_SERVICE=detection:50051
    
    volumes:
      - ./config:/config
      - ./data:/data
    
    ports:
      - "5000:5000"
      - "8554:8554"
      - "1984:1984"
    
    depends_on:
      - detection
  
  # Detection service - separate container with GPU
  detection:
    image: nvr-detection:latest
    restart: unless-stopped
    
    runtime: nvidia
    
    environment:
      - CUDA_VISIBLE_DEVICES=0
      - MODEL=yolov12n
      - BATCH_SIZE=4
    
    # Only expose gRPC port internally
    expose:
      - "50051"
    
    # No volume mounts needed - stateless service
    
    # Resource limits
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: 1
              capabilities: [gpu]
```

### Easy User Experience - Web UI

**UI Flow:**

1. User notices detection is slow
2. Opens Settings → Services
3. Sees list of all services with status:
   ```
   ┌─────────────────────────────────────────┐
   │ Service Management                       │
   ├─────────────────────────────────────────┤
   │ ✓ Camera Management     [Local]  Healthy│
   │ ⚠ AI Detection          [Local]  Slow    │
   │   ↳ Latency: 850ms (target: 200ms)      │
   │   ↳ Queue depth: 45 frames               │
   │ ✓ Face Recognition      [Local]  Healthy│
   │ ✓ Event Management      [Local]  Healthy│
   └─────────────────────────────────────────┘
   ```

4. Clicks "Scale" button next to AI Detection
5. UI shows options:
   ```
   Scale AI Detection Service
   
   ○ Add more resources to current container
   ● Offload to dedicated service container
   ○ Add multiple detection workers
   
   Configuration:
   [x] Use GPU (recommended)
   [ ] Use CPU only
   
   Container resources:
   CPUs: [2] cores
   Memory: [4] GB
   GPU: [NVIDIA GeForce RTX 3060]
   
   [Generate Docker Compose] [Done]
   ```

6. UI generates docker-compose.hybrid.yml
7. User runs: `docker-compose -f docker-compose.hybrid.yml up -d detection`
8. System automatically reconnects and offloads detection workload

### Implementation: Dual-Mode Service

```go
// services/detection/service.go
package detection

// Local implementation (runs in monolithic container)
type LocalDetectionService struct {
    model *yolo.Model
}

func NewLocalDetectionService() *LocalDetectionService {
    return &LocalDetectionService{
        model: yolo.LoadModel("yolov12n.pt"),
    }
}

func (s *LocalDetectionService) Detect(ctx context.Context, req *DetectionRequest) (*DetectionResponse, error) {
    // Run detection locally
    results, err := s.model.Detect(req.ImageData)
    if err != nil {
        return nil, err
    }
    
    return &DetectionResponse{
        Detections: results,
    }, nil
}

// Remote implementation (connects to external service)
type RemoteDetectionService struct {
    client pb.DetectionServiceClient
    conn   *grpc.ClientConn
}

func NewRemoteDetectionService(address string) *RemoteDetectionService {
    conn, err := grpc.Dial(address, grpc.WithInsecure())
    if err != nil {
        log.Fatal("Failed to connect to detection service:", err)
    }
    
    return &RemoteDetectionService{
        client: pb.NewDetectionServiceClient(conn),
        conn:   conn,
    }
}

func (s *RemoteDetectionService) Detect(ctx context.Context, req *DetectionRequest) (*DetectionResponse, error) {
    // Call remote gRPC service
    resp, err := s.client.Detect(ctx, &pb.DetectionRequest{
        ImageData: req.ImageData,
        Confidence: req.Confidence,
    })
    
    if err != nil {
        return nil, err
    }
    
    return &DetectionResponse{
        Detections: resp.Detections,
    }, nil
}

// gRPC server (for remote service)
type DetectionServer struct {
    pb.UnimplementedDetectionServiceServer
    local *LocalDetectionService
}

func (s *DetectionServer) Detect(ctx context.Context, req *pb.DetectionRequest) (*pb.DetectionResponse, error) {
    // Delegate to local implementation
    resp, err := s.local.Detect(ctx, &DetectionRequest{
        ImageData:  req.ImageData,
        Confidence: req.Confidence,
    })
    
    if err != nil {
        return nil, err
    }
    
    return &pb.DetectionResponse{
        Detections: resp.Detections,
    }, nil
}
```

### Detection Service Standalone Container

```dockerfile
# Dockerfile.detection
FROM nvidia/cuda:12.0-runtime-ubuntu22.04

# Install Python and dependencies
RUN apt-get update && apt-get install -y \
    python3.11 \
    python3-pip \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Install Python packages
COPY services/detection/requirements.txt .
RUN pip3 install -r requirements.txt

# Copy service code
COPY services/detection/ .
COPY proto/ ./proto/

# gRPC port
EXPOSE 50051

# Health check
HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD python3 -c "import grpc; grpc.channel_ready_future(grpc.insecure_channel('localhost:50051')).result(timeout=5)"

CMD ["python3", "grpc_server.py"]
```

### Automatic Failover

```go
// internal/services/detection_with_failover.go
package services

type DetectionServiceWithFailover struct {
    primary   DetectionService
    secondary DetectionService  // Optional fallback
    
    // Circuit breaker
    failures     int
    lastFailure  time.Time
    circuitOpen  bool
}

func (s *DetectionServiceWithFailover) Detect(ctx context.Context, req *DetectionRequest) (*DetectionResponse, error) {
    // Check circuit breaker
    if s.circuitOpen {
        if time.Since(s.lastFailure) > 30*time.Second {
            // Try to recover
            s.circuitOpen = false
            s.failures = 0
        } else if s.secondary != nil {
            // Use fallback
            log.Warn("Circuit open, using fallback detection service")
            return s.secondary.Detect(ctx, req)
        }
    }
    
    // Try primary service
    resp, err := s.primary.Detect(ctx, req)
    
    if err != nil {
        s.failures++
        s.lastFailure = time.Now()
        
        if s.failures >= 3 {
            s.circuitOpen = true
            log.Error("Detection service circuit breaker opened after 3 failures")
        }
        
        // Try fallback if available
        if s.secondary != nil {
            log.Warn("Primary detection failed, using fallback")
            return s.secondary.Detect(ctx, req)
        }
        
        return nil, err
    }
    
    // Success - reset failure count
    if s.failures > 0 {
        s.failures = 0
    }
    
    return resp, nil
}
```

### Scaling Multiple Workers

```yaml
# docker-compose.scale.yml
version: '3.8'

services:
  nvr:
    image: nvr:latest
    environment:
      - MODE=hybrid
      # Load balance across multiple detection workers
      - DETECTION_SERVICES=detection-1:50051,detection-2:50051,detection-3:50051
    
  # Multiple detection workers for higher throughput
  detection-1:
    image: nvr-detection:latest
    runtime: nvidia
    environment:
      - CUDA_VISIBLE_DEVICES=0
    
  detection-2:
    image: nvr-detection:latest
    runtime: nvidia
    environment:
      - CUDA_VISIBLE_DEVICES=0
      
  detection-3:
    image: nvr-detection:latest
    runtime: nvidia
    environment:
      - CUDA_VISIBLE_DEVICES=0
```

**Load Balancer in Go:**

```go
// internal/services/detection_load_balancer.go
package services

type LoadBalancedDetectionService struct {
    backends []DetectionService
    current  int
    mu       sync.Mutex
}

func NewLoadBalancedDetectionService(addresses []string) *LoadBalancedDetectionService {
    backends := make([]DetectionService, len(addresses))
    for i, addr := range addresses {
        backends[i] = NewRemoteDetectionService(addr)
    }
    
    return &LoadBalancedDetectionService{
        backends: backends,
    }
}

func (s *LoadBalancedDetectionService) Detect(ctx context.Context, req *DetectionRequest) (*DetectionResponse, error) {
    // Round-robin load balancing
    s.mu.Lock()
    backend := s.backends[s.current]
    s.current = (s.current + 1) % len(s.backends)
    s.mu.Unlock()
    
    return backend.Detect(ctx, req)
}
```

### Benefits of This Approach

1. **Start Simple:** Users deploy monolithic by default
2. **Scale What You Need:** Only offload bottleneck services
3. **No Rewrite:** Same code works in both modes
4. **Easy Rollback:** Can always go back to monolithic
5. **Gradual Migration:** Can eventually move everything to microservices
6. **Resource Efficient:** Don't run separate containers for services that don't need it
7. **GPU Isolation:** Detection service gets dedicated GPU without affecting other services
8. **Independent Scaling:** Can run 1 main container + 5 detection workers
9. **Easy Troubleshooting:** Services can be tested independently

### Monitoring Service Health

```go
// internal/monitoring/service_health.go
package monitoring

type ServiceHealth struct {
    Name          string
    Mode          string  // "local" or "remote"
    Status        string  // "healthy", "degraded", "down"
    Latency       time.Duration
    ErrorRate     float64
    RequestRate   float64
    QueueDepth    int
    LastCheckTime time.Time
}

func (m *Monitor) CheckServiceHealth(service DetectionService) *ServiceHealth {
    start := time.Now()
    
    err := service.Health(context.Background())
    latency := time.Since(start)
    
    health := &ServiceHealth{
        Name:          "detection",
        Latency:       latency,
        LastCheckTime: time.Now(),
    }
    
    if err != nil {
        health.Status = "down"
    } else if latency > 200*time.Millisecond {
        health.Status = "degraded"
    } else {
        health.Status = "healthy"
    }
    
    return health
}
```

This hybrid approach gives users the best of both worlds:
- **Simplicity** of monolithic deployment
- **Scalability** of microservices
- **Flexibility** to scale only what they need
- **No lock-in** - can easily change deployment strategy

---

## 🧪 TESTING REQUIREMENTS

**CRITICAL: All code changes must maintain test coverage and quality standards.**

### Mandatory Testing Protocol

After EVERY code change, the following must be executed:

#### 1. Unit Tests
```bash
# Backend (Go) - Run all tests with coverage
cd /Users/joshua.seidel/nvr-prototype && go test ./... -cover

# Frontend (React/TypeScript) - Run all tests with coverage
cd /Users/joshua.seidel/nvr-prototype/web-ui && npm test -- --coverage
```

#### 2. Code Coverage Requirements
- **Minimum coverage: 80%** for all packages
- Coverage must be checked after every change
- New code must include tests that maintain or improve coverage

#### 3. Regression Testing
- All existing tests must pass before committing
- Any failing tests must be fixed before moving on
- Do not skip tests or mark them as pending without explicit approval

### Testing Best Practices

**Backend (Go):**
- Use table-driven tests for comprehensive coverage
- Mock external dependencies (database, HTTP clients, file system)
- Test error paths as thoroughly as happy paths
- Use `testify/assert` for clear assertions
- Include integration tests for API handlers

**Frontend (React):**
- Use React Testing Library with Vitest
- Mock API calls with MSW (Mock Service Worker)
- Test component rendering, user interactions, and state changes
- Include accessibility testing where applicable
- Wrap components with necessary providers (QueryClient, Router, Toast, etc.)

### Pre-Commit Checklist
Before committing any code:
1. ✅ Run `go test ./... -cover` - all tests pass, coverage >= 80%
2. ✅ Run `npm test -- --coverage` in web-ui - all tests pass, coverage >= 80%
3. ✅ Run `go build ./...` - code compiles without errors
4. ✅ Run linter if available
5. ✅ Verify no regressions in existing functionality

### Coverage Commands Reference
```bash
# Detailed Go coverage report by package
go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out

# Generate HTML coverage report
go test ./... -coverprofile=coverage.out && go tool cover -html=coverage.out -o coverage.html

# Frontend detailed coverage
cd web-ui && npm test -- --coverage --reporter=verbose
```

### Test File Organization
- Backend: `*_test.go` files alongside source files
- Frontend: `*.test.tsx` files alongside components, or in `__tests__` directories
- Shared test utilities: `internal/testutil/` (Go), `src/test/` (React)

---

## 📦 WEEK 4.5: PLUGIN-BASED ARCHITECTURE CONVERSION

**Goal:** Convert the NVR system to a fully plugin-based architecture where all features (including core services) are plugins that can be updated independently without rebuilding the main container.

### Architecture Philosophy

Following the Scrypted model where even stock features are plugins:

```
┌─────────────────────────────────────────────────────────────────────┐
│                         NVR CORE (Minimal)                          │
│  • Plugin Loader & Lifecycle Manager                                │
│  • Event Bus (NATS embedded)                                        │
│  • Config Manager                                                   │
│  • Health Monitor                                                   │
│  • SQLite Database                                                  │
│  • API Gateway (routes to plugin APIs)                              │
│  Size: ~15MB binary                                                 │
└─────────────────────────────────────────────────────────────────────┘
                                    ↓ loads ↓
┌─────────────────────────────────────────────────────────────────────┐
│                         CORE PLUGINS (Auto-Install)                  │
├──────────────┬──────────────┬──────────────┬──────────────┬─────────┤
│  Recording   │  Streaming   │  Detection   │   Events     │   API   │
│   Plugin     │   Plugin     │   Plugin     │   Plugin     │  Plugin │
│  (Go/gRPC)   │  (go2rtc)    │  (Python)    │  (Go)        │  (Go)   │
└──────────────┴──────────────┴──────────────┴──────────────┴─────────┘
┌─────────────────────────────────────────────────────────────────────┐
│                      CAMERA PLUGINS (User-Installed)                 │
├──────────────┬──────────────┬──────────────┬──────────────┬─────────┤
│   Reolink    │    Wyze      │    ONVIF     │  Frigate     │  Custom │
│   Plugin     │   Plugin     │   Plugin     │   Import     │ Plugins │
└──────────────┴──────────────┴──────────────┴──────────────┴─────────┘
┌─────────────────────────────────────────────────────────────────────┐
│                     INTEGRATION PLUGINS (Optional)                   │
├──────────────┬──────────────┬──────────────┬──────────────┬─────────┤
│    Home      │   HomeKit    │    MQTT      │   Telegram   │  Webhook│
│  Assistant   │   Bridge     │   Client     │    Bot       │  Notify │
└──────────────┴──────────────┴──────────────┴──────────────┴─────────┘
```

### Core Plugin Types

#### 1. **Critical Plugins** (Auto-installed, cannot be disabled)
These are essential for NVR operation:
- `nvr-core-api` - REST API endpoints
- `nvr-core-events` - Event storage and broadcasting
- `nvr-core-config` - Configuration management

#### 2. **Core Plugins** (Auto-installed, can be disabled)
These ship with the system but can be individually updated:
- `nvr-recording` - Video recording and storage
- `nvr-streaming` - go2rtc integration for live streams
- `nvr-detection` - AI object/face/LPR detection
- `nvr-timeline` - Timeline and playback

#### 3. **Camera Plugins** (Pre-installed or user-installed)
- `nvr-camera-reolink` - Reolink camera support
- `nvr-camera-wyze` - Wyze camera support (requires Wyze Bridge)
- `nvr-camera-onvif` - Generic ONVIF support

#### 4. **Integration Plugins** (User-installed)
- `nvr-integration-homeassistant`
- `nvr-integration-homekit`
- `nvr-integration-mqtt`
- `nvr-integration-notifications`

### Implementation Plan

#### Step 4.5.1: Create Plugin Interface Contracts

```go
// internal/plugin/contracts/service.go
package contracts

import (
    "context"
)

// ServicePlugin is the base interface for all service plugins
type ServicePlugin interface {
    PluginBase

    // Dependencies returns plugin IDs this plugin depends on
    Dependencies() []string

    // Routes returns HTTP routes this plugin provides (mounted at /api/v1/plugins/{id}/)
    Routes() http.Handler

    // EventSubscriptions returns event types this plugin wants to receive
    EventSubscriptions() []string

    // HandleEvent processes an event from the event bus
    HandleEvent(ctx context.Context, event Event) error
}

// CriticalPlugin cannot be disabled or uninstalled
type CriticalPlugin interface {
    ServicePlugin
    IsCritical() bool // Always returns true
}

// RecordingPlugin provides video recording capabilities
type RecordingPlugin interface {
    ServicePlugin

    StartRecording(ctx context.Context, cameraID string) error
    StopRecording(ctx context.Context, cameraID string) error
    GetSegments(ctx context.Context, cameraID string, start, end time.Time) ([]Segment, error)
    GetTimeline(ctx context.Context, cameraID string, start, end time.Time) (*Timeline, error)
    ExportVideo(ctx context.Context, cameraID string, start, end time.Time) (io.ReadCloser, error)
}

// StreamingPlugin provides live video streaming
type StreamingPlugin interface {
    ServicePlugin

    AddStream(ctx context.Context, cameraID, streamURL string) error
    RemoveStream(ctx context.Context, cameraID string) error
    GetStreamURL(ctx context.Context, cameraID string, format StreamFormat) (string, error)
    GetSnapshot(ctx context.Context, cameraID string) ([]byte, error)
}

// DetectionPlugin provides AI detection capabilities
type DetectionPlugin interface {
    ServicePlugin

    StartDetection(ctx context.Context, cameraID string, config DetectionConfig) error
    StopDetection(ctx context.Context, cameraID string) error
    GetDetectionStatus(ctx context.Context, cameraID string) (*DetectionStatus, error)
    ConfigureZones(ctx context.Context, cameraID string, zones []Zone) error
}
```

#### Step 4.5.2: Implement Plugin Loader with Auto-Install

```go
// internal/core/loader.go
package core

// PluginLoader handles discovery, installation, and lifecycle of plugins
type PluginLoader struct {
    pluginsDir     string
    bundledPlugins map[string][]byte  // Embedded plugin binaries
    registry       *PluginRegistry
    eventBus       *EventBus
    logger         *slog.Logger
}

// BundledCorePlugins are compiled into the binary but run as separate processes
var BundledCorePlugins = []string{
    "nvr-core-api",
    "nvr-core-events",
    "nvr-core-config",
    "nvr-recording",
    "nvr-streaming",
    "nvr-detection",
    "nvr-camera-reolink",
    "nvr-camera-wyze",
}

func (l *PluginLoader) Start(ctx context.Context) error {
    // 1. Ensure core plugins are installed
    if err := l.ensureCorePlugins(ctx); err != nil {
        return fmt.Errorf("failed to install core plugins: %w", err)
    }

    // 2. Scan for all installed plugins
    plugins, err := l.scanPlugins()
    if err != nil {
        return fmt.Errorf("failed to scan plugins: %w", err)
    }

    // 3. Build dependency graph and start in correct order
    order := l.topologicalSort(plugins)

    // 4. Start plugins
    for _, pluginID := range order {
        if err := l.startPlugin(ctx, pluginID); err != nil {
            if l.isCritical(pluginID) {
                return fmt.Errorf("critical plugin failed to start: %s: %w", pluginID, err)
            }
            l.logger.Error("Plugin failed to start", "id", pluginID, "error", err)
        }
    }

    return nil
}

func (l *PluginLoader) ensureCorePlugins(ctx context.Context) error {
    for _, pluginID := range BundledCorePlugins {
        installed, version := l.isInstalled(pluginID)
        bundledVersion := l.getBundledVersion(pluginID)

        if !installed || version != bundledVersion {
            l.logger.Info("Installing/updating core plugin",
                "id", pluginID,
                "version", bundledVersion)
            if err := l.installBundled(ctx, pluginID); err != nil {
                return err
            }
        }
    }
    return nil
}
```

#### Step 4.5.3: Convert Recording Service to Plugin

**Before (internal/recording/service.go):**
```go
// Tightly coupled to main binary
type Service struct {
    config    *config.Config
    db        *sql.DB
    // ...
}
```

**After (plugins/nvr-recording/main.go):**
```go
package main

import (
    "github.com/nvr-system/nvr/sdk"
)

type RecordingPlugin struct {
    sdk.BasePlugin

    db        *sql.DB
    config    RecordingConfig
    recorders map[string]*Recorder
}

func (p *RecordingPlugin) Manifest() sdk.PluginManifest {
    return sdk.PluginManifest{
        ID:          "nvr-recording",
        Name:        "Recording Service",
        Version:     "1.0.0",
        Description: "Core video recording and storage",
        Category:    "core",
        Critical:    false, // Can be disabled if user doesn't want recording
        Dependencies: []string{
            "nvr-streaming", // Needs streams to record
        },
        Capabilities: []string{
            "recording",
            "playback",
            "export",
            "timeline",
        },
        ConfigSchema: recordingConfigSchema,
    }
}

func (p *RecordingPlugin) Start(ctx context.Context) error {
    // Subscribe to camera events to auto-start recording
    p.SubscribeEvents("camera.added", "camera.removed", "camera.updated")

    // Start recording for all configured cameras
    cameras := p.GetConfig().Cameras
    for _, cam := range cameras {
        if cam.Recording.Enabled {
            go p.startRecorder(ctx, cam.ID)
        }
    }

    return nil
}

func (p *RecordingPlugin) Routes() http.Handler {
    r := chi.NewRouter()
    r.Get("/segments", p.handleListSegments)
    r.Get("/segments/{id}", p.handleGetSegment)
    r.Post("/export", p.handleExport)
    r.Get("/timeline/{cameraId}", p.handleTimeline)
    // ... more routes
    return r
}

func main() {
    sdk.RunPlugin(&RecordingPlugin{})
}
```

#### Step 4.5.4: Plugin SDK for Easy Development

```go
// sdk/plugin.go
package sdk

import (
    "context"
    "encoding/json"
    "net"
    "net/http"
    "os"

    "github.com/nats-io/nats.go"
)

// RunPlugin is the main entry point for plugins
func RunPlugin(p Plugin) {
    // Read config from environment/stdin
    config := readConfig()

    // Connect to NATS event bus
    nc, err := nats.Connect(config.NATSUrl)
    if err != nil {
        log.Fatal("Failed to connect to NATS:", err)
    }

    // Create plugin runtime
    runtime := &PluginRuntime{
        plugin: p,
        nats:   nc,
        config: config,
    }

    // Initialize plugin
    if err := p.Initialize(context.Background(), runtime); err != nil {
        log.Fatal("Failed to initialize plugin:", err)
    }

    // Start plugin
    if err := p.Start(context.Background()); err != nil {
        log.Fatal("Failed to start plugin:", err)
    }

    // Start HTTP server for plugin API
    listener, err := net.Listen("unix", config.SocketPath)
    if err != nil {
        log.Fatal("Failed to create socket:", err)
    }

    http.Serve(listener, p.Routes())
}

// BasePlugin provides common functionality for all plugins
type BasePlugin struct {
    runtime *PluginRuntime
}

func (p *BasePlugin) PublishEvent(eventType string, data interface{}) error {
    return p.runtime.nats.Publish("events."+eventType, marshal(data))
}

func (p *BasePlugin) SubscribeEvents(eventTypes ...string) {
    for _, et := range eventTypes {
        p.runtime.nats.Subscribe("events."+et, func(m *nats.Msg) {
            p.HandleEvent(context.Background(), parseEvent(m))
        })
    }
}

func (p *BasePlugin) GetConfig() *Config {
    return p.runtime.config
}

func (p *BasePlugin) CallPlugin(pluginID, method string, args interface{}) (interface{}, error) {
    // RPC to other plugins via NATS request-reply
    resp, err := p.runtime.nats.Request(
        fmt.Sprintf("plugins.%s.%s", pluginID, method),
        marshal(args),
        5*time.Second,
    )
    if err != nil {
        return nil, err
    }
    return unmarshal(resp.Data), nil
}
```

#### Step 4.5.5: Event Bus with NATS

```go
// internal/core/eventbus.go
package core

import (
    "github.com/nats-io/nats-server/v2/server"
    "github.com/nats-io/nats.go"
)

// EventBus provides pub/sub messaging between plugins
type EventBus struct {
    server *server.Server  // Embedded NATS server
    conn   *nats.Conn
}

// NewEventBus creates an embedded NATS server for plugin communication
func NewEventBus(dataDir string) (*EventBus, error) {
    opts := &server.Options{
        Host:      "127.0.0.1",
        Port:      4222,
        NoSigs:    true,
        JetStream: true,
        StoreDir:  filepath.Join(dataDir, "nats"),
    }

    ns, err := server.NewServer(opts)
    if err != nil {
        return nil, err
    }

    go ns.Start()

    // Wait for server to be ready
    if !ns.ReadyForConnections(5 * time.Second) {
        return nil, fmt.Errorf("NATS server not ready")
    }

    // Connect to embedded server
    nc, err := nats.Connect(ns.ClientURL())
    if err != nil {
        return nil, err
    }

    return &EventBus{
        server: ns,
        conn:   nc,
    }, nil
}

// Event types for plugin communication
const (
    EventCameraAdded     = "camera.added"
    EventCameraRemoved   = "camera.removed"
    EventCameraUpdated   = "camera.updated"
    EventDetection       = "detection.detected"
    EventRecordingStart  = "recording.started"
    EventRecordingStop   = "recording.stopped"
    EventPluginStarted   = "plugin.started"
    EventPluginStopped   = "plugin.stopped"
    EventConfigChanged   = "config.changed"
)
```

#### Step 4.5.6: Hot-Reload Plugin Updates

```go
// internal/core/updater.go
package core

// PluginUpdater handles live plugin updates without system restart
type PluginUpdater struct {
    loader   *PluginLoader
    eventBus *EventBus
}

func (u *PluginUpdater) UpdatePlugin(ctx context.Context, pluginID string, newVersion string) error {
    plugin := u.loader.GetPlugin(pluginID)
    if plugin == nil {
        return fmt.Errorf("plugin not found: %s", pluginID)
    }

    // 1. Download new version
    newBinary, err := u.downloadVersion(pluginID, newVersion)
    if err != nil {
        return err
    }

    // 2. Validate new version
    if err := u.validatePlugin(newBinary); err != nil {
        return err
    }

    // 3. Graceful shutdown of old version
    //    - Stop accepting new requests
    //    - Wait for in-flight requests to complete (max 30s)
    //    - Save state to shared storage
    u.eventBus.Publish(EventPluginStopping, pluginID)
    if err := plugin.GracefulStop(30 * time.Second); err != nil {
        u.logger.Warn("Plugin did not stop gracefully", "id", pluginID, "error", err)
        plugin.ForceStop()
    }

    // 4. Replace binary
    if err := u.replaceBinary(pluginID, newBinary); err != nil {
        // Rollback - restart old version
        u.loader.startPlugin(ctx, pluginID)
        return err
    }

    // 5. Start new version
    if err := u.loader.startPlugin(ctx, pluginID); err != nil {
        // Rollback - restore and restart old version
        u.restoreOldVersion(pluginID)
        u.loader.startPlugin(ctx, pluginID)
        return err
    }

    // 6. Health check new version
    if err := u.healthCheck(pluginID); err != nil {
        // Rollback
        u.rollback(ctx, pluginID)
        return err
    }

    u.eventBus.Publish(EventPluginUpdated, map[string]string{
        "id":          pluginID,
        "old_version": plugin.Version(),
        "new_version": newVersion,
    })

    return nil
}
```

#### Step 4.5.7: New Main Binary (Minimal Core)

```go
// cmd/nvr/main.go - New minimal version
package main

import (
    "context"
    "log/slog"
    "os"
    "os/signal"
    "syscall"

    "github.com/nvr-system/nvr/internal/core"
)

func main() {
    logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
    slog.SetDefault(logger)

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Initialize paths
    dataDir := getEnv("DATA_PATH", "/data")
    pluginsDir := getEnv("PLUGINS_PATH", "/plugins")

    // Initialize SQLite database (shared by all plugins)
    db, err := core.OpenDatabase(filepath.Join(dataDir, "nvr.db"))
    if err != nil {
        slog.Error("Failed to open database", "error", err)
        os.Exit(1)
    }
    defer db.Close()

    // Initialize embedded NATS event bus
    eventBus, err := core.NewEventBus(dataDir)
    if err != nil {
        slog.Error("Failed to start event bus", "error", err)
        os.Exit(1)
    }
    defer eventBus.Stop()

    // Initialize plugin loader
    loader := core.NewPluginLoader(pluginsDir, eventBus, logger)

    // Start all plugins (including auto-installing core plugins)
    if err := loader.Start(ctx); err != nil {
        slog.Error("Failed to start plugins", "error", err)
        os.Exit(1)
    }

    // Initialize API gateway (routes to plugins)
    gateway := core.NewAPIGateway(loader, eventBus)

    // Start HTTP server
    go func() {
        addr := getEnv("LISTEN_ADDR", ":5000")
        slog.Info("Starting API gateway", "addr", addr)
        if err := gateway.ListenAndServe(addr); err != nil {
            slog.Error("Server error", "error", err)
            cancel()
        }
    }()

    // Graceful shutdown
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan

    slog.Info("Shutting down...")
    cancel()
    loader.StopAll()
    slog.Info("Shutdown complete")
}
```

### Directory Structure After Conversion

```
nvr-prototype/
├── cmd/
│   └── nvr/
│       └── main.go              # Minimal core binary (~15MB)
├── internal/
│   └── core/
│       ├── loader.go            # Plugin loader
│       ├── eventbus.go          # NATS event bus
│       ├── gateway.go           # API gateway
│       ├── registry.go          # Plugin registry
│       └── database.go          # Shared database
├── sdk/
│   ├── plugin.go                # Plugin SDK
│   ├── types.go                 # Shared types
│   └── config.go                # Config helpers
├── plugins/
│   ├── nvr-core-api/
│   │   ├── main.go
│   │   ├── handlers.go
│   │   └── manifest.yaml
│   ├── nvr-core-events/
│   │   ├── main.go
│   │   └── manifest.yaml
│   ├── nvr-recording/
│   │   ├── main.go
│   │   ├── recorder.go
│   │   ├── segment.go
│   │   └── manifest.yaml
│   ├── nvr-streaming/
│   │   ├── main.go
│   │   ├── go2rtc.go
│   │   └── manifest.yaml
│   ├── nvr-detection/
│   │   ├── main.py              # Python plugin
│   │   ├── requirements.txt
│   │   └── manifest.yaml
│   ├── nvr-camera-reolink/
│   │   ├── main.go
│   │   └── manifest.yaml
│   └── nvr-camera-wyze/
│       ├── main.go
│       └── manifest.yaml
├── web-ui/                      # Frontend (unchanged)
└── docker/
    ├── Dockerfile               # Core + bundled plugins
    ├── Dockerfile.plugin-dev    # For building plugins
    └── docker-compose.yml
```

### Migration Path

1. **Phase 1: Create Plugin SDK** (Day 1-2)
   - Define plugin interfaces
   - Create base plugin implementation
   - Add NATS embedded server

2. **Phase 2: Convert Recording Service** (Day 2-3)
   - Move to plugins/nvr-recording
   - Implement plugin interface
   - Test hot-reload

3. **Phase 3: Convert Streaming Service** (Day 3-4)
   - Move go2rtc management to plugin
   - Implement plugin interface

4. **Phase 4: Convert Detection Service** (Day 4-5)
   - Already external, just need plugin wrapper
   - Add NATS communication

5. **Phase 5: Convert Camera Plugins** (Day 5-6)
   - Move Reolink/Wyze to standalone plugins
   - Update manifest format

6. **Phase 6: New Core Binary** (Day 6-7)
   - Implement minimal main.go
   - Add API gateway
   - Bundle core plugins

7. **Phase 7: Testing & Documentation** (Day 7-8)
   - Integration tests
   - Update docker builds
   - Document plugin development

### Configuration Changes

```yaml
# config.yaml - Plugin configuration
plugins:
  nvr-recording:
    enabled: true
    config:
      segment_duration: 10
      retention_days: 30

  nvr-streaming:
    enabled: true
    config:
      go2rtc_port: 1984

  nvr-detection:
    enabled: true
    config:
      default_fps: 5
      min_confidence: 0.5

  nvr-camera-reolink:
    enabled: true

  nvr-camera-wyze:
    enabled: false  # Requires Wyze Bridge
```

### Benefits of This Architecture

1. **Zero-Downtime Updates**: Individual plugins can be updated without affecting others
2. **Modular**: Users can disable features they don't need
3. **Extensible**: Third-party plugins can be easily added
4. **Testable**: Each plugin can be tested in isolation
5. **Scalable**: Detection plugin can run on separate machines
6. **Recoverable**: Failed plugins don't crash the whole system

---

## 🎨 WEEK 4.5 PART 2: UI PLUGIN ARCHITECTURE

**Goal:** Make the frontend UI modular with a core shell and plugin-based pages that can be hot-reloaded without rebuilding the main application.

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                      UI CORE SHELL (~150KB)                          │
│  • App shell (header, sidebar, theme provider)                      │
│  • Plugin registry & dynamic loader                                 │
│  • Shared component library (@nvr/ui-components)                    │
│  • API client & WebSocket manager                                   │
│  • Route registration system                                        │
│  • Authentication context                                           │
└─────────────────────────────────────────────────────────────────────┘
         ↓ Module Federation (Core) ↓        ↓ Dynamic Load (3rd Party) ↓
┌──────────────────────────────────────┐  ┌────────────────────────────┐
│        CORE UI PLUGINS               │  │   THIRD-PARTY UI PLUGINS   │
│  • nvr-ui-dashboard                  │  │  • Custom camera views     │
│  • nvr-ui-cameras                    │  │  • Integration dashboards  │
│  • nvr-ui-recordings                 │  │  • Analytics widgets       │
│  • nvr-ui-events                     │  │  • Custom settings panels  │
│  • nvr-ui-settings                   │  │                            │
│  • nvr-ui-plugins-manager            │  │                            │
└──────────────────────────────────────┘  └────────────────────────────┘
```

### UI Plugin Types

#### 1. **Core UI Plugins** (Module Federation)
Shipped with NVR, loaded via Webpack Module Federation for optimal performance:

| Plugin ID | Pages | Description |
|-----------|-------|-------------|
| `nvr-ui-dashboard` | `/` | Main dashboard with camera grid |
| `nvr-ui-cameras` | `/cameras`, `/cameras/:id`, `/cameras/add` | Camera management |
| `nvr-ui-recordings` | `/recordings`, `/recordings/:id` | Recording playback & timeline |
| `nvr-ui-events` | `/events`, `/search` | Event browser & search |
| `nvr-ui-settings` | `/settings/*` | System settings |
| `nvr-ui-plugins` | `/settings/plugins` | Plugin manager UI |

#### 2. **Third-Party UI Plugins** (Dynamic Script Loading)
Loaded at runtime from plugin bundles:

```typescript
// Plugin registers routes and components dynamically
window.__NVR_UI_PLUGINS__.register({
  id: 'my-custom-plugin',
  routes: [
    { path: '/custom/dashboard', component: CustomDashboard },
    { path: '/settings/custom', component: CustomSettings },
  ],
  navItems: [
    { label: 'Custom Dashboard', path: '/custom/dashboard', icon: 'dashboard' },
  ],
  widgets: [
    { id: 'custom-widget', component: CustomWidget, zones: ['dashboard'] },
  ],
});
```

### Implementation Details

#### UI Shell Structure

```
web-ui/
├── packages/
│   ├── shell/                    # Core shell application
│   │   ├── src/
│   │   │   ├── App.tsx           # Main app with plugin loading
│   │   │   ├── PluginLoader.tsx  # Dynamic plugin loader
│   │   │   ├── PluginRouter.tsx  # Dynamic route registration
│   │   │   ├── Layout.tsx        # App shell layout
│   │   │   └── contexts/
│   │   │       ├── PluginContext.tsx
│   │   │       └── ThemeContext.tsx
│   │   └── package.json
│   │
│   ├── ui-components/            # Shared component library
│   │   ├── src/
│   │   │   ├── Button.tsx
│   │   │   ├── Card.tsx
│   │   │   ├── VideoPlayer.tsx
│   │   │   ├── Timeline.tsx
│   │   │   └── index.ts
│   │   └── package.json
│   │
│   └── api-client/               # Shared API client
│       ├── src/
│       │   ├── client.ts
│       │   ├── hooks.ts          # React Query hooks
│       │   └── websocket.ts
│       └── package.json
│
├── plugins/
│   ├── nvr-ui-dashboard/
│   │   ├── src/
│   │   │   ├── index.tsx         # Plugin entry
│   │   │   ├── Dashboard.tsx
│   │   │   └── CameraGrid.tsx
│   │   ├── manifest.json
│   │   └── package.json
│   │
│   ├── nvr-ui-cameras/
│   │   ├── src/
│   │   │   ├── index.tsx
│   │   │   ├── CameraList.tsx
│   │   │   ├── CameraDetail.tsx
│   │   │   └── AddCamera.tsx
│   │   ├── manifest.json
│   │   └── package.json
│   │
│   ├── nvr-ui-recordings/
│   │   ├── src/
│   │   │   ├── index.tsx
│   │   │   ├── Recordings.tsx
│   │   │   └── TimelinePlayer.tsx
│   │   ├── manifest.json
│   │   └── package.json
│   │
│   ├── nvr-ui-events/
│   │   ├── src/
│   │   │   ├── index.tsx
│   │   │   ├── Events.tsx
│   │   │   └── Search.tsx
│   │   ├── manifest.json
│   │   └── package.json
│   │
│   └── nvr-ui-settings/
│       ├── src/
│       │   ├── index.tsx
│       │   ├── Settings.tsx
│       │   ├── Health.tsx
│       │   └── Plugins.tsx
│       ├── manifest.json
│       └── package.json
│
└── package.json                  # Workspace root
```

#### Plugin Manifest (manifest.json)

```json
{
  "id": "nvr-ui-dashboard",
  "name": "Dashboard",
  "version": "1.0.0",
  "description": "Main NVR dashboard with camera grid",
  "type": "ui",
  "category": "core",

  "routes": [
    {
      "path": "/",
      "component": "Dashboard",
      "exact": true,
      "navItem": {
        "label": "Dashboard",
        "icon": "home",
        "order": 1
      }
    }
  ],

  "widgets": [
    {
      "id": "camera-grid",
      "component": "CameraGrid",
      "zones": ["dashboard"],
      "defaultProps": {
        "columns": 3
      }
    }
  ],

  "dependencies": {
    "@nvr/ui-components": "^1.0.0",
    "@nvr/api-client": "^1.0.0"
  },

  "remoteEntry": "./remoteEntry.js"
}
```

#### Plugin Loader Component

```typescript
// packages/shell/src/PluginLoader.tsx
import React, { Suspense, lazy, useEffect, useState } from 'react';

interface UIPlugin {
  id: string;
  name: string;
  version: string;
  routes: RouteConfig[];
  widgets: WidgetConfig[];
  component?: React.ComponentType;
}

interface PluginLoaderProps {
  children: React.ReactNode;
}

// Module Federation dynamic imports for core plugins
const corePlugins: Record<string, () => Promise<{ default: UIPlugin }>> = {
  'nvr-ui-dashboard': () => import('nvr_ui_dashboard/Plugin'),
  'nvr-ui-cameras': () => import('nvr_ui_cameras/Plugin'),
  'nvr-ui-recordings': () => import('nvr_ui_recordings/Plugin'),
  'nvr-ui-events': () => import('nvr_ui_events/Plugin'),
  'nvr-ui-settings': () => import('nvr_ui_settings/Plugin'),
};

export const PluginLoader: React.FC<PluginLoaderProps> = ({ children }) => {
  const [plugins, setPlugins] = useState<Map<string, UIPlugin>>(new Map());
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadPlugins();
  }, []);

  const loadPlugins = async () => {
    const loadedPlugins = new Map<string, UIPlugin>();

    // Load core plugins via Module Federation
    for (const [id, loader] of Object.entries(corePlugins)) {
      try {
        const module = await loader();
        loadedPlugins.set(id, module.default);
      } catch (error) {
        console.error(`Failed to load core plugin: ${id}`, error);
      }
    }

    // Load third-party plugins from API
    try {
      const response = await fetch('/api/v1/plugins?type=ui');
      const thirdPartyPlugins = await response.json();

      for (const pluginInfo of thirdPartyPlugins) {
        try {
          const plugin = await loadThirdPartyPlugin(pluginInfo);
          loadedPlugins.set(plugin.id, plugin);
        } catch (error) {
          console.error(`Failed to load plugin: ${pluginInfo.id}`, error);
        }
      }
    } catch (error) {
      console.error('Failed to fetch third-party plugins', error);
    }

    setPlugins(loadedPlugins);
    setLoading(false);
  };

  const loadThirdPartyPlugin = async (info: any): Promise<UIPlugin> => {
    // Load plugin script dynamically
    return new Promise((resolve, reject) => {
      const script = document.createElement('script');
      script.src = `/api/v1/plugins/${info.id}/ui/bundle.js`;
      script.onload = () => {
        const plugin = window.__NVR_UI_PLUGINS__?.get(info.id);
        if (plugin) {
          resolve(plugin);
        } else {
          reject(new Error(`Plugin ${info.id} did not register`));
        }
      };
      script.onerror = reject;
      document.body.appendChild(script);
    });
  };

  if (loading) {
    return <LoadingScreen />;
  }

  return (
    <PluginContext.Provider value={{ plugins, loadPlugin: loadThirdPartyPlugin }}>
      {children}
    </PluginContext.Provider>
  );
};
```

#### Dynamic Route Registration

```typescript
// packages/shell/src/PluginRouter.tsx
import React, { useMemo } from 'react';
import { Routes, Route } from 'react-router-dom';
import { usePlugins } from './contexts/PluginContext';
import { Layout } from './Layout';

export const PluginRouter: React.FC = () => {
  const { plugins } = usePlugins();

  const routes = useMemo(() => {
    const allRoutes: RouteConfig[] = [];

    plugins.forEach((plugin) => {
      plugin.routes.forEach((route) => {
        allRoutes.push({
          ...route,
          pluginId: plugin.id,
        });
      });
    });

    // Sort by order for consistent rendering
    return allRoutes.sort((a, b) => (a.order || 100) - (b.order || 100));
  }, [plugins]);

  return (
    <Routes>
      <Route path="/" element={<Layout plugins={plugins} />}>
        {routes.map((route) => (
          <Route
            key={`${route.pluginId}-${route.path}`}
            path={route.path}
            element={
              <PluginErrorBoundary pluginId={route.pluginId}>
                <route.component />
              </PluginErrorBoundary>
            }
          />
        ))}
      </Route>
    </Routes>
  );
};
```

#### Webpack Module Federation Config

```javascript
// packages/shell/webpack.config.js
const { ModuleFederationPlugin } = require('webpack').container;

module.exports = {
  plugins: [
    new ModuleFederationPlugin({
      name: 'shell',
      remotes: {
        nvr_ui_dashboard: 'nvr_ui_dashboard@/plugins/nvr-ui-dashboard/remoteEntry.js',
        nvr_ui_cameras: 'nvr_ui_cameras@/plugins/nvr-ui-cameras/remoteEntry.js',
        nvr_ui_recordings: 'nvr_ui_recordings@/plugins/nvr-ui-recordings/remoteEntry.js',
        nvr_ui_events: 'nvr_ui_events@/plugins/nvr-ui-events/remoteEntry.js',
        nvr_ui_settings: 'nvr_ui_settings@/plugins/nvr-ui-settings/remoteEntry.js',
      },
      shared: {
        react: { singleton: true, requiredVersion: '^18.0.0' },
        'react-dom': { singleton: true, requiredVersion: '^18.0.0' },
        'react-router-dom': { singleton: true },
        '@tanstack/react-query': { singleton: true },
        '@nvr/ui-components': { singleton: true },
        '@nvr/api-client': { singleton: true },
      },
    }),
  ],
};

// plugins/nvr-ui-dashboard/webpack.config.js
module.exports = {
  plugins: [
    new ModuleFederationPlugin({
      name: 'nvr_ui_dashboard',
      filename: 'remoteEntry.js',
      exposes: {
        './Plugin': './src/index.tsx',
        './Dashboard': './src/Dashboard.tsx',
        './CameraGrid': './src/CameraGrid.tsx',
      },
      shared: {
        react: { singleton: true, requiredVersion: '^18.0.0' },
        'react-dom': { singleton: true, requiredVersion: '^18.0.0' },
        'react-router-dom': { singleton: true },
        '@tanstack/react-query': { singleton: true },
        '@nvr/ui-components': { singleton: true },
        '@nvr/api-client': { singleton: true },
      },
    }),
  ],
};
```

#### Plugin Error Boundary

```typescript
// packages/shell/src/PluginErrorBoundary.tsx
import React, { Component, ErrorInfo, ReactNode } from 'react';

interface Props {
  pluginId: string;
  children: ReactNode;
  fallback?: ReactNode;
}

interface State {
  hasError: boolean;
  error?: Error;
}

export class PluginErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false };
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error(`Plugin ${this.props.pluginId} crashed:`, error, errorInfo);

    // Report to backend for monitoring
    fetch('/api/v1/plugins/errors', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        pluginId: this.props.pluginId,
        error: error.message,
        stack: error.stack,
        componentStack: errorInfo.componentStack,
      }),
    }).catch(() => {});
  }

  render() {
    if (this.state.hasError) {
      return this.props.fallback || (
        <div className="plugin-error">
          <h3>Plugin Error</h3>
          <p>The plugin "{this.props.pluginId}" encountered an error.</p>
          <button onClick={() => this.setState({ hasError: false })}>
            Try Again
          </button>
        </div>
      );
    }

    return this.props.children;
  }
}
```

### Hot-Reload Implementation

#### Backend Support for UI Plugin Updates

```go
// internal/core/gateway.go - Add UI plugin serving
func (gw *APIGateway) serveUIPlugin(w http.ResponseWriter, r *http.Request) {
    pluginID := chi.URLParam(r, "pluginId")
    filePath := chi.URLParam(r, "*")

    // Get plugin directory
    pluginDir := filepath.Join(gw.uiPluginsDir, pluginID, "dist")

    // Serve static files with cache headers
    w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
    http.ServeFile(w, r, filepath.Join(pluginDir, filePath))
}

// WebSocket notification for plugin updates
func (gw *APIGateway) notifyPluginUpdate(pluginID string, version string) {
    gw.eventBus.Publish("ui.plugin.updated", map[string]string{
        "plugin_id": pluginID,
        "version":   version,
    })
}
```

#### Frontend Hot-Reload Handler

```typescript
// packages/shell/src/hooks/usePluginUpdates.ts
import { useEffect } from 'react';
import { useWebSocket } from '@nvr/api-client';
import { usePlugins } from '../contexts/PluginContext';

export const usePluginUpdates = () => {
  const { reloadPlugin } = usePlugins();
  const { subscribe } = useWebSocket();

  useEffect(() => {
    const unsubscribe = subscribe('ui.plugin.updated', async (event) => {
      const { plugin_id, version } = event;

      console.log(`Plugin ${plugin_id} updated to ${version}, reloading...`);

      // Clear module cache for Module Federation
      if (window.__webpack_require__) {
        // Clear cached remote
        delete window.__webpack_require__.m[`webpack/container/remote/${plugin_id}`];
      }

      // Reload the plugin
      await reloadPlugin(plugin_id);

      // Show notification
      toast.info(`Plugin "${plugin_id}" updated to v${version}`);
    });

    return unsubscribe;
  }, [reloadPlugin, subscribe]);
};
```

### Widget System

Plugins can provide widgets that can be placed in designated zones:

```typescript
// packages/shell/src/WidgetZone.tsx
import React from 'react';
import { usePlugins } from './contexts/PluginContext';

interface WidgetZoneProps {
  zone: string;  // 'dashboard', 'camera-detail', 'sidebar', etc.
  context?: any; // Context data passed to widgets
}

export const WidgetZone: React.FC<WidgetZoneProps> = ({ zone, context }) => {
  const { plugins } = usePlugins();

  const widgets = useMemo(() => {
    const zoneWidgets: WidgetConfig[] = [];

    plugins.forEach((plugin) => {
      plugin.widgets
        .filter((w) => w.zones.includes(zone))
        .forEach((w) => zoneWidgets.push({ ...w, pluginId: plugin.id }));
    });

    return zoneWidgets.sort((a, b) => (a.order || 100) - (b.order || 100));
  }, [plugins, zone]);

  return (
    <div className="widget-zone" data-zone={zone}>
      {widgets.map((widget) => (
        <PluginErrorBoundary key={widget.id} pluginId={widget.pluginId}>
          <widget.component {...widget.defaultProps} context={context} />
        </PluginErrorBoundary>
      ))}
    </div>
  );
};

// Usage in Dashboard
const Dashboard = () => (
  <div className="dashboard">
    <WidgetZone zone="dashboard-top" />
    <CameraGrid />
    <WidgetZone zone="dashboard-bottom" />
  </div>
);
```

### Navigation System

```typescript
// packages/shell/src/Navigation.tsx
import React from 'react';
import { NavLink } from 'react-router-dom';
import { usePlugins } from './contexts/PluginContext';

export const Navigation: React.FC = () => {
  const { plugins } = usePlugins();

  const navItems = useMemo(() => {
    const items: NavItem[] = [];

    plugins.forEach((plugin) => {
      plugin.routes
        .filter((r) => r.navItem)
        .forEach((r) => {
          items.push({
            ...r.navItem,
            path: r.path,
            pluginId: plugin.id,
          });
        });
    });

    return items.sort((a, b) => a.order - b.order);
  }, [plugins]);

  return (
    <nav className="sidebar-nav">
      {navItems.map((item) => (
        <NavLink
          key={item.path}
          to={item.path}
          className={({ isActive }) => isActive ? 'active' : ''}
        >
          <Icon name={item.icon} />
          <span>{item.label}</span>
        </NavLink>
      ))}
    </nav>
  );
};
```

### Migration Path

1. **Phase 1: Create Monorepo Structure** (Day 1)
   - Set up pnpm/yarn workspaces
   - Create packages/shell, packages/ui-components, packages/api-client
   - Move shared code to packages

2. **Phase 2: Extract Dashboard Plugin** (Day 2)
   - Create plugins/nvr-ui-dashboard
   - Configure Module Federation
   - Test hot-reload

3. **Phase 3: Extract Remaining Plugins** (Day 3-4)
   - Cameras, Recordings, Events, Settings
   - Maintain backward compatibility

4. **Phase 4: Third-Party Plugin Support** (Day 5)
   - Dynamic script loading
   - Plugin SDK for external developers
   - Documentation

### Benefits

1. **Hot-Reload UI Updates**: Update any UI plugin without page refresh
2. **Independent Versioning**: Each plugin has its own version
3. **Smaller Initial Bundle**: Only load what's needed
4. **Plugin Isolation**: Crashed plugins don't break the app
5. **Third-Party Extensions**: Easy to add custom views
6. **A/B Testing**: Can load different versions of plugins
7. **Lazy Loading**: Plugins load on-demand

---

## 🔄 WEEK 11: QUEUE-BASED DETECTION WITH HORIZONTAL SCALING

**Goal:** Enable horizontal scaling of detection workers while maintaining sub-second responsiveness (<500ms frame-to-detection latency).

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                         FRAME PRODUCERS                              │
│  Camera 1 → Frame Grabber → ─┐                                      │
│  Camera 2 → Frame Grabber → ─┼──→ NATS JetStream                    │
│  Camera N → Frame Grabber → ─┘    (detection.frames.{camera_id})    │
└─────────────────────────────────────────────────────────────────────┘
                                        ↓
┌─────────────────────────────────────────────────────────────────────┐
│                    NATS JETSTREAM (Queue Group)                      │
│                                                                      │
│  Stream: DETECTION_FRAMES                                           │
│  Subjects: detection.frames.* (per camera)                          │
│  Consumer: detection-workers (queue group)                          │
│  Retention: 5 seconds (work queue pattern)                          │
│  Max Pending: 1000 frames                                           │
└─────────────────────────────────────────────────────────────────────┘
                                        ↓
        ┌───────────────────────────────┼───────────────────────────┐
        ↓                               ↓                           ↓
┌──────────────┐               ┌──────────────┐            ┌──────────────┐
│  Detection   │               │  Detection   │            │  Detection   │
│  Worker 1    │               │  Worker 2    │            │  Worker N    │
│  (GPU)       │               │  (GPU)       │            │  (CPU)       │
│              │               │              │            │              │
│ YOLOv12 +    │               │ YOLOv12 +    │            │ YOLOv12 +    │
│ InsightFace  │               │ InsightFace  │            │ InsightFace  │
└──────────────┘               └──────────────┘            └──────────────┘
        ↓                               ↓                           ↓
        └───────────────────────────────┼───────────────────────────┘
                                        ↓
┌─────────────────────────────────────────────────────────────────────┐
│                     NATS JETSTREAM (Results)                         │
│                                                                      │
│  Stream: DETECTION_RESULTS                                          │
│  Subjects: detection.results.{camera_id}                            │
│  Retention: 1 hour                                                  │
└─────────────────────────────────────────────────────────────────────┘
                                        ↓
┌─────────────────────────────────────────────────────────────────────┐
│                      RESULT PROCESSOR                                │
│  • Aggregates detections                                            │
│  • Object tracking (maintain track IDs across frames)               │
│  • Event creation (new object, object left, etc.)                   │
│  • Zone filtering                                                   │
│  • WebSocket broadcast                                              │
└─────────────────────────────────────────────────────────────────────┘
```

### Why Sub-Second is Achievable

| Component | Latency Budget | Notes |
|-----------|---------------|-------|
| Frame grab | 20-50ms | From go2rtc MJPEG |
| NATS publish | 1-2ms | Local embedded server |
| Queue wait | 0-100ms | Depends on worker availability |
| Detection | 30-100ms | YOLOv12n on GPU |
| NATS result | 1-2ms | Local embedded server |
| Processing | 5-10ms | Event creation, tracking |
| **Total** | **60-265ms** | Well under 500ms target |

### Implementation

#### Step 11.1: JetStream Stream Configuration

```go
// plugins/nvr-detection/queue.go
package main

import (
    "github.com/nats-io/nats.go"
    "github.com/nats-io/nats.go/jetstream"
)

func setupStreams(js jetstream.JetStream) error {
    // Frame input stream - work queue pattern
    _, err := js.CreateOrUpdateStream(context.Background(), jetstream.StreamConfig{
        Name:        "DETECTION_FRAMES",
        Subjects:    []string{"detection.frames.*"},
        Storage:     jetstream.MemoryStorage,  // Fast, in-memory
        Retention:   jetstream.WorkQueuePolicy,
        MaxAge:      5 * time.Second,          // Discard old frames
        MaxMsgs:     10000,
        MaxBytes:    1024 * 1024 * 1024,       // 1GB max
        Discard:     jetstream.DiscardOld,
        Replicas:    1,                        // Single node
    })
    if err != nil {
        return fmt.Errorf("failed to create frames stream: %w", err)
    }

    // Results stream - for aggregation and replay
    _, err = js.CreateOrUpdateStream(context.Background(), jetstream.StreamConfig{
        Name:        "DETECTION_RESULTS",
        Subjects:    []string{"detection.results.*"},
        Storage:     jetstream.MemoryStorage,
        Retention:   jetstream.LimitsPolicy,
        MaxAge:      1 * time.Hour,
        MaxMsgs:     100000,
        Replicas:    1,
    })
    if err != nil {
        return fmt.Errorf("failed to create results stream: %w", err)
    }

    return nil
}
```

#### Step 11.2: Frame Message Format

```go
// sdk/detection.go
package sdk

import (
    "time"

    "github.com/vmihailenco/msgpack/v5"
)

// DetectionFrame is published by frame grabbers
type DetectionFrame struct {
    CameraID    string    `msgpack:"camera_id"`
    Timestamp   time.Time `msgpack:"timestamp"`
    FrameNumber uint64    `msgpack:"frame_number"`
    Width       int       `msgpack:"width"`
    Height      int       `msgpack:"height"`
    Format      string    `msgpack:"format"`  // "jpeg", "raw"
    Data        []byte    `msgpack:"data"`    // Image data

    // Detection config for this camera
    MinConfidence float64  `msgpack:"min_confidence"`
    EnabledTypes  []string `msgpack:"enabled_types"` // ["person", "car", "face"]
    Zones         []Zone   `msgpack:"zones,omitempty"`
}

// DetectionResult is published by detection workers
type DetectionResult struct {
    CameraID      string       `msgpack:"camera_id"`
    Timestamp     time.Time    `msgpack:"timestamp"`
    FrameNumber   uint64       `msgpack:"frame_number"`
    WorkerID      string       `msgpack:"worker_id"`
    ProcessTimeMs float64      `msgpack:"process_time_ms"`
    Detections    []Detection  `msgpack:"detections"`
    MotionScore   float64      `msgpack:"motion_score"`
}

// Use MessagePack for efficient serialization (faster than JSON, smaller than Protobuf for this use case)
func (f *DetectionFrame) Marshal() ([]byte, error) {
    return msgpack.Marshal(f)
}

func (f *DetectionFrame) Unmarshal(data []byte) error {
    return msgpack.Unmarshal(data, f)
}
```

#### Step 11.3: Frame Producer (in Streaming Plugin)

```go
// plugins/nvr-streaming/producer.go
package main

import (
    "context"
    "time"

    "github.com/nats-io/nats.go/jetstream"
    "github.com/nvr-system/nvr/sdk"
)

type FrameProducer struct {
    js       jetstream.JetStream
    cameraID string
    config   CameraDetectionConfig

    frameNum uint64
    ticker   *time.Ticker
    stopCh   chan struct{}
}

func NewFrameProducer(js jetstream.JetStream, cameraID string, config CameraDetectionConfig) *FrameProducer {
    fps := config.DetectionFPS
    if fps <= 0 {
        fps = 5  // Default 5 FPS
    }

    return &FrameProducer{
        js:       js,
        cameraID: cameraID,
        config:   config,
        ticker:   time.NewTicker(time.Second / time.Duration(fps)),
        stopCh:   make(chan struct{}),
    }
}

func (p *FrameProducer) Start(ctx context.Context) {
    go func() {
        for {
            select {
            case <-ctx.Done():
                return
            case <-p.stopCh:
                return
            case <-p.ticker.C:
                p.grabAndPublish(ctx)
            }
        }
    }()
}

func (p *FrameProducer) grabAndPublish(ctx context.Context) {
    // Grab frame from go2rtc (fast MJPEG endpoint)
    frameData, err := p.grabFrame(ctx)
    if err != nil {
        return // Skip this frame
    }

    p.frameNum++

    frame := &sdk.DetectionFrame{
        CameraID:      p.cameraID,
        Timestamp:     time.Now(),
        FrameNumber:   p.frameNum,
        Width:         1920,  // From camera config
        Height:        1080,
        Format:        "jpeg",
        Data:          frameData,
        MinConfidence: p.config.MinConfidence,
        EnabledTypes:  p.config.EnabledTypes,
        Zones:         p.config.Zones,
    }

    data, _ := frame.Marshal()

    // Publish with async ack (fire and forget for low latency)
    subject := fmt.Sprintf("detection.frames.%s", p.cameraID)
    p.js.PublishAsync(subject, data)
}

func (p *FrameProducer) grabFrame(ctx context.Context) ([]byte, error) {
    // Fast HTTP request to go2rtc snapshot endpoint
    url := fmt.Sprintf("http://localhost:1984/api/frame.jpeg?src=%s", p.cameraID)

    req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    return io.ReadAll(resp.Body)
}
```

#### Step 11.4: Detection Worker

```python
# plugins/nvr-detection-worker/worker.py
import asyncio
import os
import time
from dataclasses import dataclass
from typing import List, Optional

import msgpack
import nats
from nats.js.api import ConsumerConfig, DeliverPolicy, AckPolicy
from ultralytics import YOLO
import numpy as np
import cv2

@dataclass
class DetectionConfig:
    worker_id: str
    nats_url: str
    model_path: str
    device: str  # "cuda:0", "cpu", etc.
    batch_size: int = 1

class DetectionWorker:
    def __init__(self, config: DetectionConfig):
        self.config = config
        self.model = None
        self.nc = None
        self.js = None

    async def start(self):
        # Load model
        print(f"Loading model: {self.config.model_path}")
        self.model = YOLO(self.config.model_path)
        self.model.to(self.config.device)

        # Warm up model
        dummy = np.zeros((640, 640, 3), dtype=np.uint8)
        self.model.predict(dummy, verbose=False)
        print("Model warmed up")

        # Connect to NATS
        self.nc = await nats.connect(self.config.nats_url)
        self.js = self.nc.jetstream()

        # Create pull consumer for work queue
        consumer = await self.js.create_consumer(
            "DETECTION_FRAMES",
            ConsumerConfig(
                name=f"worker-{self.config.worker_id}",
                deliver_policy=DeliverPolicy.ALL,
                ack_policy=AckPolicy.EXPLICIT,
                max_ack_pending=self.config.batch_size * 2,
                # Queue group for load balancing
                deliver_group="detection-workers",
            )
        )

        print(f"Worker {self.config.worker_id} started, consuming frames...")

        # Process frames
        while True:
            try:
                # Fetch batch of messages
                msgs = await consumer.fetch(
                    batch=self.config.batch_size,
                    timeout=1.0
                )

                for msg in msgs:
                    await self.process_frame(msg)

            except asyncio.TimeoutError:
                continue
            except Exception as e:
                print(f"Error: {e}")
                await asyncio.sleep(0.1)

    async def process_frame(self, msg):
        start_time = time.time()

        # Decode frame
        frame_data = msgpack.unpackb(msg.data)

        # Decode JPEG
        img_array = np.frombuffer(frame_data[b'data'], dtype=np.uint8)
        img = cv2.imdecode(img_array, cv2.IMREAD_COLOR)

        # Run detection
        results = self.model.predict(
            img,
            conf=frame_data.get(b'min_confidence', 0.5),
            verbose=False
        )

        # Convert results
        detections = []
        for r in results:
            for box in r.boxes:
                detections.append({
                    'type': r.names[int(box.cls)],
                    'confidence': float(box.conf),
                    'bbox': {
                        'x': float(box.xyxyn[0][0]),
                        'y': float(box.xyxyn[0][1]),
                        'width': float(box.xyxyn[0][2] - box.xyxyn[0][0]),
                        'height': float(box.xyxyn[0][3] - box.xyxyn[0][1]),
                    }
                })

        process_time = (time.time() - start_time) * 1000

        # Publish result
        result = {
            'camera_id': frame_data[b'camera_id'].decode(),
            'timestamp': frame_data[b'timestamp'],
            'frame_number': frame_data[b'frame_number'],
            'worker_id': self.config.worker_id,
            'process_time_ms': process_time,
            'detections': detections,
        }

        subject = f"detection.results.{result['camera_id']}"
        await self.js.publish(subject, msgpack.packb(result))

        # Ack the message
        await msg.ack()

        # Log periodically
        if frame_data[b'frame_number'] % 100 == 0:
            print(f"Processed frame {frame_data[b'frame_number']} in {process_time:.1f}ms")

if __name__ == "__main__":
    config = DetectionConfig(
        worker_id=os.environ.get("WORKER_ID", "worker-1"),
        nats_url=os.environ.get("NATS_URL", "nats://localhost:4222"),
        model_path=os.environ.get("MODEL_PATH", "yolov12n.pt"),
        device=os.environ.get("DEVICE", "cuda:0"),
        batch_size=int(os.environ.get("BATCH_SIZE", "1")),
    )

    asyncio.run(DetectionWorker(config).start())
```

#### Step 11.5: Result Processor

```go
// plugins/nvr-detection/processor.go
package main

import (
    "context"
    "sync"
    "time"

    "github.com/nats-io/nats.go/jetstream"
    "github.com/nvr-system/nvr/sdk"
)

// ResultProcessor aggregates detection results and creates events
type ResultProcessor struct {
    js       jetstream.JetStream
    eventBus *sdk.EventBus

    // Track objects across frames
    trackers map[string]*ObjectTracker  // per camera
    mu       sync.RWMutex
}

type ObjectTracker struct {
    activeObjects map[string]*TrackedObject
    lastUpdate    time.Time
}

type TrackedObject struct {
    ID          string
    Type        string
    FirstSeen   time.Time
    LastSeen    time.Time
    LastBBox    sdk.BoundingBox
    Confidence  float64
    FrameCount  int
}

func (p *ResultProcessor) Start(ctx context.Context) error {
    // Subscribe to all detection results
    consumer, err := p.js.CreateOrUpdateConsumer(ctx, "DETECTION_RESULTS", jetstream.ConsumerConfig{
        Name:          "result-processor",
        DeliverPolicy: jetstream.DeliverNewPolicy,
        AckPolicy:     jetstream.AckExplicitPolicy,
    })
    if err != nil {
        return err
    }

    // Process results
    go func() {
        for {
            msgs, err := consumer.Fetch(100, jetstream.FetchMaxWait(time.Second))
            if err != nil {
                continue
            }

            for msg := range msgs.Messages() {
                p.processResult(ctx, msg)
            }
        }
    }()

    // Cleanup stale trackers
    go p.cleanupLoop(ctx)

    return nil
}

func (p *ResultProcessor) processResult(ctx context.Context, msg jetstream.Msg) {
    var result sdk.DetectionResult
    if err := result.Unmarshal(msg.Data()); err != nil {
        msg.Nak()
        return
    }

    p.mu.Lock()
    tracker, ok := p.trackers[result.CameraID]
    if !ok {
        tracker = &ObjectTracker{
            activeObjects: make(map[string]*TrackedObject),
        }
        p.trackers[result.CameraID] = tracker
    }
    p.mu.Unlock()

    // Update tracker with new detections
    newObjects, leftObjects := tracker.Update(result.Detections, result.Timestamp)

    // Create events for new objects
    for _, obj := range newObjects {
        event := sdk.Event{
            Type:       "detection",
            CameraID:   result.CameraID,
            Timestamp:  obj.FirstSeen,
            ObjectType: obj.Type,
            Confidence: obj.Confidence,
            BoundingBox: obj.LastBBox,
            TrackID:    obj.ID,
        }
        p.eventBus.Publish("events.detection", event)
    }

    // Create events for objects that left
    for _, obj := range leftObjects {
        event := sdk.Event{
            Type:       "detection.ended",
            CameraID:   result.CameraID,
            Timestamp:  obj.LastSeen,
            ObjectType: obj.Type,
            TrackID:    obj.ID,
            Duration:   obj.LastSeen.Sub(obj.FirstSeen),
        }
        p.eventBus.Publish("events.detection.ended", event)
    }

    // Broadcast to WebSocket for real-time UI updates
    p.eventBus.Publish("ws.detection", sdk.WSMessage{
        Type:     "detection",
        CameraID: result.CameraID,
        Data:     result,
    })

    msg.Ack()
}

// IOU-based object tracking
func (t *ObjectTracker) Update(detections []sdk.Detection, timestamp time.Time) (new, left []*TrackedObject) {
    t.lastUpdate = timestamp

    matched := make(map[string]bool)

    for _, det := range detections {
        // Find best matching existing object
        bestMatch := ""
        bestIOU := 0.5  // Minimum IOU threshold

        for id, obj := range t.activeObjects {
            if obj.Type != det.Type {
                continue
            }
            iou := calculateIOU(obj.LastBBox, det.BoundingBox)
            if iou > bestIOU {
                bestIOU = iou
                bestMatch = id
            }
        }

        if bestMatch != "" {
            // Update existing object
            obj := t.activeObjects[bestMatch]
            obj.LastSeen = timestamp
            obj.LastBBox = det.BoundingBox
            obj.Confidence = det.Confidence
            obj.FrameCount++
            matched[bestMatch] = true
        } else {
            // New object
            obj := &TrackedObject{
                ID:         generateTrackID(),
                Type:       det.Type,
                FirstSeen:  timestamp,
                LastSeen:   timestamp,
                LastBBox:   det.BoundingBox,
                Confidence: det.Confidence,
                FrameCount: 1,
            }
            t.activeObjects[obj.ID] = obj
            new = append(new, obj)
        }
    }

    // Check for objects that left (not seen for 2 seconds)
    staleThreshold := timestamp.Add(-2 * time.Second)
    for id, obj := range t.activeObjects {
        if obj.LastSeen.Before(staleThreshold) {
            left = append(left, obj)
            delete(t.activeObjects, id)
        }
    }

    return new, left
}
```

#### Step 11.6: Docker Compose for Scaling

```yaml
# docker-compose.scale.yml
version: '3.8'

services:
  nvr-core:
    image: nvr-system/nvr:latest
    ports:
      - "5000:5000"
    environment:
      - NATS_URL=nats://nats:4222
    volumes:
      - nvr-data:/data
    depends_on:
      - nats

  nats:
    image: nats:latest
    command: ["--jetstream", "--store_dir=/data"]
    volumes:
      - nats-data:/data
    ports:
      - "4222:4222"

  # Detection workers - scale as needed
  detection-worker:
    image: nvr-system/detection-worker:latest
    deploy:
      replicas: 3  # Scale based on camera count / GPU availability
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: 1
              capabilities: [gpu]
    environment:
      - NATS_URL=nats://nats:4222
      - DEVICE=cuda:0
      - MODEL_PATH=/models/yolov12n.pt
    volumes:
      - models:/models
    depends_on:
      - nats

volumes:
  nvr-data:
  nats-data:
  models:
```

### Scaling Guidelines

| Cameras | Detection FPS | Workers Needed | GPU Memory |
|---------|---------------|----------------|------------|
| 1-4     | 5 FPS each    | 1 worker       | 2GB        |
| 5-10    | 5 FPS each    | 2 workers      | 4GB total  |
| 11-20   | 5 FPS each    | 3-4 workers    | 8GB total  |
| 20-50   | 3 FPS each    | 5-8 workers    | 16GB total |
| 50+     | 2 FPS each    | 10+ workers    | Scale out  |

### Monitoring & Metrics

```go
// plugins/nvr-detection/metrics.go
package main

import (
    "github.com/prometheus/client_golang/prometheus"
)

var (
    framesPublished = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "nvr_detection_frames_published_total",
            Help: "Total frames published to detection queue",
        },
        []string{"camera_id"},
    )

    framesProcessed = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "nvr_detection_frames_processed_total",
            Help: "Total frames processed by detection workers",
        },
        []string{"camera_id", "worker_id"},
    )

    processingLatency = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "nvr_detection_processing_latency_ms",
            Help:    "Detection processing latency in milliseconds",
            Buckets: []float64{10, 25, 50, 100, 200, 500, 1000},
        },
        []string{"camera_id", "worker_id"},
    )

    queueDepth = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "nvr_detection_queue_depth",
            Help: "Current number of frames waiting in queue",
        },
        []string{"stream"},
    )

    activeWorkers = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "nvr_detection_active_workers",
            Help: "Number of active detection workers",
        },
    )
)
```

### Latency Optimization Tips

1. **Use Memory Storage**: NATS JetStream memory storage for frames (not disk)
2. **MessagePack**: Use msgpack instead of JSON for faster serialization
3. **Batch Processing**: Workers can process 2-4 frames in parallel on GPU
4. **Connection Pooling**: Reuse HTTP connections for frame grabbing
5. **Image Preprocessing**: Resize to 640x640 before sending to queue
6. **GPU Memory Pinning**: Use CUDA pinned memory for faster transfers
7. **Async Acknowledgment**: Don't wait for acks on frame publishing

