/**
 * NVR API Client
 *
 * This module provides type-safe API calls to the NVR backend.
 * Ports are dynamically discovered from the backend at runtime.
 */

import { getApiUrl, getGo2RTCUrl, getGo2RTCWsUrl, fetchPorts } from './ports';

// Initialize port discovery on module load
fetchPorts().catch(() => {
  console.warn('[API] Failed to fetch ports, using defaults');
});

// Get API base URL dynamically
function getApiBase(): string {
  // Check for environment override first
  if (import.meta.env.VITE_API_URL) {
    return import.meta.env.VITE_API_URL;
  }
  return getApiUrl();
}

// For backwards compatibility - this is now a getter
const API_BASE = getApiBase();

// ============================================================================
// Types
// ============================================================================

export interface ApiResponse<T> {
  success: boolean;
  data?: T;
  error?: {
    code: string;
    message: string;
    details?: ValidationError[];
  };
  meta?: {
    total: number;
    page: number;
    per_page: number;
    total_pages: number;
  };
}

export interface ValidationError {
  field: string;
  message: string;
}

export type AspectRatio = '16:9' | '21:9' | '4:3' | '3:4' | '1:1' | 'auto';

export interface Camera {
  id: string;
  name: string;
  status: 'online' | 'offline' | 'error' | 'starting';
  enabled: boolean;
  manufacturer?: string;
  model?: string;
  stream_url?: string;
  last_seen?: string;
  fps_current?: number;
  bitrate_current?: number;
  resolution_current?: string;
  error_message?: string;
  display_aspect_ratio?: AspectRatio;
  created_at: string;
  updated_at: string;
  // Plugin association (empty = manually added camera)
  plugin_id?: string;
  plugin_camera_id?: string;
}

// Camera capability types for plugin-managed cameras
export interface CameraCapabilities {
  has_ptz: boolean;
  has_audio: boolean;
  has_two_way_audio: boolean;
  has_snapshot: boolean;
  device_type: string;
  is_doorbell: boolean;
  is_nvr: boolean;
  is_battery: boolean;
  has_ai_detection: boolean;
  ai_types?: string[];
  protocols: string[];
  current_protocol: string;
  ptz_presets?: PTZPreset[];
  features?: Record<string, unknown>;
  // If true, this camera is managed by a plugin
  is_plugin_managed?: boolean;
  plugin_id?: string;
  plugin_camera_id?: string;
}

export interface PTZPreset {
  id: string;
  name: string;
}

export interface PTZCommand {
  action: 'pan' | 'tilt' | 'zoom' | 'stop' | 'preset';
  direction?: number;  // -1.0 to 1.0
  speed?: number;      // 0.0 to 1.0
  preset?: string;     // preset id for preset action
}

export interface ProtocolOption {
  id: string;
  name: string;
  description?: string;
  stream_url?: string;
}

export interface DeviceInfo {
  model: string;
  manufacturer: string;
  serial?: string;
  firmware_version?: string;
  hardware_version?: string;
  channel_count: number;
  device_type?: string;
  plugin_id?: string;
  plugin_camera_id?: string;
}

export type StreamRole = 'detect' | 'record' | 'audio' | 'motion';

export interface CameraConfig {
  name: string;
  stream: {
    url: string;
    sub_url?: string;
    username?: string;
    password?: string;
    // Which stream to use for each role: 'main' or 'sub'
    roles?: {
      detect?: 'main' | 'sub';
      record?: 'main' | 'sub';
      audio?: 'main' | 'sub';
      motion?: 'main' | 'sub';
    };
  };
  manufacturer?: string;
  model?: string;
  display_aspect_ratio?: AspectRatio;
  recording?: {
    enabled?: boolean;
    mode?: 'continuous' | 'motion' | 'events';
    pre_buffer_seconds?: number;
    post_buffer_seconds?: number;
    retention?: {
      default_days?: number;
      events_days?: number;
    };
  };
  detection?: {
    enabled?: boolean;
    fps?: number;
    models?: string[];
    show_overlay?: boolean;
    min_confidence?: number;
  };
  audio?: {
    enabled?: boolean;
    two_way?: boolean;
  };
  motion?: {
    enabled?: boolean;
    threshold?: number;
    method?: 'frame_diff' | 'mog2' | 'knn';
  };
}

export interface Event {
  id: string;
  camera_id: string;
  event_type: string;
  label?: string;
  timestamp: number;
  end_timestamp?: number;
  confidence: number;
  thumbnail_path?: string;
  acknowledged: boolean;
  created_at: string;
}

export interface Stats {
  cameras: {
    total: number;
    online: number;
    offline: number;
  };
  events: {
    today: number;
    unacknowledged: number;
    total: number;
  };
  storage: {
    database_size: number;
  };
}

export interface DetectionConfig {
  backend: string;
  fps: number;
  objects: {
    enabled: boolean;
    model: string;
    confidence: number;
    classes: string[];
  };
  faces: {
    enabled: boolean;
    model: string;
    confidence: number;
  };
  lpr: {
    enabled: boolean;
    model: string;
    confidence: number;
  };
}

export interface SystemConfig {
  version: string;
  system: {
    name: string;
    timezone: string;
    storage_path: string;
    max_storage_gb: number;
    updates?: {
      github_token?: string;
    };
    deployment: {
      mode: string;
    };
    logging: {
      level: string;
      format: string;
    };
  };
  cameras_count: number;
  detection?: DetectionConfig;
  storage: {
    recordings: string;
    thumbnails: string;
    snapshots: string;
    exports: string;
    retention: {
      default_days: number;
    };
  };
  preferences: {
    ui: {
      theme: string;
      language: string;
      dashboard: {
        grid_columns: number;
        show_fps: boolean;
      };
    };
    timeline: {
      default_range_hours: number;
      thumbnail_interval_seconds: number;
    };
    events: {
      auto_acknowledge_after_days: number;
      group_similar_events: boolean;
      group_window_seconds: number;
    };
  };
}

export interface HealthStatus {
  status: 'healthy' | 'degraded' | 'unhealthy';
  version: string;
  go2rtc: boolean;
  database: string;
}

// ============================================================================
// API Error
// ============================================================================

