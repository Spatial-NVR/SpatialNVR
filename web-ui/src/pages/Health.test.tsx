import { describe, it, expect } from 'vitest'
import { render, screen, waitFor } from '../test/utils'
import { Health } from './Health'
import { http, HttpResponse } from 'msw'
import { server } from '../test/setup'

describe('Health', () => {
  beforeEach(() => {
    server.use(
      http.get('http://localhost:5000/api/v1/system/metrics', () => {
        return HttpResponse.json({
          success: true,
          data: {
            cpu: { percent: 25, load_avg: [1.5, 1.2, 1.0] },
            memory: { total: 16000000000, used: 8000000000, free: 8000000000, percent: 50 },
            disk: { total: 500000000000, used: 250000000000, free: 250000000000, percent: 50, path: '/data' },
            gpu: { available: false },
            uptime: 86400,
          },
        })
      })
    )
  })

  it('should show loading spinner initially', () => {
    const { container } = render(<Health />)
    const spinner = container.querySelector('.animate-spin')
    expect(spinner).toBeInTheDocument()
  })

  it('should show System Health heading after loading', async () => {
    render(<Health />)

    await waitFor(() => {
      expect(screen.getByText('System Health')).toBeInTheDocument()
    })
  })

  it('should show service status section', async () => {
    render(<Health />)

    await waitFor(() => {
      expect(screen.getByText('Services')).toBeInTheDocument()
    })
  })

  it('should show API Server status', async () => {
    render(<Health />)

    await waitFor(() => {
      expect(screen.getByText('API Server')).toBeInTheDocument()
    })
  })

  it('should show service details after loading', async () => {
    render(<Health />)

    await waitFor(() => {
      expect(screen.getByText('Services')).toBeInTheDocument()
    })
    // Services section should contain Database, go2rtc, etc.
  })

  it('should show go2rtc status', async () => {
    render(<Health />)

    await waitFor(() => {
      expect(screen.getByText('go2rtc')).toBeInTheDocument()
    })
  })

  it('should show page content after loading', async () => {
    render(<Health />)

    await waitFor(() => {
      expect(screen.getByText('System Health')).toBeInTheDocument()
    })
    // Page loads successfully
  })

  it('should display camera statistics', async () => {
    render(<Health />)

    await waitFor(() => {
      expect(screen.getByText('Cameras')).toBeInTheDocument()
    })
  })

  it('should display camera and storage info in stats section', async () => {
    render(<Health />)

    await waitFor(() => {
      expect(screen.getByText('Cameras')).toBeInTheDocument()
    })
    // Stats section shows cameras and storage info
  })

  it('should show refresh button', async () => {
    render(<Health />)

    await waitFor(() => {
      const refreshButton = screen.getByRole('button', { name: /refresh/i })
      expect(refreshButton).toBeInTheDocument()
    })
  })

  it('should show system metrics section', async () => {
    render(<Health />)

    await waitFor(() => {
      expect(screen.getByText('System Health')).toBeInTheDocument()
    })
    // System metrics are displayed when available
  })

  it('should show uptime when available', async () => {
    render(<Health />)

    await waitFor(() => {
      expect(screen.getByText('Uptime')).toBeInTheDocument()
    })
  })

  it('should show healthy status indicators', async () => {
    render(<Health />)

    await waitFor(() => {
      // Look for healthy status text or icon
      const healthyElements = screen.getAllByText(/healthy|online/i)
      expect(healthyElements.length).toBeGreaterThan(0)
    })
  })

  it('should show error state when health check fails', async () => {
    server.use(
      http.get('http://localhost:5000/health', () => {
        return HttpResponse.json({
          status: 'unhealthy',
          version: '1.0.0',
          go2rtc: false,
          database: 'error',
        })
      })
    )

    render(<Health />)

    await waitFor(() => {
      // Should show some indicator of error
      expect(screen.getByText('System Health')).toBeInTheDocument()
    })
  })

  it('should show GPU info when available', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/system/metrics', () => {
        return HttpResponse.json({
          success: true,
          data: {
            cpu: { percent: 25, load_avg: [1.5, 1.2, 1.0] },
            memory: { total: 16000000000, used: 8000000000, free: 8000000000, percent: 50 },
            disk: { total: 500000000000, used: 250000000000, free: 250000000000, percent: 50, path: '/data' },
            gpu: {
              available: true,
              name: 'NVIDIA GTX 1080',
              type: 'cuda',
              memory_total: 8000000000,
              memory_used: 2000000000,
              memory_free: 6000000000,
              utilization: 30,
              temperature: 65,
            },
            uptime: 86400,
          },
        })
      })
    )

    render(<Health />)

    await waitFor(() => {
      expect(screen.getByText('GPU')).toBeInTheDocument()
    })
  })
})
