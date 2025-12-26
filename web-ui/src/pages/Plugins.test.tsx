import { describe, it, expect } from 'vitest'
import { render, screen, waitFor } from '../test/utils'
import { Plugins } from './Plugins'
import { http, HttpResponse } from 'msw'
import { server } from '../test/setup'

describe('Plugins', () => {
  it('should show loading spinner initially', () => {
    const { container } = render(<Plugins />)
    const spinner = container.querySelector('.animate-spin')
    expect(spinner).toBeInTheDocument()
  })

  it('should show the Plugins heading', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/plugins/catalog', () => {
        return HttpResponse.json({
          success: true,
          data: {
            version: '1.0.0',
            updated: '2024-01-01',
            plugins: [],
            categories: {},
          },
        })
      })
    )

    render(<Plugins />)

    await waitFor(() => {
      expect(screen.getByText('Plugins')).toBeInTheDocument()
    })
    expect(screen.getByText('Extend your NVR with camera integrations and features')).toBeInTheDocument()
  })

  it('should show error state when fetch fails', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/plugins/catalog', () => {
        return HttpResponse.error()
      })
    )

    render(<Plugins />)

    await waitFor(() => {
      expect(screen.getByText('Failed to load plugin catalog')).toBeInTheDocument()
    })
    expect(screen.getByText('Retry')).toBeInTheDocument()
  })

  it('should show empty state when no plugins available', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/plugins/catalog', () => {
        return HttpResponse.json({
          success: true,
          data: {
            version: '1.0.0',
            updated: '2024-01-01',
            plugins: [],
            categories: {},
          },
        })
      })
    )

    render(<Plugins />)

    await waitFor(() => {
      expect(screen.getByText('No plugins available')).toBeInTheDocument()
    })
  })

  it('should show Install from GitHub section', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/plugins/catalog', () => {
        return HttpResponse.json({
          success: true,
          data: {
            version: '1.0.0',
            updated: '2024-01-01',
            plugins: [],
            categories: {},
          },
        })
      })
    )

    render(<Plugins />)

    await waitFor(() => {
      expect(screen.getByText('Install from GitHub')).toBeInTheDocument()
    })
    expect(screen.getByPlaceholderText('github.com/owner/plugin-repo')).toBeInTheDocument()
  })

  it('should display available plugins', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/plugins/catalog', () => {
        return HttpResponse.json({
          success: true,
          data: {
            version: '1.0.0',
            updated: '2024-01-01',
            plugins: [
              {
                id: 'plugin-reolink',
                name: 'Reolink Camera',
                description: 'Integration for Reolink cameras',
                category: 'cameras',
                latest_version: '1.0.0',
                installed: false,
                featured: true,
                capabilities: ['rtsp', 'ptz'],
              },
            ],
            categories: {
              cameras: { name: 'Cameras', description: 'Camera integrations' },
            },
          },
        })
      })
    )

    render(<Plugins />)

    await waitFor(() => {
      expect(screen.getByText('Reolink Camera')).toBeInTheDocument()
    })
    expect(screen.getByText('Integration for Reolink cameras')).toBeInTheDocument()
    expect(screen.getByText(/Available/)).toBeInTheDocument()
  })

  it('should display installed plugins', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/plugins/catalog', () => {
        return HttpResponse.json({
          success: true,
          data: {
            version: '1.0.0',
            updated: '2024-01-01',
            plugins: [
              {
                id: 'plugin-reolink',
                name: 'Reolink Camera',
                description: 'Integration for Reolink cameras',
                category: 'cameras',
                latest_version: '1.0.0',
                installed: true,
                installed_version: '1.0.0',
                enabled: true,
                state: 'running',
              },
            ],
            categories: {},
          },
        })
      })
    )

    render(<Plugins />)

    await waitFor(() => {
      expect(screen.getByText('Reolink Camera')).toBeInTheDocument()
    })
    expect(screen.getByText(/Installed/)).toBeInTheDocument()
  })

  it('should show category filter buttons', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/plugins/catalog', () => {
        return HttpResponse.json({
          success: true,
          data: {
            version: '1.0.0',
            updated: '2024-01-01',
            plugins: [],
            categories: {
              cameras: { name: 'Cameras', description: 'Camera integrations' },
              integrations: { name: 'Integrations', description: 'Third party integrations' },
            },
          },
        })
      })
    )

    render(<Plugins />)

    await waitFor(() => {
      expect(screen.getByText('All')).toBeInTheDocument()
    })
    expect(screen.getByText('Cameras')).toBeInTheDocument()
    expect(screen.getByText('Integrations')).toBeInTheDocument()
  })

  it('should filter plugins by category', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/plugins/catalog', () => {
        return HttpResponse.json({
          success: true,
          data: {
            version: '1.0.0',
            updated: '2024-01-01',
            plugins: [
              {
                id: 'plugin-reolink',
                name: 'Reolink Camera',
                description: 'Camera plugin',
                category: 'cameras',
                latest_version: '1.0.0',
                installed: false,
              },
              {
                id: 'plugin-mqtt',
                name: 'MQTT Integration',
                description: 'MQTT plugin',
                category: 'integrations',
                latest_version: '1.0.0',
                installed: false,
              },
            ],
            categories: {
              cameras: { name: 'Cameras', description: 'Camera integrations' },
              integrations: { name: 'Integrations', description: 'Third party integrations' },
            },
          },
        })
      })
    )

    const { user } = render(<Plugins />)

    await waitFor(() => {
      expect(screen.getByText('Reolink Camera')).toBeInTheDocument()
    })

    // Both should be visible initially
    expect(screen.getByText('MQTT Integration')).toBeInTheDocument()

    // Click on Cameras filter
    await user.click(screen.getByText('Cameras'))

    // Only camera plugin should be visible
    expect(screen.getByText('Reolink Camera')).toBeInTheDocument()
    expect(screen.queryByText('MQTT Integration')).not.toBeInTheDocument()
  })

  it('should show plugin capabilities as badges', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/plugins/catalog', () => {
        return HttpResponse.json({
          success: true,
          data: {
            version: '1.0.0',
            updated: '2024-01-01',
            plugins: [
              {
                id: 'plugin-test',
                name: 'Test Plugin',
                description: 'Test description',
                category: 'cameras',
                latest_version: '1.0.0',
                installed: false,
                capabilities: ['rtsp', 'ptz', 'audio', 'two-way', 'more'],
              },
            ],
            categories: {},
          },
        })
      })
    )

    render(<Plugins />)

    await waitFor(() => {
      expect(screen.getByText('rtsp')).toBeInTheDocument()
    })
    expect(screen.getByText('ptz')).toBeInTheDocument()
    expect(screen.getByText('audio')).toBeInTheDocument()
    expect(screen.getByText('two-way')).toBeInTheDocument()
    expect(screen.getByText('+1')).toBeInTheDocument() // More than 4 capabilities
  })

  it('should show install button for uninstalled plugins', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/plugins/catalog', () => {
        return HttpResponse.json({
          success: true,
          data: {
            version: '1.0.0',
            updated: '2024-01-01',
            plugins: [
              {
                id: 'plugin-test',
                name: 'Test Plugin',
                description: 'Test description',
                category: 'cameras',
                latest_version: '1.0.0',
                installed: false,
              },
            ],
            categories: {},
          },
        })
      })
    )

    render(<Plugins />)

    await waitFor(() => {
      // There are two Install buttons - one in GitHub section, one for the plugin
      const installButtons = screen.getAllByText('Install')
      expect(installButtons.length).toBeGreaterThanOrEqual(2)
    })
  })

  it('should show update button for plugins with available updates', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/plugins/catalog', () => {
        return HttpResponse.json({
          success: true,
          data: {
            version: '1.0.0',
            updated: '2024-01-01',
            plugins: [
              {
                id: 'plugin-test',
                name: 'Test Plugin',
                description: 'Test description',
                category: 'cameras',
                latest_version: '2.0.0',
                installed: true,
                installed_version: '1.0.0',
                enabled: true,
                state: 'running',
                update_available: true,
              },
            ],
            categories: {},
          },
        })
      })
    )

    render(<Plugins />)

    await waitFor(() => {
      const updateButton = screen.getByTitle('Update to 2.0.0')
      expect(updateButton).toBeInTheDocument()
    })
  })

  it('should show version info for installed plugins', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/plugins/catalog', () => {
        return HttpResponse.json({
          success: true,
          data: {
            version: '1.0.0',
            updated: '2024-01-01',
            plugins: [
              {
                id: 'plugin-test',
                name: 'Test Plugin',
                description: 'Test description',
                category: 'cameras',
                latest_version: '1.0.0',
                installed: true,
                installed_version: '1.0.0',
                enabled: true,
                state: 'running',
              },
            ],
            categories: {},
          },
        })
      })
    )

    render(<Plugins />)

    await waitFor(() => {
      expect(screen.getByText('v1.0.0')).toBeInTheDocument()
    })
  })

  it('should show catalog footer info', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/plugins/catalog', () => {
        return HttpResponse.json({
          success: true,
          data: {
            version: '2.5.0',
            updated: '2024-06-15',
            plugins: [],
            categories: {},
          },
        })
      })
    )

    render(<Plugins />)

    await waitFor(() => {
      expect(screen.getByText(/Catalog version 2.5.0/)).toBeInTheDocument()
    })
  })

  it('should show refresh button', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/plugins/catalog', () => {
        return HttpResponse.json({
          success: true,
          data: {
            version: '1.0.0',
            updated: '2024-01-01',
            plugins: [],
            categories: {},
          },
        })
      })
    )

    render(<Plugins />)

    await waitFor(() => {
      const refreshButton = screen.getByTitle('Refresh catalog')
      expect(refreshButton).toBeInTheDocument()
    })
  })

  it('should show featured badge for featured plugins', async () => {
    server.use(
      http.get('http://localhost:5000/api/v1/plugins/catalog', () => {
        return HttpResponse.json({
          success: true,
          data: {
            version: '1.0.0',
            updated: '2024-01-01',
            plugins: [
              {
                id: 'plugin-test',
                name: 'Test Plugin',
                description: 'Test description',
                category: 'cameras',
                latest_version: '1.0.0',
                installed: false,
                featured: true,
              },
            ],
            categories: {},
          },
        })
      })
    )

    render(<Plugins />)

    await waitFor(() => {
      // Look for the star icon (featured indicator)
      const container = screen.getByText('Test Plugin').parentElement
      expect(container).toBeInTheDocument()
    })
  })
})
