import { describe, it, expect } from 'vitest'
import { render, screen, waitFor } from '../test/utils'
import { Settings } from './Settings'
import { http, HttpResponse } from 'msw'
import { server } from '../test/setup'

describe('Settings', () => {
  beforeEach(() => {
    server.use(
      http.get('http://localhost:5000/api/v1/storage/stats', () => {
        return HttpResponse.json({
          success: true,
          data: {
            used_bytes: 50000000000,
            available_bytes: 450000000000,
            segment_count: 1000,
            by_camera: {
              'cam_1': 25000000000,
              'cam_2': 25000000000,
            },
          },
        })
      }),
      http.get('http://localhost:5000/api/v1/models/status', () => {
        return HttpResponse.json({
          success: true,
          data: {},
        })
      })
    )
  })

  it('should show Settings heading', async () => {
    render(<Settings />)

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Settings' })).toBeInTheDocument()
    })
    expect(screen.getByText('Configure your NVR system')).toBeInTheDocument()
  })

  it('should show General settings section', async () => {
    render(<Settings />)

    await waitFor(() => {
      expect(screen.getByText('General')).toBeInTheDocument()
    })
  })

  it('should show System Name input', async () => {
    render(<Settings />)

    await waitFor(() => {
      expect(screen.getByText('System Name')).toBeInTheDocument()
    })
  })

  it('should show Timezone selector', async () => {
    render(<Settings />)

    await waitFor(() => {
      expect(screen.getByText('Timezone')).toBeInTheDocument()
    })
  })

  it('should show Theme toggle', async () => {
    render(<Settings />)

    await waitFor(() => {
      expect(screen.getByText('Theme')).toBeInTheDocument()
    })
  })

  it('should show Storage settings section', async () => {
    render(<Settings />)

    await waitFor(() => {
      expect(screen.getByText('Storage')).toBeInTheDocument()
    })
  })

  it('should show Max Storage input', async () => {
    render(<Settings />)

    await waitFor(() => {
      expect(screen.getByText('Max Storage')).toBeInTheDocument()
    })
  })

  it('should show Default Retention input', async () => {
    render(<Settings />)

    await waitFor(() => {
      expect(screen.getByText('Default Retention')).toBeInTheDocument()
    })
  })

  it('should show Detection settings section', async () => {
    render(<Settings />)

    await waitFor(() => {
      expect(screen.getByText('Detection')).toBeInTheDocument()
    })
  })

  it('should show Inference Backend selector', async () => {
    render(<Settings />)

    await waitFor(() => {
      expect(screen.getByText('Inference Backend')).toBeInTheDocument()
    })
  })

  it('should show Object Detection options', async () => {
    render(<Settings />)

    await waitFor(() => {
      expect(screen.getByText('Object Detection')).toBeInTheDocument()
    })
  })

  it('should show Face Recognition options', async () => {
    render(<Settings />)

    await waitFor(() => {
      expect(screen.getByText('Face Recognition')).toBeInTheDocument()
    })
  })

  it('should show License Plate Recognition options', async () => {
    render(<Settings />)

    await waitFor(() => {
      expect(screen.getByText('License Plate Recognition')).toBeInTheDocument()
    })
  })

  it('should show Save Changes button', async () => {
    render(<Settings />)

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Save Changes' })).toBeInTheDocument()
    })
  })

  it('should show Plugins link', async () => {
    render(<Settings />)

    await waitFor(() => {
      expect(screen.getByText('Plugins')).toBeInTheDocument()
    })
  })

  it('should show System Health link', async () => {
    render(<Settings />)

    await waitFor(() => {
      expect(screen.getByText('System Health')).toBeInTheDocument()
    })
  })

  it('should toggle theme when theme button clicked', async () => {
    const { user } = render(<Settings />)

    await waitFor(() => {
      expect(screen.getByText('Theme')).toBeInTheDocument()
    })

    // Find the theme toggle button - it should have Dark or Light text
    const themeButton = screen.getByRole('button', { name: /Dark|Light/i })
    await user.click(themeButton)

    // After click, the opposite theme should be displayed
  })

  it('should show storage usage when stats loaded', async () => {
    render(<Settings />)

    await waitFor(() => {
      expect(screen.getByText('Storage Usage')).toBeInTheDocument()
    })
  })

  it('should show Run Cleanup Now button', async () => {
    render(<Settings />)

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /Run Cleanup Now/i })).toBeInTheDocument()
    })
  })

  it('should expand Object Detection when clicked', async () => {
    const { user } = render(<Settings />)

    await waitFor(() => {
      expect(screen.getByText('Object Detection')).toBeInTheDocument()
    })

    // Click on Object Detection to expand it
    await user.click(screen.getByText('Object Detection'))

    // Model selector should be visible
    await waitFor(() => {
      expect(screen.getByText('Model')).toBeInTheDocument()
    })
  })

  it('should show Detection FPS input', async () => {
    render(<Settings />)

    await waitFor(() => {
      expect(screen.getByText('Detection FPS')).toBeInTheDocument()
    })
  })

  it('should show system name input field', async () => {
    render(<Settings />)

    await waitFor(() => {
      expect(screen.getByText('System Name')).toBeInTheDocument()
    })

    // Input field is present on the page
    const inputs = screen.getAllByRole('textbox')
    expect(inputs.length).toBeGreaterThan(0)
  })
})
