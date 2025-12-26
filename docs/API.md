# NVR System API Documentation

## Overview

The NVR System provides a RESTful API for managing cameras, events, timeline data, configuration, and plugins. All API endpoints are prefixed with `/api/v1`.

## Base URL

```
http://localhost:5000/api/v1
```

## Authentication

Authentication is handled via API keys or bearer tokens (implementation-dependent). Include the token in the `Authorization` header:

```
Authorization: Bearer <token>
```

## Response Format

All responses follow a standard JSON structure:

### Success Response

```json
{
  "success": true,
  "data": { ... },
  "meta": {
    "total": 100,
    "page": 1,
    "per_page": 20,
    "total_pages": 5
  }
}
```

### Error Response

```json
{
  "success": false,
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable error message",
    "details": []
  }
}
```

---

## Health Check

### GET /health

Check system health status.

**Response**

```json
{
  "status": "healthy",
  "version": "0.1.0",
  "uptime": 3600,
  "components": {
    "database": "healthy",
    "go2rtc": "healthy"
  }
}
```

---

## Cameras

### GET /api/v1/cameras

List all cameras.

**Query Parameters**

| Parameter | Type | Description |
|-----------|------|-------------|
| limit | int | Maximum number of results (default: 50) |
| offset | int | Offset for pagination |
| enabled | bool | Filter by enabled status |

**Response**

```json
{
  "success": true,
  "data": [
    {
      "id": "front_door",
      "name": "Front Door Camera",
      "status": "online",
      "enabled": true,
      "manufacturer": "Reolink",
      "model": "RLC-810A",
      "stream_url": "rtsp://192.168.1.100:554/stream",
      "last_seen": "2024-01-15T10:30:00Z",
      "fps_current": 25.0,
      "bitrate_current": 4000000,
      "resolution_current": "1920x1080",
      "created_at": "2024-01-01T00:00:00Z",
      "updated_at": "2024-01-15T10:30:00Z"
    }
  ],
  "meta": {
    "total": 5,
    "page": 1,
    "per_page": 50,
    "total_pages": 1
  }
}
```

### POST /api/v1/cameras

Create a new camera.

**Request Body**

```json
{
  "name": "Front Door Camera",
  "stream": {
    "url": "rtsp://192.168.1.100:554/stream",
    "sub_url": "rtsp://192.168.1.100:554/substream",
    "username": "admin",
    "password": "password123"
  },
  "manufacturer": "Reolink",
  "model": "RLC-810A",
  "detection": {
    "enabled": true,
    "fps": 5,
    "objects": ["person", "vehicle"]
  },
  "recording": {
    "enabled": true,
    "pre_buffer_seconds": 5,
    "post_buffer_seconds": 10
  }
}
```

**Response** (201 Created)

```json
{
  "success": true,
  "data": {
    "id": "Front_Door_Camera_a1b2c3d4",
    "name": "Front Door Camera",
    "status": "starting",
    "enabled": true,
    "created_at": "2024-01-15T10:30:00Z",
    "updated_at": "2024-01-15T10:30:00Z"
  }
}
```

### GET /api/v1/cameras/{id}

Get a specific camera.

**Path Parameters**

| Parameter | Type | Description |
|-----------|------|-------------|
| id | string | Camera ID |

**Response**

```json
{
  "success": true,
  "data": {
    "id": "front_door",
    "name": "Front Door Camera",
    "status": "online",
    "enabled": true,
    ...
  }
}
```

### PUT /api/v1/cameras/{id}

Update a camera.

**Request Body**

Same as POST /cameras

**Response**

```json
{
  "success": true,
  "data": {
    "id": "front_door",
    ...
  }
}
```

### DELETE /api/v1/cameras/{id}

Delete a camera.

**Response** (204 No Content)

No body returned.

### GET /api/v1/cameras/{id}/snapshot

Get a snapshot image from a camera.

**Response**

Returns a JPEG image with `Content-Type: image/jpeg`.

---

## Events

### GET /api/v1/events

List events with optional filtering.

**Query Parameters**

| Parameter | Type | Description |
|-----------|------|-------------|
| camera_id | string | Filter by camera |
| event_type | string | Filter by type (motion, person, vehicle, etc.) |
| start_time | string | ISO 8601 start time |
| end_time | string | ISO 8601 end time |
| limit | int | Maximum results (default: 50) |
| offset | int | Offset for pagination |
| unacknowledged | bool | Only unacknowledged events |

**Response**

```json
{
  "success": true,
  "data": [
    {
      "id": "evt_123abc",
      "camera_id": "front_door",
      "event_type": "person",
      "label": "person",
      "timestamp": "2024-01-15T10:30:00Z",
      "end_timestamp": "2024-01-15T10:30:15Z",
      "confidence": 0.95,
      "thumbnail_path": "/data/thumbnails/evt_123abc.jpg",
      "video_segment_id": "seg_456def",
      "acknowledged": false,
      "tags": ["daytime", "front"],
      "notes": "",
      "created_at": "2024-01-15T10:30:00Z"
    }
  ],
  "meta": {
    "total": 150,
    "page": 1,
    "per_page": 50,
    "total_pages": 3
  }
}
```

### GET /api/v1/events/{id}

