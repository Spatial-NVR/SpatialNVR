/**
 * MSEPlayer - Media Source Extensions player for go2rtc streams
 * Based on Frigate's MSEPlayer implementation
 */
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { getGo2RTCWsUrl } from '../../hooks/usePorts'

// Detect iOS/Safari
const isIOS = /iPad|iPhone|iPod/.test(navigator.userAgent) ||
  (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1)
const isSafari = /^((?!chrome|android).)*safari/i.test(navigator.userAgent)

// Supported codecs for MSE - matches Frigate's list
const CODECS = [
  'avc1.640029',      // H.264 high 4.1
  'avc1.64002A',      // H.264 high 4.2
  'avc1.640033',      // H.264 high 5.1
  'hvc1.1.6.L153.B0', // H.265 main 5.1
  'mp4a.40.2',        // AAC LC
  'mp4a.40.5',        // AAC HE
  'flac',             // FLAC
  'opus',             // Opus
]

interface MSEPlayerProps {
  camera: string
  className?: string
  playbackEnabled?: boolean
  audioEnabled?: boolean
  volume?: number
  onPlaying?: () => void
  onError?: (error: string) => void
}

export default function MSEPlayer({
  camera,
  className,
  playbackEnabled = true,
  audioEnabled = false,
  volume = 1,
  onPlaying,
  onError,
}: MSEPlayerProps) {
  const RECONNECT_TIMEOUT = 10000

  const [isPlaying, setIsPlaying] = useState(false)
  const [wsState, setWsState] = useState<number>(WebSocket.CLOSED)
  const [connectTS, setConnectTS] = useState<number>(0)

  const videoRef = useRef<HTMLVideoElement>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const reconnectTIDRef = useRef<number | null>(null)
  const ondataRef = useRef<((data: ArrayBufferLike) => void) | null>(null)
  const onmessageRef = useRef<Record<string, (msg: { value: string; type: string }) => void>>({})
  const msRef = useRef<MediaSource | null>(null)
  const onConnectRef = useRef<() => void>(() => {})

  const wsURL = useMemo(() => {
    const streamName = camera.toLowerCase().replace(/\s+/g, '_')
    return `${getGo2RTCWsUrl()}/api/ws?src=${streamName}`
  }, [camera])

  const handleError = useCallback((error: string, description: string) => {
    console.error(`[MSEPlayer] ${camera} - ${error}: ${description}`)
    onError?.(error)
  }, [camera, onError])

  const play = useCallback(() => {
    const video = videoRef.current
    if (!video) return

    video.play().catch((err) => {
      if (err.name === 'NotAllowedError' && !video.muted) {
        video.muted = true
        video.play().catch(() => {})
      }
    })
  }, [])

  const send = useCallback((value: object) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(value))
    }
  }, [])

  const codecs = useCallback((isSupported: (type: string) => boolean) => {
    return CODECS.filter((codec) =>
      isSupported(`video/mp4; codecs="${codec}"`)
    ).join()
  }, [])

  const onDisconnect = useCallback(() => {
    setIsPlaying(false)

    if (wsRef.current) {
      setWsState(WebSocket.CLOSED)
      wsRef.current.close()
      wsRef.current = null
    }
  }, [])

  const reconnect = useCallback((timeout?: number) => {
    setWsState(WebSocket.CONNECTING)
    wsRef.current = null

    const delay = timeout ?? Math.max(RECONNECT_TIMEOUT - (Date.now() - connectTS), 0)

    reconnectTIDRef.current = window.setTimeout(() => {
      reconnectTIDRef.current = null
      onConnectRef.current()
    }, delay)
  }, [connectTS])

  const onMse = useCallback(() => {
    const MediaSourceConstructor = 'ManagedMediaSource' in window
      ? (window as unknown as { ManagedMediaSource: typeof MediaSource }).ManagedMediaSource
      : MediaSource

    msRef.current?.addEventListener('sourceopen', () => {
      send({ type: 'mse', value: codecs(MediaSourceConstructor.isTypeSupported) })
    }, { once: true })

    if (videoRef.current && msRef.current) {
      if ('ManagedMediaSource' in window) {
        videoRef.current.disableRemotePlayback = true
        ;(videoRef.current as HTMLVideoElement & { srcObject: MediaSource | null }).srcObject = msRef.current
      } else {
        videoRef.current.src = URL.createObjectURL(msRef.current)
        videoRef.current.srcObject = null
      }
    }

    play()

    onmessageRef.current['mse'] = (msg) => {
      if (msg.type !== 'mse') return

      let sb: SourceBuffer | undefined
      try {
        sb = msRef.current?.addSourceBuffer(msg.value)
        if (sb?.mode) {
          sb.mode = 'segments'
        }
      } catch (e) {
        if (e instanceof DOMException && e.name === 'InvalidStateError') {
          onDisconnect()
          handleError('mse-decode', 'Browser reported InvalidStateError')
          return
        }
        throw e
      }

      const buf = new Uint8Array(2 * 1024 * 1024)
      let bufLen = 0

      sb?.addEventListener('updateend', () => {
        if (sb.updating) return

        try {
          if (bufLen > 0) {
            const data = buf.slice(0, bufLen)
            bufLen = 0
            sb.appendBuffer(data)
          } else if (sb.buffered && sb.buffered.length) {
            const end = sb.buffered.end(sb.buffered.length - 1) - 15
            const start = sb.buffered.start(0)
            if (end > start) {
              sb.remove(start, end)
              msRef.current?.setLiveSeekableRange(end, end + 15)
            }
          }
        } catch {
          // no-op
        }
      })

      ondataRef.current = (data) => {
        if (sb?.updating || bufLen > 0) {
          const b = new Uint8Array(data)
          buf.set(b, bufLen)
          bufLen += b.byteLength
        } else {
          try {
            sb?.appendBuffer(data as ArrayBuffer)
          } catch {
            // no-op
          }
        }
      }
    }
  }, [codecs, handleError, onDisconnect, play, send])

  const onOpen = useCallback(() => {
    setWsState(WebSocket.OPEN)

    wsRef.current?.addEventListener('message', (ev) => {
      if (typeof ev.data === 'string') {
        const msg = JSON.parse(ev.data)
        for (const mode in onmessageRef.current) {
          onmessageRef.current[mode](msg)
        }
      } else {
        ondataRef.current?.(ev.data)
      }
    })

    ondataRef.current = null
    onmessageRef.current = {}

    onMse()
  }, [onMse])

  const onClose = useCallback(() => {
    if (wsState === WebSocket.CLOSED) return
    reconnect()
  }, [wsState, reconnect])

  const onConnect = useCallback(() => {
    if (!videoRef.current?.isConnected || !wsURL || wsRef.current) return

    setWsState(WebSocket.CONNECTING)
    setConnectTS(Date.now())

    wsRef.current = new WebSocket(wsURL)
    wsRef.current.binaryType = 'arraybuffer'
    wsRef.current.addEventListener('open', onOpen)
    wsRef.current.addEventListener('close', onClose)
  }, [wsURL, onOpen, onClose])

  // Keep the ref updated with the latest onConnect
  onConnectRef.current = onConnect

  const handlePause = useCallback(() => {
    if (isPlaying && playbackEnabled) {
      videoRef.current?.play()
    }
  }, [isPlaying, playbackEnabled])

  const handleLoadedData = useCallback(() => {
    setIsPlaying(true)
    onPlaying?.()
  }, [onPlaying])

  // Initialize connection
  useEffect(() => {
    if (!playbackEnabled) return

    console.log('[MSEPlayer] Initializing for', camera, 'wsURL:', wsURL)

    const MediaSourceConstructor = 'ManagedMediaSource' in window
      ? (window as unknown as { ManagedMediaSource: typeof MediaSource }).ManagedMediaSource
      : MediaSource

    msRef.current = new MediaSourceConstructor()
    onConnect()

    return () => {
      onDisconnect()
      if (reconnectTIDRef.current) {
        clearTimeout(reconnectTIDRef.current)
      }
    }
  }, [playbackEnabled, onConnect, onDisconnect, camera, wsURL])

  // Handle visibility changes
  useEffect(() => {
    if (!playbackEnabled) return

    const listener = () => {
      if (document.hidden) {
        onDisconnect()
      } else if (videoRef.current?.isConnected) {
        onConnect()
      }
    }

    document.addEventListener('visibilitychange', listener)
    return () => document.removeEventListener('visibilitychange', listener)
  }, [playbackEnabled, onConnect, onDisconnect])

  // Sync volume
  useEffect(() => {
    if (videoRef.current && volume !== undefined) {
      videoRef.current.volume = volume
    }
  }, [volume])

  return (
    <video
      ref={videoRef}
      className={className}
      playsInline
      preload="auto"
      muted={!audioEnabled}
      onLoadedData={handleLoadedData}
      onPause={handlePause}
      onError={(e) => {
        const target = e.target as HTMLVideoElement
        if (target.error?.code === MediaError.MEDIA_ERR_NETWORK) {
          onDisconnect()
          handleError('network', 'Browser reported a network error')
        }
        if (target.error?.code === MediaError.MEDIA_ERR_DECODE && (isSafari || isIOS)) {
          onDisconnect()
          handleError('decode', 'Browser reported decoding errors')
        }
      }}
    />
  )
}
