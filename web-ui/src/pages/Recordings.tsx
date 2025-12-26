import { useState, useRef, useEffect, useCallback, useMemo } from 'react'
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
  ZoomIn,
  ZoomOut,
  Download,
  Scissors,
  AlertCircle,
  User,
  Car,
  Dog,
  Package,
  Eye,
  X,
  Check
} from 'lucide-react'
import { cameraApi, eventApi, type Camera as CameraType, type Event } from '../lib/api'
import { usePorts } from '../hooks/usePorts'

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

// Timeline zoom levels (hours shown)
const ZOOM_LEVELS = [1, 2, 4, 6, 12, 24]

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

// Export state interface
interface ExportState {
  active: boolean
  startTime: Date | null
  endTime: Date | null
  exporting: boolean
}

export function Recordings() {
  const { apiUrl } = usePorts()
  const [selectedCamera, setSelectedCamera] = useState<string>('')
  const [selectedDate, setSelectedDate] = useState<string>(
    new Date().toISOString().split('T')[0]
  )
  const [currentTime, setCurrentTime] = useState<Date | null>(null)
  const [isPlaying, setIsPlaying] = useState(false)
  const [isMuted, setIsMuted] = useState(true)
  const [zoomLevel, setZoomLevel] = useState(24) // Hours to show
  const [timelineOffset, setTimelineOffset] = useState(0) // Offset in hours from start of day
  const [isDragging, setIsDragging] = useState(false)
  const [dragTime, setDragTime] = useState<Date | null>(null) // Separate state for drag preview
  const [videoStartTime, setVideoStartTime] = useState<Date | null>(null) // Track when current video segment started
  const [playbackSpeed, setPlaybackSpeed] = useState(1)
  const [showEventPanel, setShowEventPanel] = useState(false)
  const [hoverTime, setHoverTime] = useState<Date | null>(null)
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  const [_hoverThumbnail, setHoverThumbnail] = useState<string | null>(null)
  const [exportState, setExportState] = useState<ExportState>({
    active: false,
    startTime: null,
    endTime: null,
    exporting: false
  })

  const videoRef = useRef<HTMLVideoElement>(null)
  const timelineRef = useRef<HTMLDivElement>(null)
  const playheadRef = useRef<HTMLDivElement>(null)

  const { data: cameras } = useQuery({
    queryKey: ['cameras'],
    queryFn: cameraApi.list,
  })

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
      return data.data
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

      // eventApi.list returns { data: Event[], total: number }
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

  // Calculate timeline bounds based on zoom level
  const getTimelineBounds = useCallback(() => {
    const dayStart = new Date(selectedDate)
    dayStart.setHours(0, 0, 0, 0)

    const viewStart = new Date(dayStart.getTime() + timelineOffset * 60 * 60 * 1000)
    const viewEnd = new Date(viewStart.getTime() + zoomLevel * 60 * 60 * 1000)

    return { viewStart, viewEnd, dayStart }
  }, [selectedDate, timelineOffset, zoomLevel])

  // Calculate events within current view
  const visibleEvents = useMemo(() => {
    if (!events) return []
    const { viewStart, viewEnd } = getTimelineBounds()
    return events.filter(e => {
      const eventTime = new Date(e.timestamp * 1000)
      return eventTime >= viewStart && eventTime <= viewEnd
    })
  }, [events, getTimelineBounds])

  // Format time for display
  const formatTime = (date: Date) => {
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  }

  const formatTimeShort = (date: Date) => {
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  }

  // Get stream URL for a timestamp
  const getStreamUrl = (timestamp: Date) => {
    return `${apiUrl}/api/v1/recordings/timeline/${selectedCamera}/stream?t=${timestamp.toISOString()}`
  }

  // Seek to a specific time
  const seekToTime = useCallback((time: Date) => {
    setCurrentTime(time)
    setVideoStartTime(time) // Remember when this segment started
    setDragTime(null) // Clear any drag preview
    if (videoRef.current) {
      const url = getStreamUrl(time)
      videoRef.current.src = url
      videoRef.current.load()
      if (isPlaying) {
        videoRef.current.play().catch(() => {})
      }
    }
  }, [selectedCamera, isPlaying])

  // Handle timeline click to seek
  const handleTimelineClick = useCallback((e: React.MouseEvent<HTMLDivElement>) => {
    if (!timelineRef.current) return

    const rect = timelineRef.current.getBoundingClientRect()
    const clickX = e.clientX - rect.left
    const percent = clickX / rect.width

    const { viewStart, viewEnd } = getTimelineBounds()
    const duration = viewEnd.getTime() - viewStart.getTime()
    const clickTime = new Date(viewStart.getTime() + percent * duration)

    seekToTime(clickTime)
  }, [getTimelineBounds, seekToTime])


  // Start/stop playhead drag
  const startDrag = (e: React.MouseEvent) => {
    e.stopPropagation() // Prevent click from also firing
    setIsDragging(true)
    setDragTime(currentTime) // Initialize drag from current position
  }

  const stopDrag = useCallback(() => {
    if (isDragging && dragTime) {
      seekToTime(dragTime) // Load video at the dropped position
    }
    setIsDragging(false)
    setDragTime(null)
  }, [isDragging, dragTime, seekToTime])

  // Keyboard shortcuts
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.target instanceof HTMLInputElement) return

      switch (e.key) {
        case ' ':
          e.preventDefault()
          setIsPlaying(p => !p)
          break
        case 'ArrowLeft':
          if (currentTime) {
            seekToTime(new Date(currentTime.getTime() - 10000)) // -10s
          }
          break
        case 'ArrowRight':
          if (currentTime) {
            seekToTime(new Date(currentTime.getTime() + 10000)) // +10s
          }
          break
        case 'm':
          setIsMuted(m => !m)
          break
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [currentTime, seekToTime])

  // Update video muted state
  useEffect(() => {
    if (videoRef.current) {
      videoRef.current.muted = isMuted
    }
  }, [isMuted])

  // Play/pause video
  useEffect(() => {
    if (videoRef.current) {
      if (isPlaying) {
        videoRef.current.play().catch(() => {})
      } else {
        videoRef.current.pause()
      }
    }
  }, [isPlaying])

  // Track video time and update currentTime
  useEffect(() => {
    if (!videoRef.current || !videoStartTime) return

    const video = videoRef.current
    const handleTimeUpdate = () => {
      // Only update if not dragging
      if (isDragging) return

      // Update currentTime based on video progress
      // videoStartTime is when this segment started, video.currentTime is offset into segment
      const offset = video.currentTime * 1000
      setCurrentTime(new Date(videoStartTime.getTime() + offset))
    }

    video.addEventListener('timeupdate', handleTimeUpdate)
    return () => video.removeEventListener('timeupdate', handleTimeUpdate)
  }, [videoStartTime, isDragging])

  // Global mouse move and up listeners for smooth dragging
  useEffect(() => {
    const handleGlobalMouseMove = (e: MouseEvent) => {
      if (!isDragging || !timelineRef.current) return

      const rect = timelineRef.current.getBoundingClientRect()
      const dragX = Math.max(0, Math.min(e.clientX - rect.left, rect.width))
      const percent = dragX / rect.width

      const { viewStart, viewEnd } = getTimelineBounds()
      const duration = viewEnd.getTime() - viewStart.getTime()
      const newDragTime = new Date(viewStart.getTime() + percent * duration)

      setDragTime(newDragTime)
    }

    window.addEventListener('mousemove', handleGlobalMouseMove)
    window.addEventListener('mouseup', stopDrag)
    return () => {
      window.removeEventListener('mousemove', handleGlobalMouseMove)
      window.removeEventListener('mouseup', stopDrag)
    }
  }, [stopDrag, isDragging, getTimelineBounds])

  // Update playback speed
  useEffect(() => {
    if (videoRef.current) {
      videoRef.current.playbackRate = playbackSpeed
    }
  }, [playbackSpeed])

  // Handle timeline hover for thumbnail preview
  const handleTimelineMouseMove = useCallback((e: React.MouseEvent<HTMLDivElement>) => {
    if (!timelineRef.current || isDragging) return

    const rect = timelineRef.current.getBoundingClientRect()
    const hoverX = e.clientX - rect.left
    const percent = hoverX / rect.width

    const { viewStart, viewEnd } = getTimelineBounds()
    const duration = viewEnd.getTime() - viewStart.getTime()
    const time = new Date(viewStart.getTime() + percent * duration)

    setHoverTime(time)
    // TODO: Fetch thumbnail for this timestamp
    // setHoverThumbnail(`${API_BASE}/api/v1/recordings/thumbnail/${selectedCamera}?t=${time.toISOString()}`)
  }, [isDragging, getTimelineBounds])

  const handleTimelineMouseLeave = useCallback(() => {
    setHoverTime(null)
    setHoverThumbnail(null)
  }, [])

  // Export functions
  const startExport = useCallback(() => {
    setExportState({
      active: true,
      startTime: currentTime,
      endTime: null,
      exporting: false
    })
  }, [currentTime])

  const setExportEnd = useCallback(() => {
    if (exportState.active && currentTime) {
      setExportState(prev => ({
        ...prev,
        endTime: currentTime
      }))
    }
  }, [exportState.active, currentTime])

  const cancelExport = useCallback(() => {
    setExportState({
      active: false,
      startTime: null,
      endTime: null,
      exporting: false
    })
  }, [])

  const performExport = useCallback(async () => {
    if (!exportState.startTime || !exportState.endTime || !selectedCamera) return

    setExportState(prev => ({ ...prev, exporting: true }))

    try {
      const response = await fetch(`${apiUrl}/api/v1/recordings/export`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          camera_id: selectedCamera,
          start_time: exportState.startTime.toISOString(),
          end_time: exportState.endTime.toISOString()
        })
      })

      if (response.ok) {
        const data = await response.json()
        // Trigger download if path is returned
        if (data.data?.output_path) {
          window.open(`${apiUrl}/api/v1/recordings/download?path=${encodeURIComponent(data.data.output_path)}`)
        }
        cancelExport()
      }
    } catch (error) {
      console.error('Export failed:', error)
    } finally {
      setExportState(prev => ({ ...prev, exporting: false }))
    }
  }, [exportState.startTime, exportState.endTime, selectedCamera, cancelExport])

  // Jump to event
  const jumpToEvent = useCallback((event: Event) => {
    const eventTime = new Date(event.timestamp * 1000)
    seekToTime(eventTime)
    setShowEventPanel(false)
  }, [seekToTime])

  // Calculate playhead position - uses dragTime when dragging, currentTime otherwise
  const getPlayheadPosition = (): number | null => {
    const displayTime = isDragging ? dragTime : currentTime
    if (!displayTime) return null

    const { viewStart, viewEnd } = getTimelineBounds()
    const duration = viewEnd.getTime() - viewStart.getTime()
    const position = (displayTime.getTime() - viewStart.getTime()) / duration

    if (position < 0 || position > 1) return null
    return position * 100
  }

  // Get the time to display (drag preview or current)
  const displayTime = isDragging ? dragTime : currentTime

  // Navigate date
  const navigateDate = (direction: 'prev' | 'next') => {
    const current = new Date(selectedDate)
    current.setDate(current.getDate() + (direction === 'prev' ? -1 : 1))
    setSelectedDate(current.toISOString().split('T')[0])
    setCurrentTime(null)
    setIsPlaying(false)
  }

  // Zoom controls
  const zoomIn = () => {
    const idx = ZOOM_LEVELS.indexOf(zoomLevel)
    if (idx > 0) {
      // Center the zoom on current view center
      const centerOffset = timelineOffset + zoomLevel / 2
      const newZoom = ZOOM_LEVELS[idx - 1]
      setZoomLevel(newZoom)
      setTimelineOffset(Math.max(0, Math.min(24 - newZoom, centerOffset - newZoom / 2)))
    }
  }

  const zoomOut = () => {
    const idx = ZOOM_LEVELS.indexOf(zoomLevel)
    if (idx < ZOOM_LEVELS.length - 1) {
      const centerOffset = timelineOffset + zoomLevel / 2
      const newZoom = ZOOM_LEVELS[idx + 1]
      setZoomLevel(newZoom)
      setTimelineOffset(Math.max(0, Math.min(24 - newZoom, centerOffset - newZoom / 2)))
    }
  }

  // Generate time markers for timeline
  const getTimeMarkers = () => {
    const { viewStart } = getTimelineBounds()
    const markers: { position: number; label: string }[] = []

    // Determine interval based on zoom level
    let intervalMinutes = 60
    if (zoomLevel <= 2) intervalMinutes = 15
    else if (zoomLevel <= 6) intervalMinutes = 30

    const intervalMs = intervalMinutes * 60 * 1000
    const durationMs = zoomLevel * 60 * 60 * 1000

    let markerTime = new Date(Math.ceil(viewStart.getTime() / intervalMs) * intervalMs)

    while (markerTime.getTime() < viewStart.getTime() + durationMs) {
      const position = ((markerTime.getTime() - viewStart.getTime()) / durationMs) * 100
      markers.push({
        position,
        label: formatTimeShort(markerTime)
      })
      markerTime = new Date(markerTime.getTime() + intervalMs)
    }

    return markers
  }

  // Check if there's any recording for the selected camera/date
  const hasRecordings = timeline?.segments?.some(s => s.type === 'recording')

  const playheadPosition = getPlayheadPosition()

  return (
    <div className="flex flex-col h-[calc(100vh-4rem)]">
      {/* Header */}
      <div className="flex items-center justify-between p-4 border-b border-border">
        <div>
          <h1 className="text-xl font-bold">Playback</h1>
          <p className="text-sm text-muted-foreground">
            {currentTime ? formatTime(currentTime) : 'Select a time on the timeline'}
          </p>
        </div>

        <div className="flex items-center gap-4">
          {/* Camera selector */}
          <div className="flex items-center gap-2">
            <Camera size={18} className="text-muted-foreground" />
            <select
              value={selectedCamera}
              onChange={(e) => {
                setSelectedCamera(e.target.value)
                setCurrentTime(null)
                setIsPlaying(false)
              }}
              className="bg-background border border-border rounded-md px-3 py-1.5 text-sm min-w-[180px]"
            >
              <option value="">Select Camera</option>
              {cameras?.map((cam: CameraType) => (
                <option key={cam.id} value={cam.id}>
                  {cam.name}
                </option>
              ))}
            </select>
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
      </div>

      {/* Main content */}
      <div className="flex-1 flex flex-col min-h-0">
        {!selectedCamera ? (
          <div className="flex-1 flex flex-col items-center justify-center text-center">
            <Camera size={64} className="text-muted-foreground mb-4" />
            <h2 className="text-lg font-medium mb-2">Select a Camera</h2>
            <p className="text-muted-foreground">
              Choose a camera to view recorded footage
            </p>
          </div>
        ) : timelineLoading ? (
          <div className="flex-1 flex items-center justify-center">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
          </div>
        ) : !hasRecordings ? (
          <div className="flex-1 flex flex-col items-center justify-center text-center">
            <Clock size={64} className="text-muted-foreground mb-4" />
            <h2 className="text-lg font-medium mb-2">No Recordings</h2>
            <p className="text-muted-foreground">
              No recordings found for {new Date(selectedDate).toLocaleDateString([], {
                weekday: 'long',
                month: 'long',
                day: 'numeric'
              })}
            </p>
          </div>
        ) : (
          <>
            {/* Video player area */}
            <div className="flex-1 bg-black flex items-center justify-center min-h-0">
              {currentTime ? (
                <video
                  ref={videoRef}
                  className="max-w-full max-h-full"
                  muted={isMuted}
                  playsInline
                  onEnded={() => {
                    // When segment ends, try to load the next one
                    if (currentTime) {
                      const nextTime = new Date(currentTime.getTime() + 1000)
                      seekToTime(nextTime)
                    }
                  }}
                  onError={() => {
                    setIsPlaying(false)
                  }}
                />
              ) : (
                <div className="text-white/50 text-center">
                  <Play size={64} className="mx-auto mb-4 opacity-50" />
                  <p>Click on the timeline below to start playback</p>
                </div>
              )}
            </div>

            {/* Playback controls */}
            <div className="bg-card border-t border-border p-3">
              <div className="flex items-center justify-between mb-3">
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
                  {displayTime ? (
                    <>
                      <span className="text-muted-foreground mr-2">
                        {new Date(selectedDate).toLocaleDateString([], { month: 'short', day: 'numeric' })}
                      </span>
                      <span className={isDragging ? 'text-primary' : ''}>
                        {formatTime(displayTime)}
                      </span>
                      {isDragging && <span className="text-primary ml-2">(seeking...)</span>}
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

                  {/* Export clip button */}
                  {!exportState.active ? (
                    <button
                      onClick={startExport}
                      className="p-2 hover:bg-accent rounded-md transition-colors"
                      title="Export clip"
                      disabled={!currentTime}
                    >
                      <Scissors size={18} />
                    </button>
                  ) : (
                    <div className="flex items-center gap-1 bg-accent rounded px-2">
                      {!exportState.endTime ? (
                        <>
                          <span className="text-xs">Set end:</span>
                          <button
                            onClick={setExportEnd}
                            className="p-1.5 bg-primary text-primary-foreground rounded"
                            title="Set clip end"
                          >
                            <Check size={14} />
                          </button>
                        </>
                      ) : (
                        <>
                          <button
                            onClick={performExport}
                            className="p-1.5 bg-primary text-primary-foreground rounded"
                            title="Download clip"
                            disabled={exportState.exporting}
                          >
                            <Download size={14} />
                          </button>
                        </>
                      )}
                      <button
                        onClick={cancelExport}
                        className="p-1.5 hover:bg-destructive/20 rounded"
                        title="Cancel"
                      >
                        <X size={14} />
                      </button>
                    </div>
                  )}

                  {/* Event panel toggle */}
                  <button
                    onClick={() => setShowEventPanel(!showEventPanel)}
                    className={`p-2 hover:bg-accent rounded-md transition-colors ${showEventPanel ? 'bg-accent' : ''}`}
                    title="Show events"
                  >
                    <AlertCircle size={18} />
                    {events && events.length > 0 && (
                      <span className="absolute -top-1 -right-1 bg-primary text-primary-foreground text-[10px] rounded-full w-4 h-4 flex items-center justify-center">
                        {events.length > 99 ? '99+' : events.length}
                      </span>
                    )}
                  </button>

                  <div className="w-px h-6 bg-border mx-1" />

                  <button
                    onClick={zoomIn}
                    className="p-2 hover:bg-accent rounded-md transition-colors"
                    disabled={zoomLevel === ZOOM_LEVELS[0]}
                    title="Zoom in"
                  >
                    <ZoomIn size={18} />
                  </button>
                  <span className="text-xs text-muted-foreground w-12 text-center">
                    {zoomLevel}h
                  </span>
                  <button
                    onClick={zoomOut}
                    className="p-2 hover:bg-accent rounded-md transition-colors"
                    disabled={zoomLevel === ZOOM_LEVELS[ZOOM_LEVELS.length - 1]}
                    title="Zoom out"
                  >
                    <ZoomOut size={18} />
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
                </div>
              </div>

              {/* Timeline */}
              <div className="relative">
                {/* Time markers */}
                <div className="relative h-5 mb-1">
                  {getTimeMarkers().map((marker, i) => (
                    <span
                      key={i}
                      className="absolute text-[10px] text-muted-foreground -translate-x-1/2"
                      style={{ left: `${marker.position}%` }}
                    >
                      {marker.label}
                    </span>
                  ))}
                </div>

                {/* Timeline bar */}
                <div
                  ref={timelineRef}
                  className={`relative h-10 bg-muted/30 rounded cursor-pointer overflow-hidden ${isDragging ? 'cursor-ew-resize' : ''}`}
                  onClick={handleTimelineClick}
                  onMouseMove={handleTimelineMouseMove}
                  onMouseLeave={handleTimelineMouseLeave}
                >
                  {/* Recording segments */}
                  {timeline?.segments?.map((segment, i) => {
                    if (segment.type !== 'recording') return null

                    const { viewStart, viewEnd } = getTimelineBounds()
                    const viewDuration = viewEnd.getTime() - viewStart.getTime()

                    const segStart = new Date(segment.start_time)
                    const segEnd = new Date(segment.end_time)

                    // Calculate position within current view
                    const left = Math.max(0, ((segStart.getTime() - viewStart.getTime()) / viewDuration) * 100)
                    const right = Math.min(100, ((segEnd.getTime() - viewStart.getTime()) / viewDuration) * 100)
                    const width = right - left

                    if (width <= 0 || left >= 100 || right <= 0) return null

                    return (
                      <div
                        key={i}
                        className={`absolute top-1 bottom-1 rounded ${
                          segment.has_events ? 'bg-primary' : 'bg-primary/60'
                        }`}
                        style={{ left: `${left}%`, width: `${width}%` }}
                      />
                    )
                  })}

                  {/* Export range overlay */}
                  {exportState.active && exportState.startTime && (
                    (() => {
                      const { viewStart, viewEnd } = getTimelineBounds()
                      const viewDuration = viewEnd.getTime() - viewStart.getTime()
                      const startPos = ((exportState.startTime.getTime() - viewStart.getTime()) / viewDuration) * 100
                      const endPos = exportState.endTime
                        ? ((exportState.endTime.getTime() - viewStart.getTime()) / viewDuration) * 100
                        : currentTime
                          ? ((currentTime.getTime() - viewStart.getTime()) / viewDuration) * 100
                          : startPos

                      return (
                        <div
                          className="absolute top-0 bottom-0 bg-yellow-500/30 border-l-2 border-r-2 border-yellow-500"
                          style={{
                            left: `${Math.max(0, Math.min(startPos, endPos))}%`,
                            width: `${Math.max(0, Math.abs(endPos - startPos))}%`
                          }}
                        />
                      )
                    })()
                  )}

                  {/* Event markers */}
                  {visibleEvents.map((event, i) => {
                    const { viewStart, viewEnd } = getTimelineBounds()
                    const viewDuration = viewEnd.getTime() - viewStart.getTime()
                    const eventTime = new Date(event.timestamp * 1000)
                    const position = ((eventTime.getTime() - viewStart.getTime()) / viewDuration) * 100

                    if (position < 0 || position > 100) return null

                    const eventInfo = getEventInfo(event.event_type)
                    const Icon = eventInfo.icon

                    return (
                      <div
                        key={event.id || i}
                        className="absolute top-0 bottom-0 w-0.5 cursor-pointer group"
                        style={{ left: `${position}%` }}
                        onClick={(e) => {
                          e.stopPropagation()
                          jumpToEvent(event)
                        }}
                        title={`${eventInfo.label}: ${event.label || event.event_type}`}
                      >
                        <div
                          className="absolute inset-0"
                          style={{ backgroundColor: eventInfo.color }}
                        />
                        <div
                          className="absolute -top-1 left-1/2 -translate-x-1/2 w-3 h-3 rounded-full flex items-center justify-center shadow-sm opacity-80 group-hover:opacity-100 group-hover:scale-125 transition-all"
                          style={{ backgroundColor: eventInfo.color }}
                        >
                          <Icon size={8} className="text-white" />
                        </div>
                      </div>
                    )
                  })}

                  {/* Hour grid lines */}
                  {Array.from({ length: Math.ceil(zoomLevel) + 1 }).map((_, i) => {
                    const position = (i / zoomLevel) * 100
                    if (position > 100) return null
                    return (
                      <div
                        key={i}
                        className="absolute top-0 h-full border-l border-border/30"
                        style={{ left: `${position}%` }}
                      />
                    )
                  })}

                  {/* Hover time indicator */}
                  {hoverTime && !isDragging && (
                    (() => {
                      const { viewStart, viewEnd } = getTimelineBounds()
                      const viewDuration = viewEnd.getTime() - viewStart.getTime()
                      const position = ((hoverTime.getTime() - viewStart.getTime()) / viewDuration) * 100

                      return (
                        <div
                          className="absolute top-0 h-full w-px bg-white/40 pointer-events-none z-5"
                          style={{ left: `${position}%` }}
                        >
                          <div className="absolute -top-7 left-1/2 -translate-x-1/2 bg-black/80 text-white px-2 py-0.5 rounded text-[10px] whitespace-nowrap">
                            {formatTime(hoverTime)}
                          </div>
                        </div>
                      )
                    })()
                  )}

                  {/* Playhead */}
                  {playheadPosition !== null && (
                    <div
                      ref={playheadRef}
                      className={`absolute top-0 h-full w-1 z-10 cursor-ew-resize transition-colors ${
                        isDragging ? 'bg-primary' : 'bg-white'
                      }`}
                      style={{ left: `${playheadPosition}%`, transform: 'translateX(-50%)' }}
                      onMouseDown={startDrag}
                    >
                      {/* Playhead handle - larger hit area for easier dragging */}
                      <div className={`absolute -top-2 left-1/2 -translate-x-1/2 w-4 h-4 rounded-full shadow-lg transition-all ${
                        isDragging ? 'bg-primary scale-125' : 'bg-white hover:scale-110'
                      }`} />
                      {/* Time tooltip when dragging */}
                      {isDragging && dragTime && (
                        <div className="absolute -top-8 left-1/2 -translate-x-1/2 bg-primary text-primary-foreground px-2 py-1 rounded text-xs whitespace-nowrap shadow-lg">
                          {formatTime(dragTime)}
                        </div>
                      )}
                    </div>
                  )}
                </div>

                {/* Scroll controls for zoomed timeline */}
                {zoomLevel < 24 && (
                  <div className="flex items-center justify-center gap-2 mt-2">
                    <button
                      onClick={() => setTimelineOffset(Math.max(0, timelineOffset - zoomLevel / 2))}
                      className="p-1 hover:bg-accent rounded"
                      disabled={timelineOffset === 0}
                    >
                      <ChevronLeft size={16} />
                    </button>
                    <div className="text-xs text-muted-foreground">
                      {formatTimeShort(getTimelineBounds().viewStart)} - {formatTimeShort(getTimelineBounds().viewEnd)}
                    </div>
                    <button
                      onClick={() => setTimelineOffset(Math.min(24 - zoomLevel, timelineOffset + zoomLevel / 2))}
                      className="p-1 hover:bg-accent rounded"
                      disabled={timelineOffset >= 24 - zoomLevel}
                    >
                      <ChevronRight size={16} />
                    </button>
                  </div>
                )}
              </div>
            </div>
          </>
        )}
      </div>

      {/* Event panel overlay */}
      {showEventPanel && events && (
        <div className="fixed inset-y-0 right-0 w-80 bg-card border-l border-border shadow-xl z-50 flex flex-col">
          <div className="flex items-center justify-between p-4 border-b border-border">
            <h3 className="font-semibold">Events</h3>
            <button
              onClick={() => setShowEventPanel(false)}
              className="p-1 hover:bg-accent rounded"
            >
              <X size={18} />
            </button>
          </div>

          <div className="flex-1 overflow-y-auto">
            {events.length === 0 ? (
              <div className="p-4 text-center text-muted-foreground">
                <AlertCircle size={32} className="mx-auto mb-2 opacity-50" />
                <p className="text-sm">No events for this date</p>
              </div>
            ) : (
              <div className="divide-y divide-border">
                {events.map((event) => {
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
                        className="w-8 h-8 rounded-full flex items-center justify-center shrink-0"
                        style={{ backgroundColor: `${eventInfo.color}20` }}
                      >
                        <Icon size={16} style={{ color: eventInfo.color }} />
                      </div>
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center justify-between gap-2">
                          <span className="font-medium text-sm truncate">
                            {event.label || eventInfo.label}
                          </span>
                          <span className="text-xs text-muted-foreground shrink-0">
                            {formatTime(eventTime)}
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

          {/* Event legend */}
          <div className="p-3 border-t border-border bg-muted/30">
            <div className="text-xs text-muted-foreground mb-2">Event Types</div>
            <div className="flex flex-wrap gap-2">
              {Object.entries(EVENT_ICONS).filter(([key]) => key !== 'default').map(([key, info]) => {
                const Icon = info.icon
                return (
                  <div
                    key={key}
                    className="flex items-center gap-1 text-xs"
                  >
                    <Icon size={12} style={{ color: info.color }} />
                    <span>{info.label}</span>
                  </div>
                )
              })}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
