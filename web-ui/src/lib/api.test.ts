import { describe, it, expect, beforeEach } from 'vitest'
import { server } from '../test/setup'
import { http, HttpResponse } from 'msw'
import {
  cameraApi,
  eventApi,
  statsApi,
  configApi,
  healthApi,
  zonesApi,
  pluginsApi,
  ApiError,
} from './api'

describe('cameraApi', () => {
  describe('list', () => {
    it('should return list of cameras', async () => {
      const cameras = await cameraApi.list()
      expect(cameras).toHaveLength(2)
      expect(cameras[0].id).toBe('cam_1')
      expect(cameras[0].name).toBe('Front Door')
      expect(cameras[1].status).toBe('offline')
    })

    it('should throw ApiError on failure', async () => {
      server.use(
        http.get('http://localhost:5000/api/v1/cameras', () => {
          return HttpResponse.json(
            { success: false, error: { code: 'SERVER_ERROR', message: 'Internal error' } },
            { status: 500 }
          )
        })
      )

      await expect(cameraApi.list()).rejects.toThrow(ApiError)
    })
  })

  describe('get', () => {
    beforeEach(() => {
      server.use(
        http.get('http://localhost:5000/api/v1/cameras/:id', ({ params }) => {
          if (params.id === 'cam_1') {
            return HttpResponse.json({
              success: true,
              data: {
                id: 'cam_1',
                name: 'Front Door',
                status: 'online',
                enabled: true,
                created_at: '2024-01-01T00:00:00Z',
                updated_at: '2024-01-01T00:00:00Z',
              },
            })
          }
          return HttpResponse.json(
            { success: false, error: { code: 'NOT_FOUND', message: 'Camera not found' } },
            { status: 404 }
          )
        })
      )
    })

    it('should return camera by id', async () => {
      const camera = await cameraApi.get('cam_1')
      expect(camera.id).toBe('cam_1')
      expect(camera.name).toBe('Front Door')
    })

    it('should throw on not found', async () => {
      await expect(cameraApi.get('nonexistent')).rejects.toThrow(ApiError)
    })
  })

  describe('create', () => {
    beforeEach(() => {
      server.use(
        http.post('http://localhost:5000/api/v1/cameras', async ({ request }) => {
          const body = await request.json() as { name: string }
          return HttpResponse.json({
            success: true,
            data: {
              id: 'cam_new',
              name: body.name,
              status: 'starting',
              enabled: true,
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T00:00:00Z',
            },
          })
        })
      )
    })

    it('should create a new camera', async () => {
      const camera = await cameraApi.create({
        name: 'New Camera',
        stream: { url: 'rtsp://example.com/stream' },
      })
      expect(camera.id).toBe('cam_new')
      expect(camera.name).toBe('New Camera')
    })
  })

  describe('update', () => {
    beforeEach(() => {
      server.use(
        http.put('http://localhost:5000/api/v1/cameras/:id', async ({ params, request }) => {
          const body = await request.json() as { name?: string }
          return HttpResponse.json({
            success: true,
            data: {
              id: params.id,
              name: body.name || 'Updated',
              status: 'online',
              enabled: true,
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-02T00:00:00Z',
            },
          })
        })
      )
    })

    it('should update camera', async () => {
      const camera = await cameraApi.update('cam_1', { name: 'Updated Name' })
      expect(camera.name).toBe('Updated Name')
    })
  })

  describe('delete', () => {
    beforeEach(() => {
      server.use(
        http.delete('http://localhost:5000/api/v1/cameras/:id', () => {
          return HttpResponse.json({ success: true })
        })
      )
    })

    it('should delete camera', async () => {
      await expect(cameraApi.delete('cam_1')).resolves.toBeUndefined()
    })
  })

  describe('getSnapshotUrl', () => {
    it('should return snapshot URL', () => {
      const url = cameraApi.getSnapshotUrl('cam_1')
      expect(url).toBe('http://localhost:5000/api/v1/cameras/cam_1/snapshot')
    })
  })
})

describe('eventApi', () => {
  describe('list', () => {
    it('should return events with pagination', async () => {
      const result = await eventApi.list({ camera_id: 'cam_1' })
      expect(result.data).toHaveLength(1)
      expect(result.data[0].event_type).toBe('person')
    })
  })

  describe('get', () => {
    beforeEach(() => {
      server.use(
        http.get('http://localhost:5000/api/v1/events/:id', ({ params }) => {
          return HttpResponse.json({
            success: true,
            data: {
              id: params.id,
              camera_id: 'cam_1',
              event_type: 'person',
              timestamp: 1704067200,
              confidence: 0.95,
              acknowledged: false,
              created_at: '2024-01-01T00:00:00Z',
            },
          })
        })
      )
    })

    it('should return event by id', async () => {
      const event = await eventApi.get('evt_1')
      expect(event.id).toBe('evt_1')
      expect(event.event_type).toBe('person')
    })
  })

  describe('acknowledge', () => {
    beforeEach(() => {
      server.use(
        http.put('http://localhost:5000/api/v1/events/:id/acknowledge', () => {
          return HttpResponse.json({ success: true })
        })
      )
    })

    it('should acknowledge event', async () => {
      await expect(eventApi.acknowledge('evt_1')).resolves.toBeUndefined()
    })
  })
})

describe('statsApi', () => {
  describe('get', () => {
    it('should return system stats', async () => {
      const stats = await statsApi.get()
      expect(stats.cameras.total).toBe(2)
      expect(stats.cameras.online).toBe(1)
      expect(stats.events.today).toBe(5)
    })
  })
})

