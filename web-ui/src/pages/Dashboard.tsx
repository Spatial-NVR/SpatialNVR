import { useQuery } from '@tanstack/react-query'
import { Camera, Plus, AlertTriangle, User, Car, Package, ServerOff, RefreshCw, LayoutDashboard } from 'lucide-react'
import { cameraApi, eventsApi, type Camera as CameraType, type Event, ApiError } from '../lib/api'
import { VideoPlayer } from '../components/VideoPlayer'
import { GridLayoutSelector } from '../components/GridLayoutSelector'
import { CameraGroups } from '../components/CameraGroups'
import { useViewState } from '../hooks/useViewState'
import { memo, useState } from 'react'
import { usePorts } from '../hooks/usePorts'

export function Dashboard() {
  const [showSidebar, setShowSidebar] = useState(false)

  const { data: cameras, isLoading: camerasLoading, error: camerasError, refetch: refetchCameras } = useQuery({
    queryKey: ['cameras'],
    queryFn: cameraApi.list,
    refetchInterval: 30000,
    staleTime: 25000,
    retry: 2,
  })

  const { data: eventsData, isLoading: eventsLoading, error: eventsError } = useQuery({
    queryKey: ['events', { limit: 20 }],
    queryFn: () => eventsApi.list({ per_page: 20 }),
    refetchInterval: 10000,
    staleTime: 8000,
    retry: 2,
  })

  const cameraIds = cameras?.map(c => c.id) ?? []

  const {
    layout,
    setLayout,
    getGridClasses,
    getVisibleCameras,
    presets,
    savePreset,
    loadPreset,
    deletePreset,
    groups,
    selectedGroupId,
    setSelectedGroupId,
    createGroup,
    updateGroup,
    deleteGroup,
    tourActive,
    toggleTour,
    tourInterval,
    setTourInterval,
  } = useViewState(cameraIds)

  const isLoading = camerasLoading || eventsLoading
  const events = (eventsData?.data ?? []).filter(e => e.event_type !== 'state_change')
  const hasCameras = cameras && cameras.length > 0

  const visibleCameraIds = getVisibleCameras()
  const visibleCameras = cameras?.filter(c => visibleCameraIds.includes(c.id)) ?? []

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
    <div className="flex gap-4">
      {/* Sidebar for groups (collapsible on mobile) */}
      {hasCameras && (
        <div className={`${showSidebar ? 'block' : 'hidden'} lg:block w-64 flex-shrink-0`}>
          <CameraGroups
            groups={groups}
            cameras={cameras}
            selectedGroupId={selectedGroupId}
            onSelectGroup={setSelectedGroupId}
            onCreateGroup={createGroup}
            onUpdateGroup={updateGroup}
            onDeleteGroup={deleteGroup}
          />
        </div>
      )}

      {/* Main content */}
      <div className="flex-1 space-y-4">
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

        {/* Header with controls */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div className="flex items-center gap-4">
            {/* Mobile sidebar toggle */}
            <button
              onClick={() => setShowSidebar(!showSidebar)}
              className="lg:hidden p-2 bg-card border rounded-lg hover:bg-accent transition-colors"
            >
              <LayoutDashboard size={18} />
            </button>

            <div>
              <h1 className="text-2xl font-bold flex items-center gap-2">
                Live View
                {tourActive && (
                  <span className="text-sm font-normal text-green-500 animate-pulse">
                    Tour Active
                  </span>
                )}
              </h1>
              <p className="text-sm text-muted-foreground">
                {hasCameras
                  ? selectedGroupId
                    ? `${visibleCameras.length} cameras in group`
                    : `${cameras.filter(c => c.status === 'online').length} of ${cameras.length} cameras online`
                  : 'No cameras configured'
                }
              </p>
            </div>
          </div>

          {hasCameras && (
            <div className="flex items-center gap-3 flex-wrap">
              <GridLayoutSelector
                layout={layout}
                onLayoutChange={setLayout}
                presets={presets}
                onSavePreset={savePreset}
                onLoadPreset={loadPreset}
                onDeletePreset={deletePreset}
                tourActive={tourActive}
                onTourToggle={toggleTour}
                tourInterval={tourInterval}
                onTourIntervalChange={setTourInterval}
              />

              <a
                href="/cameras/add"
                className="inline-flex items-center gap-2 px-4 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors text-sm"
              >
                <Plus size={18} />
                Add Camera
              </a>
            </div>
          )}
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
        ) : visibleCameras.length === 0 ? (
          <div className="bg-card rounded-lg border p-12">
            <div className="text-center">
              <Camera className="h-16 w-16 mx-auto mb-4 text-muted-foreground opacity-50" />
              <h2 className="text-xl font-semibold mb-2">No cameras in this view</h2>
              <p className="text-muted-foreground mb-4">
                Select a different group or adjust the layout
              </p>
              <button
                onClick={() => setSelectedGroupId(null)}
                className="inline-flex items-center gap-2 px-4 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors"
              >
                Show All Cameras
              </button>
            </div>
          </div>
        ) : (
          <div className={`grid ${getGridClasses()} gap-1 bg-black/50 rounded-lg overflow-hidden`}>
            {visibleCameras.map((camera) => (
              <CameraCard
                key={camera.id}
                camera={camera}
                isFullscreen={tourActive || layout === '1x1'}
              />
            ))}
          </div>
        )}
      </div>
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

const CameraCard = memo(function CameraCard({
  camera,
  isFullscreen = false
}: {
  camera: CameraType
  isFullscreen?: boolean
}) {
  const statusColors = {
    online: 'bg-green-500',
    offline: 'bg-gray-500',
    error: 'bg-red-500',
    starting: 'bg-yellow-500',
  }

  const getAspectClass = () => {
    if (isFullscreen) {
      return 'aspect-video md:aspect-[21/9]'
    }

    const aspectClasses: Record<string, string> = {
      '16:9': 'aspect-video',
      '21:9': 'aspect-[21/9]',
      '4:3': 'aspect-[4/3]',
      '1:1': 'aspect-square',
      'auto': 'aspect-video',
    }
    return aspectClasses[camera.display_aspect_ratio || '16:9'] || 'aspect-video'
  }

  return (
    <a
      href={`/cameras/${camera.id}`}
      className={`block ${getAspectClass()} bg-card rounded-lg border overflow-hidden hover:ring-2 hover:ring-primary transition-all ${
        isFullscreen ? 'min-h-[300px]' : ''
      }`}
    >
      <div className="relative h-full">
        {camera.status === 'online' ? (
          <VideoPlayer
            cameraId={camera.id}
            className="h-full"
            fit="cover"
            audioEnabled={isFullscreen}
          />
        ) : (
          <div className="absolute inset-0 bg-black flex items-center justify-center">
            <Camera className="h-10 w-10 text-gray-600" />
          </div>
        )}

        <div className="absolute bottom-0 left-0 right-0 bg-gradient-to-t from-black/80 to-transparent p-3 z-10">
          <div className="flex items-center justify-between">
            <div className="min-w-0">
              <div className={`font-medium text-white truncate ${isFullscreen ? 'text-lg' : 'text-sm'}`}>
                {camera.name}
              </div>
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
  return prevProps.camera.id === nextProps.camera.id &&
         prevProps.camera.status === nextProps.camera.status &&
         prevProps.camera.name === nextProps.camera.name &&
         prevProps.isFullscreen === nextProps.isFullscreen
})
