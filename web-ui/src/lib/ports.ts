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
const DEFAULT_PORTS: PortConfig = {
  api: 12000,
  nats: 12001,
  web_ui: 12002,
  go2rtc_api: 12010,
  go2rtc_rtsp: 12011,
  go2rtc_webrtc: 12012,
  spatial: 12020,
  detection: 12021,
};

// Cached port configuration
let cachedPorts: PortConfig | null = null;
let fetchPromise: Promise<PortConfig> | null = null;

// Get the API base URL - this is the one port we must know initially
// In production, this is served from the same origin
// In development, it defaults to 12000
function getApiBase(): string {
  if (import.meta.env.VITE_API_URL) {
    return import.meta.env.VITE_API_URL;
  }
  // In production, use same origin; in development, use port 12000
  if (window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1') {
    return 'http://localhost:12000';
  }
  return window.location.origin;
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
 * Get the go2rtc base URL
 */
export function getGo2RTCUrl(): string {
  const ports = getPorts();
  return `http://localhost:${ports.go2rtc_api}`;
}

/**
 * Get the go2rtc WebSocket URL
 */
export function getGo2RTCWsUrl(): string {
  const ports = getPorts();
  return `ws://localhost:${ports.go2rtc_api}`;
}

/**
 * Get the API base URL
 */
export function getApiUrl(): string {
  const ports = getPorts();
  return `http://localhost:${ports.api}`;
}
