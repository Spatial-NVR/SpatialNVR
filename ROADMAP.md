# NVR System Development Roadmap

## Phase 1: Foundation (Weeks 1-3) - COMPLETE

### Week 1: Core Infrastructure
- [x] Project structure and Go modules
- [x] Configuration management (YAML, env vars)
- [x] SQLite database with migrations
- [x] Basic HTTP server with Chi router
- [x] Health check endpoints
- [x] Docker compose setup

### Week 2: Camera Management
- [x] Camera CRUD API
- [x] go2rtc integration for stream handling
- [x] Multiple protocol support (RTSP, ONVIF)
- [x] Stream proxy endpoints
- [x] React frontend with camera management UI
- [x] Live view with MSE/WebRTC playback

### Week 3: Video Processing & Storage
- [x] FFmpeg-based video segmentation
- [x] HLS segment storage service
- [x] Recording database model and repository
- [x] Retention policy management
- [x] Timeline query API
- [x] Hardware acceleration auto-detection (CUDA, VideoToolbox, VAAPI, QSV)

---

## Phase 2: Intelligence (Weeks 4-6)

### Week 4: Event Detection & Audio Support - COMPLETE
- [x] Motion detection zones (configurable polygons)
- [x] Motion sensitivity settings per camera
- [x] Event database model and storage
- [x] Event notification framework
- [x] **Audio support for video playback**
- [x] **Two-way audio communication** (intercom)
- [x] **Video doorbell support** (ring detection, live answer)
- [x] AI detection integration hooks (person, vehicle, animal)
- [x] **Plugin marketplace and hot-reload** (advanced from Week 9)

### Week 5: Timeline & Playback UI - COMPLETE
- [x] Timeline scrubber component (with zoom 1h-24h, playhead drag)
- [x] Event markers on timeline (color-coded icons for person, vehicle, animal, motion)
- [x] Continuous recording playback (segment auto-continue on video end)
- [x] Clip export functionality (start/end selection with visual range overlay)
- [x] Hover time preview on timeline
- [x] Quick jump to events (event list panel with filtering)
- [x] Playback speed control (0.25x to 4x)

### Week 6: Multi-camera Views
- [ ] Configurable grid layouts (2x2, 3x3, 4x4, custom)
- [ ] Saved view presets
- [ ] Camera grouping
- [ ] PTZ control integration
- [ ] Tour mode (auto-cycle cameras)

---

## Phase 3: AI & Search (Weeks 7-8)

### Week 7: AI Semantic Search
- [ ] **Image embedding pipeline** (CLIP or similar)
- [ ] **Face detection and recognition**
- [ ] **License plate recognition (LPR/ANPR)**
- [ ] **Object classification and tagging**
- [ ] **Natural language search** ("show me the red car yesterday")
- [ ] **Similar image search**
- [ ] **AI-generated event descriptions**
- [ ] Vector database integration (for embeddings)
- [ ] Search results UI with filters

### Week 8: Rich Notification System
- [ ] **Notification template engine** (customizable messages)
- [ ] **Notification rules engine** (conditions, schedules, cooldowns)
- [ ] **PWA push notifications** (web push API)
- [ ] **Mobile app notification foundation** (for future iOS/Android)
- [ ] **Email notifications** (SMTP/SendGrid)
- [ ] **Webhook notifications** (for integrations)
- [ ] **SMS notifications** (Twilio/SNS)
- [ ] **Notification history and management UI**
- [ ] **Quiet hours / Do Not Disturb schedules**
- [ ] **Per-camera notification preferences**

---

## Phase 4: Integrations (Week 9)

### Week 9: Camera Integrations & Plugin Foundation
- [ ] **Reolink native integration** (API, PTZ, AI events)
- [ ] **Wyze Bridge integration** (via Docker)
- [ ] **Amcrest/Dahua integration**
- [ ] **ONVIF device discovery improvements**
- [ ] **Plugin architecture design**
  - Plugin manifest format
  - Plugin lifecycle management
  - Plugin API (events, storage, UI extensions)
  - Plugin isolation and sandboxing
- [ ] **Plugin installation from local files**
- [ ] **Foundation for marketplace** (separate repository)

---

## Phase 5: Production Ready (Weeks 10-12)

### Week 10: Performance & Reliability
- [ ] Connection pooling and retry logic
- [ ] Memory optimization for many cameras
- [ ] Database query optimization
- [ ] Caching layer (Redis optional)
- [ ] Graceful degradation when cameras offline
- [ ] Automatic stream recovery

### Week 11: Security & Access Control
- [ ] User authentication (local accounts)
- [ ] Role-based access control (admin, viewer, etc.)
- [ ] Camera-level permissions
- [ ] Audit logging
- [ ] HTTPS/TLS support
- [ ] API key management

### Week 12: Deployment & Documentation
- [ ] Production Docker images
- [ ] Kubernetes manifests
- [ ] Installation scripts
- [ ] User documentation
- [ ] API documentation
- [ ] Backup and restore procedures

---

## Future Considerations

### Marketplace (Separate Repository)
- Plugin discovery and browsing
- Version management
- User ratings and reviews
- Developer portal
- Plugin signing and verification

### Native Mobile Apps
- iOS app (Swift/SwiftUI)
- Android app (Kotlin/Compose)
- Live view with hardware decoding
- Push notification handling
- Offline clip viewing

### Advanced Features
- Cloud backup integration (S3, GCS, B2)
- Multi-site/remote NVR federation
- Analytics dashboard
- Time-lapse generation
- Privacy masking zones
- Integration with home automation (Home Assistant, etc.)
