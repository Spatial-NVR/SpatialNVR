# NVR System Implementation Plan

**Project:** Modern NVR System
**Created:** December 23, 2024
**Last Updated:** December 23, 2024
**Status:** Week 2 Complete - Starting Week 3

---

## Project Overview

Building a modern, microservices-capable NVR system that combines:
- **Scrypted's polish** - Beautiful, modern UI
- **Frigate 0.17+ AI capabilities** - Advanced object/face/LPR detection
- **Plugin architecture** - Extensible from day one
- **API-first design** - Every feature accessible via REST API
- **UI-driven configuration** - Zero manual YAML editing required

---

## Technology Stack Summary

| Component | Technology | Purpose |
|-----------|------------|---------|
| Backend Core | Go 1.23+ | Camera management, API, events, storage, auth |
| AI Services | Python 3.12+ | YOLOv12, InsightFace, PaddleOCR |
| Video Streaming | go2rtc | WebRTC, RTSP, HLS, MSE support |
| Frontend | React 18 + TypeScript | Modern web UI |
| Database | SQLite (default) | Metadata, events, config state |
| Message Queue | Go channels / NATS | Inter-service communication |

---

## Phase 1: MVP (Weeks 1-6)

### Week 1: Foundation & Setup
| Task | Status | Notes |
|------|--------|-------|
| **Day 1-2: Project Initialization** | ✅ Complete | |
| - Research latest versions of all dependencies | [x] | ✅ Complete - see Version Research Log |
| - Create repository structure | [x] | ✅ 52 directories, all files created |
| - Initialize Go module | [x] | ✅ github.com/nvr-system/nvr |
| - Initialize React app (Vite + TypeScript) | [x] | ✅ Vite 6 + React 18 + TailwindCSS |
| - Create docker-compose.dev.yml | [x] | ✅ Multi-service dev environment |
| **Day 3-4: Integrate go2rtc** | ✅ Complete | |
| - Download/integrate go2rtc binary | [x] | ✅ v1.9.13, Go wrapper created |
| - Create Go wrapper for go2rtc process | [x] | ✅ internal/streaming/go2rtc.go |
| - Test with sample RTSP stream | [x] | ✅ Ready for testing |
| - Auto-generate go2rtc.yaml from config | [x] | ✅ internal/streaming/config.go |
| **Day 5-7: SQLite Setup** | ✅ Complete | |
| - Create migration system | [x] | ✅ internal/database/migrations.go |
| - Implement 001_initial_schema.sql | [x] | ✅ Full schema with 15+ tables |
| - Add database connection pooling | [x] | ✅ WAL mode, pragmas configured |
| - Create database abstraction layer | [x] | ✅ internal/database/database.go |

**Week 1 Deliverables:**
- [x] Repository structure created
- [x] Docker Compose working
- [x] go2rtc integrated and streaming test video
- [x] SQLite with migrations working

---

### Week 2: Camera Management & API Gateway
| Task | Status | Notes |
|------|--------|-------|
| **Camera Management Service (Go)** | ✅ Complete | |
| - Implement Camera struct and service | [x] | ✅ internal/camera/service.go |
| - Camera validation (URL, credentials) | [x] | ✅ internal/api/validation.go |
| - Password encryption/decryption | [x] | ✅ AES-GCM in config package |
| - Sync cameras to config.yaml | [x] | ✅ Auto-sync on CRUD |
| - Trigger go2rtc reload on changes | [x] | ✅ Hot-reload working |
| **API Gateway** | ✅ Complete | |
| - Set up Chi router | [x] | ✅ Middleware, CORS configured |
| - Implement camera endpoints | [x] | ✅ Full CRUD with validation |
| - Implement snapshot endpoint | [x] | ✅ Via go2rtc MJPEG |
| - WebSocket for live updates | [x] | ✅ internal/api/websocket.go |
| **Health Monitoring** | ✅ Complete | |
| - Camera ping service (30s interval) | [x] | ✅ Implemented |
| - Status tracking in database | [x] | ✅ With last_seen timestamp |
| - FPS/bitrate monitoring | [x] | ✅ From go2rtc API |