export class ApiError extends Error {
  code: string;
  status: number;
  details?: ValidationError[];

  constructor(message: string, code: string, status: number, details?: ValidationError[]) {
    super(message);
    this.name = 'ApiError';
    this.code = code;
    this.status = status;
    this.details = details;
  }
}

// ============================================================================
// HTTP Client
// ============================================================================

async function request<T>(
  endpoint: string,
  options: RequestInit = {}
): Promise<T> {
  const url = `${API_BASE}${endpoint}`;

  const headers: HeadersInit = {
    'Content-Type': 'application/json',
    ...options.headers,
  };

  let response: Response;
  try {
    response = await fetch(url, {
      ...options,
      headers,
    });
  } catch {
    // Network error - backend not reachable
    throw new ApiError(
      'Cannot connect to NVR backend. Please ensure the server is running.',
      'NETWORK_ERROR',
      0
    );
  }

  // Handle non-JSON responses (like snapshots)
  const contentType = response.headers.get('content-type');
  if (contentType && !contentType.includes('application/json')) {
    if (!response.ok) {
      throw new ApiError('Request failed', 'REQUEST_FAILED', response.status);
    }
    return response.blob() as unknown as T;
  }

  // Handle 204 No Content (e.g., DELETE requests)
  if (response.status === 204) {
    return undefined as unknown as T;
  }

  const json = await response.json();

  // Handle both wrapped {success, data} and unwrapped responses
  // This allows compatibility with different backend response formats
  if ('success' in json && 'data' in json) {
    // Wrapped format: {success: boolean, data: T, error?: {...}}
    const data = json as ApiResponse<T>;
    if (!response.ok || !data.success) {
      throw new ApiError(
        data.error?.message || 'Request failed',
        data.error?.code || 'UNKNOWN_ERROR',
        response.status,
        data.error?.details
      );
    }
    return data.data as T;
  } else if ('error' in json && typeof json.error === 'string') {
    // Error response: {error: "message"}
    throw new ApiError(
      json.error,
      'REQUEST_FAILED',
      response.status
    );
  } else {
    // Unwrapped format: T directly
    if (!response.ok) {
      throw new ApiError(
        'Request failed',
        'REQUEST_FAILED',
        response.status
      );
    }
    return json as T;
  }
}

// ============================================================================
// Camera API
// ============================================================================

export const cameraApi = {
  /**
   * List all cameras
   */
  list: async (): Promise<Camera[]> => {
    return request<Camera[]>('/api/v1/cameras');
  },

  /**
   * Get a single camera by ID
   */
  get: async (id: string): Promise<Camera> => {
    return request<Camera>(`/api/v1/cameras/${id}`);
  },

  /**
   * Create a new camera
   */
  create: async (config: CameraConfig): Promise<Camera> => {
    return request<Camera>('/api/v1/cameras', {
      method: 'POST',
      body: JSON.stringify(config),
    });
  },

  /**
   * Update an existing camera
   */
  update: async (id: string, config: Partial<CameraConfig>): Promise<Camera> => {
    return request<Camera>(`/api/v1/cameras/${id}`, {
      method: 'PUT',
      body: JSON.stringify(config),
    });
  },

  /**
   * Delete a camera
   */
  delete: async (id: string): Promise<void> => {
    await request<void>(`/api/v1/cameras/${id}`, {
      method: 'DELETE',
    });
  },

  /**
   * Get full camera configuration (including recording, detection, etc.)
   */
  getConfig: async (id: string): Promise<CameraConfig & { id: string; enabled: boolean }> => {
    return request<CameraConfig & { id: string; enabled: boolean }>(`/api/v1/cameras/${id}/config`);
  },

  /**
   * Get camera snapshot as blob
   */
  getSnapshot: async (id: string): Promise<Blob> => {
    return request<Blob>(`/api/v1/cameras/${id}/snapshot`);
  },

  /**
   * Get snapshot URL for display
   */
  getSnapshotUrl: (id: string): string => {
    return `${API_BASE}/api/v1/cameras/${id}/snapshot`;
  },

  /**
   * Get camera capabilities (for plugin-managed cameras, may include plugin info)
   */
  getCapabilities: async (id: string): Promise<CameraCapabilities> => {
    return request<CameraCapabilities>(`/api/v1/cameras/${id}/capabilities`);
  },

  /**
   * Get PTZ presets for a camera
   */
  getPTZPresets: async (id: string): Promise<PTZPreset[]> => {
    const result = await request<PTZPreset[] | { presets: PTZPreset[]; plugin_id?: string }>(`/api/v1/cameras/${id}/ptz/presets`);
    // Handle both array and object response
    if (Array.isArray(result)) {
      return result;
    }
    return result.presets || [];
  },

  /**
   * Send PTZ control command
   * For plugin-managed cameras, this returns plugin info to call via plugin RPC
   */
  ptzControl: async (id: string, command: PTZCommand): Promise<void> => {
    await request<void>(`/api/v1/cameras/${id}/ptz/control`, {
      method: 'POST',
      body: JSON.stringify(command),
    });
  },

  /**
   * Get available streaming protocols for a camera
   */
  getProtocols: async (id: string): Promise<ProtocolOption[]> => {
    const result = await request<ProtocolOption[] | { protocols: ProtocolOption[]; plugin_id?: string }>(`/api/v1/cameras/${id}/protocols`);
    // Handle both array and object response
    if (Array.isArray(result)) {
      return result;
    }
    return result.protocols || [];
  },

  /**
   * Set the streaming protocol for a plugin-managed camera
   */
  setProtocol: async (id: string, protocol: string): Promise<void> => {
    await request<void>(`/api/v1/cameras/${id}/protocol`, {
      method: 'PUT',
      body: JSON.stringify({ protocol }),
    });
  },

  /**
   * Get device information for a camera
   */
  getDeviceInfo: async (id: string): Promise<DeviceInfo> => {
    return request<DeviceInfo>(`/api/v1/cameras/${id}/device-info`);
  },
};

// ============================================================================
// Event API
// ============================================================================

