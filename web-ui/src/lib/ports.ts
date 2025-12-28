// Service Port Discovery
// Dynamically discovers service ports from the backend at runtime
// Ports can change on each boot/load, so we fetch them once and cache in memory

export interface PortConfig {
  api: number;
  nats: number;
  web_ui: number;
  go2rtc_api: number;
  go2rtc_rtsp: number;
  go2rtc_webrtc: number;
  spatial: number;
  detection: number;
}

// Default ports (fallback before discovery completes)
// All services now use standard ports
const DEFAULT_PORTS: PortConfig = {
  api: 8080,            // Standard web port for API and Web UI
  nats: 4222,           // Standard NATS port
  web_ui: 8080,         // Same as API (served by Go backend)
  go2rtc_api: 1984,     // Standard go2rtc API port
  go2rtc_rtsp: 8554,    // Standard RTSP port
  go2rtc_webrtc: 8555,  // Standard WebRTC port
  spatial: 12020,
  detection: 12021,
};

// Cached port configuration
let cachedPorts: PortConfig | null = null;
let fetchPromise: Promise<PortConfig> | null = null;

// Get the API base URL - this is the one port we must know initially
// In production or behind reverse proxy, use same origin
// In development (localhost with non-standard port), defaults to 8080
function getApiBase(): string {
  if (import.meta.env.VITE_API_URL) {
    return import.meta.env.VITE_API_URL;
  }
  // Behind reverse proxy (standard ports or external hostname), use origin
  const port = window.location.port;
  if (!port || port === '80' || port === '443') {
    return window.location.origin;
  }
  // External hostname likely means reverse proxy
  const host = window.location.hostname;
  if (host !== 'localhost' && host !== '127.0.0.1') {
    return window.location.origin;
  }
  // Local development - use explicit port
  return 'http://localhost:8080';
}

/**
 * Fetch port configuration from the backend
 * This is called once at startup and cached
 */
export async function fetchPorts(): Promise<PortConfig> {
  // Return cached if available
  if (cachedPorts) {
    return cachedPorts;
  }

  // Return existing fetch promise if in progress
  if (fetchPromise) {
    return fetchPromise;
  }

  // Start fetch
  fetchPromise = (async () => {
    try {
      const apiBase = getApiBase();
      const response = await fetch(`${apiBase}/api/v1/system/ports`);

      if (!response.ok) {
        console.warn('[Ports] Failed to fetch ports, using defaults');
        cachedPorts = DEFAULT_PORTS;
        return DEFAULT_PORTS;
      }

      const data = await response.json();

      if (data.success && data.data) {
        cachedPorts = data.data as PortConfig;
        console.log('[Ports] Discovered service ports:', cachedPorts);
        return cachedPorts;
      }

      console.warn('[Ports] Invalid response, using defaults');
      cachedPorts = DEFAULT_PORTS;
      return DEFAULT_PORTS;
    } catch (error) {
      console.warn('[Ports] Error fetching ports, using defaults:', error);
      cachedPorts = DEFAULT_PORTS;
      return DEFAULT_PORTS;
    } finally {
      fetchPromise = null;
    }
  })();

  return fetchPromise;
}

/**
 * Get cached ports synchronously (returns defaults if not yet fetched)
 * Use this in components that need immediate access
 */
export function getPorts(): PortConfig {
  return cachedPorts || DEFAULT_PORTS;
}

/**
 * Check if ports have been fetched
 */
export function isPortsLoaded(): boolean {
  return cachedPorts !== null;
}

/**
 * Clear cached ports (for testing or reconnection)
 */
export function clearPortsCache(): void {
  cachedPorts = null;
  fetchPromise = null;
}

/**
 * Check if we're running behind a reverse proxy (standard ports or different from default)
 * Returns true if we should use the browser's origin instead of explicit port
 */
function isReverseProxy(): boolean {
  const port = window.location.port;
  // Standard ports (80/443) or no port means likely behind reverse proxy
  if (!port || port === '80' || port === '443') {
    return true;
  }
  // Not localhost/127.0.0.1 usually means reverse proxy
  const host = window.location.hostname;
  if (host !== 'localhost' && host !== '127.0.0.1') {
    return true;
  }
  return false;
}

/**
 * Get the base URL for API connections
 * Respects reverse proxy configuration - uses origin when behind proxy
 */
function getBaseUrl(): string {
  // Behind reverse proxy, use the browser's origin (includes correct protocol/host/port)
  if (isReverseProxy()) {
    return window.location.origin;
  }
  // Local development - use explicit port
  const ports = getPorts();
  return `http://${window.location.hostname}:${ports.api}`;
}

/**
 * Get the go2rtc base URL (proxied through backend API)
 * This allows the UI to only need the API port exposed
 */
export function getGo2RTCUrl(): string {
  // Route through the backend proxy at /go2rtc/
  return `${getBaseUrl()}/go2rtc`;
}

/**
 * Get the go2rtc WebSocket URL (proxied through backend API)
 */
export function getGo2RTCWsUrl(): string {
  const base = getBaseUrl();
  // Convert http/https to ws/wss
  const wsUrl = base.replace(/^http/, 'ws');
  return `${wsUrl}/go2rtc`;
}

/**
 * Get the API base URL
 */
export function getApiUrl(): string {
  return getBaseUrl();
}