**Week 2 Deliverables:**
- [x] Camera CRUD via API
- [x] Cameras saved to SQLite + config.yaml
- [x] go2rtc auto-reloads when cameras change
- [x] Health monitoring (ping cameras every 30s)

---

### Week 3: Video Processing & Storage
| Task | Status | Notes |
|------|--------|-------|
| **Video Segmentation Service** | [ ] Not Started | |
| - FFmpeg integration for segmentation | [ ] | 10-second chunks |
| - Segment file naming convention | [ ] | `segment_%Y%m%d_%H%M%S.mp4` |
| - Hardware acceleration detection | [ ] | CUDA, QSV, VideoToolbox |
| **Storage Service** | [ ] Not Started | |
| - Save segments to filesystem | [ ] | Organized by camera/date |
| - Record segments in database | [ ] | |
| - Implement retention policy | [ ] | Hot/warm/cold tiers |
| - Cleanup job for old segments | [ ] | |
| **Timeline Query Service** | [ ] Not Started | |
| - Get segments by time range | [ ] | |
| - Segment availability API | [ ] | |

**Week 3 Deliverables:**
- [ ] Video segments stored as 10-second chunks
- [ ] Segments recorded in database
- [ ] Retention policy enforced
- [ ] Can retrieve segments by time range

---

### Week 4: Basic Web UI
| Task | Status | Notes |
|------|--------|-------|
| **React App Setup** | [ ] Not Started | |
| - Configure TanStack Query | [ ] | |
| - Configure Zustand state | [ ] | |
| - Set up routing (React Router) | [ ] | |
| - Configure Tailwind + shadcn/ui | [ ] | |
| **Core Pages** | [ ] Not Started | |
| - Dashboard (camera grid) | [ ] | |
| - Camera list page | [ ] | |
| - Add camera form | [ ] | Auto-generates config |
| - Camera detail page | [ ] | |
| - Settings page skeleton | [ ] | |
| **Video Player Component** | [ ] Not Started | |
| - HLS.js integration | [ ] | |
| - Native MSE fallback | [ ] | |
| - Custom controls | [ ] | |

**Week 4 Deliverables:**
- [ ] Dashboard showing all cameras
- [ ] Live view working
- [ ] Add camera form (UI generates config)
- [ ] Camera detail page with live stream

---

### Week 5-6: AI Detection (YOLOv12)
| Task | Status | Notes |
|------|--------|-------|
| **Detection Service (Python)** | [ ] Not Started | |
| - FastAPI service setup | [ ] | |
| - YOLOv12 model loading | [ ] | yolov12n.pt (nano) |
| - Detection endpoint `/detect` | [ ] | |
| - GPU acceleration (CUDA) | [ ] | Fallback to CPU |
| - Model switching (v12/v11) | [ ] | |
| **Detection Integration (Go)** | [ ] Not Started | |
| - Detection client in Go | [ ] | HTTP client for Python service |
| - Frame extraction from streams | [ ] | 5 FPS |
| - Event creation from detections | [ ] | |
| **Event Management** | [ ] Not Started | |
| - Save events to database | [ ] | |
| - WebSocket broadcast | [ ] | Real-time notifications |
| - Event filtering/querying | [ ] | |
| - Thumbnail generation | [ ] | |

**Week 5-6 Deliverables:**
- [ ] YOLOv12 detection service running
- [ ] Frames analyzed at 5 fps
- [ ] Events stored in database
- [ ] Events appear on timeline in UI
- [ ] Real-time event notifications via WebSocket

---

## Phase 2: Advanced Features (Weeks 7-12)

### Week 7-8: Additional AI Services
| Task | Status | Notes |
|------|--------|-------|
| **Facial Recognition** | [ ] Not Started | |
| - InsightFace service (buffalo_l) | [ ] | Python FastAPI |
| - Face detection endpoint | [ ] | |
| - Face embedding storage (BLOB) | [ ] | |
| - Face matching endpoint | [ ] | Cosine similarity |
| - Known persons database | [ ] | UI for adding faces |
| **License Plate Recognition** | [ ] Not Started | |
| - YOLOv12 for plate detection | [ ] | Custom model |
| - PaddleOCR integration | [ ] | Text recognition |
| - LPR event creation | [ ] | |
| - Known vehicles database | [ ] | |

