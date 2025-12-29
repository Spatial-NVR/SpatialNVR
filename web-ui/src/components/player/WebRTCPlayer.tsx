/**
 * WebRTCPlayer - WebRTC player for go2rtc streams
 * Based on Frigate's WebRTCPlayer implementation
 */
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { getGo2RTCWsUrl } from '../../hooks/usePorts'

interface WebRTCPlayerProps {
  camera: string
  className?: string
  playbackEnabled?: boolean
  audioEnabled?: boolean
  volume?: number
  onPlaying?: () => void
  onError?: (error: string) => void
}

export default function WebRTCPlayer({
  camera,
  className,
  playbackEnabled = true,
  audioEnabled = false,
  volume = 1,
  onPlaying,
  onError,
}: WebRTCPlayerProps) {
  const wsURL = useMemo(() => {
    const streamName = camera.toLowerCase().replace(/\s+/g, '_')
    return `${getGo2RTCWsUrl()}/api/ws?src=${streamName}`
  }, [camera])

  const handleError = useCallback((error: string, description: string) => {
    console.error(`[WebRTCPlayer] ${camera} - ${error}: ${description}`)
    onError?.(error)
  }, [camera, onError])

  const pcRef = useRef<RTCPeerConnection | undefined>()
  const videoRef = useRef<HTMLVideoElement | null>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const videoLoadTimeoutRef = useRef<ReturnType<typeof setTimeout>>()
  const [bufferTimeout, setBufferTimeout] = useState<ReturnType<typeof setTimeout>>()

  const PeerConnection = useCallback(async (media: string) => {
    if (!videoRef.current) return

    const pc = new RTCPeerConnection({
      bundlePolicy: 'max-bundle',
      iceServers: [{ urls: 'stun:stun.l.google.com:19302' }],
    })

    const localTracks: MediaStreamTrack[] = []

    // Add video and audio transceivers for receiving
    if (/video|audio/.test(media)) {
      const tracks = ['video', 'audio']
        .filter((kind) => media.indexOf(kind) >= 0)
        .map((kind) => {
          const transceiver = pc.addTransceiver(kind as 'video' | 'audio', { direction: 'recvonly' })
          return transceiver.receiver.track
        })
      localTracks.push(...tracks)
    }

    // Set the stream on the video element
    videoRef.current.srcObject = new MediaStream(localTracks)
    return pc
  }, [])

  const connect = useCallback(async (aPc: Promise<RTCPeerConnection | undefined>) => {
    if (!aPc) return

    pcRef.current = await aPc
    if (!pcRef.current) return

    wsRef.current = new WebSocket(wsURL)

    wsRef.current.addEventListener('open', () => {
      pcRef.current?.addEventListener('icecandidate', (ev) => {
        if (!ev.candidate) return
        const msg = {
          type: 'webrtc/candidate',
          value: ev.candidate.candidate,
        }
        wsRef.current?.send(JSON.stringify(msg))
      })

      pcRef.current
        ?.createOffer()
        .then((offer) => pcRef.current?.setLocalDescription(offer))
        .then(() => {
          const msg = {
            type: 'webrtc/offer',
            value: pcRef.current?.localDescription?.sdp,
          }
          wsRef.current?.send(JSON.stringify(msg))
        })
    })

    wsRef.current.addEventListener('message', (ev) => {
      const msg = JSON.parse(ev.data)
      if (msg.type === 'webrtc/candidate') {
        pcRef.current?.addIceCandidate({ candidate: msg.value, sdpMid: '0' })
      } else if (msg.type === 'webrtc/answer') {
        pcRef.current?.setRemoteDescription({
          type: 'answer',
          sdp: msg.value,
        })
      }
    })

    wsRef.current.addEventListener('error', () => {
      handleError('websocket', 'WebSocket connection error')
    })
  }, [wsURL, handleError])

  // Initialize connection - use a small delay to ensure video element is in DOM
  useEffect(() => {
    if (!playbackEnabled) return

    const initConnection = () => {
      if (!videoRef.current) {
        // Video element not yet mounted, try again shortly
        setTimeout(initConnection, 50)
        return
      }

      console.log('[WebRTCPlayer] Initializing connection for', camera)
      const aPc = PeerConnection('video+audio')
      connect(aPc)
    }

    initConnection()

    return () => {
      if (pcRef.current) {
        pcRef.current.close()
        pcRef.current = undefined
      }
      if (wsRef.current) {
        wsRef.current.close()
        wsRef.current = null
      }
    }
  }, [camera, connect, PeerConnection, playbackEnabled])

  // Sync volume
  useEffect(() => {
    if (videoRef.current && volume !== undefined) {
      videoRef.current.volume = volume
    }
  }, [volume])

  // Connection timeout
  useEffect(() => {
    videoLoadTimeoutRef.current = setTimeout(() => {
      handleError('timeout', 'WebRTC connection timed out')
    }, 10000)

    return () => {
      if (videoLoadTimeoutRef.current) {
        clearTimeout(videoLoadTimeoutRef.current)
      }
    }
  }, [handleError])

  const handleLoadedData = useCallback(() => {
    if (videoLoadTimeoutRef.current) {
      clearTimeout(videoLoadTimeoutRef.current)
    }
    onPlaying?.()
  }, [onPlaying])

  return (
    <video
      ref={videoRef}
      className={className}
      autoPlay
      playsInline
      muted={!audioEnabled}
      onLoadedData={handleLoadedData}
      onProgress={
        onError
          ? () => {
              if (videoRef.current?.paused) return

              if (bufferTimeout) {
                clearTimeout(bufferTimeout)
                setBufferTimeout(undefined)
              }

              setBufferTimeout(
                setTimeout(() => {
                  if (document.visibilityState === 'visible' && pcRef.current) {
                    handleError('stalled', 'Media playback stalled')
                  }
                }, 5000)
              )
            }
          : undefined
      }
      onError={(e) => {
        const target = e.target as HTMLVideoElement
        if (target.error?.code === MediaError.MEDIA_ERR_NETWORK) {
          handleError('network', 'Browser reported a network error')
        }
      }}
    />
  )
}
