import { useState, useRef, useEffect, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Calendar,
  Clock,
  Camera,
  Play,
  Pause,
  SkipBack,
  SkipForward,
  ChevronLeft,
  ChevronRight,
  Maximize,
  Volume2,
  VolumeX,
  Download,
  AlertCircle,
  User,
  Car,
  Dog,
  Package,
  Eye,
  Radio,
  List,
  GripVertical
} from 'lucide-react'
import { cameraApi, eventApi, type Camera as CameraType, type Event } from '../lib/api'
import { usePorts } from '../hooks/usePorts'
import { VideoPlayer } from '../components/VideoPlayer'

interface TimelineSegment {
  start_time: string
  end_time: string
  type: 'recording' | 'gap'
  has_events: boolean
  event_count: number
  segment_ids?: string[]
}

interface TimelineData {
  camera_id: string
  start_time: string
  end_time: string
  segments: TimelineSegment[]
  total_size: number
  total_hours: number
}

// Playback speed options
const PLAYBACK_SPEEDS = [0.25, 0.5, 1, 1.5, 2, 4]

// Event type icons and colors
const EVENT_ICONS: Record<string, { icon: React.ElementType; color: string; label: string }> = {
  person: { icon: User, color: '#3b82f6', label: 'Person' },
  vehicle: { icon: Car, color: '#f59e0b', label: 'Vehicle' },
  car: { icon: Car, color: '#f59e0b', label: 'Car' },
  animal: { icon: Dog, color: '#10b981', label: 'Animal' },
  package: { icon: Package, color: '#8b5cf6', label: 'Package' },
  motion: { icon: Eye, color: '#6b7280', label: 'Motion' },
  default: { icon: AlertCircle, color: '#ef4444', label: 'Event' }
}

// Get event icon info
function getEventInfo(eventType: string) {
  return EVENT_ICONS[eventType.toLowerCase()] || EVENT_ICONS.default
}

// Camera thumbnail component with snapshot
function CameraThumbnail({
  camera,
  isSelected,
  onClick,
  apiUrl,
}: {
  camera: CameraType
  isSelected: boolean
  onClick: () => void
  apiUrl: string
}) {
  const [snapshotUrl, setSnapshotUrl] = useState<string | null>(null)
  const [hasError, setHasError] = useState(false)

  useEffect(() => {
    // Fetch snapshot on mount and periodically refresh
    const fetchSnapshot = async () => {
      try {
        const response = await fetch(`${apiUrl}/api/v1/cameras/${camera.id}/snapshot`)
        if (response.ok) {
          const blob = await response.blob()
          const url = URL.createObjectURL(blob)
          setSnapshotUrl((prev) => {
            if (prev) URL.revokeObjectURL(prev)
            return url
          })
          setHasError(false)
        } else {
          setHasError(true)
        }
      } catch {
        setHasError(true)
      }
    }

    // Only fetch if camera is online
    if (camera.status === 'online') {
      fetchSnapshot()
      // Refresh every 30 seconds
      const interval = setInterval(fetchSnapshot, 30000)
      return () => {
        clearInterval(interval)
        if (snapshotUrl) URL.revokeObjectURL(snapshotUrl)
      }
    }
  }, [camera.id, camera.status, apiUrl])

  return (
    <button
      onClick={onClick}
      className={`shrink-0 relative rounded-lg overflow-hidden border-2 transition-colors ${
        isSelected ? 'border-primary' : 'border-transparent hover:border-primary/50'
      }`}
      style={{ width: '120px', height: '68px' }}
    >
      {/* Camera thumbnail - snapshot or placeholder */}
      {snapshotUrl && !hasError ? (
        <img
          src={snapshotUrl}
          alt={camera.name}
          className="w-full h-full object-cover"
          onError={() => setHasError(true)}
        />
      ) : (
        <div className="w-full h-full bg-muted flex items-center justify-center">
          <Camera size={24} className="text-muted-foreground" />
        </div>
      )}

      {/* Camera name overlay */}
      <div className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/80 to-transparent px-2 py-1">
        <span className="text-white text-xs truncate block">{camera.name}</span>
      </div>

      {/* Status indicator */}
      <div
        className={`absolute top-1 right-1 w-2 h-2 rounded-full ${
          camera.status === 'online'
            ? 'bg-green-500'
            : camera.status === 'error'
            ? 'bg-red-500'
            : 'bg-gray-500'
        }`}
      />
    </button>
  )
}