export const eventApi = {
  /**
   * List events with optional filters
   */
  list: async (params?: {
    camera_id?: string;
    type?: string;
    page?: number;
    per_page?: number;
    limit?: number;
    offset?: number;
  }): Promise<{ data: Event[]; total: number }> => {
    const searchParams = new URLSearchParams();
    if (params?.camera_id) searchParams.set('camera_id', params.camera_id);
    if (params?.type) searchParams.set('type', params.type);
    if (params?.limit) searchParams.set('limit', params.limit.toString());
    if (params?.offset) searchParams.set('offset', params.offset.toString());
    // Legacy page/per_page support
    if (params?.page) searchParams.set('page', params.page.toString());
    if (params?.per_page) searchParams.set('per_page', params.per_page.toString());

    const query = searchParams.toString();
    const endpoint = `/api/v1/events${query ? `?${query}` : ''}`;

    // Backend returns { events: [...] }, normalize to { data: [...] }
    const response = await request<{ events?: Event[]; data?: Event[]; total: number }>(endpoint);
    return {
      data: response.events || response.data || [],
      total: response.total,
    };
  },

  /**
   * Get a single event by ID
   */
  get: async (id: string): Promise<Event> => {
    return request<Event>(`/api/v1/events/${id}`);
  },

  /**
   * Acknowledge an event
   */
  acknowledge: async (id: string): Promise<void> => {
    await request<void>(`/api/v1/events/${id}/acknowledge`, {
      method: 'POST',
    });
  },
};

// Alias for eventsApi
export const eventsApi = eventApi;

// ============================================================================
// Stats API
// ============================================================================

export const statsApi = {
  /**
   * Get system statistics
   */
  get: async (): Promise<Stats> => {
    return request<Stats>('/api/v1/stats');
  },
};

// ============================================================================
// Config API
// ============================================================================

export interface ConfigUpdate {
  system?: {
    name?: string;
    timezone?: string;
    max_storage_gb?: number;
    updates?: {
      github_token?: string;
    };
  };
  storage?: {
    retention?: {
      default_days?: number;
    };
  };
  detection?: {
    backend?: string;
    fps?: number;
    objects?: {
      enabled?: boolean;
      model?: string;
      confidence?: number;
      classes?: string[];
    };
    faces?: {
      enabled?: boolean;
      model?: string;
      confidence?: number;
    };
    lpr?: {
      enabled?: boolean;
      model?: string;
      confidence?: number;
    };
  };
  preferences?: {
    ui?: {
      theme?: string;
      language?: string;
      dashboard?: {
        grid_columns?: number;
        show_fps?: boolean;
      };
    };
    timeline?: {
      default_range_hours?: number;
      thumbnail_interval_seconds?: number;
    };
    events?: {
      auto_acknowledge_after_days?: number;
      group_similar_events?: boolean;
      group_window_seconds?: number;
    };
  };
}

export const configApi = {
  /**
   * Get system configuration
   */
  get: async (): Promise<SystemConfig> => {
    return request<SystemConfig>('/api/v1/config');
  },

  /**
   * Update system configuration
   */
  update: async (config: ConfigUpdate): Promise<{ message: string }> => {
    return request<{ message: string }>('/api/v1/config', {
      method: 'PUT',
      body: JSON.stringify(config),
    });
  },
};

// ============================================================================
// Health API
// ============================================================================

export const healthApi = {
  /**
   * Get system health status
   */
  check: async (): Promise<HealthStatus> => {
    const response = await fetch(`${API_BASE}/health`);
    return response.json();
  },
};

// ============================================================================
// Timeline API
// ============================================================================

export interface TimelineSegment {
  id: string;
  camera_id: string;
  start_time: string;
  end_time: string;
  duration: number;
  file_path: string;
  file_size: number;
  has_motion: boolean;
  has_events: boolean;
}

export interface StorageStats {
  total_bytes: number;
  used_bytes: number;
  available_bytes: number;
  segment_count: number;
  by_camera: Record<string, number>;
  by_tier: Record<string, number>;
}

export const timelineApi = {
  /**
   * Get timeline for a camera
   */
  get: async (cameraId: string, params?: {
    start?: string;
    end?: string;
  }): Promise<TimelineSegment[]> => {
    const searchParams = new URLSearchParams();
    if (params?.start) searchParams.set('start', params.start);
    if (params?.end) searchParams.set('end', params.end);

    const query = searchParams.toString();
    const endpoint = `/api/v1/timeline/${cameraId}${query ? `?${query}` : ''}`;

    const result = await request<{ segments: TimelineSegment[] }>(endpoint);
    return result.segments;
  },

  /**
   * Get segments for a camera
   */
  getSegments: async (cameraId: string, params?: {
    start?: string;
    end?: string;
  }): Promise<TimelineSegment[]> => {
    const searchParams = new URLSearchParams();
    if (params?.start) searchParams.set('start', params.start);
    if (params?.end) searchParams.set('end', params.end);

    const query = searchParams.toString();
    const endpoint = `/api/v1/timeline/${cameraId}/segments${query ? `?${query}` : ''}`;

    const result = await request<{ segments: TimelineSegment[] }>(endpoint);
    return result.segments;
  },
};

// ============================================================================
// Storage API
// ============================================================================

export const storageApi = {
  /**
   * Get storage statistics
   */
  getStats: async (): Promise<StorageStats> => {
    return request<StorageStats>('/api/v1/recordings/storage');
  },

  /**
   * Run retention cleanup manually
   */
  runRetention: async (): Promise<{ message: string; segments_deleted: number; bytes_freed: number }> => {
    return request<{ message: string; segments_deleted: number; bytes_freed: number }>('/api/v1/recordings/retention/run', {
      method: 'POST',
    });
  },
};

// ============================================================================
// Recording Control API
// ============================================================================

export interface RecordingStatus {
  camera_id: string;
  state: 'idle' | 'starting' | 'running' | 'stopping' | 'error';
  current_segment?: string;
  segment_start?: string;
  bytes_written: number;
  segments_created: number;
  uptime: number;
  last_error?: string;
  last_error_time?: string;
}