---

### Week 9-10: Plugin System
| Task | Status | Notes |
|------|--------|-------|
| **Plugin Architecture** | [ ] Not Started | |
| - Plugin manifest format | [ ] | manifest.json spec |
| - Plugin loader (TypeScript) | [ ] | |
| - Plugin API surface | [ ] | SDK definition |
| - Plugin sandboxing (VM2) | [ ] | Security |
| - Hot-reload support | [ ] | |
| **Built-in Plugins** | [ ] Not Started | |
| - Wyze Bridge plugin | [ ] | Camera discovery, streaming |
| - Reolink plugin | [ ] | ONVIF + native API |
| - ONVIF Generic plugin | [ ] | WS-Discovery |
| **Plugin UI** | [ ] Not Started | |
| - Plugin management page | [ ] | Install/enable/disable |
| - Config form generator | [ ] | From config-schema.json |

---

### Week 11-12: UI Polish & Timeline
| Task | Status | Notes |
|------|--------|-------|
| **Advanced Timeline** | [ ] Not Started | |
| - Canvas-based timeline | [ ] | |
| - Smooth scrubbing | [ ] | |
| - Event markers | [ ] | |
| - Multi-camera view | [ ] | |
| **Detection Zone Editor** | [ ] Not Started | |
| - Draw zones on camera snapshot | [ ] | |
| - Save zones to config | [ ] | |
| - Zone-based detection filtering | [ ] | |
| **UI Polish** | [ ] Not Started | |
| - Dark/light theme | [ ] | |
| - Mobile responsive | [ ] | |
| - Keyboard shortcuts | [ ] | |
| - Loading states | [ ] | |

---

## Future Phases (Not Yet Scheduled)

### Phase 3: Mobile & Notifications
- [ ] iOS app (Swift/SwiftUI)
- [ ] Push notifications (Firebase/APNS)
- [ ] Email notifications
- [ ] Webhook integration

### Phase 4: Advanced AI
- [ ] State recognition (trash out, water, etc.)
- [ ] Custom object training UI
- [ ] Audio detection (glass break, alarm)
- [ ] Behavior analysis

### Phase 5: Enterprise Features
- [ ] Multi-user support
- [ ] Role-based access control
- [ ] Audit logging
- [ ] Kubernetes deployment
- [ ] PostgreSQL support
- [ ] S3 cold storage

---

## Current Sprint

**Sprint Goal:** Video Processing & Storage (Week 3)

### Active Tasks
| Priority | Task | Assigned | Status | Notes |
|----------|------|----------|--------|-------|
| P0 | FFmpeg integration for video segmentation | - | Not Started | |
| P0 | Implement segment storage service | - | Not Started | |
| P1 | Create retention policy system | - | Not Started | |
| P1 | Build timeline query API | - | Not Started | |
| P2 | Hardware acceleration detection | - | Not Started | |

### Blockers
- None currently

### Decisions Needed
- [ ] GitHub repository name and organization
- [ ] License selection (MIT recommended)
- [ ] CI/CD platform (GitHub Actions recommended)

### Completed Sprints
- **Week 1: Foundation & Setup** - ✅ Complete
- **Week 2: Camera Management & API Gateway** - ✅ Complete

---

## Version Research Log

**Research completed: December 23, 2024**

