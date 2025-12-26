import { describe, it, expect, vi } from 'vitest'
import { render, screen, waitFor } from '../test/utils'
import { Dashboard } from './Dashboard'
import { http, HttpResponse } from 'msw'
import { server } from '../test/setup'

// Mock the VideoPlayer component since it requires complex WebSocket setup
vi.mock('../components/VideoPlayer', () => ({
  VideoPlayer: ({ cameraId, className }: { cameraId: string; className?: string }) => (
    <div data-testid={`video-player-${cameraId}`} className={className}>
      Video Player Mock
    </div>
  ),
}))

describe('Dashboard', () => {
  it('should show loading spinner initially', () => {
    const { container } = render(<Dashboard />)

    // The loading spinner has animate-spin class
    const spinner = container.querySelector('.animate-spin')
    expect(spinner).toBeInTheDocument()
  })

  it('should show "No cameras yet" when no cameras exist', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/cameras', () => {
        return HttpResponse.json({ success: true, data: [] })
      }),
      http.get('http://localhost:5000/api/v1/events', () => {
        return HttpResponse.json({ success: true, data: { data: [], total: 0 } })
      })
    )

    render(<Dashboard />)

    await waitFor(() => {
      expect(screen.getByText('No cameras yet')).toBeInTheDocument()
    })
  })

  it('should show Add Camera link when no cameras exist', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/cameras', () => {
        return HttpResponse.json({ success: true, data: [] })
      }),
      http.get('http://localhost:5000/api/v1/events', () => {
        return HttpResponse.json({ success: true, data: { data: [], total: 0 } })
      })
    )

    render(<Dashboard />)

    await waitFor(() => {
      const addCameraLinks = screen.getAllByText('Add Camera')
      expect(addCameraLinks.length).toBeGreaterThan(0)
    })
  })

  it('should display cameras when they exist', async () => {
    server.use(
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
          ],
        })
      }),
      http.get('http://localhost:5000/api/v1/events', () => {
        return HttpResponse.json({ success: true, data: { data: [], total: 0 } })
      })
    )

    render(<Dashboard />)

    await waitFor(() => {
      expect(screen.getByText('Front Door')).toBeInTheDocument()
    })
  })

  it('should show camera count in header', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/cameras', () => {
        return HttpResponse.json({
          success: true,
          data: [
            { id: 'cam_1', name: 'Camera 1', status: 'online', enabled: true, created_at: '2024-01-01T00:00:00Z', updated_at: '2024-01-01T00:00:00Z' },
            { id: 'cam_2', name: 'Camera 2', status: 'offline', enabled: true, created_at: '2024-01-01T00:00:00Z', updated_at: '2024-01-01T00:00:00Z' },
          ],
        })
      }),
      http.get('http://localhost:5000/api/v1/events', () => {
        return HttpResponse.json({ success: true, data: { data: [], total: 0 } })
      })
    )

    render(<Dashboard />)

    await waitFor(() => {
      expect(screen.getByText('1 of 2 cameras online')).toBeInTheDocument()
    })
  })

  it('should display Live View header', async () => {
    render(<Dashboard />)

    await waitFor(() => {
      expect(screen.getByText('Live View')).toBeInTheDocument()
    })
  })

  it('should show "No recent events" when events list is empty', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/cameras', () => {
        return HttpResponse.json({ success: true, data: [] })
      }),
      http.get('http://localhost:5000/api/v1/events', () => {
        return HttpResponse.json({ success: true, data: { data: [], total: 0 } })
      })
    )

    render(<Dashboard />)

    await waitFor(() => {
      expect(screen.getByText('No recent events')).toBeInTheDocument()
    })
  })

  it('should display events when they exist', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/cameras', () => {
        return HttpResponse.json({ success: true, data: [] })
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
                timestamp: Date.now() / 1000,
                confidence: 0.95,
                acknowledged: false,
                created_at: '2024-01-01T00:00:00Z',
              },
            ],
            total: 1,
          },
        })
      })
    )

    render(<Dashboard />)

    await waitFor(() => {
      expect(screen.getByText('person')).toBeInTheDocument()
    })
  })

  it('should show Recent Activity section', async () => {
    render(<Dashboard />)

    await waitFor(() => {
      expect(screen.getByText('Recent Activity')).toBeInTheDocument()
    })
  })

  it('should show "View all" link for events', async () => {
    render(<Dashboard />)

    await waitFor(() => {
      expect(screen.getByText('View all')).toBeInTheDocument()
    })
  })

  it('should render camera with offline status indicator', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/cameras', () => {
        return HttpResponse.json({
          success: true,
          data: [
            {
              id: 'cam_1',
              name: 'Offline Camera',
              status: 'offline',
              enabled: true,
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T00:00:00Z',
            },
          ],
        })
      }),
      http.get('http://localhost:5000/api/v1/events', () => {
        return HttpResponse.json({ success: true, data: { data: [], total: 0 } })
      })
    )

    render(<Dashboard />)

    await waitFor(() => {
      expect(screen.getByText('Offline Camera')).toBeInTheDocument()
    })
  })

  it('should display fps when available', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/cameras', () => {
        return HttpResponse.json({
          success: true,
          data: [
            {
              id: 'cam_1',
              name: 'Camera with FPS',
              status: 'online',
              enabled: true,
              fps_current: 30,
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T00:00:00Z',
            },
          ],
        })
      }),
      http.get('http://localhost:5000/api/v1/events', () => {
        return HttpResponse.json({ success: true, data: { data: [], total: 0 } })
      })
    )

    render(<Dashboard />)

    await waitFor(() => {
      expect(screen.getByText('30fps')).toBeInTheDocument()
    })
  })
})