export const recordingApi = {
  /**
   * Get recording status for a camera
   */
  getStatus: async (cameraId: string): Promise<RecordingStatus> => {
    return request<RecordingStatus>(`/api/v1/recordings/status/${cameraId}`);
  },

  /**
   * Get recording status for all cameras
   */
  getAllStatus: async (): Promise<Record<string, RecordingStatus>> => {
    return request<Record<string, RecordingStatus>>('/api/v1/recordings/status');
  },

  /**
   * Start recording for a camera
   */
  start: async (cameraId: string): Promise<{ camera_id: string; status: string }> => {
    return request<{ camera_id: string; status: string }>(`/api/v1/recordings/cameras/${cameraId}/start`, {
      method: 'POST',
    });
  },

  /**
   * Stop recording for a camera
   */
  stop: async (cameraId: string): Promise<{ camera_id: string; status: string }> => {
    return request<{ camera_id: string; status: string }>(`/api/v1/recordings/cameras/${cameraId}/stop`, {
      method: 'POST',
    });
  },

  /**
   * Restart recording for a camera
   */
  restart: async (cameraId: string): Promise<{ camera_id: string; status: string }> => {
    return request<{ camera_id: string; status: string }>(`/api/v1/recordings/cameras/${cameraId}/restart`, {
      method: 'POST',
    });
  },
};

// ============================================================================
// WebSocket Client
// ============================================================================

export type WebSocketMessageType =
  | 'event'
  | 'camera_state'
  | 'detection'
  | 'stats'
  | 'doorbell'
  | 'audio_state'
  | 'ping'
  | 'pong'
  | 'subscribe'
  | 'unsubscribe';

export interface WebSocketMessage {
  type: WebSocketMessageType;
  timestamp: string;
  data?: unknown;
}

export class NVRWebSocket {
  private ws: WebSocket | null = null;
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 5;
  private reconnectDelay = 1000;
  private listeners: Map<string, Set<(data: unknown) => void>> = new Map();

  // Lazily compute the WebSocket URL to avoid issues with early module loading
  private getWsUrl(): string {
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    try {
      const apiHost = new URL(getApiBase()).host;
      return `${wsProtocol}//${apiHost}/ws`;
    } catch {
      // Fallback to current origin if URL parsing fails
      console.warn('[NVRWebSocket] Failed to parse API URL, using current host');
      return `${wsProtocol}//${window.location.host}/ws`;
    }
  }

  connect(): void {
    if (this.ws?.readyState === WebSocket.OPEN) {
      return;
    }

    const url = this.getWsUrl();
    this.ws = new WebSocket(url);

    this.ws.onopen = () => {
      console.log('WebSocket connected');
      this.reconnectAttempts = 0;
    };

    this.ws.onmessage = (event) => {
      try {
        const message: WebSocketMessage = JSON.parse(event.data);
        this.emit(message.type, message.data);
        this.emit('*', message); // Wildcard listener
      } catch (e) {
        console.error('Failed to parse WebSocket message', e);
      }
    };

    this.ws.onclose = () => {
      console.log('WebSocket disconnected');
      this.scheduleReconnect();
    };

    this.ws.onerror = (error) => {
      console.error('WebSocket error', error);
    };
  }

  private scheduleReconnect(): void {
    if (this.reconnectAttempts >= this.maxReconnectAttempts) {
      console.error('Max reconnect attempts reached');
      return;
    }

    this.reconnectAttempts++;
    const delay = this.reconnectDelay * Math.pow(2, this.reconnectAttempts - 1);

    setTimeout(() => {
      console.log(`Reconnecting WebSocket (attempt ${this.reconnectAttempts})`);
      this.connect();
    }, delay);
  }

  disconnect(): void {
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }

  send(message: WebSocketMessage): void {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(message));
    }
  }

  subscribe(cameras: string[]): void {
    this.send({
      type: 'subscribe',
      timestamp: new Date().toISOString(),
      data: cameras,
    });
  }

  unsubscribe(cameras: string[]): void {
    this.send({
      type: 'unsubscribe',
      timestamp: new Date().toISOString(),
      data: cameras,
    });
  }

  on(event: string, callback: (data: unknown) => void): () => void {
    if (!this.listeners.has(event)) {
      this.listeners.set(event, new Set());
    }
    this.listeners.get(event)!.add(callback);

    // Return unsubscribe function
    return () => {
      this.listeners.get(event)?.delete(callback);
    };
  }

  off(event: string, callback: (data: unknown) => void): void {
    this.listeners.get(event)?.delete(callback);
  }

  isConnected(): boolean {
    return this.ws?.readyState === WebSocket.OPEN;
  }

  private emit(event: string, data: unknown): void {
    this.listeners.get(event)?.forEach((callback) => callback(data));
    // Emit connect/disconnect events
    if (event === 'open') {
      this.listeners.get('connect')?.forEach((callback) => callback(data));
    } else if (event === 'close') {
      this.listeners.get('disconnect')?.forEach((callback) => callback(data));
    }
  }
}

// Singleton WebSocket instance
export const nvrWebSocket = new NVRWebSocket();

// ============================================================================
// Models API
// ============================================================================

export interface ModelInfo {
  id: string;
  name: string;
  type: string;
  format: string;
  url: string;
  size: number;
  description: string;
}

export interface ModelDownloadStatus {
  model_id: string;
  status: 'not_started' | 'pending' | 'downloading' | 'completed' | 'failed';
  progress: number;
  error?: string;
  bytes_total: number;
  bytes_done: number;
}

export const modelsApi = {
  /**
   * List all available models
   */
  list: async (): Promise<ModelInfo[]> => {
    return request<ModelInfo[]>('/api/v1/models');
  },

  /**
   * Get download status for all models
   */
  getStatuses: async (): Promise<Record<string, ModelDownloadStatus>> => {
    return request<Record<string, ModelDownloadStatus>>('/api/v1/models/status');
  },

  /**
   * Start downloading a model
   */
  download: async (modelId: string): Promise<{ message: string }> => {
    return request<{ message: string }>('/api/v1/models/download', {
      method: 'POST',
      body: JSON.stringify({ model_id: modelId }),
    });
  },
};

// ============================================================================
// Stream URLs
// ============================================================================

// go2rtc ports are dynamically discovered from the backend
// Use getGo2RTCUrl() and getGo2RTCWsUrl() for dynamic port resolution

