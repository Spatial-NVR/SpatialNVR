/**
 * LivePlayer - Wrapper component that manages MSE/WebRTC players
 * Based on Frigate's LivePlayer implementation
 *
 * Key design decisions (from Frigate):
 * - Audio state is managed here and passed down to players
 * - Players only receive `audioEnabled` and `volume` props
 * - No complex state syncing - video element is source of truth
 * - Automatic fallback from WebRTC to MSE on error
 */
import { useState, useCallback, useEffect, memo } from 'react'
import { Loader2, AlertCircle, Volume2, VolumeX, Maximize, RefreshCw, Box, BoxSelect } from 'lucide-react'
import MSEPlayer from './MSEPlayer'
import WebRTCPlayer from './WebRTCPlayer'
import DetectionOverlay from '../DetectionOverlay'
import useDetections from '../../hooks/useDetections'

// Check if browser supports MediaSource
const supportsMediaSource = (): boolean => {
  return 'MediaSource' in window || 'ManagedMediaSource' in window
}

type LiveMode = 'webrtc' | 'mse'

interface LivePlayerProps {
  cameraId: string
  className?: string
  fit?: 'cover' | 'contain'
  showDetectionToggle?: boolean
  initialDetectionOverlay?: boolean
  showRecordingIndicator?: boolean
  aspectRatio?: string
  maxHeight?: string
  // External audio control - if provided, syncs with external state
  audioEnabled?: boolean
  onAudioChange?: (enabled: boolean) => void
}

function LivePlayer({
  cameraId,
  className = '',
  fit = 'contain',
  showDetectionToggle = true,
  initialDetectionOverlay = false,
  showRecordingIndicator = true,
  aspectRatio,
  maxHeight,
  audioEnabled: externalAudioEnabled,
  onAudioChange,
}: LivePlayerProps) {
  // Determine initial mode based on platform
  const getInitialMode = (): LiveMode => {
    // WebRTC is preferred for better audio support
    // Falls back to MSE if WebRTC fails
    return 'webrtc'
  }

  const [mode, setMode] = useState<LiveMode>(getInitialMode)
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Audio state - use external control if provided, otherwise internal state
  const [internalAudioEnabled, setInternalAudioEnabled] = useState(false)
  const audioEnabled = externalAudioEnabled !== undefined ? externalAudioEnabled : internalAudioEnabled
  const setAudioEnabled = onAudioChange || setInternalAudioEnabled
  const volume = 1 // Fixed volume for now, could be made stateful later

  // Other UI state
  const [showDetections, setShowDetections] = useState(initialDetectionOverlay)
  const [isRecording, setIsRecording] = useState(false)

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
          const status = json.data || json
          setIsRecording(status?.state === 'running' || status?.state === 'starting')
        }
      } catch {
        // Ignore errors
      }
    }

    checkRecordingStatus()
    const interval = setInterval(checkRecordingStatus, 5000)
    return () => clearInterval(interval)
  }, [cameraId, showRecordingIndicator])

  // Handle player ready
  const handlePlaying = useCallback(() => {
    console.log('[LivePlayer] Playing in mode:', mode)
    setIsLoading(false)
    setError(null)
  }, [mode])

  // Handle player error - fallback to next mode
  const handleError = useCallback((playerError: string) => {
    console.log(`[LivePlayer] ${mode} error:`, playerError)

    if (mode === 'webrtc') {
      // Try MSE as fallback
      if (supportsMediaSource()) {
        console.log('[LivePlayer] Falling back to MSE')
        setMode('mse')
        setIsLoading(true)
      } else {
        setError('Streaming not supported on this browser')
        setIsLoading(false)
      }
    } else {
      // MSE failed, show error
      setError(`Playback failed: ${playerError}`)
      setIsLoading(false)
    }
  }, [mode])

  // Toggle audio - simple boolean toggle
  const toggleAudio = useCallback(() => {
    setAudioEnabled(!audioEnabled)
  }, [audioEnabled, setAudioEnabled])

  // Toggle fullscreen
  const toggleFullscreen = useCallback(() => {
    const container = document.querySelector(`[data-camera-id="${cameraId}"]`)
    if (container) {
      if (document.fullscreenElement) {
        document.exitFullscreen()
      } else {
        container.requestFullscreen()
      }
    }
  }, [cameraId])

  // Retry connection
  const retry = useCallback(() => {
    setError(null)
    setIsLoading(true)
    setMode('webrtc')
  }, [])

  // Toggle detections
  const toggleDetections = useCallback(() => {
    setShowDetections((prev) => !prev)
  }, [])

  // Build container styles
  const containerStyle: React.CSSProperties = {}
  if (aspectRatio) {
    containerStyle.aspectRatio = aspectRatio
  }
  if (maxHeight) {
    containerStyle.maxHeight = maxHeight
  }

  // Player class name
  const playerClassName = `w-full h-full ${fit === 'cover' ? 'object-cover' : 'object-contain'}`

  return (
    <div
      className={`relative bg-black rounded-lg overflow-hidden ${className}`}
      style={containerStyle}
      data-camera-id={cameraId}
    >
      {/* Player - render based on current mode */}
      {mode === 'webrtc' ? (
        <WebRTCPlayer
          camera={cameraId}
          className={playerClassName}
          playbackEnabled={true}
          audioEnabled={audioEnabled}
          volume={volume}
          onPlaying={handlePlaying}
          onError={handleError}
        />
      ) : (
        <MSEPlayer
          camera={cameraId}
          className={playerClassName}
          playbackEnabled={true}
          audioEnabled={audioEnabled}
          volume={volume}
          onPlaying={handlePlaying}
          onError={handleError}
        />
      )}

      {/* Detection bounding box overlay */}
      <DetectionOverlay
        detections={detections}
        visible={showDetections}
        showLabels={true}
        showConfidence={true}
        minConfidence={0.3}
      />

      {/* Recording indicator */}
      {showRecordingIndicator && isRecording && (
        <div className="absolute top-2 right-2 flex items-center gap-1.5" title="Recording">
          <span className="relative flex h-3 w-3">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-red-400 opacity-75" />
            <span className="relative inline-flex rounded-full h-3 w-3 bg-red-500" />
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
            {mode === 'webrtc' ? 'Connecting (WebRTC)...' : 'Connecting (MSE)...'}
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

      {/* Controls overlay */}
      {!isLoading && !error && cameraId && (
        <div className="absolute bottom-0 left-0 right-0 p-2 bg-gradient-to-t from-black/60 to-transparent opacity-100 md:opacity-0 md:hover:opacity-100 transition-opacity">
          <div className="flex items-center justify-between">
            <span className="text-white/50 text-xs uppercase">
              {mode === 'webrtc' ? 'WebRTC' : 'MSE'}
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
                onClick={toggleAudio}
                className="p-1.5 rounded hover:bg-white/20 transition-colors"
                title={audioEnabled ? 'Mute' : 'Unmute'}
              >
                {audioEnabled ? (
                  <Volume2 className="h-4 w-4 text-white" />
                ) : (
                  <VolumeX className="h-4 w-4 text-white" />
                )}
              </button>
              <button
                onClick={toggleFullscreen}
                className="p-1.5 rounded hover:bg-white/20 transition-colors"
                title="Fullscreen"
              >
                <Maximize className="h-4 w-4 text-white" />
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

export default memo(LivePlayer)
