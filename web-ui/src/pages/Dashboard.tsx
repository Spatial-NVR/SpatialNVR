import { useQuery } from '@tanstack/react-query'
import { Camera, Plus, AlertTriangle, User, Car, Package, ServerOff, RefreshCw } from 'lucide-react'
import { cameraApi, eventsApi, type Camera as CameraType, type Event, ApiError } from '../lib/api'
import { VideoPlayer } from '../components/VideoPlayer'
import { memo } from 'react'
import { usePorts } from '../hooks/usePorts'

export function Dashboard() {
  const { data: cameras, isLoading: camerasLoading, error: camerasError, refetch: refetchCameras } = useQuery({
    queryKey: ['cameras'],
    queryFn: cameraApi.list,
    refetchInterval: 30000, // Reduced frequency to prevent video restarts
    staleTime: 25000,
    retry: 2,
  })

  const { data: eventsData, isLoading: eventsLoading, error: eventsError } = useQuery({
    queryKey: ['events', { limit: 20 }],
    queryFn: () => eventsApi.list({ per_page: 20 }),
    refetchInterval: 10000, // Reduced from 5s
    staleTime: 8000,
    retry: 2,
  })

  const isLoading = camerasLoading || eventsLoading
  const events = eventsData?.data ?? []
  const hasCameras = cameras && cameras.length > 0

  // Check for connection errors
  const isConnectionError = (error: unknown): boolean => {
    if (!error) return false
    if (error instanceof ApiError && error.code === 'NETWORK_ERROR') return true
    if (error instanceof TypeError && error.message.includes('fetch')) return true
    return false
  }

  const hasBackendError = isConnectionError(camerasError) || isConnectionError(eventsError)

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
      </div>
    )
  }

  // Show backend connection error
  if (hasBackendError) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="bg-card rounded-lg border border-destructive/50 p-8 max-w-md text-center">
          <ServerOff className="h-12 w-12 mx-auto mb-4 text-destructive" />
          <h2 className="text-xl font-semibold mb-2">Backend Not Running</h2>
          <p className="text-muted-foreground mb-4">
            Cannot connect to the NVR backend server. Please ensure the backend is running on port 12000.
          </p>
          <div className="text-sm text-muted-foreground mb-4 font-mono bg-muted p-2 rounded">
            CONFIG_PATH=./data/config.yaml DATA_PATH=./data ./nvr
          </div>
          <button
            onClick={() => refetchCameras()}
            className="inline-flex items-center gap-2 px-4 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors"
          >
            <RefreshCw size={16} />
            Retry Connection
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Event Ticker */}
      <div className="bg-card rounded-lg border">
        <div className="flex items-center justify-between px-4 py-2 border-b">
          <h2 className="text-sm font-medium text-muted-foreground">Recent Activity</h2>
          <a href="/events" className="text-xs text-primary hover:underline">View all</a>
        </div>
        <div className="overflow-x-auto">
          {events.length === 0 ? (
            <div className="px-4 py-6 text-center text-muted-foreground text-sm">
              No recent events
            </div>
          ) : (
            <div className="flex gap-2 p-3 min-w-max">
              {events.slice(0, 10).map((event) => (
                <EventCard key={event.id} event={event} />
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Live View</h1>
          <p className="text-sm text-muted-foreground">
            {hasCameras
              ? `${cameras.filter(c => c.status === 'online').length} of ${cameras.length} cameras online`
              : 'No cameras configured'
            }
          </p>
        </div>
        <a
          href="/cameras/add"
          className="inline-flex items-center gap-2 px-4 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors text-sm"
        >
          <Plus size={18} />
          Add Camera
        </a>
      </div>

      {/* Camera Grid */}
      {!hasCameras ? (
        <div className="bg-card rounded-lg border p-12">
          <div className="text-center">
            <Camera className="h-16 w-16 mx-auto mb-4 text-muted-foreground opacity-50" />
            <h2 className="text-xl font-semibold mb-2">No cameras yet</h2>
            <p className="text-muted-foreground mb-4">
              Add your first camera to start monitoring
            </p>
            <a
              href="/cameras/add"
              className="inline-flex items-center gap-2 px-4 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors"
            >
              <Plus size={20} />
              Add Camera
            </a>
          </div>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-0.5 auto-rows-auto bg-black/50 rounded-lg overflow-hidden">
          {cameras.map((camera) => (
            <CameraCard key={camera.id} camera={camera} />
          ))}
        </div>
      )}
    </div>
  )
}

function EventCard({ event }: { event: Event }) {
  const { apiUrl } = usePorts()

  const getEventIcon = (type: string) => {
    switch (type) {
      case 'person': return User
      case 'vehicle': case 'car': return Car
      case 'motion': return AlertTriangle
      default: return Package
    }
  }

  const Icon = getEventIcon(event.event_type)
  const time = new Date(event.timestamp * 1000)
  const timeStr = time.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })

  return (
    <a
      href={`/events?id=${event.id}`}
      className="flex-shrink-0 w-32 bg-background rounded-lg border p-2 hover:border-primary transition-colors"
    >
      <div className="aspect-video bg-black rounded mb-2 flex items-center justify-center overflow-hidden">
        {event.thumbnail_path ? (
          <img
            src={`${apiUrl}${event.thumbnail_path}`}
            alt=""
            className="w-full h-full object-cover"
            onError={(e) => {
              (e.target as HTMLImageElement).style.display = 'none'
            }}
          />
        ) : (
          <Icon className="h-6 w-6 text-gray-600" />
        )}
      </div>
      <div className="space-y-0.5">
        <div className="flex items-center gap-1">
          <Icon className="h-3 w-3 text-muted-foreground" />
          <span className="text-xs font-medium capitalize truncate">{event.event_type}</span>
        </div>
        <div className="text-xs text-muted-foreground">{timeStr}</div>
      </div>
    </a>
  )
}

const CameraCard = memo(function CameraCard({ camera }: { camera: CameraType }) {
  const statusColors = {
    online: 'bg-green-500',
    offline: 'bg-gray-500',
    error: 'bg-red-500',
    starting: 'bg-yellow-500',
  }

  const aspectClasses: Record<string, string> = {
    '16:9': 'aspect-video',
    '21:9': 'aspect-[21/9]',
    '4:3': 'aspect-[4/3]',
    '1:1': 'aspect-square',
    'auto': 'aspect-video', // fallback for auto
  }

  const aspectClass = aspectClasses[camera.display_aspect_ratio || '16:9'] || 'aspect-video'

  return (
    <a
      href={`/cameras/${camera.id}`}
      className={`block ${aspectClass} bg-card rounded-lg border overflow-hidden hover:ring-2 hover:ring-primary transition-all`}
    >
      <div className="relative h-full">
        {camera.status === 'online' ? (
          <VideoPlayer cameraId={camera.id} className="h-full" fit="cover" />
        ) : (
          <div className="absolute inset-0 bg-black flex items-center justify-center">
            <Camera className="h-10 w-10 text-gray-600" />
          </div>
        )}

        <div className="absolute bottom-0 left-0 right-0 bg-gradient-to-t from-black/80 to-transparent p-2 z-10">
          <div className="flex items-center justify-between">
            <div className="min-w-0">
              <div className="font-medium text-white text-sm truncate">{camera.name}</div>
            </div>
            <div className="flex items-center gap-2 flex-shrink-0">
              {camera.fps_current && (
                <span className="text-xs text-gray-300">{camera.fps_current.toFixed(0)}fps</span>
              )}
              <div className={`w-2 h-2 rounded-full ${statusColors[camera.status]}`} />
            </div>
          </div>
        </div>
      </div>
    </a>
  )
}, (prevProps, nextProps) => {
  // Only re-render if essential props change
  return prevProps.camera.id === nextProps.camera.id &&
         prevProps.camera.status === nextProps.camera.status &&
         prevProps.camera.name === nextProps.camera.name
})