export const streamUrls = {
  /**
   * Get WebRTC stream URL
   */
  webrtc: (cameraId: string): string => {
    return `${getGo2RTCWsUrl()}/api/ws?src=${cameraId}`;
  },

  /**
   * Get HLS stream URL
   */
  hls: (cameraId: string): string => {
    return `${getGo2RTCUrl()}/api/stream.m3u8?src=${cameraId}`;
  },

  /**
   * Get MSE stream URL
   */
  mse: (cameraId: string): string => {
    return `${getGo2RTCWsUrl()}/api/ws?src=${cameraId}`;
  },

  /**
   * Get MJPEG stream URL
   */
  mjpeg: (cameraId: string): string => {
    return `${getGo2RTCUrl()}/api/frame.jpeg?src=${cameraId}`;
  },
};

// ============================================================================
// Audio API
// ============================================================================

export interface AudioCapabilities {
  has_microphone: boolean;
  has_speaker: boolean;
  two_way_audio: boolean;
  audio_codec: string;
  sample_rate: number;
  channels: number;
}

export interface AudioSession {
  id: string;
  camera_id: string;
  user_id: string;
  started_at: string;
  active: boolean;
}

export interface AudioSessionResponse {
  session: AudioSession;
  websocket_url: string;
  webrtc_url?: string;
  backchannel_url?: string;
}

export const audioApi = {
  /**
   * Get audio capabilities for a camera
   */
  getCapabilities: async (cameraId: string): Promise<AudioCapabilities> => {
    return request<AudioCapabilities>(`/api/v1/audio/capabilities/${cameraId}`);
  },

  /**
   * List active audio sessions
   */
  listSessions: async (): Promise<AudioSession[]> => {
    return request<AudioSession[]>('/api/v1/audio/sessions');
  },

  /**
   * Start an audio session with a camera
   */
  startSession: async (cameraId: string): Promise<AudioSessionResponse> => {
    return request<AudioSessionResponse>(`/api/v1/audio/sessions/${cameraId}/start`, {
      method: 'POST',
    });
  },

  /**
   * Stop an audio session
   */
  stopSession: async (sessionId: string): Promise<void> => {
    await request<void>(`/api/v1/audio/sessions/${sessionId}/stop`, {
      method: 'POST',
    });
  },
};

// ============================================================================
// Doorbell API
// ============================================================================

export const doorbellApi = {
  /**
   * Answer a doorbell (starts two-way audio)
   */
  answer: async (cameraId: string): Promise<AudioSessionResponse> => {
    return request<AudioSessionResponse>(`/api/v1/doorbell/${cameraId}/answer`, {
      method: 'POST',
    });
  },

  /**
   * Get doorbell snapshot
   */
  getSnapshot: async (cameraId: string): Promise<Blob> => {
    return request<Blob>(`/api/v1/doorbell/${cameraId}/snapshot`);
  },
};

// ============================================================================
// Motion Zones API
// ============================================================================

export interface Point {
  x: number;
  y: number;
}

export interface MotionZone {
  id: string;
  camera_id: string;
  name: string;
  enabled: boolean;
  points: Point[];
  object_types?: string[];
  min_confidence: number;
  min_size?: number;
  sensitivity: number;
  cooldown_seconds: number;
  notifications: boolean;
  recording: boolean;
  color?: string;
  created_at: string;
  updated_at: string;
}

export interface ZoneCreateRequest {
  camera_id: string;
  name: string;
  enabled: boolean;
  points: Point[];
  object_types?: string[];
  min_confidence?: number;
  min_size?: number;
  sensitivity?: number;
  cooldown_seconds?: number;
  notifications?: boolean;
  recording?: boolean;
  color?: string;
}

export interface ZoneUpdateRequest {
  name: string;
  enabled: boolean;
  points: Point[];
  object_types?: string[];
  min_confidence?: number;
  min_size?: number;
  sensitivity?: number;
  cooldown_seconds?: number;
  notifications?: boolean;
  recording?: boolean;
  color?: string;
}

export const zonesApi = {
  /**
   * List all zones, optionally filtered by camera
   */
  list: async (cameraId?: string): Promise<MotionZone[]> => {
    const query = cameraId ? `?camera_id=${cameraId}` : '';
    return request<MotionZone[]>(`/api/v1/zones${query}`);
  },

  /**
   * Get a single zone by ID
   */
  get: async (id: string): Promise<MotionZone> => {
    return request<MotionZone>(`/api/v1/zones/${id}`);
  },

  /**
   * Create a new zone
   */
  create: async (zone: ZoneCreateRequest): Promise<MotionZone> => {
    return request<MotionZone>('/api/v1/zones', {
      method: 'POST',
      body: JSON.stringify(zone),
    });
  },

  /**
   * Update an existing zone
   */
  update: async (id: string, zone: ZoneUpdateRequest): Promise<MotionZone> => {
    return request<MotionZone>(`/api/v1/zones/${id}`, {
      method: 'PUT',
      body: JSON.stringify(zone),
    });
  },

  /**
   * Delete a zone
   */
  delete: async (id: string): Promise<void> => {
    await request<void>(`/api/v1/zones/${id}`, {
      method: 'DELETE',
    });
  },
};

// ============================================================================
// Plugin Types
// ============================================================================

export interface CatalogPlugin {
  id: string;
  name: string;
  description: string;
  repo: string;
  version?: string;
  author?: string;
  category: string;
  featured?: boolean;
  icon?: string;
  capabilities?: string[];
  installed: boolean;
  installed_version?: string;
  latest_version?: string;
  update_available: boolean;
  enabled: boolean;
  state?: string;
  builtin?: boolean;
}

export interface CatalogCategory {
  name: string;
  description?: string;
  icon?: string;
}

export interface PluginCatalog {
  version: string;
  updated?: string;
  plugins: CatalogPlugin[];
  categories: Record<string, CatalogCategory>;
}

export interface Plugin {
  id: string;
  name: string;
  version: string;
  description?: string;
  enabled?: boolean;
  state: string;  // running, stopped, error
  status?: string;  // legacy field
  health: string;  // healthy, degraded, unhealthy
  builtin: boolean;
  camera_count?: number;
}

