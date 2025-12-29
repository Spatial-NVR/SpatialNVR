/**
 * VideoPlayer - Main video player component
 *
 * This is now a thin wrapper around LivePlayer, which handles
 * all the streaming logic (MSE, WebRTC) with proper audio support.
 *
 * The implementation is based on Frigate's player architecture where:
 * - Audio state is simple: audioEnabled boolean + volume number
 * - Video element muted prop is the single source of truth
 * - No complex state syncing between React and video element
 */
import { memo } from 'react'
import LivePlayer from './player/LivePlayer'

export interface VideoPlayerProps {
  cameraId: string
  autoPlay?: boolean
  muted?: boolean
  className?: string
  fit?: 'cover' | 'contain'
  showDetectionToggle?: boolean
  initialDetectionOverlay?: boolean
  showRecordingIndicator?: boolean
  aspectRatio?: string
  maxHeight?: string
  // External audio control
  audioEnabled?: boolean
  onAudioChange?: (enabled: boolean) => void
}

export const VideoPlayer = memo(function VideoPlayer({
  cameraId,
  className = '',
  fit = 'contain',
  showDetectionToggle = true,
  initialDetectionOverlay = false,
  showRecordingIndicator = true,
  aspectRatio,
  maxHeight,
  audioEnabled,
  onAudioChange,
}: VideoPlayerProps) {
  return (
    <LivePlayer
      cameraId={cameraId}
      className={className}
      fit={fit}
      showDetectionToggle={showDetectionToggle}
      initialDetectionOverlay={initialDetectionOverlay}
      showRecordingIndicator={showRecordingIndicator}
      aspectRatio={aspectRatio}
      maxHeight={maxHeight}
      audioEnabled={audioEnabled}
      onAudioChange={onAudioChange}
    />
  )
})