Get a specific event.

**Response**

```json
{
  "success": true,
  "data": {
    "id": "evt_123abc",
    ...
  }
}
```

### PUT /api/v1/events/{id}/acknowledge

Acknowledge an event.

**Response**

```json
{
  "success": true,
  "data": {
    "acknowledged": true
  }
}
```

---

## Timeline

### GET /api/v1/timeline/{cameraId}

Get timeline data for a camera.

**Path Parameters**

| Parameter | Type | Description |
|-----------|------|-------------|
| cameraId | string | Camera ID |

**Query Parameters**

| Parameter | Type | Description |
|-----------|------|-------------|
| start | string | ISO 8601 start time |
| end | string | ISO 8601 end time |

**Response**

```json
{
  "success": true,
  "data": {
    "camera_id": "front_door",
    "segments": [
      {
        "start": "2024-01-15T10:00:00Z",
        "end": "2024-01-15T10:05:00Z",
        "type": "recording",
        "has_events": true
      }
    ]
  }
}
```

### GET /api/v1/timeline/{cameraId}/segments

Get detailed segments for a camera timeline.

**Response**

```json
{
  "success": true,
  "data": {
    "camera_id": "front_door",
    "segments": [...]
  }
}
```

---

## Configuration

### GET /api/v1/config

Get current system configuration.

**Response**

```json
{
  "success": true,
  "data": {
    "version": "1.0",
    "system": {
      "name": "My NVR",
      "timezone": "America/New_York",
      "storage_path": "/data",
      "max_storage_gb": 1000
    },
    "cameras": [...],
    "detectors": {...},
    "plugins": {...}
  }
}
```

### PUT /api/v1/config

Update system configuration.

**Request Body**

```json
{
  "system": {
    "name": "Updated NVR Name",
    "timezone": "America/Los_Angeles"
  }
}
```

**Response**

```json
{
  "success": true,
  "data": {
    "message": "Configuration updated successfully"
  }
}
```

---

## Statistics

### GET /api/v1/stats

Get system statistics.

**Query Parameters**

| Parameter | Type | Description |
|-----------|------|-------------|
| camera_id | string | Filter stats by camera |

**Response**

```json
{
  "success": true,
  "data": {
    "cameras": {
      "total": 5,
      "online": 4,
      "offline": 1
    },
    "events": {
      "today": 42,
      "unacknowledged": 5
    },
    "storage": {
      "used_gb": 450.5,
      "total_gb": 1000,
      "percent": 45.05
    },
    "uptime_seconds": 86400
  }
}
```

---

## Plugins

### GET /api/v1/plugins

List all plugins.

**Response**

```json
{
  "success": true,
  "data": {
    "coral_tpu": {
      "enabled": true,
      "config": {
        "device_path": "/dev/apex_0"
      }
    },
    "frigate_detector": {
      "enabled": false,
      "config": {}
    }
  }
}
```

### POST /api/v1/plugins/{id}/enable

Enable a plugin.

**Response**

```json
{
  "success": true,
  "data": {
    "message": "Plugin enabled"
  }
}
```

### POST /api/v1/plugins/{id}/disable

Disable a plugin.

**Response**

```json
{
  "success": true,
  "data": {
    "message": "Plugin disabled"
  }
}
```

---

## WebSocket

### GET /ws

WebSocket connection for real-time updates.

**Connection**

```javascript
const ws = new WebSocket('ws://localhost:5000/ws');

ws.onopen = () => {
  // Subscribe to events
  ws.send(JSON.stringify({
    type: 'subscribe',
    cameras: ['front_door', 'back_yard']
  }));
};

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Received:', data);
};
```

**Message Types**

#### Subscribe

```json
{
  "type": "subscribe",
  "cameras": ["front_door", "back_yard"]
}
```

#### Event Notification

```json
{
  "type": "event",
  "data": {
    "id": "evt_123abc",
    "camera_id": "front_door",
    "event_type": "person",
    "timestamp": "2024-01-15T10:30:00Z",
    "confidence": 0.95
  }
}
```

#### Camera Status Update

```json
{
  "type": "camera_status",
  "data": {
    "camera_id": "front_door",
    "status": "online",
    "fps": 25.0,
    "bitrate": 4000000
  }
}
```

---

## Error Codes

| Code | Description |
|------|-------------|
| BAD_REQUEST | Invalid request parameters |
| VALIDATION_ERROR | Validation failed (details in response) |
| NOT_FOUND | Resource not found |
| UNAUTHORIZED | Authentication required |
| FORBIDDEN | Insufficient permissions |
| CONFLICT | Resource conflict |
| INTERNAL_ERROR | Internal server error |

---

## Rate Limiting

API requests are rate-limited to prevent abuse:

- 100 requests per minute per IP address
- 1000 requests per hour per API key

Rate limit headers are included in responses:

```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1705315260
```

---

## Pagination

List endpoints support pagination with the following parameters:

| Parameter | Type | Description |
|-----------|------|-------------|
| limit | int | Items per page (max 100, default 50) |
| offset | int | Number of items to skip |

Response includes metadata:

```json
{
  "meta": {
    "total": 500,
    "page": 2,
    "per_page": 50,
    "total_pages": 10
  }
}
```