export interface PluginStatus {
  id: string;
  name: string;
  version: string;
  description: string;
  category: string;
  state: string;
  enabled: boolean;
  builtin: boolean;
  critical: boolean;
  capabilities?: string[];
  dependencies?: string[];
  startedAt?: string;
  lastError?: string;
  health: {
    state: string;
    message?: string;
    lastChecked?: string;
  };
  metrics?: {
    memory?: number;
    cpu?: number;
    uptime?: number;
  };
}

export interface PluginConfig {
  pluginId: string;
  config: Record<string, unknown>;
  schema?: Record<string, {
    type: string;
    description?: string;
    default?: unknown;
  }>;
}

export interface PluginLogEntry {
  timestamp: string;
  level: string;
  message: string;
  metadata?: Record<string, unknown>;
}

export interface PluginLogs {
  pluginId: string;
  logs: PluginLogEntry[];
  total: number;
}

export interface PluginInstallResult {
  message: string;
  plugin: {
    id: string;
    name: string;
    version: string;
  };
}

// ============================================================================
// Plugin Settings Types (Scrypted-style declarative UI)
// ============================================================================

export type SettingType =
  | 'string'
  | 'number'
  | 'integer'
  | 'boolean'
  | 'password'
  | 'textarea'
  | 'button'
  | 'device'
  | 'interface'
  | 'clippath'
  | 'time'
  | 'date'
  | 'datetime';

export interface SettingChoice {
  title: string;
  value: unknown;
}

/**
 * Setting represents a single configuration option exposed by a plugin.
 * Plugins return an array of these, and the NVR renders them generically.
 * This enables a Scrypted-style architecture where plugins describe their UI
 * rather than providing custom UI code.
 */
export interface Setting {
  /** Unique identifier for this setting */
  key: string;
  /** Display name shown to users */
  title: string;
  /** Additional context/help text */
  description?: string;
  /** Input type determines what component is rendered */
  type: SettingType;
  /** Group settings into sections */
  group?: string;
  /** Secondary grouping within a group */
  subgroup?: string;
  /** Current value */
  value?: unknown;
  /** Placeholder text for inputs */
  placeholder?: string;
  /** Options for dropdown/select */
  choices?: SettingChoice[];
  /** Allow selecting multiple choices */
  multiple?: boolean;
  /** Prevent user modification */
  readonly?: boolean;
  /** [min, max] range for number inputs */
  range?: [number, number];
  /** Filter device picker by capability */
  deviceFilter?: string;
  /** Apply value immediately without save button */
  immediate?: boolean;
  /** Allow typing custom values in addition to choices */
  combobox?: boolean;
}

// ============================================================================
// Plugins API
// ============================================================================

export const pluginsApi = {
  /**
   * List all plugins (both builtin and installed)
   */
  list: async (): Promise<Plugin[]> => {
    return request<Plugin[]>('/api/v1/plugins');
  },

  /**
   * Get the plugin catalog with installation status
   */
  getCatalog: async (): Promise<PluginCatalog> => {
    return request<PluginCatalog>('/api/v1/plugins/catalog');
  },

  /**
   * Reload the plugin catalog from disk
   */
  reloadCatalog: async (): Promise<{ message: string; plugin_count: number }> => {
    return request<{ message: string; plugin_count: number }>('/api/v1/plugins/catalog/reload', {
      method: 'POST',
    });
  },

  /**
   * Install a plugin from the catalog by ID (hot-reload)
   */
  installFromCatalog: async (pluginId: string): Promise<PluginInstallResult> => {
    return request<PluginInstallResult>(`/api/v1/plugins/catalog/${pluginId}/install`, {
      method: 'POST',
    });
  },

  /**
   * Install a plugin from a GitHub repository URL (hot-reload)
   */
  install: async (repoUrl: string): Promise<PluginInstallResult> => {
    return request<PluginInstallResult>('/api/v1/plugins/install', {
      method: 'POST',
      body: JSON.stringify({ repo_url: repoUrl }),
    });
  },

  /**
   * Enable a plugin
   */
  enable: async (pluginId: string): Promise<{ id: string; enabled: boolean; status: string }> => {
    return request<{ id: string; enabled: boolean; status: string }>(`/api/v1/plugins/${pluginId}/enable`, {
      method: 'POST',
    });
  },

  /**
   * Disable a plugin
   */
  disable: async (pluginId: string): Promise<{ id: string; enabled: boolean; status: string }> => {
    return request<{ id: string; enabled: boolean; status: string }>(`/api/v1/plugins/${pluginId}/disable`, {
      method: 'POST',
    });
  },

  /**
   * Update a plugin to the latest version
   */
  update: async (pluginId: string): Promise<PluginInstallResult> => {
    return request<PluginInstallResult>(`/api/v1/plugins/${pluginId}/update`, {
      method: 'POST',
    });
  },

  /**
   * Uninstall a plugin
   */
  uninstall: async (pluginId: string): Promise<{ message: string; id: string }> => {
    return request<{ message: string; id: string }>(`/api/v1/plugins/${pluginId}`, {
      method: 'DELETE',
    });
  },

  /**
   * Get tracked repositories for update checking
   */
  getTrackedRepos: async (): Promise<Array<{
    plugin_id: string;
    repo_url: string;
    installed_tag: string;
    latest_tag: string;
    update_available: boolean;
    last_checked: string;
  }>> => {
    return request('/api/v1/plugins/repos');
  },

  /**
   * Get detailed plugin status
   */
  getStatus: async (pluginId: string): Promise<PluginStatus> => {
    return request<PluginStatus>(`/api/v1/plugins/${pluginId}`);
  },

  /**
   * Restart a plugin
   */
  restart: async (pluginId: string): Promise<{ id: string; status: string }> => {
    return request<{ id: string; status: string }>(`/api/v1/plugins/${pluginId}/restart`, {
      method: 'POST',
    });
  },

  /**
   * Rescan the plugins directory to discover newly installed plugins
   */
  rescan: async (): Promise<{ success: boolean; plugins: Array<{ id: string; name: string; version: string; state: string; isBuiltin: boolean }>; message: string }> => {
    return request('/api/v1/plugins/rescan', {
      method: 'POST',
    });
  },

  /**
   * Get plugin configuration
   */
  getConfig: async (pluginId: string): Promise<PluginConfig> => {
    return request<PluginConfig>(`/api/v1/plugins/${pluginId}/config`);
  },

  /**
   * Update plugin configuration
   */
  setConfig: async (pluginId: string, config: Record<string, unknown>): Promise<{ message: string }> => {
    return request<{ message: string }>(`/api/v1/plugins/${pluginId}/config`, {
      method: 'PUT',
      body: JSON.stringify(config),
    });
  },

  /**
   * Get plugin logs
   */
  getLogs: async (pluginId: string, options?: { lines?: number; level?: string }): Promise<PluginLogs> => {
    const params = new URLSearchParams();
    if (options?.lines) params.set('lines', options.lines.toString());
    if (options?.level) params.set('level', options.level);
    const query = params.toString();
    return request<PluginLogs>(`/api/v1/plugins/${pluginId}/logs${query ? `?${query}` : ''}`);
  },

  /**
   * Send RPC command to a plugin
   */
  rpc: async <T = unknown>(pluginId: string, method: string, params: Record<string, unknown> = {}): Promise<T> => {
    return request<T>(`/api/v1/plugins/${pluginId}/rpc`, {
      method: 'POST',
      body: JSON.stringify({ method, params }),
    });
  },

  /**
   * Get plugin settings (Scrypted-style declarative UI)
   * Returns an array of Setting objects that describe the plugin's configuration UI
   */
  getSettings: async (pluginId: string): Promise<Setting[]> => {
    const result = await pluginsApi.rpc<Setting[] | { settings: Setting[] }>(pluginId, 'get_settings', {});
    // Handle both array and object response formats
    if (Array.isArray(result)) {
      return result;
    }
    if (result && typeof result === 'object' && 'settings' in result) {
      return result.settings;
    }
    return [];
  },

  /**
   * Update a single plugin setting
   * For button types, this triggers the associated action
   */
  putSetting: async (pluginId: string, key: string, value: unknown): Promise<void> => {
    await pluginsApi.rpc(pluginId, 'put_setting', { key, value });
  },
};