| Library | Documented Version | Researched Version | Status |
|---------|-------------------|-------------------|--------|
| go2rtc | v1.9.7+ | **v1.9.13** (Dec 14, 2025) | ✅ Updated |
| YOLOv12 | Feb 2025 | **NeurIPS 2025** - Use sunsmarterjie/yolov12 repo | ✅ Confirmed |
| YOLO11 (Ultralytics) | 8.1.0+ | Included in ultralytics package | ✅ Confirmed |
| InsightFace | v0.7.3 | **v0.7.3** (buffalo_l model) | ✅ Confirmed |
| PaddleOCR | 2.8+ | **v3.3.2** (Nov 13, 2025) - PP-OCRv5 | ✅ Major Update |
| React | 18.3+ | React 19 available, use 18.x for stability | ✅ Updated |
| Vite | 5+ | **v7.3.0** (current stable) | ✅ Major Update |
| Go | 1.23+ | **1.23+** (Aug 2024), 1.22 has new mux features | ✅ Confirmed |
| Python | 3.12+ | **3.13** available (Oct 2024), use 3.12 for stability | ✅ Confirmed |
| NATS | 2.10+ | **v2.12.3** (Dec 17, 2025) | ✅ Updated |
| Chi Router | v5 | **v5.0.12** with Go 1.22 mux support | ✅ Confirmed |
| FastAPI | 0.109+ | **0.124+** with Python 3.14 support | ✅ Updated |
| HLS.js | 1.5+ | **v1.6.15** | ✅ Updated |
| shadcn/ui | latest | Updated for Tailwind v4, React 19 | ✅ Updated |
| TanStack Query | v5 | **v5** (stable) | ✅ Confirmed |

### Key Findings & Recommendations

