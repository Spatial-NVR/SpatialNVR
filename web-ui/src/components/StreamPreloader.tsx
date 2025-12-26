// StreamPreloader - Pre-connects to all camera streams on app load for instant playback
import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { cameraApi } from '../lib/api'
import { streamManager } from '../lib/streamManager'

export function StreamPreloader() {
  const { data: cameras } = useQuery({
    queryKey: ['cameras'],
    queryFn: cameraApi.list,
    // Fetch cameras immediately and keep refreshing
    staleTime: 30000,
    refetchInterval: 60000,
  })

  // Pre-connect to all online cameras when the list is loaded
  useEffect(() => {
    if (!cameras || cameras.length === 0) return

    // Get IDs of all online cameras
    const onlineCameraIds = cameras
      .filter(cam => cam.status === 'online' && cam.enabled !== false)
      .map(cam => cam.id)

    if (onlineCameraIds.length > 0) {
      console.log('[StreamPreloader] Pre-connecting to cameras:', onlineCameraIds)
      streamManager.preconnect(onlineCameraIds)
    }

    // Cleanup on unmount
    return () => {
      // Don't disconnect - keep streams alive for instant playback
    }
  }, [cameras])

  // This component renders nothing - it just manages streams in the background
  return null
}
