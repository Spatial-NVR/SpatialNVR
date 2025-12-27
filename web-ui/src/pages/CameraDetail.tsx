import { useParams, useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, Settings, Trash2, RefreshCw, X, Save, Copy, Check, Volume2, VolumeX, HardDrive, Mic, MicOff, Circle, Square, Loader2, AlertCircle, List, Clock, Bell } from 'lucide-react'
import { Link } from 'react-router-dom'
import { cameraApi, CameraConfig, storageApi, audioApi, AudioSessionResponse, recordingApi, eventApi, timelineApi } from '../lib/api'
import { VideoPlayer } from '../components/VideoPlayer'
import { MotionZoneEditor } from '../components/MotionZoneEditor'
import { useState, useEffect } from 'react'
import { usePorts } from '../hooks/usePorts'
import { useToast } from '../components/Toast'

// Format bytes to human readable
function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

type SettingsTab = 'general' | 'streams' | 'recording' | 'detection' | 'zones' | 'audio';
type EventsViewMode = 'list' | 'timeline';

export function CameraDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { go2rtcUrl, go2rtcWsUrl, ports } = usePorts()
  const { addToast } = useToast()
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const [showSettings, setShowSettings] = useState(false)
  const [settingsTab, setSettingsTab] = useState<SettingsTab>('general')
  const [isRefreshing, setIsRefreshing] = useState(false)
  const [copiedUrl, setCopiedUrl] = useState<string | null>(null)
  const [audioEnabled, setAudioEnabled] = useState(false)
  const [talkSession, setTalkSession] = useState<AudioSessionResponse | null>(null)
  const [isTalking, setIsTalking] = useState(false)
  const [eventsViewMode, setEventsViewMode] = useState<EventsViewMode>('list')
  const [timelinePosition, setTimelinePosition] = useState(Date.now() / 1000)

  // Form state for settings
  const [formData, setFormData] = useState<Partial<CameraConfig>>({
    name: '',
    stream: { url: '', roles: { detect: 'sub', record: 'main', audio: 'main', motion: 'sub' } },
    manufacturer: '',
    model: '',
    display_aspect_ratio: '16:9',
    recording: { enabled: false, mode: 'motion', retention: { default_days: 30 } },
    detection: { enabled: true, show_overlay: false, min_confidence: 0.5 },
    audio: { enabled: false },
    motion: { enabled: true, threshold: 0.02, method: 'frame_diff' },
  })

  const { data: camera, isLoading, refetch } = useQuery({
    queryKey: ['camera', id],
    queryFn: () => cameraApi.get(id!),
    enabled: !!id,
    refetchInterval: 10000,
  })

  // Fetch storage stats
  const { data: storageStats } = useQuery({
    queryKey: ['storage'],
    queryFn: () => storageApi.getStats(),
    refetchInterval: 30000, // Refresh every 30 seconds
  })

  // Get storage used by this camera
  const cameraStorageBytes = id && storageStats?.by_camera ? (storageStats.by_camera[id] || 0) : 0

  // Fetch events for this camera
  const { data: eventsData } = useQuery({
    queryKey: ['camera-events', id],
    queryFn: () => eventApi.list({ camera_id: id, limit: 50 }),
    enabled: !!id,
    refetchInterval: 30000,
  })

  const events = eventsData?.data || []

  // Fetch timeline segments
  const { data: timelineSegments } = useQuery({
    queryKey: ['camera-timeline', id],
    queryFn: () => timelineApi.get(id!),
    enabled: !!id,
    refetchInterval: 60000,
  })

  // Fetch full camera config when settings modal opens
  const { data: cameraConfig } = useQuery({
    queryKey: ['camera-config', id],
    queryFn: () => cameraApi.getConfig(id!),
    enabled: !!id && showSettings,
  })

  // Fetch recording status
  const { data: recordingStatus, refetch: refetchRecordingStatus } = useQuery({
    queryKey: ['recording-status', id],
    queryFn: () => recordingApi.getStatus(id!),
    enabled: !!id,
    refetchInterval: 3000, // Check every 3 seconds
  })

  // Recording control mutations
  const startRecordingMutation = useMutation({
    mutationFn: () => recordingApi.start(id!),
    onSuccess: () => {
      refetchRecordingStatus()
    },
  })

  const stopRecordingMutation = useMutation({
    mutationFn: () => recordingApi.stop(id!),
    onSuccess: () => {
      refetchRecordingStatus()
    },
  })

  const isRecording = recordingStatus?.state === 'running' || recordingStatus?.state === 'starting'
  const isRecordingBusy = startRecordingMutation.isPending || stopRecordingMutation.isPending

  // Update form when camera data loads (basic info from camera API)
  useEffect(() => {
    if (camera && !showSettings) {
      setFormData(prev => ({
        ...prev,
        name: camera.name,
        stream: {
          ...prev.stream,
          url: camera.stream_url || '',
        },
        manufacturer: camera.manufacturer || '',
        model: camera.model || '',
        display_aspect_ratio: camera.display_aspect_ratio || '16:9',
      }))
    }
  }, [camera, showSettings])

  // When full config is loaded (settings modal open), update form with all settings
  useEffect(() => {
    if (cameraConfig && showSettings) {
      setFormData({
        name: cameraConfig.name || '',
        stream: {
          url: cameraConfig.stream?.url || '',
          sub_url: cameraConfig.stream?.sub_url || '',
          username: cameraConfig.stream?.username || '',
          roles: cameraConfig.stream?.roles || { detect: 'sub', record: 'main', audio: 'main', motion: 'sub' },
        },
        manufacturer: cameraConfig.manufacturer || '',
        model: cameraConfig.model || '',
        recording: {
          enabled: cameraConfig.recording?.enabled ?? false,
          mode: cameraConfig.recording?.mode || 'motion',
          pre_buffer_seconds: cameraConfig.recording?.pre_buffer_seconds ?? 5,
          post_buffer_seconds: cameraConfig.recording?.post_buffer_seconds ?? 10,
          retention: {
            default_days: cameraConfig.recording?.retention?.default_days ?? 30,
          },
        },
        detection: {
          enabled: cameraConfig.detection?.enabled ?? true,
          fps: cameraConfig.detection?.fps ?? 5,
          show_overlay: cameraConfig.detection?.show_overlay ?? false,
          min_confidence: cameraConfig.detection?.min_confidence ?? 0.5,
        },
        audio: {
          enabled: cameraConfig.audio?.enabled ?? false,
          two_way: cameraConfig.audio?.two_way ?? false,
        },
        motion: {
          enabled: cameraConfig.motion?.enabled ?? true,
          threshold: cameraConfig.motion?.threshold ?? 0.02,
          method: cameraConfig.motion?.method || 'frame_diff',
        },
      })
    }
  }, [cameraConfig, showSettings])

  const deleteMutation = useMutation({
    mutationFn: () => cameraApi.delete(id!),
    onSuccess: () => {
      addToast('success', 'Camera deleted successfully')
      queryClient.invalidateQueries({ queryKey: ['cameras'] })
      navigate('/')
    },
    onError: (error: Error) => {
      console.error('Delete failed:', error)
      addToast('error', `Failed to delete camera: ${error.message}`)
      setShowDeleteConfirm(false)
    },
  })

  const updateMutation = useMutation({
    mutationFn: (config: Partial<CameraConfig>) => cameraApi.update(id!, config),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['camera', id] })
      queryClient.invalidateQueries({ queryKey: ['camera-config', id] })
      queryClient.invalidateQueries({ queryKey: ['cameras'] })
      setShowSettings(false)
    },
  })

  const handleSave = () => {
    updateMutation.mutate(formData)
  }

  const copyToClipboard = async (url: string, label: string) => {
    await navigator.clipboard.writeText(url)
    setCopiedUrl(label)
    setTimeout(() => setCopiedUrl(null), 2000)
  }

  const handleTalkToggle = async () => {
    if (talkSession) {
      // Stop talking
      try {
        await audioApi.stopSession(talkSession.session.id)
      } catch (error) {
        console.error('Failed to stop audio session:', error)
      }
      // Stop the microphone stream
      const micStream = (window as unknown as { _micStream?: MediaStream })._micStream
      if (micStream) {
        micStream.getTracks().forEach(track => track.stop())
        delete (window as unknown as { _micStream?: MediaStream })._micStream
      }
      setTalkSession(null)
      setIsTalking(false)
    } else {
      // Start talking - first request microphone permission
      setIsTalking(true)
      try {
        // Request microphone access from browser
        const stream = await navigator.mediaDevices.getUserMedia({ audio: true })

        // Start the audio session with the backend
        const session = await audioApi.startSession(id!)
        setTalkSession(session)

        // TODO: Send audio stream to backend via WebSocket
        // For now, just keep the stream active to show permission was granted
        console.log('Microphone access granted, audio session started:', session)

        // Store the stream so we can stop it later
        ;(window as unknown as { _micStream?: MediaStream })._micStream = stream
      } catch (error) {
        console.error('Failed to start audio session:', error)
        // Check if it was a permission error
        if (error instanceof DOMException && error.name === 'NotAllowedError') {
          alert('Microphone access was denied. Please allow microphone access in your browser settings.')
        }
        setIsTalking(false)
      }
    }
  }

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
      </div>
    )
  }

  if (!camera) {
    return (
      <div className="text-center py-12">
        <p className="text-muted-foreground">Camera not found</p>
        <Link to="/cameras" className="text-primary hover:underline mt-2 inline-block">
          Back to cameras
        </Link>
      </div>
    )
  }

  const statusColors = {
    online: 'bg-green-500',
    offline: 'bg-gray-500',
    error: 'bg-red-500',
    starting: 'bg-yellow-500',
  }

  // Generate stream name (lowercase, underscores for spaces)
  const streamName = id!.toLowerCase().replace(/\s+/g, '_')

  // All available stream URLs
  const streamUrls = [
    { label: 'MSE/WebSocket', url: `${go2rtcWsUrl}/api/ws?src=${streamName}`, description: 'Low latency browser streaming' },
    { label: 'WebRTC', url: `${go2rtcUrl}/api/webrtc?src=${streamName}`, description: 'Ultra-low latency P2P' },
    { label: 'HLS', url: `${go2rtcUrl}/api/stream.m3u8?src=${streamName}`, description: 'HTTP Live Streaming' },
    { label: 'RTSP', url: `rtsp://localhost:${ports.go2rtc_rtsp}/${streamName}`, description: 'Re-streamed RTSP' },
    { label: 'MJPEG', url: `${go2rtcUrl}/api/stream.mjpeg?src=${streamName}`, description: 'Motion JPEG stream' },
    { label: 'Snapshot', url: `${go2rtcUrl}/api/frame.jpeg?src=${streamName}`, description: 'Single frame JPEG' },
  ]

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Link
          to="/cameras"
          className="p-2 rounded-lg hover:bg-accent transition-colors"
        >
          <ArrowLeft size={24} />
        </Link>
        <div className="flex-1">
          <div className="flex items-center gap-3">
            <h1 className="text-3xl font-bold">{camera.name}</h1>
            <div className={`w-3 h-3 rounded-full ${statusColors[camera.status]}`} />
          </div>
          <p className="text-muted-foreground">
            {camera.manufacturer} {camera.model}
          </p>
        </div>
        <div className="flex gap-2">
          <button
            onClick={async () => {
              setIsRefreshing(true)
              await refetch()
              setIsRefreshing(false)
            }}
            disabled={isRefreshing}
            className="p-2 rounded-lg hover:bg-accent transition-colors disabled:opacity-50"
            title="Refresh"
          >
            <RefreshCw size={20} className={isRefreshing ? 'animate-spin' : ''} />
          </button>
          <button
            onClick={() => setShowSettings(true)}
            className="p-2 rounded-lg hover:bg-accent transition-colors"
            title="Settings"
          >
            <Settings size={20} />
          </button>
          <button
            onClick={() => setShowDeleteConfirm(true)}
            className="p-2 rounded-lg hover:bg-destructive/10 text-destructive transition-colors"
            title="Delete"
          >
            <Trash2 size={20} />
          </button>
        </div>
      </div>

      {/* Live View */}
      <div className="relative">
        <VideoPlayer
          cameraId={id!}
          className="w-full mx-auto"
          muted={!audioEnabled}
          showDetectionToggle={true}
          initialDetectionOverlay={formData.detection?.show_overlay ?? false}
          aspectRatio={formData.display_aspect_ratio?.replace(':', '/') || '16/9'}
          maxHeight={formData.display_aspect_ratio === '3:4' ? '70vh' : undefined}
        />
        {/* Audio controls overlay */}
        <div className="absolute top-3 left-3 flex gap-2">
          <button
            onClick={() => setAudioEnabled(!audioEnabled)}
            className={`p-2 rounded-lg transition-colors ${
              audioEnabled
                ? 'bg-blue-500/80 hover:bg-blue-500 text-white'
                : 'bg-black/50 hover:bg-black/70 text-white/80'
            }`}
            title={audioEnabled ? 'Mute audio' : 'Enable audio'}
          >
            {audioEnabled ? <Volume2 size={20} /> : <VolumeX size={20} />}
          </button>

          {/* Two-way audio (Talk) button */}
          <button
            onClick={handleTalkToggle}
            disabled={isTalking && !talkSession}
            className={`p-2 rounded-lg transition-colors ${
              talkSession
                ? 'bg-green-500/80 hover:bg-green-500 text-white animate-pulse'
                : isTalking
                  ? 'bg-yellow-500/80 text-white'
                  : 'bg-black/50 hover:bg-black/70 text-white/80'
            }`}
            title={talkSession ? 'Stop talking' : 'Talk through camera'}
          >
            {talkSession ? <Mic size={20} /> : <MicOff size={20} />}
          </button>
        </div>
      </div>

      {/* Camera Info */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <div className="bg-card rounded-lg border p-6">
          <h2 className="text-lg font-semibold mb-4">Camera Information</h2>
          <dl className="space-y-3">
            <div className="flex justify-between">
              <dt className="text-muted-foreground">Status</dt>
              <dd className="font-medium capitalize">{camera.status}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-muted-foreground">Resolution</dt>
              <dd className="font-medium">{camera.resolution_current || '-'}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-muted-foreground">FPS</dt>
              <dd className="font-medium">{camera.fps_current?.toFixed(1) || '-'}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-muted-foreground">Bitrate</dt>
              <dd className="font-medium">
                {camera.bitrate_current ? `${(camera.bitrate_current / 1000).toFixed(0)} kbps` : '-'}
              </dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-muted-foreground">Last Seen</dt>
              <dd className="font-medium">
                {camera.last_seen ? new Date(camera.last_seen).toLocaleString() : '-'}
              </dd>
            </div>
            {camera.error_message && (
              <div className="flex justify-between">
                <dt className="text-muted-foreground">Error</dt>
                <dd className="font-medium text-destructive text-sm">{camera.error_message}</dd>
              </div>
            )}
            <div className="flex justify-between pt-3 border-t">
              <dt className="text-muted-foreground flex items-center gap-2">
                <HardDrive size={16} />
                Storage Used
              </dt>
              <dd className="font-medium">
                {formatBytes(cameraStorageBytes)}
              </dd>
            </div>

            {/* Recording Controls */}
            <div className="pt-3 border-t">
              <div className="flex items-center justify-between mb-2">
                <span className="text-muted-foreground flex items-center gap-2">
                  {isRecording ? (
                    <span className="relative flex h-3 w-3">
                      <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-red-400 opacity-75"></span>
                      <span className="relative inline-flex rounded-full h-3 w-3 bg-red-500"></span>
                    </span>
                  ) : (
                    <Circle size={16} className="text-gray-400" />
                  )}
                  Recording
                </span>
                <span className={`font-medium capitalize ${
                  recordingStatus?.state === 'running' ? 'text-green-500' :
                  recordingStatus?.state === 'starting' ? 'text-yellow-500' :
                  recordingStatus?.state === 'error' ? 'text-red-500' :
                  'text-gray-500'
                }`}>
                  {recordingStatus?.state || 'Unknown'}
                </span>
              </div>

              <div className="flex gap-2">
                {isRecording ? (
                  <button
                    onClick={() => stopRecordingMutation.mutate()}
                    disabled={isRecordingBusy}
                    className="flex-1 flex items-center justify-center gap-2 px-3 py-2 bg-red-500/10 hover:bg-red-500/20 text-red-500 rounded-md text-sm font-medium disabled:opacity-50 transition-colors"
                  >
                    {isRecordingBusy ? (
                      <Loader2 size={16} className="animate-spin" />
                    ) : (
                      <Square size={16} />
                    )}
                    Stop Recording
                  </button>
                ) : (
                  <button
                    onClick={() => startRecordingMutation.mutate()}
                    disabled={isRecordingBusy}
                    className="flex-1 flex items-center justify-center gap-2 px-3 py-2 bg-green-500/10 hover:bg-green-500/20 text-green-500 rounded-md text-sm font-medium disabled:opacity-50 transition-colors"
                  >
                    {isRecordingBusy ? (
                      <Loader2 size={16} className="animate-spin" />
                    ) : (
                      <Circle size={16} className="fill-current" />
                    )}
                    Start Recording
                  </button>
                )}
              </div>

              {recordingStatus?.last_error && (
                <div className="mt-2 flex items-start gap-2 text-xs text-red-500">
                  <AlertCircle size={14} className="mt-0.5 shrink-0" />
                  <span>{recordingStatus.last_error}</span>
                </div>
              )}

              {recordingStatus?.state === 'running' && (
                <div className="mt-2 text-xs text-muted-foreground">
                  <div className="flex justify-between">
                    <span>Segments:</span>
                    <span>{recordingStatus.segments_created}</span>
                  </div>
                  <div className="flex justify-between">
                    <span>Written:</span>
                    <span>{formatBytes(recordingStatus.bytes_written)}</span>
                  </div>
                  <div className="flex justify-between">
                    <span>Uptime:</span>
                    <span>{Math.floor(recordingStatus.uptime / 60)}m {Math.floor(recordingStatus.uptime % 60)}s</span>
                  </div>
                </div>
              )}
            </div>
          </dl>
        </div>

        <div className="bg-card rounded-lg border p-6">
          <h2 className="text-lg font-semibold mb-4">Stream URLs</h2>
          <dl className="space-y-3 text-sm">
            {streamUrls.map(({ label, url, description }) => (
              <div key={label}>
                <dt className="text-muted-foreground mb-1 flex items-center justify-between">
                  <span>{label}</span>
                  <span className="text-xs opacity-60">{description}</span>
                </dt>
                <dd className="font-mono text-xs bg-background p-2 rounded overflow-x-auto flex items-center justify-between gap-2">
                  <span className="truncate">{url}</span>
                  <button
                    onClick={() => copyToClipboard(url, label)}
                    className="flex-shrink-0 p-1 hover:bg-accent rounded transition-colors"
                    title="Copy URL"
                  >
                    {copiedUrl === label ? (
                      <Check size={14} className="text-green-500" />
                    ) : (
                      <Copy size={14} />
                    )}
                  </button>
                </dd>
              </div>
            ))}
          </dl>
        </div>
      </div>

      {/* Source URL Info */}
      <div className="bg-card rounded-lg border p-6">
        <h2 className="text-lg font-semibold mb-4">Source Configuration</h2>
        <div className="space-y-2">
          <div>
            <span className="text-muted-foreground text-sm">Original Stream URL:</span>
            <code className="ml-2 font-mono text-sm bg-background px-2 py-1 rounded">
              {camera.stream_url || 'Not configured'}
            </code>
          </div>
          <p className="text-xs text-muted-foreground mt-3">
            <strong>Tip:</strong> For HTTPS FLV streams, prefix the URL with <code className="bg-background px-1 rounded">ffmpeg:</code>
            (e.g., <code className="bg-background px-1 rounded">ffmpeg:https://example.com/stream.flv</code>).
            The system automatically handles RTSP streams, but non-standard protocols may need the ffmpeg prefix.
          </p>
        </div>
      </div>

      {/* Events Section with Timeline */}
      <div className="bg-card rounded-lg border p-6">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold">Events</h2>
          <div className="flex border rounded-lg overflow-hidden">
            <button
              onClick={() => setEventsViewMode('list')}
              className={`px-3 py-2 flex items-center gap-1 ${eventsViewMode === 'list' ? 'bg-primary text-primary-foreground' : 'hover:bg-accent'}`}
              title="List view"
            >
              <List size={18} />
            </button>
            <button
              onClick={() => setEventsViewMode('timeline')}
              className={`px-3 py-2 flex items-center gap-1 ${eventsViewMode === 'timeline' ? 'bg-primary text-primary-foreground' : 'hover:bg-accent'}`}
              title="Timeline view"
            >
              <Clock size={18} />
            </button>
          </div>
        </div>

        {eventsViewMode === 'list' ? (
          /* List View */
          events.length > 0 ? (
            <div className="space-y-3 max-h-96 overflow-y-auto">
              {events.map((event) => (
                <div
                  key={event.id}
                  className="flex items-center gap-3 p-3 bg-background rounded-lg"
                >
                  <div className="w-16 h-12 bg-muted rounded flex items-center justify-center overflow-hidden">
                    {event.thumbnail_path ? (
                      <img
                        src={event.thumbnail_path}
                        alt="Event"
                        className="w-full h-full object-cover"
                      />
                    ) : (
                      <Bell size={16} className="opacity-50" />
                    )}
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center justify-between">
                      <span className="font-medium capitalize truncate">
                        {event.label || event.event_type}
                      </span>
                      <span className="text-xs text-muted-foreground">
                        {Math.round(event.confidence * 100)}%
                      </span>
                    </div>
                    <p className="text-sm text-muted-foreground">
                      {new Date(event.timestamp * 1000).toLocaleString()}
                    </p>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="text-center py-8">
              <Bell className="h-12 w-12 mx-auto mb-3 opacity-30" />
              <p className="text-muted-foreground">No events yet for this camera</p>
            </div>
          )
        ) : (
          /* Timeline View */
          <div className="space-y-4">
            {/* Timeline scrubber */}
            <div className="relative">
              <div className="h-16 bg-background rounded-lg overflow-hidden">
                {/* Timeline bar showing recordings */}
                <div className="h-8 relative bg-muted/30 rounded">
                  {timelineSegments && timelineSegments.length > 0 ? (
                    <>
                      {/* Calculate time range for display (last 24 hours) */}
                      {(() => {
                        const now = Date.now()
                        const dayAgo = now - 24 * 60 * 60 * 1000
                        const timeRange = now - dayAgo

                        return timelineSegments.map((segment) => {
                          const startTime = new Date(segment.start_time).getTime()
                          const endTime = new Date(segment.end_time).getTime()

                          // Skip if outside visible range
                          if (endTime < dayAgo || startTime > now) return null

                          const left = Math.max(0, ((startTime - dayAgo) / timeRange) * 100)
                          const right = Math.min(100, ((endTime - dayAgo) / timeRange) * 100)
                          const width = right - left

                          return (
                            <div
                              key={segment.id}
                              className={`absolute top-0 h-full ${segment.has_events ? 'bg-blue-500/60' : 'bg-green-500/40'}`}
                              style={{
                                left: `${left}%`,
                                width: `${width}%`,
                              }}
                              title={`${new Date(segment.start_time).toLocaleTimeString()} - ${new Date(segment.end_time).toLocaleTimeString()}`}
                            />
                          )
                        })
                      })()}
                    </>
                  ) : (
                    <div className="absolute inset-0 flex items-center justify-center text-xs text-muted-foreground">
                      No recordings available
                    </div>
                  )}

                  {/* Current position indicator */}
                  <input
                    type="range"
                    min={Date.now() / 1000 - 24 * 60 * 60}
                    max={Date.now() / 1000}
                    value={timelinePosition}
                    onChange={(e) => setTimelinePosition(parseFloat(e.target.value))}
                    className="absolute inset-0 w-full h-full opacity-0 cursor-pointer"
                    title="Drag to navigate timeline"
                  />
                  <div
                    className="absolute top-0 h-full w-0.5 bg-primary pointer-events-none"
                    style={{
                      left: `${((timelinePosition - (Date.now() / 1000 - 24 * 60 * 60)) / (24 * 60 * 60)) * 100}%`,
                    }}
                  />
                </div>

                {/* Time labels */}
                <div className="flex justify-between text-xs text-muted-foreground px-1 mt-1">
                  <span>24h ago</span>
                  <span>12h ago</span>
                  <span>Now</span>
                </div>
              </div>

              {/* Current time display */}
              <div className="text-center mt-2">
                <span className="text-sm font-mono">
                  {new Date(timelinePosition * 1000).toLocaleString()}
                </span>
              </div>
            </div>

            {/* Events at current position */}
            <div>
              <h3 className="text-sm font-medium mb-2 text-muted-foreground">Events near selected time</h3>
              <div className="relative">
                {/* Timeline vertical line */}
                <div className="absolute left-4 top-0 bottom-0 w-0.5 bg-border" />

                <div className="space-y-4">
                  {events
                    .filter((event) => {
                      const eventTime = event.timestamp
                      const diff = Math.abs(eventTime - timelinePosition)
                      return diff < 3600 // Within 1 hour of selected position
                    })
                    .slice(0, 10)
                    .map((event) => (
                      <div key={event.id} className="relative pl-10">
                        {/* Timeline dot */}
                        <div className={`absolute left-2.5 top-2 w-3 h-3 rounded-full border-2 ${
                          event.acknowledged
                            ? 'bg-muted border-muted-foreground'
                            : 'bg-primary border-primary'
                        }`} />

                        <div className="flex items-start gap-3 bg-background p-3 rounded-lg">
                          {/* Thumbnail */}
                          <div className="w-16 h-12 bg-muted rounded flex-shrink-0 overflow-hidden">
                            {event.thumbnail_path ? (
                              <img
                                src={event.thumbnail_path}
                                alt="Event"
                                className="w-full h-full object-cover"
                              />
                            ) : (
                              <div className="w-full h-full flex items-center justify-center">
                                <Bell size={16} className="opacity-50" />
                              </div>
                            )}
                          </div>

                          {/* Event details */}
                          <div className="flex-1 min-w-0">
                            <div className="flex items-center justify-between">
                              <span className="font-medium capitalize">
                                {event.label || event.event_type}
                              </span>
                              <span className="text-xs text-muted-foreground">
                                {new Date(event.timestamp * 1000).toLocaleTimeString()}
                              </span>
                            </div>
                            <p className="text-sm text-muted-foreground">
                              Confidence: {Math.round(event.confidence * 100)}%
                            </p>
                          </div>
                        </div>
                      </div>
                    ))}
                  {events.filter((event) => Math.abs(event.timestamp - timelinePosition) < 3600).length === 0 && (
                    <div className="pl-10 text-sm text-muted-foreground py-4">
                      No events near this time. Drag the timeline to explore.
                    </div>
                  )}
                </div>
              </div>
            </div>
          </div>
        )}
      </div>

      {/* Delete Confirmation Modal */}
      {showDeleteConfirm && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-card rounded-lg border p-6 max-w-md w-full mx-4">
            <h2 className="text-xl font-semibold mb-2">Delete Camera</h2>
            <p className="text-muted-foreground mb-4">
              Are you sure you want to delete "{camera.name}"? This action cannot be undone.
            </p>
            <div className="flex gap-3 justify-end">
              <button
                onClick={() => setShowDeleteConfirm(false)}
                className="px-4 py-2 rounded-lg hover:bg-accent transition-colors"
              >
                Cancel
              </button>
              <button
                onClick={() => {
                  console.log('Deleting camera:', id)
                  deleteMutation.mutate()
                }}
                disabled={deleteMutation.isPending}
                className="px-4 py-2 bg-destructive text-destructive-foreground rounded-lg hover:bg-destructive/90 transition-colors disabled:opacity-50"
              >
                {deleteMutation.isPending ? 'Deleting...' : 'Delete'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Settings Modal */}
      {showSettings && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-card rounded-lg border max-w-2xl w-full mx-4 max-h-[90vh] overflow-hidden flex flex-col">
            <div className="flex items-center justify-between p-6 border-b">
              <h2 className="text-xl font-semibold">Camera Settings</h2>
              <button
                onClick={() => setShowSettings(false)}
                className="p-1 rounded hover:bg-accent transition-colors"
              >
                <X size={20} />
              </button>
            </div>

            {/* Tabs */}
            <div className="flex border-b px-6 overflow-x-auto">
              {(['general', 'streams', 'recording', 'detection', 'zones', 'audio'] as SettingsTab[]).map((tab) => (
                <button
                  key={tab}
                  onClick={() => setSettingsTab(tab)}
                  className={`px-4 py-3 text-sm font-medium border-b-2 transition-colors whitespace-nowrap ${
                    settingsTab === tab
                      ? 'border-primary text-primary'
                      : 'border-transparent text-muted-foreground hover:text-foreground'
                  }`}
                >
                  {tab.charAt(0).toUpperCase() + tab.slice(1)}
                </button>
              ))}
            </div>

            <div className="p-6 overflow-y-auto flex-1">
              {/* General Tab */}
              {settingsTab === 'general' && (
                <div className="space-y-4">
                  <div>
                    <label className="block text-sm font-medium mb-1">Name</label>
                    <input
                      type="text"
                      value={formData.name || ''}
                      onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                      className="w-full px-3 py-2 bg-background border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary"
                    />
                  </div>

                  <div className="grid grid-cols-2 gap-4">
                    <div>
                      <label className="block text-sm font-medium mb-1">Manufacturer</label>
                      <input
                        type="text"
                        value={formData.manufacturer || ''}
                        onChange={(e) => setFormData({ ...formData, manufacturer: e.target.value })}
                        className="w-full px-3 py-2 bg-background border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary"
                        placeholder="Hikvision, Reolink, etc."
                      />
                    </div>
                    <div>
                      <label className="block text-sm font-medium mb-1">Model</label>
                      <input
                        type="text"
                        value={formData.model || ''}
                        onChange={(e) => setFormData({ ...formData, model: e.target.value })}
                        className="w-full px-3 py-2 bg-background border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary"
                        placeholder="DS-2CD2385FWD"
                      />
                    </div>
                  </div>

                  <div>
                    <label className="block text-sm font-medium mb-1">Display Aspect Ratio</label>
                    <div className="grid grid-cols-6 gap-2">
                      {(['16:9', '21:9', '4:3', '3:4', '1:1', 'auto'] as const).map((ratio) => (
                        <button
                          key={ratio}
                          type="button"
                          onClick={() => setFormData({ ...formData, display_aspect_ratio: ratio })}
                          className={`px-3 py-2 text-sm rounded-lg border transition-colors ${
                            formData.display_aspect_ratio === ratio
                              ? 'bg-primary text-primary-foreground border-primary'
                              : 'bg-background hover:bg-accent border-border'
                          }`}
                        >
                          {ratio}
                        </button>
                      ))}
                    </div>
                    <p className="text-xs text-muted-foreground mt-1">
                      Controls how the camera appears in the Live View grid
                    </p>
                  </div>
                </div>
              )}

              {/* Streams Tab */}
              {settingsTab === 'streams' && (
                <div className="space-y-6">
                  <div className="space-y-4">
                    <h3 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">Stream URLs</h3>
                    <div>
                      <label className="block text-sm font-medium mb-1">Main Stream URL</label>
                      <input
                        type="text"
                        value={formData.stream?.url || ''}
                        onChange={(e) => setFormData({ ...formData, stream: { ...formData.stream, url: e.target.value } })}
                        className="w-full px-3 py-2 bg-background border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary font-mono text-sm"
                        placeholder="rtsp://user:pass@192.168.1.100:554/stream"
                      />
                      <p className="text-xs text-muted-foreground mt-1">
                        Primary high-resolution stream (typically 4K or 1080p)
                      </p>
                    </div>

                    <div>
                      <label className="block text-sm font-medium mb-1">Sub-stream URL</label>
                      <input
                        type="text"
                        value={formData.stream?.sub_url || ''}
                        onChange={(e) => setFormData({ ...formData, stream: { ...formData.stream, url: formData.stream?.url || '', sub_url: e.target.value } })}
                        className="w-full px-3 py-2 bg-background border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary font-mono text-sm"
                        placeholder="rtsp://user:pass@192.168.1.100:554/substream"
                      />
                      <p className="text-xs text-muted-foreground mt-1">
                        Lower resolution stream for detection and thumbnails (optional)
                      </p>
                    </div>

                    <div className="grid grid-cols-2 gap-4">
                      <div>
                        <label className="block text-sm font-medium mb-1">Username</label>
                        <input
                          type="text"
                          value={formData.stream?.username || ''}
                          onChange={(e) => setFormData({ ...formData, stream: { ...formData.stream, url: formData.stream?.url || '', username: e.target.value } })}
                          className="w-full px-3 py-2 bg-background border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary"
                          placeholder="admin"
                        />
                      </div>
                      <div>
                        <label className="block text-sm font-medium mb-1">Password</label>
                        <input
                          type="password"
                          value={formData.stream?.password || ''}
                          onChange={(e) => setFormData({ ...formData, stream: { ...formData.stream, url: formData.stream?.url || '', password: e.target.value } })}
                          className="w-full px-3 py-2 bg-background border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary"
                          placeholder="********"
                        />
                      </div>
                    </div>
                  </div>

                  <div className="space-y-4 pt-4 border-t">
                    <h3 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">Stream Roles</h3>
                    <p className="text-xs text-muted-foreground">
                      Choose which stream to use for each feature. Use sub-stream for CPU-intensive tasks.
                    </p>

                    {(['detect', 'record', 'audio', 'motion'] as const).map((role) => (
                      <div key={role} className="flex items-center justify-between">
                        <span className="text-sm font-medium capitalize">{role === 'detect' ? 'Object Detection' : role === 'motion' ? 'Motion Detection' : role.charAt(0).toUpperCase() + role.slice(1)}</span>
                        <div className="flex gap-2">
                          {(['main', 'sub'] as const).map((stream) => (
                            <button
                              key={stream}
                              type="button"
                              onClick={() => setFormData({
                                ...formData,
                                stream: {
                                  ...formData.stream,
                                  url: formData.stream?.url || '',
                                  roles: { ...formData.stream?.roles, [role]: stream }
                                }
                              })}
                              disabled={stream === 'sub' && !formData.stream?.sub_url}
                              className={`px-3 py-1.5 text-sm rounded-lg border transition-colors ${
                                formData.stream?.roles?.[role] === stream
                                  ? 'bg-primary text-primary-foreground border-primary'
                                  : 'bg-background hover:bg-accent border-border disabled:opacity-50 disabled:cursor-not-allowed'
                              }`}
                            >
                              {stream.charAt(0).toUpperCase() + stream.slice(1)}
                            </button>
                          ))}
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* Recording Tab */}
              {settingsTab === 'recording' && (
                <div className="space-y-6">
                  <div className="flex items-center justify-between">
                    <div>
                      <h3 className="text-sm font-medium">Enable Recording</h3>
                      <p className="text-xs text-muted-foreground">Record video from this camera</p>
                    </div>
                    <button
                      type="button"
                      onClick={() => setFormData({
                        ...formData,
                        recording: { ...formData.recording, enabled: !formData.recording?.enabled }
                      })}
                      className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                        formData.recording?.enabled ? 'bg-primary' : 'bg-gray-600'
                      }`}
                    >
                      <span
                        className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                          formData.recording?.enabled ? 'translate-x-6' : 'translate-x-1'
                        }`}
                      />
                    </button>
                  </div>

                  {formData.recording?.enabled && (
                    <>
                      <div>
                        <label className="block text-sm font-medium mb-2">Recording Mode</label>
                        <div className="grid grid-cols-3 gap-2">
                          {(['continuous', 'motion', 'events'] as const).map((mode) => (
                            <button
                              key={mode}
                              type="button"
                              onClick={() => setFormData({
                                ...formData,
                                recording: { ...formData.recording, mode }
                              })}
                              className={`px-3 py-2 text-sm rounded-lg border transition-colors ${
                                formData.recording?.mode === mode
                                  ? 'bg-primary text-primary-foreground border-primary'
                                  : 'bg-background hover:bg-accent border-border'
                              }`}
                            >
                              {mode.charAt(0).toUpperCase() + mode.slice(1)}
                            </button>
                          ))}
                        </div>
                        <p className="text-xs text-muted-foreground mt-1">
                          {formData.recording?.mode === 'continuous' && 'Record 24/7 regardless of activity'}
                          {formData.recording?.mode === 'motion' && 'Only record when motion is detected'}
                          {formData.recording?.mode === 'events' && 'Only record when objects are detected'}
                        </p>
                      </div>

                      <div className="grid grid-cols-2 gap-4">
                        <div>
                          <label className="block text-sm font-medium mb-1">Pre-buffer (seconds)</label>
                          <input
                            type="number"
                            min="0"
                            max="30"
                            value={formData.recording?.pre_buffer_seconds ?? 5}
                            onChange={(e) => setFormData({
                              ...formData,
                              recording: { ...formData.recording, pre_buffer_seconds: parseInt(e.target.value) || 0 }
                            })}
                            className="w-full px-3 py-2 bg-background border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary"
                          />
                          <p className="text-xs text-muted-foreground mt-1">
                            Seconds to record before event
                          </p>
                        </div>
                        <div>
                          <label className="block text-sm font-medium mb-1">Post-buffer (seconds)</label>
                          <input
                            type="number"
                            min="0"
                            max="60"
                            value={formData.recording?.post_buffer_seconds ?? 5}
                            onChange={(e) => setFormData({
                              ...formData,
                              recording: { ...formData.recording, post_buffer_seconds: parseInt(e.target.value) || 0 }
                            })}
                            className="w-full px-3 py-2 bg-background border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary"
                          />
                          <p className="text-xs text-muted-foreground mt-1">
                            Seconds to record after event ends
                          </p>
                        </div>
                      </div>

                      <div>
                        <label className="block text-sm font-medium mb-1">Retention (days)</label>
                        <input
                          type="number"
                          min="1"
                          max="365"
                          value={formData.recording?.retention?.default_days ?? 30}
                          onChange={(e) => setFormData({
                            ...formData,
                            recording: {
                              ...formData.recording,
                              retention: {
                                ...formData.recording?.retention,
                                default_days: parseInt(e.target.value) || 30
                              }
                            }
                          })}
                          className="w-full px-3 py-2 bg-background border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary"
                        />
                        <p className="text-xs text-muted-foreground mt-1">
                          How long to keep recordings before auto-deleting
                        </p>
                      </div>
                    </>
                  )}
                </div>
              )}

              {/* Detection Tab */}
              {settingsTab === 'detection' && (
                <div className="space-y-6">
                  <div className="flex items-center justify-between">
                    <div>
                      <h3 className="text-sm font-medium">Enable Object Detection</h3>
                      <p className="text-xs text-muted-foreground">Detect people, vehicles, animals, etc.</p>
                    </div>
                    <button
                      type="button"
                      onClick={() => setFormData({
                        ...formData,
                        detection: { ...formData.detection, enabled: !formData.detection?.enabled }
                      })}
                      className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                        formData.detection?.enabled ? 'bg-primary' : 'bg-gray-600'
                      }`}
                    >
                      <span
                        className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                          formData.detection?.enabled ? 'translate-x-6' : 'translate-x-1'
                        }`}
                      />
                    </button>
                  </div>

                  {formData.detection?.enabled && (
                    <>
                      <div className="flex items-center justify-between">
                        <div>
                          <h3 className="text-sm font-medium">Show Detection Overlay</h3>
                          <p className="text-xs text-muted-foreground">Display bounding boxes on live view</p>
                        </div>
                        <button
                          type="button"
                          onClick={() => setFormData({
                            ...formData,
                            detection: { ...formData.detection, show_overlay: !formData.detection?.show_overlay }
                          })}
                          className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                            formData.detection?.show_overlay ? 'bg-primary' : 'bg-gray-600'
                          }`}
                        >
                          <span
                            className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                              formData.detection?.show_overlay ? 'translate-x-6' : 'translate-x-1'
                            }`}
                          />
                        </button>
                      </div>

                      <div>
                        <label className="block text-sm font-medium mb-1">
                          Minimum Confidence: {Math.round((formData.detection?.min_confidence ?? 0.5) * 100)}%
                        </label>
                        <input
                          type="range"
                          min="0.1"
                          max="0.95"
                          step="0.05"
                          value={formData.detection?.min_confidence ?? 0.5}
                          onChange={(e) => setFormData({
                            ...formData,
                            detection: { ...formData.detection, min_confidence: parseFloat(e.target.value) }
                          })}
                          className="w-full"
                        />
                        <p className="text-xs text-muted-foreground mt-1">
                          Only show detections above this confidence threshold
                        </p>
                      </div>

                      <div className="flex items-center justify-between pt-4 border-t">
                        <div>
                          <h3 className="text-sm font-medium">Enable Motion Detection</h3>
                          <p className="text-xs text-muted-foreground">Skip object detection when no motion</p>
                        </div>
                        <button
                          type="button"
                          onClick={() => setFormData({
                            ...formData,
                            motion: { ...formData.motion, enabled: !formData.motion?.enabled }
                          })}
                          className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                            formData.motion?.enabled ? 'bg-primary' : 'bg-gray-600'
                          }`}
                        >
                          <span
                            className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                              formData.motion?.enabled ? 'translate-x-6' : 'translate-x-1'
                            }`}
                          />
                        </button>
                      </div>

                      {formData.motion?.enabled && (
                        <>
                          <div>
                            <label className="block text-sm font-medium mb-2">Motion Detection Method</label>
                            <div className="grid grid-cols-3 gap-2">
                              {(['frame_diff', 'mog2', 'knn'] as const).map((method) => (
                                <button
                                  key={method}
                                  type="button"
                                  onClick={() => setFormData({
                                    ...formData,
                                    motion: { ...formData.motion, method }
                                  })}
                                  className={`px-3 py-2 text-sm rounded-lg border transition-colors ${
                                    formData.motion?.method === method
                                      ? 'bg-primary text-primary-foreground border-primary'
                                      : 'bg-background hover:bg-accent border-border'
                                  }`}
                                >
                                  {method === 'frame_diff' ? 'Frame Diff' : method.toUpperCase()}
                                </button>
                              ))}
                            </div>
                          </div>

                          <div>
                            <label className="block text-sm font-medium mb-1">
                              Motion Threshold: {Math.round((formData.motion?.threshold ?? 0.02) * 100)}%
                            </label>
                            <input
                              type="range"
                              min="0.005"
                              max="0.1"
                              step="0.005"
                              value={formData.motion?.threshold ?? 0.02}
                              onChange={(e) => setFormData({
                                ...formData,
                                motion: { ...formData.motion, threshold: parseFloat(e.target.value) }
                              })}
                              className="w-full"
                            />
                            <p className="text-xs text-muted-foreground mt-1">
                              Percentage of pixels that must change to trigger motion
                            </p>
                          </div>
                        </>
                      )}
                    </>
                  )}
                </div>
              )}

              {/* Zones Tab */}
              {settingsTab === 'zones' && (
                <div className="space-y-4">
                  <div className="p-3 bg-muted/50 rounded-lg">
                    <p className="text-sm text-muted-foreground">
                      Motion zones let you define specific areas of the camera view to monitor.
                      Only motion or objects detected within these zones will trigger events and recordings.
                    </p>
                  </div>
                  <MotionZoneEditor
                    cameraId={id!}
                    snapshotUrl={cameraApi.getSnapshotUrl(id!)}
                  />
                </div>
              )}

              {/* Audio Tab */}
              {settingsTab === 'audio' && (
                <div className="space-y-6">
                  <div className="flex items-center justify-between">
                    <div>
                      <h3 className="text-sm font-medium">Enable Audio</h3>
                      <p className="text-xs text-muted-foreground">Record and stream audio from camera</p>
                    </div>
                    <button
                      type="button"
                      onClick={() => setFormData({
                        ...formData,
                        audio: { ...formData.audio, enabled: !formData.audio?.enabled }
                      })}
                      className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                        formData.audio?.enabled ? 'bg-primary' : 'bg-gray-600'
                      }`}
                    >
                      <span
                        className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                          formData.audio?.enabled ? 'translate-x-6' : 'translate-x-1'
                        }`}
                      />
                    </button>
                  </div>

                  {formData.audio?.enabled && (
                    <div className="flex items-center justify-between">
                      <div>
                        <h3 className="text-sm font-medium">Two-Way Audio</h3>
                        <p className="text-xs text-muted-foreground">Enable microphone to speak through camera</p>
                      </div>
                      <button
                        type="button"
                        onClick={() => setFormData({
                          ...formData,
                          audio: { ...formData.audio, two_way: !formData.audio?.two_way }
                        })}
                        className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                          formData.audio?.two_way ? 'bg-primary' : 'bg-gray-600'
                        }`}
                      >
                        <span
                          className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                            formData.audio?.two_way ? 'translate-x-6' : 'translate-x-1'
                          }`}
                        />
                      </button>
                    </div>
                  )}

                  <div className="p-4 bg-muted/50 rounded-lg">
                    <p className="text-sm text-muted-foreground">
                      Audio requires camera support and may increase bandwidth usage. Two-way audio requires a compatible camera with speaker capabilities.
                    </p>
                  </div>
                </div>
              )}

              {updateMutation.isError && (
                <div className="p-3 bg-destructive/10 text-destructive rounded-lg text-sm mt-4">
                  Failed to update camera. Please try again.
                </div>
              )}
            </div>

            <div className="flex gap-3 justify-end p-6 border-t">
              <button
                onClick={() => setShowSettings(false)}
                className="px-4 py-2 rounded-lg hover:bg-accent transition-colors"
              >
                Cancel
              </button>
              <button
                onClick={handleSave}
                disabled={updateMutation.isPending}
                className="px-4 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors disabled:opacity-50 flex items-center gap-2"
              >
                {updateMutation.isPending ? (
                  <>
                    <RefreshCw size={16} className="animate-spin" />
                    Saving...
                  </>
                ) : (
                  <>
                    <Save size={16} />
                    Save Changes
                  </>
                )}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