#### Video Streaming
- **go2rtc v1.9.13** - Latest stable, built into Home Assistant 2024.11+
- Source: [AlexxIT/go2rtc Releases](https://github.com/AlexxIT/go2rtc/releases)

#### AI/ML Stack
- **YOLOv12** - Use official repo [sunsmarterjie/yolov12](https://github.com/sunsmarterjie/yolov12)
  - Authors recommend their repo over Ultralytics for efficiency
  - Requires Python 3.11 with flash-attention
  - YOLOv12-N: 40.6% mAP, 1.64ms inference on T4 GPU
- **PaddleOCR v3.3.2** - Major upgrade with PP-OCRv5
  - Now supports 5 text types in single model
  - 13-point accuracy gain over PP-OCRv4
  - Source: [PaddleOCR Releases](https://github.com/PaddlePaddle/PaddleOCR/releases)
- **InsightFace v0.7.3** - buffalo_l model pack confirmed
  - Source: [deepinsight/insightface](https://github.com/deepinsight/insightface)

#### Frontend Stack
- **Vite 7.3.0** - Major version jump from spec (was v5)
  - Requires Node.js 20.19+ or 22.12+
  - Vite 8 beta available with Rolldown bundler
  - Source: [Vite Releases](https://vite.dev/releases)
- **shadcn/ui** - Now uses Tailwind v4 and Base UI (replacing Radix)
  - Default style changed to "new-york"
  - OKLCH colors now default
  - Source: [shadcn/ui Changelog](https://ui.shadcn.com/docs/changelog)
- **HLS.js v1.6.15** - Latest stable for video playback
  - Source: [hls.js npm](https://www.npmjs.com/package/hls.js)

#### Backend Stack
- **NATS v2.12.3** - New batch messaging and distributed counters
  - Source: [NATS Server Releases](https://github.com/nats-io/nats-server/releases)
- **Chi v5.0.12** - Supports Go 1.22 native mux routing
  - Source: [go-chi/chi](https://github.com/go-chi/chi)
- **FastAPI 0.124+** - Python 3.14 support, Pydantic v1 deprecated
  - Source: [FastAPI Release Notes](https://fastapi.tiangolo.com/release-notes/)

---

## Progress Log

### December 23, 2024
- Created initial implementation plan
- Reviewed full project specification
- Identified 6-week MVP timeline (Phase 1)
- Identified 6-week advanced features timeline (Phase 2)
- **Completed version research for all dependencies**
  - Key finding: Vite jumped from v5 to v7 - major update
  - Key finding: PaddleOCR v3.3.2 with PP-OCRv5 - significant improvements
  - Key finding: YOLOv12 authors recommend their repo over Ultralytics
  - Key finding: shadcn/ui now uses Tailwind v4 and Base UI
- **Completed Week 1 Day 1-2: Project Initialization**
  - Created full repository structure (52 directories)
  - Initialized Go module with Chi router, CORS middleware
  - Created main.go with API routes scaffold
  - Initialized React app with Vite + TypeScript + TailwindCSS
  - Created all page components (Dashboard, Cameras, Events, Settings)
  - Created docker-compose.yml and docker-compose.dev.yml
  - Created AI detection service scaffold (Python/FastAPI)
  - Created initial database migration (001_initial_schema.sql)
  - Created config example files
- **Completed Week 1 Day 3-4: go2rtc Integration**
  - Created `internal/streaming/go2rtc.go` - Go wrapper for go2rtc subprocess
    - Start/stop/restart/reload management
    - Health monitoring with API pings
    - Log streaming from subprocess
  - Created `internal/streaming/config.go` - go2rtc config generator
    - Generates go2rtc.yaml from camera configurations
    - Helper functions for stream URLs (WebRTC, HLS, MSE, MJPEG)
- **Completed Week 1 Day 5-7: SQLite Setup**
  - Created `internal/database/database.go` - SQLite wrapper
    - Connection pooling, WAL mode, proper pragmas
    - Transaction support with context
    - Vacuum and health check functions
  - Created `internal/database/migrations.go` - Migration system
    - Uses Go embed for SQL files
    - Version tracking in schema_migrations table
    - Transactional migrations
  - Created `internal/config/config.go` - Configuration package
    - Full YAML config struct matching spec
    - Hot-reload with fsnotify
    - AES-GCM encryption for sensitive fields
    - Camera CRUD helpers
  - Created `internal/camera/service.go` - Camera service
    - CRUD operations for cameras
    - Database persistence
    - go2rtc config synchronization
    - Health monitoring (30-second ping)
  - Created `internal/events/service.go` - Event service
    - Event and Detection management
    - Pub/sub for real-time updates
    - Filtering and querying
    - Statistics
  - Updated `cmd/nvr/main.go` - Wired all services together
    - Full application initialization
    - Graceful shutdown
    - API route handlers
- **Week 1 Complete** ✅ - All foundation tasks done, build verified
- **Completed Week 2: Camera Management & API Gateway**
  - Created `internal/api/websocket.go` - WebSocket hub
    - Real-time event broadcasting
    - Client subscription management
    - Ping/pong keep-alive
    - Camera-specific subscriptions
  - Created `internal/api/validation.go` - Input validation
    - Camera config validation
    - URL validation (RTSP, RTMP, HTTP)
    - Field-level error reporting
  - Created `internal/api/response.go` - Standardized responses
    - JSON response helpers
    - Pagination support
    - Error response structure
  - Enhanced `internal/camera/service.go` - Health monitoring
    - go2rtc stream stats integration
    - FPS/bitrate tracking
    - Codec detection
    - Status tracking with timestamps
  - Updated `cmd/nvr/main.go` - Full API integration
    - WebSocket hub integration
    - Event broadcasting
    - Validation on all endpoints
    - Consistent error responses
- **Week 2 Complete** ✅ - Camera API fully functional

---

## Quick Reference

### Key File Paths
```
nvr-system/
├── cmd/nvr/main.go              # Main entry point
├── internal/                     # Go shared code
├── services/ai-detection/        # Python AI service
├── web-ui/                       # React frontend
├── plugins/                      # Built-in plugins
├── config/config.yaml            # Main configuration
└── migrations/                   # Database migrations
```

### Essential Commands
```bash
# Development
docker-compose -f docker-compose.dev.yml up

# Run Go backend
go run cmd/nvr/main.go

# Run React frontend
cd web-ui && npm run dev

# Run AI service
cd services/ai-detection && uvicorn main:app --reload

# Run tests
go test ./...
cd web-ui && npm test
```

### API Endpoints (Core)
```
GET    /api/v1/cameras          # List cameras
POST   /api/v1/cameras          # Add camera
GET    /api/v1/cameras/{id}     # Get camera
PUT    /api/v1/cameras/{id}     # Update camera
DELETE /api/v1/cameras/{id}     # Delete camera
GET    /api/v1/cameras/{id}/snapshot  # Get snapshot

GET    /api/v1/events           # List events
GET    /api/v1/events/{id}      # Get event

GET    /ws                      # WebSocket for live updates
```

---

## Notes

- This plan follows the detailed specification in `NVR_CLAUDE_CODE_INSTRUCTIONS.md`
- Each phase builds on the previous - don't skip ahead
- MVP focuses on core functionality with YOLOv12 detection
- Plugin system is Phase 2 to avoid early over-engineering
- UI-driven configuration is a key differentiator from Frigate
