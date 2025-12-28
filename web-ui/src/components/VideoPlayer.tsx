import { useEffect, useRef, useState, memo } from 'react'
import { Camera, Loader2, AlertCircle, Volume2, VolumeX, Maximize, RefreshCw, Box, BoxSelect } from 'lucide-react'
import Hls from 'hls.js'
import DetectionOverlay from './DetectionOverlay'
import useDetections from '../hooks/useDetections'
import { getGo2RTCUrl, getGo2RTCWsUrl } from '../hooks/usePorts'

interface VideoPlayerProps {
  cameraId: string
  autoPlay?: boolean
  muted?: boolean
  className?: string
  fit?: 'cover' | 'contain'  // cover = fill & crop, contain = fit & letterbox
  showDetectionToggle?: boolean  // Show the detection overlay toggle button
  initialDetectionOverlay?: boolean  // Initial state of detection overlay
  showRecordingIndicator?: boolean  // Show recording status indicator
  aspectRatio?: string  // CSS aspect-ratio like "16/9" or "3/4" for doorbells
  maxHeight?: string  // Max height constraint like "70vh" for tall videos
}

type StreamMode = 'mse' | 'mse-h264' | 'hls' | 'mjpeg'

// Detect iOS/Safari which doesn't support MSE well
function isIOSOrSafari(): boolean {
  const ua = navigator.userAgent
  const isIOS = /iPad|iPhone|iPod/.test(ua) || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1)
  const isSafari = /^((?!chrome|android).)*safari/i.test(ua)
  return isIOS || isSafari
}

// Check if browser supports H265/HEVC in MSE
function supportsH265(): boolean {
  const codecs = [
    'video/mp4; codecs="hev1.1.6.L93.90"',
    'video/mp4; codecs="hvc1.1.6.L93.90"',
    'video/mp4; codecs="hev1.1.6.L120.90"',
    'video/mp4; codecs="hevc"'
  ]
  return codecs.some(codec => MediaSource.isTypeSupported(codec))
}

// Get list of supported MSE codecs to send to go2rtc
// go2rtc uses this to determine what codec string to send back
function getSupportedCodecs(): string {
  const videoCodecs = [
    'avc1.640029',      // H.264 high 4.1
    'avc1.64002A',      // H.264 high 4.2
    'avc1.640033',      // H.264 high 5.1
    'hvc1.1.6.L153.B0', // H.265 main 5.1
  ]

  // Audio codecs need to be tested differently - with audio/mp4 or combined with video
  const audioCodecs = [
    'mp4a.40.2',        // AAC LC
    'mp4a.40.5',        // AAC HE (SBR)
    'flac',             // FLAC (for PCM audio)
    'opus',             // OPUS
  ]

  // Test video codecs with video/mp4
  const supportedVideo = videoCodecs.filter(codec =>
    MediaSource.isTypeSupported(`video/mp4; codecs="${codec}"`)
  )

  // Test audio codecs - try both audio/mp4 and combined with a video codec
  const supportedAudio = audioCodecs.filter(codec => {
    // Try audio-only container
    if (MediaSource.isTypeSupported(`audio/mp4; codecs="${codec}"`)) {
      return true
    }
    // Try combined with H.264 (most common)
    if (supportedVideo.length > 0 && MediaSource.isTypeSupported(`video/mp4; codecs="${supportedVideo[0]},${codec}"`)) {
      return true
    }
    return false
  })

  return [...supportedVideo, ...supportedAudio].join(',')
}

// Cache the working mode per camera to avoid repeated fallback cycles
const workingModes: Record<string, StreamMode> = {}

