import { useState, useEffect, useCallback, useRef } from 'react'
import { nvrWebSocket } from '../lib/api'
import type { Detection } from '../components/DetectionOverlay'

interface DetectionEvent {
  camera_id: string
  detections: Detection[]
  timestamp?: number
  motion_detected?: boolean
}

interface UseDetectionsOptions {
  cameraId: string
  enabled?: boolean
  maxAge?: number // Max age of detections in ms before clearing
}

interface UseDetectionsResult {
  detections: Detection[]
  motionDetected: boolean
  lastUpdate: number | null
  isConnected: boolean
}

export function useDetections({
  cameraId,
  enabled = true,
  maxAge = 1000, // Clear detections after 1 second of no updates
}: UseDetectionsOptions): UseDetectionsResult {
  const [detections, setDetections] = useState<Detection[]>([])
  const [motionDetected, setMotionDetected] = useState(false)
  const [lastUpdate, setLastUpdate] = useState<number | null>(null)
  const [isConnected, setIsConnected] = useState(false)
  const clearTimeoutRef = useRef<NodeJS.Timeout | null>(null)

  // Clear stale detections
  const scheduleClear = useCallback(() => {
    if (clearTimeoutRef.current) {
      clearTimeout(clearTimeoutRef.current)
    }
    clearTimeoutRef.current = setTimeout(() => {
      setDetections([])
      setMotionDetected(false)
    }, maxAge)
  }, [maxAge])

  // Handle detection events
  const handleDetection = useCallback((data: unknown) => {
    const event = data as DetectionEvent
    if (event.camera_id !== cameraId) return

    setDetections(event.detections || [])
    setMotionDetected(event.motion_detected ?? true)
    setLastUpdate(Date.now())
    scheduleClear()
  }, [cameraId, scheduleClear])

  // Handle connection state
  const handleConnect = useCallback((_data: unknown) => {
    setIsConnected(true)
  }, [])

  const handleDisconnect = useCallback((_data: unknown) => {
    setIsConnected(false)
    setDetections([])
  }, [])

  useEffect(() => {
    if (!enabled) {
      setDetections([])
      return
    }

    // Subscribe to WebSocket events
    nvrWebSocket.on('detection', handleDetection)
    nvrWebSocket.on('connect', handleConnect)
    nvrWebSocket.on('disconnect', handleDisconnect)

    // Subscribe to this camera's events
    nvrWebSocket.subscribe([cameraId])

    // Check current connection state
    setIsConnected(nvrWebSocket.isConnected())

    return () => {
      nvrWebSocket.off('detection', handleDetection)
      nvrWebSocket.off('connect', handleConnect)
      nvrWebSocket.off('disconnect', handleDisconnect)

      if (clearTimeoutRef.current) {
        clearTimeout(clearTimeoutRef.current)
      }
    }
  }, [cameraId, enabled, handleDetection, handleConnect, handleDisconnect])

  return {
    detections,
    motionDetected,
    lastUpdate,
    isConnected,
  }
}

export default useDetections