// ============================================================================
// Spatial Tracking Types
// ============================================================================

export interface SpatialPoint {
  x: number;
  y: number;
}

export type SpatialPolygon = SpatialPoint[];

export interface MapMeta {
  building?: string;
  floor?: string;
  area?: string;
}

export interface SpatialMap {
  id: string;
  name: string;
  width: number;
  height: number;
  scale: number;
  background_image?: string;
  metadata: MapMeta;
  created_at: string;
  updated_at: string;
}

export interface CameraPlacement {
  id: string;
  map_id: string;
  camera_id: string;
  position: SpatialPoint;
  rotation: number;
  fov_angle: number;
  fov_depth: number;
  coverage_polygon?: SpatialPolygon;
  height: number;
  tilt: number;
  created_at: string;
  updated_at: string;
}

export type TransitionType = 'overlap' | 'adjacent' | 'gap';
export type EdgeDirection = 'top' | 'bottom' | 'left' | 'right';

export interface ZoneDefinition {
  edge: EdgeDirection;
  start: number;
  end: number;
}

export interface CameraTransition {
  id: string;
  from_camera_id: string;
  to_camera_id: string;
  type: TransitionType;
  bidirectional: boolean;
  expected_transit_time: number;
  transit_time_variance: number;
  exit_zone?: ZoneDefinition;
  entry_zone?: ZoneDefinition;
  created_at: string;
  updated_at: string;
}

export type TrackState = 'active' | 'transit' | 'pending' | 'lost' | 'completed';

export interface TrackSegment {
  id: string;
  track_id: string;
  camera_id: string;
  local_track_id: string;
  start_time: string;
  end_time?: string;
  entry_point?: SpatialPoint;
  exit_point?: SpatialPoint;
  exit_edge?: EdgeDirection;
  re_id_signature?: number[];
  trajectory?: SpatialPoint[];
  confidence: number;
}

export interface GlobalTrack {
  id: string;
  map_id: string;
  object_type: string;
  state: TrackState;
  current_camera_id?: string;
  expected_camera_id?: string;
  first_seen: string;
  last_seen: string;
  segments: TrackSegment[];
  total_cameras_visited: number;
  total_distance: number;
}

export interface SpatialAnalytics {
  total_tracks: number;
  active_tracks: number;
  successful_handoffs: number;
  failed_handoffs: number;
  average_transit_time: number;
  busiest_transition?: string;
  coverage_gaps: string[];
}

export interface CameraPlacementCreate {
  camera_id: string;
  position: SpatialPoint;
  rotation: number;
  fov_angle: number;
  fov_depth: number;
  coverage_polygon?: SpatialPolygon;
  height?: number;
  tilt?: number;
}

export interface CameraTransitionCreate {
  from_camera_id: string;
  to_camera_id: string;
  type: TransitionType;
  bidirectional?: boolean;
  expected_transit_time?: number;
  transit_time_variance?: number;
  exit_zone?: ZoneDefinition;
  entry_zone?: ZoneDefinition;
}

// ============================================================================
// Spatial Tracking API
// ============================================================================

// Spatial tracking is now integrated into the main API at /api/v1/spatial/*
// Instead of running on a separate port, it's a plugin in the main NVR process
const SPATIAL_API_BASE = API_BASE;

async function spatialRequest<T>(
  endpoint: string,
  options: RequestInit = {}
): Promise<T> {
  // Prepend /spatial to the endpoint path, replacing /api/v1 -> /api/v1/spatial
  const spatialEndpoint = endpoint.replace('/api/v1/', '/api/v1/spatial/');
  const url = `${SPATIAL_API_BASE}${spatialEndpoint}`;

  const headers: HeadersInit = {
    'Content-Type': 'application/json',
    ...options.headers,
  };

  let response: Response;
  try {
    response = await fetch(url, {
      ...options,
      headers,
    });
  } catch {
    throw new ApiError(
      'Cannot connect to Spatial Tracking backend. Please ensure the NVR is running.',
      'NETWORK_ERROR',
      0
    );
  }

  if (!response.ok) {
    const text = await response.text();
    throw new ApiError(
      text || 'Request failed',
      'REQUEST_FAILED',
      response.status
    );
  }

  const text = await response.text();
  if (!text) {
    return {} as T;
  }

  return JSON.parse(text) as T;
}

