import { describe, it, expect } from 'vitest'
import { render, screen, waitFor } from '../test/utils'
import { CameraList } from './CameraList'
import { http, HttpResponse } from 'msw'
import { server } from '../test/setup'

describe('CameraList', () => {
  it('should show loading spinner initially', () => {
    const { container } = render(<CameraList />)
    const spinner = container.querySelector('.animate-spin')
    expect(spinner).toBeInTheDocument()
  })

  it('should show "No cameras configured" when no cameras exist', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/cameras', () => {
        return HttpResponse.json({ success: true, data: [] })
      })
    )

    render(<CameraList />)

    await waitFor(() => {
      expect(screen.getByText('No cameras configured')).toBeInTheDocument()
    })
  })

  it('should show Add Camera button in empty state', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/cameras', () => {
        return HttpResponse.json({ success: true, data: [] })
      })
    )

    render(<CameraList />)

    await waitFor(() => {
      const addButtons = screen.getAllByText('Add Camera')
      expect(addButtons.length).toBeGreaterThan(0)
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
      })
    )

    render(<CameraList />)

    await waitFor(() => {
      expect(screen.getByText('Front Door')).toBeInTheDocument()
    })
  })

  it('should show error when fetch fails', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/cameras', () => {
        return HttpResponse.error()
      })
    )

    render(<CameraList />)

    await waitFor(() => {
      expect(screen.getByText(/Failed to load cameras/)).toBeInTheDocument()
    })
  })

  it('should show camera status indicator', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/cameras', () => {
        return HttpResponse.json({
          success: true,
          data: [
            {
              id: 'cam_1',
              name: 'Online Camera',
              status: 'online',
              enabled: true,
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T00:00:00Z',
            },
          ],
        })
      })
    )

    render(<CameraList />)

    await waitFor(() => {
      expect(screen.getByText('Online')).toBeInTheDocument()
    })
  })

  it('should show offline status for offline camera', async () => {
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
      })
    )

    render(<CameraList />)

    await waitFor(() => {
      expect(screen.getByText('Offline')).toBeInTheDocument()
    })
  })

  it('should show manufacturer and model when available', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/cameras', () => {
        return HttpResponse.json({
          success: true,
          data: [
            {
              id: 'cam_1',
              name: 'Brand Camera',
              status: 'online',
              enabled: true,
              manufacturer: 'Acme',
              model: 'HD1000',
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T00:00:00Z',
            },
          ],
        })
      })
    )

    render(<CameraList />)

    await waitFor(() => {
      expect(screen.getByText('Acme HD1000')).toBeInTheDocument()
    })
  })

  it('should show fps and bitrate when available', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/cameras', () => {
        return HttpResponse.json({
          success: true,
          data: [
            {
              id: 'cam_1',
              name: 'Stats Camera',
              status: 'online',
              enabled: true,
              fps_current: 30,
              bitrate_current: 4000000,
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T00:00:00Z',
            },
          ],
        })
      })
    )

    render(<CameraList />)

    await waitFor(() => {
      expect(screen.getByText(/30 fps/)).toBeInTheDocument()
    })
  })

  it('should display multiple cameras in grid', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/cameras', () => {
        return HttpResponse.json({
          success: true,
          data: [
            { id: 'cam_1', name: 'Camera 1', status: 'online', enabled: true, created_at: '2024-01-01T00:00:00Z', updated_at: '2024-01-01T00:00:00Z' },
            { id: 'cam_2', name: 'Camera 2', status: 'offline', enabled: true, created_at: '2024-01-01T00:00:00Z', updated_at: '2024-01-01T00:00:00Z' },
            { id: 'cam_3', name: 'Camera 3', status: 'error', enabled: true, created_at: '2024-01-01T00:00:00Z', updated_at: '2024-01-01T00:00:00Z' },
          ],
        })
      })
    )

    render(<CameraList />)

    await waitFor(() => {
      expect(screen.getByText('Camera 1')).toBeInTheDocument()
      expect(screen.getByText('Camera 2')).toBeInTheDocument()
      expect(screen.getByText('Camera 3')).toBeInTheDocument()
    })
  })

  it('should show Cameras heading', async () => {
    render(<CameraList />)

    expect(screen.getByText('Cameras')).toBeInTheDocument()
  })

  it('should show "Manage your camera sources" subtitle', async () => {
    render(<CameraList />)

    expect(screen.getByText('Manage your camera sources')).toBeInTheDocument()
  })
})
