// Hook for using pre-buffered video streams
import { useEffect, useState, useRef } from 'react'
import { streamManager } from '../lib/streamManager'

interface UsePreBufferedStreamResult {
  objectUrl: string | null
  isReady: boolean
  videoRef: React.RefObject<HTMLVideoElement>
}

export function usePreBufferedStream(cameraId: string | undefined): UsePreBufferedStreamResult {
  const [objectUrl, setObjectUrl] = useState<string | null>(null)
  const [isReady, setIsReady] = useState(false)
  const videoRef = useRef<HTMLVideoElement>(null)

  useEffect(() => {
    if (!cameraId) {
      setObjectUrl(null)
      setIsReady(false)
      return
    }

    // Check if already ready
    const existingUrl = streamManager.getObjectUrl(cameraId)
    if (existingUrl && streamManager.isReady(cameraId)) {
      setObjectUrl(existingUrl)
      setIsReady(true)

      // Attach to video element immediately
      if (videoRef.current) {
        videoRef.current.src = existingUrl
        videoRef.current.play().catch(() => {})
      }
    }

    // Subscribe to updates
    const unsubscribe = streamManager.subscribe(cameraId, (url) => {
      setObjectUrl(url)
      setIsReady(true)

      // Attach to video element
      if (videoRef.current) {
        videoRef.current.src = url
        videoRef.current.play().catch(() => {})
      }
    })

    // Ensure stream is connected
    streamManager.getStream(cameraId)

    return () => {
      unsubscribe()
    }
  }, [cameraId])

  // Auto-play when video element is available and stream is ready
  useEffect(() => {
    if (videoRef.current && objectUrl && isReady) {
      if (videoRef.current.src !== objectUrl) {
        videoRef.current.src = objectUrl
      }
      videoRef.current.play().catch(() => {})
    }
  }, [objectUrl, isReady])

  return { objectUrl, isReady, videoRef }
}