export const spatialApi = {
  // Maps
  listMaps: async (): Promise<SpatialMap[]> => {
    const result = await spatialRequest<SpatialMap[] | null>('/api/v1/maps');
    return result || [];
  },

  getMap: async (mapId: string): Promise<SpatialMap> => {
    return spatialRequest<SpatialMap>(`/api/v1/maps/${mapId}`);
  },

  createMap: async (map: Omit<SpatialMap, 'id' | 'created_at' | 'updated_at'>): Promise<SpatialMap> => {
    return spatialRequest<SpatialMap>('/api/v1/maps', {
      method: 'POST',
      body: JSON.stringify(map),
    });
  },

  updateMap: async (mapId: string, map: Partial<SpatialMap>): Promise<SpatialMap> => {
    return spatialRequest<SpatialMap>(`/api/v1/maps/${mapId}`, {
      method: 'PUT',
      body: JSON.stringify(map),
    });
  },

  deleteMap: async (mapId: string): Promise<void> => {
    await spatialRequest<void>(`/api/v1/maps/${mapId}`, {
      method: 'DELETE',
    });
  },

  // Camera Placements
  listPlacements: async (mapId: string): Promise<CameraPlacement[]> => {
    const result = await spatialRequest<CameraPlacement[] | null>(`/api/v1/maps/${mapId}/cameras`);
    return result || [];
  },

  createPlacement: async (mapId: string, placement: CameraPlacementCreate): Promise<CameraPlacement> => {
    return spatialRequest<CameraPlacement>(`/api/v1/maps/${mapId}/cameras`, {
      method: 'POST',
      body: JSON.stringify(placement),
    });
  },

  updatePlacement: async (mapId: string, placementId: string, placement: Partial<CameraPlacementCreate>): Promise<CameraPlacement> => {
    return spatialRequest<CameraPlacement>(`/api/v1/maps/${mapId}/cameras/${placementId}`, {
      method: 'PUT',
      body: JSON.stringify(placement),
    });
  },

  deletePlacement: async (mapId: string, placementId: string): Promise<void> => {
    await spatialRequest<void>(`/api/v1/maps/${mapId}/cameras/${placementId}`, {
      method: 'DELETE',
    });
  },

  // Transitions
  listTransitions: async (mapId?: string): Promise<CameraTransition[]> => {
    const query = mapId ? `?map_id=${mapId}` : '';
    const result = await spatialRequest<CameraTransition[] | null>(`/api/v1/transitions${query}`);
    return result || [];
  },

  createTransition: async (transition: CameraTransitionCreate): Promise<CameraTransition> => {
    return spatialRequest<CameraTransition>('/api/v1/transitions', {
      method: 'POST',
      body: JSON.stringify(transition),
    });
  },

  deleteTransition: async (transitionId: string): Promise<void> => {
    await spatialRequest<void>(`/api/v1/transitions/${transitionId}`, {
      method: 'DELETE',
    });
  },

  autoDetectTransitions: async (mapId: string): Promise<CameraTransition[]> => {
    const result = await spatialRequest<CameraTransition[] | null>(`/api/v1/maps/${mapId}/auto-detect-transitions`, {
      method: 'POST',
    });
    return result || [];
  },

  // Tracks
  listTracks: async (params?: { map_id?: string; state?: TrackState }): Promise<GlobalTrack[]> => {
    const searchParams = new URLSearchParams();
    if (params?.map_id) searchParams.set('map_id', params.map_id);
    if (params?.state) searchParams.set('state', params.state);
    const query = searchParams.toString();
    const result = await spatialRequest<GlobalTrack[] | null>(`/api/v1/tracks${query ? `?${query}` : ''}`);
    return result || [];
  },

  getTrack: async (trackId: string): Promise<GlobalTrack> => {
    return spatialRequest<GlobalTrack>(`/api/v1/tracks/${trackId}`);
  },

  // Analytics
  getAnalytics: async (mapId: string): Promise<SpatialAnalytics> => {
    return spatialRequest<SpatialAnalytics>(`/api/v1/maps/${mapId}/analytics`);
  },

  // Calibration
  calibrate: async (mapId: string): Promise<{ message: string; transitions_created: number }> => {
    return spatialRequest<{ message: string; transitions_created: number }>(`/api/v1/maps/${mapId}/calibrate`, {
      method: 'POST',
    });
  },
};

// ============================================================================
// Updates API
// ============================================================================

export interface UpdateComponent {
  name: string;
  current_version: string;
  latest_version?: string;
  update_available: boolean;
  repository: string;
  last_checked: string;
  auto_update: boolean;
}

export interface UpdateStatus {
  component: string;
  status: 'checking' | 'downloading' | 'extracting' | 'installing' | 'complete' | 'error' | 'available' | 'current';
  progress: number;
  message: string;
  started_at: string;
  completed_at?: string;
  error?: string;
}

export interface UpdatesResponse {
  components: UpdateComponent[];
  pending_updates: number;
  needs_restart: boolean;
}

export const updatesApi = {
  /**
   * Get all components and their update status
   */
  getUpdates: async (): Promise<UpdatesResponse> => {
    return request<UpdatesResponse>('/api/v1/updates');
  },

  /**
   * Check for updates on all components
   */
  checkUpdates: async (): Promise<UpdatesResponse> => {
    return request<UpdatesResponse>('/api/v1/updates/check', {
      method: 'POST',
    });
  },

  /**
   * Update a specific component
   */
  updateComponent: async (component: string): Promise<UpdateStatus> => {
    return request<UpdateStatus>(`/api/v1/updates/${component}`, {
      method: 'POST',
    });
  },

  /**
   * Update all components with available updates
   */
  updateAll: async (): Promise<Record<string, UpdateStatus>> => {
    return request<Record<string, UpdateStatus>>('/api/v1/updates/all', {
      method: 'POST',
    });
  },

  /**
   * Get current update status
   */
  getStatus: async (): Promise<Record<string, UpdateStatus>> => {
    return request<Record<string, UpdateStatus>>('/api/v1/updates/status');
  },
};
