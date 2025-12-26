import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'

// Create mock functions before the vi.mock call
const mockOnFn = vi.fn()
const mockOffFn = vi.fn()
const mockSubscribeFn = vi.fn()
const mockIsConnectedFn = vi.fn(() => false)

vi.mock('../lib/api', () => ({
  nvrWebSocket: {
    on: (...args: Parameters<typeof mockOnFn>) => mockOnFn(...args),
    off: (...args: Parameters<typeof mockOffFn>) => mockOffFn(...args),
    subscribe: (...args: Parameters<typeof mockSubscribeFn>) => mockSubscribeFn(...args),
    isConnected: () => mockIsConnectedFn(),
  },
}))

// Import after mock setup
import { useDetections } from './useDetections'

describe('useDetections', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('should initialize with empty detections', () => {
    const { result } = renderHook(() =>
      useDetections({ cameraId: 'cam_1' })
    )

    expect(result.current.detections).toEqual([])
    expect(result.current.motionDetected).toBe(false)
    expect(result.current.lastUpdate).toBeNull()
  })

  it('should subscribe to the camera on mount', () => {
    renderHook(() => useDetections({ cameraId: 'cam_1' }))

    expect(mockSubscribeFn).toHaveBeenCalledWith(['cam_1'])
  })

  it('should register event listeners on mount', () => {
    renderHook(() => useDetections({ cameraId: 'cam_1' }))

    expect(mockOnFn).toHaveBeenCalledWith('detection', expect.any(Function))
    expect(mockOnFn).toHaveBeenCalledWith('connect', expect.any(Function))
    expect(mockOnFn).toHaveBeenCalledWith('disconnect', expect.any(Function))
  })

  it('should unregister event listeners on unmount', () => {
    const { unmount } = renderHook(() => useDetections({ cameraId: 'cam_1' }))

    unmount()

    expect(mockOffFn).toHaveBeenCalledWith('detection', expect.any(Function))
    expect(mockOffFn).toHaveBeenCalledWith('connect', expect.any(Function))
    expect(mockOffFn).toHaveBeenCalledWith('disconnect', expect.any(Function))
  })

  it('should not subscribe when disabled', () => {
    mockSubscribeFn.mockClear()

    renderHook(() => useDetections({ cameraId: 'cam_1', enabled: false }))

    expect(mockSubscribeFn).not.toHaveBeenCalled()
  })

  it('should report connection status', () => {
    mockIsConnectedFn.mockReturnValue(true)

    const { result } = renderHook(() => useDetections({ cameraId: 'cam_1' }))

    expect(result.current.isConnected).toBe(true)
  })

  it('should handle detection events for the correct camera', async () => {
    let detectionHandler: ((data: unknown) => void) | undefined

    mockOnFn.mockImplementation((event: string, callback: (data: unknown) => void) => {
      if (event === 'detection') {
        detectionHandler = callback
      }
    })

    const { result } = renderHook(() => useDetections({ cameraId: 'cam_1' }))

    expect(detectionHandler).toBeDefined()

    // Simulate a detection event
    act(() => {
      detectionHandler!({
        camera_id: 'cam_1',
        detections: [
          { label: 'person', confidence: 0.95, bbox: { x: 0.1, y: 0.1, width: 0.2, height: 0.4 } },
        ],
        motion_detected: true,
      })
    })

    expect(result.current.detections).toHaveLength(1)
    expect(result.current.detections[0].label).toBe('person')
    expect(result.current.motionDetected).toBe(true)
    expect(result.current.lastUpdate).not.toBeNull()
  })

  it('should ignore detection events for other cameras', async () => {
    let detectionHandler: ((data: unknown) => void) | undefined

    mockOnFn.mockImplementation((event: string, callback: (data: unknown) => void) => {
      if (event === 'detection') {
        detectionHandler = callback
      }
    })

    const { result } = renderHook(() => useDetections({ cameraId: 'cam_1' }))

    // Simulate a detection event for a different camera
    act(() => {
      detectionHandler!({
        camera_id: 'cam_2', // Different camera
        detections: [
          { label: 'car', confidence: 0.9, bbox: { x: 0.1, y: 0.1, width: 0.2, height: 0.4 } },
        ],
        motion_detected: true,
      })
    })

    expect(result.current.detections).toEqual([])
  })

  it('should clear detections after maxAge timeout', async () => {
    let detectionHandler: ((data: unknown) => void) | undefined

    mockOnFn.mockImplementation((event: string, callback: (data: unknown) => void) => {
      if (event === 'detection') {
        detectionHandler = callback
      }
    })

    const { result } = renderHook(() =>
      useDetections({ cameraId: 'cam_1', maxAge: 500 })
    )

    // Simulate a detection
    act(() => {
      detectionHandler!({
        camera_id: 'cam_1',
        detections: [{ label: 'person', confidence: 0.95, bbox: { x: 0.1, y: 0.1, width: 0.2, height: 0.4 } }],
        motion_detected: true,
      })
    })

    expect(result.current.detections).toHaveLength(1)

    // Advance timers past the maxAge
    act(() => {
      vi.advanceTimersByTime(600)
    })

    expect(result.current.detections).toEqual([])
    expect(result.current.motionDetected).toBe(false)
  })

  it('should handle connect events', async () => {
    let connectHandler: ((data: unknown) => void) | undefined

    mockOnFn.mockImplementation((event: string, callback: (data: unknown) => void) => {
      if (event === 'connect') {
        connectHandler = callback
      }
    })

    const { result } = renderHook(() => useDetections({ cameraId: 'cam_1' }))

    act(() => {
      connectHandler!({})
    })

    expect(result.current.isConnected).toBe(true)
  })

  it('should handle disconnect events and clear detections', async () => {
    let disconnectHandler: ((data: unknown) => void) | undefined
    let detectionHandler: ((data: unknown) => void) | undefined

    mockOnFn.mockImplementation((event: string, callback: (data: unknown) => void) => {
      if (event === 'disconnect') {
        disconnectHandler = callback
      }
      if (event === 'detection') {
        detectionHandler = callback
      }
    })

    const { result } = renderHook(() => useDetections({ cameraId: 'cam_1' }))

    // Add a detection first
    act(() => {
      detectionHandler!({
        camera_id: 'cam_1',
        detections: [{ label: 'person', confidence: 0.95, bbox: { x: 0.1, y: 0.1, width: 0.2, height: 0.4 } }],
        motion_detected: true,
      })
    })

    expect(result.current.detections).toHaveLength(1)

    // Simulate disconnect
    act(() => {
      disconnectHandler!({})
    })

    expect(result.current.isConnected).toBe(false)
    expect(result.current.detections).toEqual([])
  })

  it('should clear detections when disabled', () => {
    let detectionHandler: ((data: unknown) => void) | undefined

    mockOnFn.mockImplementation((event: string, callback: (data: unknown) => void) => {
      if (event === 'detection') {
        detectionHandler = callback
      }
    })

    const { result, rerender } = renderHook(
      ({ enabled }) => useDetections({ cameraId: 'cam_1', enabled }),
      { initialProps: { enabled: true } }
    )

    // Add a detection
    act(() => {
      detectionHandler!({
        camera_id: 'cam_1',
        detections: [{ label: 'person', confidence: 0.95, bbox: { x: 0.1, y: 0.1, width: 0.2, height: 0.4 } }],
        motion_detected: true,
      })
    })

    expect(result.current.detections).toHaveLength(1)

    // Disable the hook
    rerender({ enabled: false })

    expect(result.current.detections).toEqual([])
  })

  it('should use default maxAge of 1000ms', () => {
    let detectionHandler: ((data: unknown) => void) | undefined

    mockOnFn.mockImplementation((event: string, callback: (data: unknown) => void) => {
      if (event === 'detection') {
        detectionHandler = callback
      }
    })

    const { result } = renderHook(() => useDetections({ cameraId: 'cam_1' }))

    act(() => {
      detectionHandler!({
        camera_id: 'cam_1',
        detections: [{ label: 'person', confidence: 0.95, bbox: { x: 0.1, y: 0.1, width: 0.2, height: 0.4 } }],
        motion_detected: true,
      })
    })

    // Should still have detections at 900ms
    act(() => {
      vi.advanceTimersByTime(900)
    })

    expect(result.current.detections).toHaveLength(1)

    // Should clear after 1000ms total
    act(() => {
      vi.advanceTimersByTime(200)
    })

    expect(result.current.detections).toEqual([])
  })
})