export function Recordings() {
  const { apiUrl } = usePorts()
  const [selectedCamera, setSelectedCamera] = useState<string>('')
  const [selectedDate, setSelectedDate] = useState<string>(
    new Date().toISOString().split('T')[0]
  )
  const [viewMode, setViewMode] = useState<'live' | 'playback'>('live')
  const [currentTime, setCurrentTime] = useState<Date | null>(null)
  const [isPlaying, setIsPlaying] = useState(false)
  const [isMuted, setIsMuted] = useState(true)
  const [videoStartTime, setVideoStartTime] = useState<Date | null>(null)
  const [playbackSpeed, setPlaybackSpeed] = useState(1)
  const [rightPanelTab, setRightPanelTab] = useState<'events' | 'timeline'>('timeline')
  const [isDragging, setIsDragging] = useState(false)
  const [dragTime, setDragTime] = useState<Date | null>(null)

  const videoRef = useRef<HTMLVideoElement>(null)
  const timelineRef = useRef<HTMLDivElement>(null)
  const verticalTimelineRef = useRef<HTMLDivElement>(null)

  const { data: cameras } = useQuery({
    queryKey: ['cameras'],
    queryFn: cameraApi.list,
  })

  // Auto-select first camera when cameras load and none selected
  useEffect(() => {
    if (cameras && cameras.length > 0 && !selectedCamera) {
      setSelectedCamera(cameras[0].id)
    }
  }, [cameras, selectedCamera])

  // Fetch timeline data for selected date
  const { data: timeline, isLoading: timelineLoading } = useQuery({
    queryKey: ['timeline', selectedCamera, selectedDate],
    queryFn: async (): Promise<TimelineData | null> => {
      if (!selectedCamera) return null
      const start = new Date(selectedDate)
      start.setHours(0, 0, 0, 0)
      const end = new Date(selectedDate)
      end.setHours(23, 59, 59, 999)

      const res = await fetch(
        `${apiUrl}/api/v1/recordings/timeline/${selectedCamera}?start=${start.toISOString()}&end=${end.toISOString()}`
      )
      const data = await res.json()
      // Backend returns Timeline directly, not wrapped in data.data
      // Ensure segments is always an array
      return {
        ...data,
        segments: data.segments || []
      }
    },
    enabled: !!selectedCamera,
  })

  // Fetch events for selected date
  const { data: events } = useQuery({
    queryKey: ['events', selectedCamera, selectedDate],
    queryFn: async (): Promise<Event[]> => {
      if (!selectedCamera) return []
      const start = new Date(selectedDate)
      start.setHours(0, 0, 0, 0)
      const end = new Date(selectedDate)
      end.setHours(23, 59, 59, 999)

      const result = await eventApi.list({
        camera_id: selectedCamera,
        per_page: 100
      })

      // Filter events to selected date
      return (result.data || []).filter(e => {
        const eventTime = new Date(e.timestamp * 1000)
        return eventTime >= start && eventTime <= end
      })
    },
    enabled: !!selectedCamera,
  })

  // Format time for display
  const formatTime = (date: Date) => {
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  }

  const formatTimeShort = (date: Date) => {
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  }

  // Get stream URL for a timestamp
  const getStreamUrl = useCallback((timestamp: Date) => {
    return `${apiUrl}/api/v1/recordings/timeline/${selectedCamera}/stream?t=${timestamp.toISOString()}`
  }, [apiUrl, selectedCamera])

  // Seek to a specific time
  const seekToTime = useCallback((time: Date, autoPlay = false) => {
    setViewMode('playback')
    setCurrentTime(time)
    setVideoStartTime(time)
    if (videoRef.current) {
      const url = getStreamUrl(time)
      videoRef.current.src = url
      videoRef.current.load()
      if (autoPlay || isPlaying) {
        setIsPlaying(true)
        videoRef.current.play().catch(() => {})
      }
    }
  }, [getStreamUrl, isPlaying])

  // Preview time during scrubbing (updates video without loading new source every time)
  const previewTime = useCallback((time: Date) => {
    setViewMode('playback')
    setDragTime(time)
    setCurrentTime(time)
    // Load video at this position for preview
    if (videoRef.current) {
      const url = getStreamUrl(time)
      if (videoRef.current.src !== url) {
        videoRef.current.src = url
        videoRef.current.load()
      }
    }
  }, [getStreamUrl])

  // Handle vertical timeline drag
  const handleTimelineDrag = useCallback((clientY: number) => {
    if (!verticalTimelineRef.current) return null

    const rect = verticalTimelineRef.current.getBoundingClientRect()
    const scrollTop = verticalTimelineRef.current.scrollTop
    const relativeY = clientY - rect.top + scrollTop

    // Each hour is 60px tall, timeline starts at midnight
    const hourHeight = 60
    const totalMinutes = (relativeY / hourHeight) * 60
    const hours = Math.floor(totalMinutes / 60)
    const minutes = Math.floor(totalMinutes % 60)

    const time = new Date(selectedDate)
    time.setHours(Math.min(23, Math.max(0, hours)), Math.min(59, Math.max(0, minutes)), 0, 0)

    return time
  }, [selectedDate])

  // Jump to event
  const jumpToEvent = useCallback((event: Event) => {
    const eventTime = new Date(event.timestamp * 1000)
    seekToTime(eventTime)
  }, [seekToTime])

  // Switch to live view
  const goLive = useCallback(() => {
    setViewMode('live')
    setCurrentTime(null)
    setVideoStartTime(null)
    setIsPlaying(false)
  }, [])

  // Track video time and update currentTime
  useEffect(() => {
    if (!videoRef.current || !videoStartTime || viewMode !== 'playback') return

    const video = videoRef.current
    const handleTimeUpdate = () => {
      const offset = video.currentTime * 1000
      setCurrentTime(new Date(videoStartTime.getTime() + offset))
    }

    video.addEventListener('timeupdate', handleTimeUpdate)
    return () => video.removeEventListener('timeupdate', handleTimeUpdate)
  }, [videoStartTime, viewMode])

  // Play/pause video
  useEffect(() => {
    if (videoRef.current && viewMode === 'playback') {
      if (isPlaying) {
        videoRef.current.play().catch(() => {})
      } else {
        videoRef.current.pause()
      }
    }
  }, [isPlaying, viewMode])

  // Update video muted state
  useEffect(() => {
    if (videoRef.current) {
      videoRef.current.muted = isMuted
    }
  }, [isMuted])

  // Update playback speed
  useEffect(() => {
    if (videoRef.current) {
      videoRef.current.playbackRate = playbackSpeed
    }
  }, [playbackSpeed])

  // Navigate date
  const navigateDate = (direction: 'prev' | 'next') => {
    const current = new Date(selectedDate)
    current.setDate(current.getDate() + (direction === 'prev' ? -1 : 1))
    setSelectedDate(current.toISOString().split('T')[0])
    setCurrentTime(null)
    setIsPlaying(false)
    if (viewMode === 'playback') {
      goLive()
    }
  }

  // Check if there's any recording for the selected camera/date
  const hasRecordings = timeline?.segments?.some(s => s.type === 'recording')

  // Get the selected camera's data
  const selectedCameraData = cameras?.find(c => c.id === selectedCamera)

  return (
    <div className="flex flex-col h-[calc(100vh-4rem)]">
      {/* Header */}
      <div className="flex items-center justify-between p-4 border-b border-border shrink-0">
        <div className="flex items-center gap-4">
          <h1 className="text-xl font-bold">Recordings</h1>

          {/* View mode toggle */}
          <div className="flex items-center gap-1 bg-muted rounded-lg p-1">
            <button
              onClick={goLive}
              className={`px-3 py-1.5 text-sm font-medium rounded-md transition-colors flex items-center gap-2 ${
                viewMode === 'live'
                  ? 'bg-background text-foreground shadow-sm'
                  : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              <Radio size={14} className={viewMode === 'live' ? 'text-red-500' : ''} />
              Live
            </button>
            <button
              onClick={() => {
                if (events && events.length > 0) {
                  jumpToEvent(events[events.length - 1])
                } else if (timeline?.segments?.some(s => s.type === 'recording')) {
                  const lastRecording = [...(timeline.segments || [])].reverse().find(s => s.type === 'recording')
                  if (lastRecording) {
                    seekToTime(new Date(lastRecording.start_time))
                  }
                }
              }}
              className={`px-3 py-1.5 text-sm font-medium rounded-md transition-colors flex items-center gap-2 ${
                viewMode === 'playback'
                  ? 'bg-background text-foreground shadow-sm'
                  : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              <Play size={14} />
              Playback
            </button>
          </div>
        </div>

        {/* Date navigation */}
        <div className="flex items-center gap-1">
          <button
            onClick={() => navigateDate('prev')}
            className="p-1.5 hover:bg-accent rounded-md transition-colors"
          >
            <ChevronLeft size={18} />
          </button>
          <div className="flex items-center gap-2">
            <Calendar size={18} className="text-muted-foreground" />
            <input
              type="date"
              value={selectedDate}
              onChange={(e) => {
                setSelectedDate(e.target.value)
                setCurrentTime(null)
                setIsPlaying(false)
              }}
              className="bg-background border border-border rounded-md px-2 py-1 text-sm"
            />
          </div>
          <button
            onClick={() => navigateDate('next')}
            className="p-1.5 hover:bg-accent rounded-md transition-colors"
            disabled={selectedDate >= new Date().toISOString().split('T')[0]}
          >
            <ChevronRight size={18} />
          </button>
        </div>
      </div>

      {/* Main content - split layout */}
      <div className="flex-1 flex min-h-0">
        {/* Left side - Video player */}
        <div className="flex-1 flex flex-col min-w-0">
          {/* Video area */}
          <div className="flex-1 bg-black flex items-center justify-center min-h-0 p-2">
            {!selectedCamera ? (
              <div className="text-white/50 text-center">
                <Camera size={64} className="mx-auto mb-4 opacity-50" />
                <p>Select a camera to view</p>
              </div>
            ) : viewMode === 'live' ? (
              <VideoPlayer
                cameraId={selectedCamera}
                className="w-full h-full"
                muted={isMuted}
                showDetectionToggle={true}
                aspectRatio={selectedCameraData?.display_aspect_ratio?.replace(':', '/') || '16/9'}
              />
            ) : (
              <video
                ref={videoRef}
                className="max-w-full max-h-full"
                muted={isMuted}
                playsInline
                onEnded={() => {
                  if (currentTime) {
                    const nextTime = new Date(currentTime.getTime() + 1000)
                    seekToTime(nextTime)
                  }
                }}
                onError={() => {
                  setIsPlaying(false)
                }}
              />
            )}
          </div>

          {/* Playback controls - only show in playback mode */}
          {viewMode === 'playback' && (
            <div className="bg-card border-t border-border p-3 shrink-0">
              <div className="flex items-center justify-between">
                {/* Left controls */}
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => currentTime && seekToTime(new Date(currentTime.getTime() - 10000))}
                    className="p-2 hover:bg-accent rounded-md transition-colors"
                    title="Back 10s"
                    disabled={!currentTime}
                  >
                    <SkipBack size={18} />
                  </button>
                  <button
                    onClick={() => setIsPlaying(!isPlaying)}
                    className="p-2 bg-primary text-primary-foreground rounded-md hover:bg-primary/90 transition-colors"
                    disabled={!currentTime}
                  >
                    {isPlaying ? <Pause size={20} /> : <Play size={20} />}
                  </button>
                  <button
                    onClick={() => currentTime && seekToTime(new Date(currentTime.getTime() + 10000))}
                    className="p-2 hover:bg-accent rounded-md transition-colors"
                    title="Forward 10s"
                    disabled={!currentTime}
                  >
                    <SkipForward size={18} />
                  </button>

                  {/* Playback speed */}
                  <select
                    value={playbackSpeed}
                    onChange={(e) => setPlaybackSpeed(parseFloat(e.target.value))}
                    className="bg-background border border-border rounded px-2 py-1 text-xs ml-2"
                    title="Playback speed"
                  >
                    {PLAYBACK_SPEEDS.map(speed => (
                      <option key={speed} value={speed}>
                        {speed}x
                      </option>
                    ))}
                  </select>
                </div>

                {/* Center - current time */}
                <div className="text-sm font-mono">
                  {currentTime ? (
                    <>
                      <span className="text-muted-foreground mr-2">
                        {new Date(selectedDate).toLocaleDateString([], { month: 'short', day: 'numeric' })}
                      </span>
                      <span>{formatTime(currentTime)}</span>
                    </>
                  ) : (
                    <span className="text-muted-foreground">--:--:--</span>
                  )}
                </div>

                {/* Right controls */}
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => setIsMuted(!isMuted)}
                    className="p-2 hover:bg-accent rounded-md transition-colors"
                    title={isMuted ? 'Unmute' : 'Mute'}
                  >
                    {isMuted ? <VolumeX size={18} /> : <Volume2 size={18} />}
                  </button>
                  <button
                    onClick={() => {
                      if (videoRef.current) {
                        if (document.fullscreenElement) {
                          document.exitFullscreen()
                        } else {
                          videoRef.current.requestFullscreen()
                        }
                      }
                    }}
                    className="p-2 hover:bg-accent rounded-md transition-colors"
                    disabled={!currentTime}
                    title="Fullscreen"
                  >
                    <Maximize size={18} />
                  </button>
                  <button
                    onClick={goLive}
                    className="px-3 py-1.5 text-xs bg-red-500 text-white rounded-md hover:bg-red-600 transition-colors flex items-center gap-1"
                  >
                    <Radio size={12} />
                    Go Live
                  </button>
                </div>
              </div>

              {/* Timeline scrubber */}
              {timeline && (
                <div
                  ref={timelineRef}
                  className="mt-3 h-8 bg-muted/50 rounded cursor-pointer overflow-hidden relative border border-border"
                  onClick={(e) => {
                    if (!timelineRef.current) return
                    const rect = timelineRef.current.getBoundingClientRect()
                    const clickX = e.clientX - rect.left
                    const percent = clickX / rect.width

                    const dayStart = new Date(selectedDate)
                    dayStart.setHours(0, 0, 0, 0)
                    const dayEnd = new Date(selectedDate)
                    dayEnd.setHours(23, 59, 59, 999)

                    const duration = dayEnd.getTime() - dayStart.getTime()
                    const clickTime = new Date(dayStart.getTime() + percent * duration)
                    seekToTime(clickTime)
                  }}
                >
                  {/* Recording segments - bright blue bars */}
                  {timeline.segments?.map((segment, i) => {
                    if (segment.type !== 'recording') return null

                    const dayStart = new Date(selectedDate)
                    dayStart.setHours(0, 0, 0, 0)
                    const dayEnd = new Date(selectedDate)
                    dayEnd.setHours(23, 59, 59, 999)
                    const dayDuration = dayEnd.getTime() - dayStart.getTime()

                    const segStart = new Date(segment.start_time)
                    const segEnd = new Date(segment.end_time)

                    const left = ((segStart.getTime() - dayStart.getTime()) / dayDuration) * 100
                    // Ensure minimum width of 0.5% so short segments are visible
                    const width = Math.max(0.5, ((segEnd.getTime() - segStart.getTime()) / dayDuration) * 100)

                    return (
                      <div
                        key={i}
                        className="absolute top-0.5 bottom-0.5 bg-blue-500 rounded shadow-sm"
                        style={{ left: `${Math.max(0, left)}%`, width: `${Math.min(100 - left, width)}%` }}
                        title={`Recording: ${segStart.toLocaleTimeString()} - ${segEnd.toLocaleTimeString()}`}
                      />
                    )
                  })}

                  {/* Current position indicator */}
                  {currentTime && (
                    (() => {
                      const dayStart = new Date(selectedDate)
                      dayStart.setHours(0, 0, 0, 0)
                      const dayEnd = new Date(selectedDate)
                      dayEnd.setHours(23, 59, 59, 999)
                      const dayDuration = dayEnd.getTime() - dayStart.getTime()
                      const position = ((currentTime.getTime() - dayStart.getTime()) / dayDuration) * 100

                      return (
                        <div
                          className="absolute top-0 h-full w-0.5 bg-white z-10"
                          style={{ left: `${position}%` }}
                        />
                      )
                    })()
                  )}

                  {/* Hour markers */}
                  {Array.from({ length: 24 }).map((_, i) => (
                    <div
                      key={i}
                      className="absolute top-0 h-full border-l border-border/30"
                      style={{ left: `${(i / 24) * 100}%` }}
                    >
                      {i % 6 === 0 && (
                        <span className="absolute -top-4 text-[10px] text-muted-foreground -translate-x-1/2">
                          {i.toString().padStart(2, '0')}:00
                        </span>
                      )}
                    </div>
                  ))}

                  {/* No recordings indicator */}
                  {!timeline.segments?.some(s => s.type === 'recording') && (
                    <div className="absolute inset-0 flex items-center justify-center">
                      <span className="text-xs text-muted-foreground">No recordings for this day</span>
                    </div>
                  )}
                </div>
              )}
            </div>
          )}

          {/* Camera thumbnails - bottom bar */}
          <div className="bg-card border-t border-border p-2 shrink-0 overflow-x-auto">
            <div className="flex gap-2">
              {cameras?.map((cam: CameraType) => (
                <CameraThumbnail
                  key={cam.id}
                  camera={cam}
                  isSelected={selectedCamera === cam.id}
                  onClick={() => {
                    setSelectedCamera(cam.id)
                    if (viewMode === 'playback') {
                      setCurrentTime(null)
                      setIsPlaying(false)
                    }
                  }}
                  apiUrl={apiUrl}
                />
              ))}
            </div>
          </div>
        </div>

        {/* Right side - Events/Timeline panel with tabs */}
        <div className="w-80 border-l border-border flex flex-col shrink-0">
          {/* Tab headers */}
          <div className="flex border-b border-border shrink-0">
            <button
              onClick={() => setRightPanelTab('timeline')}
              className={`flex-1 px-4 py-3 text-sm font-medium transition-colors flex items-center justify-center gap-2 ${
                rightPanelTab === 'timeline'
                  ? 'text-foreground border-b-2 border-primary bg-muted/30'
                  : 'text-muted-foreground hover:text-foreground hover:bg-muted/20'
              }`}
            >
              <Clock size={16} />
              Timeline
            </button>
            <button
              onClick={() => setRightPanelTab('events')}
              className={`flex-1 px-4 py-3 text-sm font-medium transition-colors flex items-center justify-center gap-2 ${
                rightPanelTab === 'events'
                  ? 'text-foreground border-b-2 border-primary bg-muted/30'
                  : 'text-muted-foreground hover:text-foreground hover:bg-muted/20'
              }`}
            >
              <List size={16} />
              Events
              {events && events.length > 0 && (
                <span className="ml-1 px-1.5 py-0.5 text-xs bg-primary/20 text-primary rounded-full">
                  {events.length}
                </span>
              )}
            </button>
          </div>

          {/* Tab content */}
          <div className="flex-1 overflow-hidden">
            {rightPanelTab === 'timeline' ? (
              /* Timeline Tab */
              <div className="h-full flex flex-col">
                {!selectedCamera ? (
                  <div className="flex-1 flex items-center justify-center text-muted-foreground">
                    <div className="text-center">
                      <Camera size={32} className="mx-auto mb-2 opacity-50" />
                      <p className="text-sm">Select a camera</p>
                    </div>
                  </div>
                ) : timelineLoading ? (
                  <div className="flex-1 flex items-center justify-center">
                    <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-primary" />
                  </div>
                ) : (
                  /* Vertical scrollable timeline */
                  <div
                    ref={verticalTimelineRef}
                    className="flex-1 overflow-y-auto relative select-none"
                    style={{ cursor: isDragging ? 'grabbing' : 'grab' }}
                    onMouseDown={(e) => {
                      e.preventDefault()
                      setIsDragging(true)
                      setIsPlaying(false)
                      const time = handleTimelineDrag(e.clientY)
                      if (time) previewTime(time)
                    }}
                    onMouseMove={(e) => {
                      if (!isDragging) return
                      const time = handleTimelineDrag(e.clientY)
                      if (time) previewTime(time)
                    }}
                    onMouseUp={() => {
                      if (isDragging && dragTime) {
                        setIsDragging(false)
                        seekToTime(dragTime, true) // Auto-play on release
                        setDragTime(null)
                      }
                    }}
                    onMouseLeave={() => {
                      if (isDragging && dragTime) {
                        setIsDragging(false)
                        seekToTime(dragTime, true)
                        setDragTime(null)
                      }
                    }}
                    onTouchStart={(e) => {
                      setIsDragging(true)
                      setIsPlaying(false)
                      const time = handleTimelineDrag(e.touches[0].clientY)
                      if (time) previewTime(time)
                    }}
                    onTouchMove={(e) => {
                      if (!isDragging) return
                      const time = handleTimelineDrag(e.touches[0].clientY)
                      if (time) previewTime(time)
                    }}
                    onTouchEnd={() => {
                      if (isDragging && dragTime) {
                        setIsDragging(false)
                        seekToTime(dragTime, true)
                        setDragTime(null)
                      }
                    }}
                  >
                    {/* Timeline content - 24 hours */}
                    <div className="relative" style={{ height: '1440px' }}> {/* 24 hours * 60px */}
                      {/* Hour markers */}
                      {Array.from({ length: 24 }).map((_, hour) => (
                        <div
                          key={hour}
                          className="absolute left-0 right-0 border-t border-border/50"
                          style={{ top: `${hour * 60}px` }}
                        >
                          <span className="absolute left-2 -top-2.5 text-xs text-muted-foreground bg-background px-1">
                            {hour.toString().padStart(2, '0')}:00
                          </span>
                          {/* Half hour marker */}
                          <div
                            className="absolute left-8 right-0 border-t border-border/20"
                            style={{ top: '30px' }}
                          />
                        </div>
                      ))}

                      {/* Recording segments - bright blue bars */}
                      {timeline?.segments?.map((segment, i) => {
                        // Show all segments that are recordings (type === 'recording' or no type for raw segments)
                        if (segment.type && segment.type !== 'recording') return null

                        const segStart = new Date(segment.start_time)
                        const segEnd = new Date(segment.end_time)

                        const dayStart = new Date(selectedDate)
                        dayStart.setHours(0, 0, 0, 0)

                        const startMinutes = (segStart.getTime() - dayStart.getTime()) / 60000
                        const endMinutes = (segEnd.getTime() - dayStart.getTime()) / 60000
                        const top = startMinutes
                        const height = Math.max(4, endMinutes - startMinutes) // Minimum 4px height

                        return (
                          <div
                            key={i}
                            className="absolute left-10 right-2 bg-blue-500 rounded border-l-4 border-blue-600 shadow-sm"
                            style={{ top: `${top}px`, height: `${height}px` }}
                            title={`Recording: ${segStart.toLocaleTimeString()} - ${segEnd.toLocaleTimeString()}`}
                          />
                        )
                      })}

                      {/* Show "No recordings" message if no segments */}
                      {(!timeline?.segments || timeline.segments.length === 0) && !timelineLoading && (
                        <div className="absolute inset-0 flex items-center justify-center">
                          <div className="text-center text-muted-foreground bg-background/80 px-4 py-2 rounded">
                            <p className="text-sm">No recordings for this day</p>
                          </div>
                        </div>
                      )}

                      {/* Event markers on timeline */}
                      {events?.map((event) => {
                        const eventInfo = getEventInfo(event.event_type)
                        const Icon = eventInfo.icon
                        const eventTime = new Date(event.timestamp * 1000)

                        const dayStart = new Date(selectedDate)
                        dayStart.setHours(0, 0, 0, 0)

                        const minutes = (eventTime.getTime() - dayStart.getTime()) / 60000
                        if (minutes < 0 || minutes > 1440) return null

                        return (
                          <button
                            key={event.id}
                            className="absolute left-12 right-2 flex items-center gap-2 px-2 py-1 rounded hover:bg-accent/50 transition-colors group z-10"
                            style={{ top: `${minutes - 10}px` }}
                            onClick={(e) => {
                              e.stopPropagation()
                              jumpToEvent(event)
                            }}
                            onMouseDown={(e) => e.stopPropagation()}
                          >
                            <div
                              className="w-5 h-5 rounded flex items-center justify-center shrink-0"
                              style={{ backgroundColor: `${eventInfo.color}30` }}
                            >
                              <Icon size={12} style={{ color: eventInfo.color }} />
                            </div>
                            <span className="text-xs truncate opacity-70 group-hover:opacity-100">
                              {event.label || eventInfo.label}
                            </span>
                            <span className="text-[10px] text-muted-foreground ml-auto shrink-0">
                              {formatTimeShort(eventTime)}
                            </span>
                          </button>
                        )
                      })}

                      {/* Current time indicator */}
                      {currentTime && !isDragging && (
                        (() => {
                          const dayStart = new Date(selectedDate)
                          dayStart.setHours(0, 0, 0, 0)
                          const minutes = (currentTime.getTime() - dayStart.getTime()) / 60000

                          if (minutes < 0 || minutes > 1440) return null

                          return (
                            <div
                              className="absolute left-0 right-0 flex items-center pointer-events-none z-20"
                              style={{ top: `${minutes}px` }}
                            >
                              <div className="w-2 h-2 rounded-full bg-red-500 ml-1" />
                              <div className="flex-1 h-0.5 bg-red-500" />
                            </div>
                          )
                        })()
                      )}

                      {/* Drag indicator */}
                      {isDragging && dragTime && (
                        (() => {
                          const dayStart = new Date(selectedDate)
                          dayStart.setHours(0, 0, 0, 0)
                          const minutes = (dragTime.getTime() - dayStart.getTime()) / 60000

                          return (
                            <div
                              className="absolute left-0 right-0 flex items-center pointer-events-none z-30"
                              style={{ top: `${minutes}px` }}
                            >
                              <div className="w-3 h-3 rounded-full bg-primary ml-0.5 shadow-lg" />
                              <div className="flex-1 h-0.5 bg-primary" />
                              <div className="absolute left-6 -top-2.5 px-2 py-0.5 bg-primary text-primary-foreground text-xs rounded shadow-lg">
                                {formatTime(dragTime)}
                              </div>
                            </div>
                          )
                        })()
                      )}

                      {/* Scrub hint */}
                      <div className="absolute top-2 right-2 flex items-center gap-1 text-xs text-muted-foreground bg-background/80 px-2 py-1 rounded">
                        <GripVertical size={12} />
                        Drag to scrub
                      </div>
                    </div>
                  </div>
                )}
              </div>
            ) : (
              /* Events Tab */
              <div className="h-full overflow-y-auto">
                {!selectedCamera ? (
                  <div className="p-4 text-center text-muted-foreground">
                    <Camera size={32} className="mx-auto mb-2 opacity-50" />
                    <p className="text-sm">Select a camera</p>
                  </div>
                ) : timelineLoading ? (
                  <div className="flex items-center justify-center p-8">
                    <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-primary" />
                  </div>
                ) : !hasRecordings && (!events || events.length === 0) ? (
                  <div className="p-4 text-center text-muted-foreground">
                    <Clock size={32} className="mx-auto mb-2 opacity-50" />
                    <p className="text-sm">No recordings or events</p>
                    <p className="text-xs mt-1">for {new Date(selectedDate).toLocaleDateString()}</p>
                  </div>
                ) : events && events.length === 0 ? (
                  <div className="p-4 text-center text-muted-foreground">
                    <AlertCircle size={32} className="mx-auto mb-2 opacity-50" />
                    <p className="text-sm">No events detected</p>
                    <p className="text-xs mt-1">Recordings available for playback</p>
                  </div>
                ) : (
                  <div className="divide-y divide-border">
                    {events?.map((event) => {
                      const eventInfo = getEventInfo(event.event_type)
                      const Icon = eventInfo.icon
                      const eventTime = new Date(event.timestamp * 1000)

                      return (
                        <button
                          key={event.id}
                          onClick={() => jumpToEvent(event)}
                          className="w-full p-3 hover:bg-accent text-left transition-colors flex items-start gap-3"
                        >
                          <div
                            className="w-10 h-10 rounded-lg flex items-center justify-center shrink-0"
                            style={{ backgroundColor: `${eventInfo.color}20` }}
                          >
                            <Icon size={20} style={{ color: eventInfo.color }} />
                          </div>
                          <div className="flex-1 min-w-0">
                            <div className="flex items-center justify-between gap-2">
                              <span className="font-medium text-sm truncate">
                                {event.label || eventInfo.label}
                              </span>
                              <span className="text-xs text-muted-foreground shrink-0">
                                {formatTimeShort(eventTime)}
                              </span>
                            </div>
                            <div className="text-xs text-muted-foreground mt-0.5">
                              {event.confidence ? `${Math.round(event.confidence * 100)}% confidence` : event.event_type}
                            </div>
                          </div>
                        </button>
                      )
                    })}
                  </div>
                )}
              </div>
            )}
          </div>

          {/* Quick actions */}
          <div className="p-3 border-t border-border bg-muted/30 shrink-0">
            <div className="flex gap-2">
              <button
                className="flex-1 px-3 py-2 text-xs bg-background border border-border rounded-md hover:bg-accent transition-colors flex items-center justify-center gap-2"
                onClick={() => {
                  // Export last 30 min
                  if (selectedCamera) {
                    const end = new Date()
                    const start = new Date(end.getTime() - 30 * 60 * 1000)
                    window.open(`${apiUrl}/api/v1/recordings/export?camera_id=${selectedCamera}&start=${start.toISOString()}&end=${end.toISOString()}`)
                  }
                }}
                disabled={!selectedCamera}
              >
                <Download size={14} />
                Export
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