export const VideoPlayer = memo(function VideoPlayer({
  cameraId,
  autoPlay = true,
  muted = true,
  className = '',
  fit = 'contain',
  showDetectionToggle = true,
  initialDetectionOverlay = false,
  showRecordingIndicator = true,
  aspectRatio,
  maxHeight,
}: VideoPlayerProps) {
  const videoRef = useRef<HTMLVideoElement>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [isMuted, setIsMuted] = useState(muted)
  const [showDetections, setShowDetections] = useState(initialDetectionOverlay)
  const [isRecording, setIsRecording] = useState(false)
  const [hasAudioTrack, setHasAudioTrack] = useState(false)

  // Get detections from WebSocket
  const { detections, motionDetected } = useDetections({
    cameraId,
    enabled: showDetections,
  })

  // Check recording status
  useEffect(() => {
    if (!cameraId || !showRecordingIndicator) return

    const checkRecordingStatus = async () => {
      try {
        const response = await fetch(`/api/v1/recordings/status/${cameraId}`)
        if (response.ok) {
          const json = await response.json()
          // API returns { success: true, data: { state: "running", ... } }
          const status = json.data || json
          // Recording states: idle, starting, running, stopping, error
          // Show recording indicator when running or starting
          setIsRecording(status.state === 'running' || status.state === 'starting')
        }
      } catch {
        // Ignore errors - recording status is optional
      }
    }

    // Check immediately and then every 5 seconds
    checkRecordingStatus()
    const interval = setInterval(checkRecordingStatus, 5000)

    return () => clearInterval(interval)
  }, [cameraId, showRecordingIndicator])

  // Use cached working mode or start with appropriate mode for platform
  const getInitialMode = (): StreamMode => {
    if (workingModes[cameraId]) return workingModes[cameraId]
    // iOS/Safari: prefer HLS as MSE support is poor
    if (isIOSOrSafari()) {
      return 'hls'
    }
    // Most browsers don't support H265, so skip straight to H264 transcoding
    return supportsH265() ? 'mse' : 'mse-h264'
  }
  const [mode, setMode] = useState<StreamMode>(getInitialMode)

  // MSE streaming - tries native H265 first, then H264 transcoded
  useEffect(() => {
    if (!cameraId || (mode !== 'mse' && mode !== 'mse-h264')) return

    const video = videoRef.current
    if (!video) return

    setIsLoading(true)
    setError(null)
    const streamName = cameraId.toLowerCase().replace(/\s+/g, '_')
    let mounted = true

    // For mse-h264 mode, request transcoded stream
    // Always include audio track for sound playback
    // Force audio=opus to transcode camera audio which has excellent browser support
    // Many cameras use AAC at low sample rates (16kHz) which browsers don't support in MSE
    // Use dynamically discovered go2rtc port
    const wsBase = getGo2RTCWsUrl()
    const wsUrl = mode === 'mse-h264'
      ? `${wsBase}/api/ws?src=${streamName}&video=h264&audio=opus`
      : `${wsBase}/api/ws?src=${streamName}&audio=opus`

    console.log(`[VideoPlayer] Connecting MSE (${mode}):`, wsUrl)
    const ws = new WebSocket(wsUrl)
    ws.binaryType = 'arraybuffer'

    const mediaSource = new MediaSource()
    video.src = URL.createObjectURL(mediaSource)

    let sourceBuffer: SourceBuffer | null = null
    const queue: Uint8Array[] = []
    let gotData = false

    const processQueue = () => {
      if (!sourceBuffer || sourceBuffer.updating || queue.length === 0) return
      try {
        sourceBuffer.appendBuffer(queue.shift()!)
        if (!gotData) {
          gotData = true
          // Cache the working mode to avoid fallback cycle on remount
          workingModes[cameraId] = mode
          if (mounted) setIsLoading(false)
          video.play().catch(() => {})
        }
      } catch {
        // Buffer overflow - trim queue
        if (queue.length > 5) queue.splice(0, queue.length - 3)
      }
    }

    ws.onopen = () => {
      // Send supported codecs to go2rtc - it will respond with the appropriate codec string
      // This is how go2rtc negotiates which codecs to use for the stream
      const supportedCodecs = getSupportedCodecs()
      console.log('[VideoPlayer] Sending supported codecs:', supportedCodecs)
      ws.send(JSON.stringify({ type: 'mse', value: supportedCodecs }))
    }

    ws.onmessage = (event) => {
      if (typeof event.data === 'string') {
        const msg = JSON.parse(event.data)
        if (msg.type === 'mse') {
          const codec = msg.value
          console.log(`[VideoPlayer] MSE codec from server:`, codec)
          console.log(`[VideoPlayer] Codec supported:`, MediaSource.isTypeSupported(codec))
          const isH265 = codec.includes('hev1') || codec.includes('hvc1') || codec.includes('hevc')
          const hasAudio = codec.includes('mp4a') || codec.includes('opus') || codec.includes('flac')
          console.log(`[VideoPlayer] Is H265: ${isH265}, Has audio: ${hasAudio}, Browser supports H265: ${supportsH265()}`)

          // Track if stream has audio for UI feedback
          if (mounted && hasAudio) {
            setHasAudioTrack(true)
          }

          const setup = () => {
            if (!MediaSource.isTypeSupported(codec)) {
              console.warn('[VideoPlayer] Codec not supported:', codec)

              // If we're in native mode and H265 isn't supported, try H264 transcoding
              if (mode === 'mse' && isH265) {
                console.log('[VideoPlayer] H265 not supported, requesting H264 transcode')
                if (mounted) setMode('mse-h264')
                return
              }

              // If H264 transcode also fails, fall back to MJPEG
              console.log('[VideoPlayer] Falling back to MJPEG')
              if (mounted) setMode('mjpeg')
              return
            }

            try {
              sourceBuffer = mediaSource.addSourceBuffer(codec)
              sourceBuffer.mode = 'segments'
              sourceBuffer.addEventListener('updateend', processQueue)
              console.log('[VideoPlayer] SourceBuffer created successfully')
            } catch (e) {
              console.error('[VideoPlayer] SourceBuffer error:', e)
              if (mode === 'mse' && isH265) {
                if (mounted) setMode('mse-h264')
              } else {
                if (mounted) setMode('mjpeg')
              }
            }
          }

          if (mediaSource.readyState === 'open') setup()
          else mediaSource.addEventListener('sourceopen', setup, { once: true })
        }
      } else {
        queue.push(new Uint8Array(event.data))
        processQueue()
      }
    }

    ws.onerror = (e) => {
      console.error('[VideoPlayer] WebSocket error:', e)
      if (mode === 'mse') {
        if (mounted) setMode('mse-h264')
      } else {
        if (mounted) setMode('mjpeg')
      }
    }

    // Timeout for stream startup (longer for H264 transcoding)
    const timeoutMs = mode === 'mse-h264' ? 15000 : 8000
    const timeout = setTimeout(() => {
      if (mounted && !gotData) {
        console.log(`[VideoPlayer] ${mode} timeout after ${timeoutMs}ms`)
        if (mode === 'mse') {
          setMode('mse-h264')
        } else {
          setMode('mjpeg')
        }
      }
    }, timeoutMs)

    return () => {
      mounted = false
      clearTimeout(timeout)
      ws.close()
      URL.revokeObjectURL(video.src)
    }
  }, [cameraId, mode])

  // HLS streaming - for iOS/Safari or as fallback
  useEffect(() => {
    if (mode !== 'hls' || !cameraId) return

    const video = videoRef.current
    if (!video) return

    setIsLoading(true)
    setError(null)
    const streamName = cameraId.toLowerCase().replace(/\s+/g, '_')
    let mounted = true
    let hls: Hls | null = null

    // go2rtc HLS endpoint
    const hlsUrl = `${getGo2RTCUrl()}/api/stream.m3u8?src=${streamName}`
    console.log('[VideoPlayer] Connecting HLS:', hlsUrl)

    // Check if native HLS is supported (Safari/iOS)
    if (video.canPlayType('application/vnd.apple.mpegurl')) {
      console.log('[VideoPlayer] Using native HLS')
      video.src = hlsUrl
      video.addEventListener('loadedmetadata', () => {
        if (mounted) {
          setIsLoading(false)
          workingModes[cameraId] = 'hls'
          setHasAudioTrack(true) // Assume audio available
        }
        video.play().catch(() => {})
      })
      video.addEventListener('error', () => {
        console.error('[VideoPlayer] Native HLS error')
        if (mounted) setMode('mjpeg')
      })
    } else if (Hls.isSupported()) {
      console.log('[VideoPlayer] Using HLS.js')
      hls = new Hls({
        enableWorker: true,
        lowLatencyMode: true,
        backBufferLength: 30,
      })
      hls.loadSource(hlsUrl)
      hls.attachMedia(video)
      hls.on(Hls.Events.MANIFEST_PARSED, () => {
        if (mounted) {
          setIsLoading(false)
          workingModes[cameraId] = 'hls'
          setHasAudioTrack(true)
        }
        video.play().catch(() => {})
      })
      hls.on(Hls.Events.ERROR, (_, data) => {
        if (data.fatal) {
          console.error('[VideoPlayer] HLS.js fatal error:', data.type)
          if (mounted) setMode('mjpeg')
        }
      })
    } else {
      console.log('[VideoPlayer] HLS not supported, falling back to MJPEG')
      if (mounted) setMode('mjpeg')
      return
    }

    // Timeout for HLS
    const timeout = setTimeout(() => {
      if (mounted && video.readyState < 2) {
        console.log('[VideoPlayer] HLS timeout')
        setMode('mjpeg')
      }
    }, 15000)

    return () => {
      mounted = false
      clearTimeout(timeout)
      if (hls) {
        hls.destroy()
      }
      video.src = ''
    }
  }, [cameraId, mode])

  // MJPEG fallback - use snapshot frames with refresh
  useEffect(() => {
    if (mode !== 'mjpeg' || !cameraId) return

    const video = videoRef.current
    if (!video) return

    const streamName = cameraId.toLowerCase().replace(/\s+/g, '_')
    const container = video.parentElement
    if (!container) return

    video.style.display = 'none'

    let img = container.querySelector('img.mjpeg-stream') as HTMLImageElement
    if (!img) {
      img = document.createElement('img')
      img.className = `mjpeg-stream w-full ${fit === 'cover' ? 'h-full object-cover' : 'h-auto'}`
      container.appendChild(img)
    }

    // Use frame.jpeg with periodic refresh instead of stream.mjpeg
    // This works even when go2rtc can't transcode to MJPEG
    let frameCount = 0
    const refreshFrame = () => {
      if (img) {
        img.src = `${getGo2RTCUrl()}/api/frame.jpeg?src=${streamName}&ts=${Date.now()}`
      }
    }

    img.onload = () => {
      setIsLoading(false)
      frameCount++
    }
    img.onerror = () => {
      if (frameCount === 0) {
        setError('Stream failed')
      }
    }

    // Start refreshing frames (roughly 2 fps for fallback)
    refreshFrame()
    const interval = setInterval(refreshFrame, 500)

    return () => {
      clearInterval(interval)
      if (img?.parentElement) {
        img.src = ''
        img.remove()
      }
      if (video) video.style.display = ''
    }
  }, [mode, cameraId, fit])

  const toggleMute = async () => {
    const video = videoRef.current
    if (!video) return

    const newMuted = !isMuted

    try {
      // Set the muted state on the video element
      video.muted = newMuted
      setIsMuted(newMuted)

      // When unmuting, ensure video is playing (user gesture requirement)
      if (!newMuted) {
        // Set volume to max when unmuting
        video.volume = 1.0

        // If video is paused, try to play it
        if (video.paused) {
          await video.play()
        }

        // Debug: Check audio state
        console.log('[VideoPlayer] Audio unmuted')
        console.log('[VideoPlayer] - video.volume:', video.volume)
        console.log('[VideoPlayer] - video.muted:', video.muted)
        console.log('[VideoPlayer] - video.paused:', video.paused)
        console.log('[VideoPlayer] - video.src type:', video.src.startsWith('blob:') ? 'MSE blob' : 'direct')
      } else {
        console.log('[VideoPlayer] Audio muted')
      }
    } catch (e) {
      console.warn('[VideoPlayer] Could not toggle audio:', e)
    }
  }

  const toggleFullscreen = () => {
    const container = videoRef.current?.parentElement
    if (container) {
      if (document.fullscreenElement) {
        document.exitFullscreen()
      } else {
        container.requestFullscreen()
      }
    }
  }

  const retry = () => {
    setError(null)
    setMode('mse')
    setIsLoading(true)
  }

  const toggleDetections = () => {
    setShowDetections(!showDetections)
  }

  // Build inline styles for aspect ratio and max height
  const containerStyle: React.CSSProperties = {}
  if (aspectRatio) {
    containerStyle.aspectRatio = aspectRatio
  }
  if (maxHeight) {
    containerStyle.maxHeight = maxHeight
  }

  return (
    <div
      className={`relative bg-black rounded-lg overflow-hidden ${className}`}
      style={containerStyle}
    >
      <video
        ref={videoRef}
        autoPlay={autoPlay}
        muted={isMuted}
        playsInline
        preload="auto"
        className={`w-full h-full ${fit === 'cover' ? 'object-cover' : 'object-contain'}`}
      />

      {/* Detection bounding box overlay */}
      <DetectionOverlay
        detections={detections}
        visible={showDetections}
        showLabels={true}
        showConfidence={true}
        minConfidence={0.3}
      />

      {/* Recording indicator - blinking red dot in upper right */}
      {showRecordingIndicator && isRecording && (
        <div className="absolute top-2 right-2 flex items-center gap-1.5" title="Recording">
          <span className="relative flex h-3 w-3">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-red-400 opacity-75"></span>
            <span className="relative inline-flex rounded-full h-3 w-3 bg-red-500"></span>
          </span>
        </div>
      )}

      {/* Motion indicator */}
      {showDetections && motionDetected && (
        <div className={`absolute ${isRecording ? 'top-8' : 'top-2'} right-2 flex items-center gap-1.5 px-2 py-1 bg-red-500/80 rounded text-white text-xs font-medium`}>
          <span className="w-2 h-2 bg-white rounded-full animate-pulse" />
          Motion
        </div>
      )}

      {/* Loading overlay */}
      {isLoading && !error && (
        <div className="absolute inset-0 flex flex-col items-center justify-center bg-black/50">
          <Loader2 className="h-8 w-8 animate-spin text-white mb-2" />
          <p className="text-white/70 text-xs">
            {mode === 'mse' ? 'Connecting (H265)...' : mode === 'mse-h264' ? 'Transcoding to H264...' : mode === 'hls' ? 'Connecting (HLS)...' : 'Loading...'}
          </p>
        </div>
      )}

      {/* Error overlay */}
      {error && (
        <div className="absolute inset-0 flex flex-col items-center justify-center bg-black/80">
          <AlertCircle className="h-10 w-10 text-red-500 mb-2" />
          <p className="text-white text-sm mb-1">{error}</p>
          <button
            onClick={retry}
            className="flex items-center gap-2 px-4 py-2 text-sm bg-white/10 hover:bg-white/20 rounded transition-colors mt-2"
          >
            <RefreshCw size={14} />
            Retry
          </button>
        </div>
      )}

      {/* No stream placeholder */}
      {!cameraId && (
        <div className="absolute inset-0 flex items-center justify-center">
          <Camera className="h-12 w-12 text-gray-600" />
        </div>
      )}

      {/* Controls overlay */}
      {!isLoading && !error && cameraId && (
        <div className="absolute bottom-0 left-0 right-0 p-2 bg-gradient-to-t from-black/60 to-transparent opacity-0 hover:opacity-100 transition-opacity">
          <div className="flex items-center justify-between">
            <span className="text-white/50 text-xs uppercase">
              {mode === 'mse' ? 'H265' : mode === 'mse-h264' ? 'H264' : mode === 'hls' ? 'HLS' : 'MJPEG'}
            </span>
            <div className="flex items-center gap-2">
              {showDetectionToggle && (
                <button
                  onClick={toggleDetections}
                  className={`p-1.5 rounded transition-colors ${
                    showDetections
                      ? 'bg-blue-500/80 hover:bg-blue-500'
                      : 'hover:bg-white/20'
                  }`}
                  title={showDetections ? 'Hide detections' : 'Show detections'}
                >
                  {showDetections ? (
                    <BoxSelect className="h-4 w-4 text-white" />
                  ) : (
                    <Box className="h-4 w-4 text-white" />
                  )}
                </button>
              )}
              <button
                onClick={toggleMute}
                className={`p-1.5 rounded transition-colors ${hasAudioTrack ? 'hover:bg-white/20' : 'opacity-50 cursor-not-allowed'}`}
                disabled={!hasAudioTrack}
                title={hasAudioTrack ? (isMuted ? 'Unmute' : 'Mute') : 'No audio track'}
              >
                {isMuted ? (
                  <VolumeX className="h-4 w-4 text-white" />
                ) : (
                  <Volume2 className="h-4 w-4 text-white" />
                )}
              </button>
              <button
                onClick={toggleFullscreen}
                className="p-1.5 rounded hover:bg-white/20 transition-colors"
              >
                <Maximize className="h-4 w-4 text-white" />
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
})