describe('configApi', () => {
  describe('get', () => {
    it('should return system config', async () => {
      const config = await configApi.get()
      expect(config.version).toBe('1.0.0')
      expect(config.system.name).toBe('NVR Test')
    })
  })

  describe('update', () => {
    beforeEach(() => {
      server.use(
        http.put('http://localhost:5000/api/v1/config', () => {
          return HttpResponse.json({
            success: true,
            data: { message: 'Config updated' },
          })
        })
      )
    })

    it('should update config', async () => {
      const result = await configApi.update({ system: { name: 'New Name' } })
      expect(result.message).toBe('Config updated')
    })
  })
})

describe('healthApi', () => {
  describe('check', () => {
    it('should return health status', async () => {
      const health = await healthApi.check()
      expect(health.status).toBe('healthy')
      expect(health.go2rtc).toBe(true)
    })
  })
})

describe('zonesApi', () => {
  describe('list', () => {
    it('should return zones', async () => {
      const zones = await zonesApi.list()
      expect(zones).toEqual([])
    })

    it('should filter by camera id', async () => {
      server.use(
        http.get('http://localhost:5000/api/v1/zones', ({ request }) => {
          const url = new URL(request.url)
          const cameraId = url.searchParams.get('camera_id')
          return HttpResponse.json({
            success: true,
            data: cameraId ? [{ id: 'zone_1', camera_id: cameraId }] : [],
          })
        })
      )

      const zones = await zonesApi.list('cam_1')
      expect(zones).toHaveLength(1)
      expect(zones[0].camera_id).toBe('cam_1')
    })
  })

  describe('create', () => {
    beforeEach(() => {
      server.use(
        http.post('http://localhost:5000/api/v1/zones', async ({ request }) => {
          const body = await request.json() as { name: string; camera_id: string }
          return HttpResponse.json({
            success: true,
            data: {
              id: 'zone_new',
              camera_id: body.camera_id,
              name: body.name,
              enabled: true,
              points: [],
              min_confidence: 0.5,
              sensitivity: 5,
              cooldown_seconds: 30,
              notifications: true,
              recording: true,
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T00:00:00Z',
            },
          })
        })
      )
    })

    it('should create zone', async () => {
      const zone = await zonesApi.create({
        camera_id: 'cam_1',
        name: 'Front Porch',
        enabled: true,
        points: [{ x: 0.1, y: 0.1 }, { x: 0.9, y: 0.1 }, { x: 0.5, y: 0.9 }],
      })
      expect(zone.id).toBe('zone_new')
      expect(zone.name).toBe('Front Porch')
    })
  })

  describe('delete', () => {
    beforeEach(() => {
      server.use(
        http.delete('http://localhost:5000/api/v1/zones/:id', () => {
          return HttpResponse.json({ success: true })
        })
      )
    })

    it('should delete zone', async () => {
      await expect(zonesApi.delete('zone_1')).resolves.toBeUndefined()
    })
  })
})

describe('pluginsApi', () => {
  describe('list', () => {
    it('should return plugins', async () => {
      const plugins = await pluginsApi.list()
      expect(plugins).toEqual([])
    })
  })

  describe('getCatalog', () => {
    it('should return catalog', async () => {
      const catalog = await pluginsApi.getCatalog()
      expect(catalog.version).toBe('1.0.0')
      expect(catalog.plugins).toEqual([])
    })
  })

  describe('install', () => {
    beforeEach(() => {
      server.use(
        http.post('http://localhost:5000/api/v1/plugins/install', () => {
          return HttpResponse.json({
            success: true,
            data: {
              message: 'Plugin installed',
              plugin: { id: 'test-plugin', name: 'Test Plugin', version: '1.0.0' },
            },
          })
        })
      )
    })

    it('should install plugin from repo', async () => {
      const result = await pluginsApi.install('https://github.com/test/plugin')
      expect(result.plugin.id).toBe('test-plugin')
    })
  })

  describe('enable/disable', () => {
    beforeEach(() => {
      server.use(
        http.post('http://localhost:5000/api/v1/plugins/:id/enable', ({ params }) => {
          return HttpResponse.json({
            success: true,
            data: { id: params.id, enabled: true, status: 'running' },
          })
        }),
        http.post('http://localhost:5000/api/v1/plugins/:id/disable', ({ params }) => {
          return HttpResponse.json({
            success: true,
            data: { id: params.id, enabled: false, status: 'stopped' },
          })
        })
      )
    })

    it('should enable plugin', async () => {
      const result = await pluginsApi.enable('test-plugin')
      expect(result.enabled).toBe(true)
    })

    it('should disable plugin', async () => {
      const result = await pluginsApi.disable('test-plugin')
      expect(result.enabled).toBe(false)
    })
  })

  describe('uninstall', () => {
    beforeEach(() => {
      server.use(
        http.delete('http://localhost:5000/api/v1/plugins/:id', ({ params }) => {
          return HttpResponse.json({
            success: true,
            data: { message: 'Plugin uninstalled', id: params.id },
          })
        })
      )
    })

    it('should uninstall plugin', async () => {
      const result = await pluginsApi.uninstall('test-plugin')
      expect(result.id).toBe('test-plugin')
    })
  })
})

describe('ApiError', () => {
  it('should have correct properties', () => {
    const error = new ApiError('Test error', 'TEST_CODE', 400, [{ field: 'name', message: 'required' }])
    expect(error.message).toBe('Test error')
    expect(error.code).toBe('TEST_CODE')
    expect(error.status).toBe(400)
    expect(error.details).toHaveLength(1)
    expect(error.name).toBe('ApiError')
  })
})
