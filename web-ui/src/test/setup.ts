import '@testing-library/jest-dom'
import { afterEach, beforeAll, afterAll } from 'vitest'
import { cleanup } from '@testing-library/react'
import { setupServer } from 'msw/node'
import { http, HttpResponse } from 'msw'

// Mock EventSource for SSE
class MockEventSource {
  onopen: (() => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  onerror: ((event: Event) => void) | null = null
  readyState = 1
  url: string

  constructor(url: string) {
    this.url = url
  }

  close() {
    this.readyState = 2
  }
}

// @ts-ignore
global.EventSource = MockEventSource

// Mock API responses
export const handlers = [
  http.get('http://localhost:5000/api/v1/cameras', () => {
    return HttpResponse.json({
      success: true,
      data: [
        {
          id: 'cam_1',
          name: 'Front Door',
          status: 'online',
          enabled: true,
          created_at: '2024-01-01T00:00:00Z',
          updated_at: '2024-01-01T00:00:00Z',
        },
        {
          id: 'cam_2',
          name: 'Backyard',
          status: 'offline',
          enabled: true,
          created_at: '2024-01-01T00:00:00Z',
          updated_at: '2024-01-01T00:00:00Z',
        },
      ],
    })
  }),

  http.get('http://localhost:5000/api/v1/events', () => {
    return HttpResponse.json({
      success: true,
      data: {
        data: [
          {
            id: 'evt_1',
            camera_id: 'cam_1',
            event_type: 'person',
            label: 'Person detected',
            timestamp: 1704067200,
            confidence: 0.95,
            acknowledged: false,
            created_at: '2024-01-01T00:00:00Z',
          },
        ],
        total: 1,
      },
    })
  }),

  http.get('http://localhost:5000/api/v1/stats', () => {
    return HttpResponse.json({
      success: true,
      data: {
        cameras: { total: 2, online: 1, offline: 1 },
        events: { today: 5, unacknowledged: 2, total: 100 },
        storage: { database_size: 1024000 },
      },
    })
  }),

  http.get('http://localhost:5000/api/v1/config', () => {
    return HttpResponse.json({
      success: true,
      data: {
        version: '1.0.0',
        system: {
          name: 'NVR Test',
          timezone: 'UTC',
          storage_path: '/data',
          max_storage_gb: 100,
          deployment: { mode: 'docker' },
          logging: { level: 'info', format: 'json' },
        },
        cameras_count: 2,
        storage: {
          recordings: '/data/recordings',
          thumbnails: '/data/thumbnails',
          snapshots: '/data/snapshots',
          exports: '/data/exports',
          retention: { default_days: 30 },
        },
        preferences: {
          ui: { theme: 'dark', language: 'en', dashboard: { grid_columns: 3, show_fps: true } },
          timeline: { default_range_hours: 24, thumbnail_interval_seconds: 60 },
          events: { auto_acknowledge_after_days: 7, group_similar_events: true, group_window_seconds: 30 },
        },
      },
    })
  }),

  http.get('http://localhost:5000/health', () => {
    return HttpResponse.json({
      status: 'healthy',
      version: '1.0.0',
      go2rtc: true,
      database: 'ok',
    })
  }),

  http.get('http://localhost:5000/api/v1/zones', () => {
    return HttpResponse.json({
      success: true,
      data: [],
    })
  }),

  http.get('http://localhost:5000/api/v1/plugins', () => {
    return HttpResponse.json({
      success: true,
      data: [],
    })
  }),

  http.get('http://localhost:5000/api/v1/plugins/catalog', () => {
    return HttpResponse.json({
      success: true,
      data: {
        version: '1.0.0',
        plugins: [],
        categories: {},
      },
    })
  }),
]

export const server = setupServer(...handlers)

beforeAll(() => server.listen({ onUnhandledRequest: 'bypass' }))
afterEach(() => {
  cleanup()
  server.resetHandlers()
})
afterAll(() => server.close())
