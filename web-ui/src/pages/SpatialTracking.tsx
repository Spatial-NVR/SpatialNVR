import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useState, useRef, useCallback, useEffect } from 'react'
import {
  Map,
  Plus,
  Trash2,
  Camera as CameraIcon,
  RotateCw,
  ZoomIn,
  ZoomOut,
  Loader2,
  AlertCircle,
  Target,
  Activity,
  Link2,
  X,
} from 'lucide-react'
import {
  spatialApi,
  cameraApi,
  SpatialMap,
  CameraPlacement,
  CameraPlacementCreate,
  CameraTransition,
  SpatialPoint,
  Camera,
} from '../lib/api'
import { useToast } from '../components/Toast'

// Utility to calculate FOV polygon points
function calculateFOVPolygon(
  position: SpatialPoint,
  rotation: number,
  fovAngle: number,
  fovDepth: number
): SpatialPoint[] {
  const halfAngle = (fovAngle / 2) * (Math.PI / 180)
  const rotationRad = rotation * (Math.PI / 180)

  const leftAngle = rotationRad - halfAngle
  const rightAngle = rotationRad + halfAngle

  return [
    position,
    {
      x: position.x + Math.cos(leftAngle) * fovDepth,
      y: position.y + Math.sin(leftAngle) * fovDepth,
    },
    {
      x: position.x + Math.cos(rightAngle) * fovDepth,
      y: position.y + Math.sin(rightAngle) * fovDepth,
    },
  ]
}

// Get transition type color
function getTransitionColor(type: string): string {
  switch (type) {
    case 'overlap': return '#22c55e' // green
    case 'adjacent': return '#3b82f6' // blue
    case 'gap': return '#f59e0b' // amber
    default: return '#6b7280' // gray
  }
}

// Map List Component
function MapList({
  maps,
  selectedMapId,
  onSelectMap,
  onCreateMap,
  onDeleteMap,
}: {
  maps: SpatialMap[]
  selectedMapId: string | null
  onSelectMap: (id: string) => void
  onCreateMap: () => void
  onDeleteMap: (id: string) => void
}) {
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between mb-3">
        <h3 className="text-sm font-medium text-muted-foreground">Floor Plans</h3>
        <button
          onClick={onCreateMap}
          className="p-1 hover:bg-accent rounded"
          title="Create new map"
        >
          <Plus className="w-4 h-4" />
        </button>
      </div>
      {maps.length === 0 ? (
        <div className="text-sm text-muted-foreground text-center py-4">
          No floor plans yet
        </div>
      ) : (
        maps.map(map => (
          <div
            key={map.id}
            className={`flex items-center justify-between p-2 rounded-lg cursor-pointer transition-colors ${
              selectedMapId === map.id
                ? 'bg-primary/10 border border-primary/30'
                : 'hover:bg-accent'
            }`}
            onClick={() => onSelectMap(map.id)}
          >
            <div className="flex items-center gap-2">
              <Map className="w-4 h-4 text-muted-foreground" />
              <div>
                <div className="text-sm font-medium">{map.name}</div>
                <div className="text-xs text-muted-foreground">
                  {map.width}x{map.height}
                </div>
              </div>
            </div>
            <button
              onClick={(e) => {
                e.stopPropagation()
                onDeleteMap(map.id)
              }}
              className="p-1 hover:bg-destructive/10 hover:text-destructive rounded opacity-0 group-hover:opacity-100"
            >
              <Trash2 className="w-4 h-4" />
            </button>
          </div>
        ))
      )}
    </div>
  )
}

// Create Map Dialog
function CreateMapDialog({
  open,
  onClose,
  onSubmit,
}: {
  open: boolean
  onClose: () => void
  onSubmit: (data: { name: string; width: number; height: number; scale: number }) => void
}) {
  const [name, setName] = useState('')
  const [width, setWidth] = useState(800)
  const [height, setHeight] = useState(600)
  const [scale, setScale] = useState(10)

  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="bg-card border border-border rounded-lg p-6 w-full max-w-md">
        <h2 className="text-lg font-semibold mb-4">Create Floor Plan</h2>
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium mb-1">Name</label>
            <input
              type="text"
              value={name}
              onChange={e => setName(e.target.value)}
              placeholder="e.g., Main Building - Floor 1"
              className="w-full px-3 py-2 bg-background border border-border rounded-md"
            />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium mb-1">Width (px)</label>
              <input
                type="number"
                value={width}
                onChange={e => setWidth(Number(e.target.value))}
                className="w-full px-3 py-2 bg-background border border-border rounded-md"
              />
            </div>
            <div>
              <label className="block text-sm font-medium mb-1">Height (px)</label>
              <input
                type="number"
                value={height}
                onChange={e => setHeight(Number(e.target.value))}
                className="w-full px-3 py-2 bg-background border border-border rounded-md"
              />
            </div>
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">Scale (pixels per meter)</label>
            <input
              type="number"
              value={scale}
              onChange={e => setScale(Number(e.target.value))}
              className="w-full px-3 py-2 bg-background border border-border rounded-md"
            />
            <p className="text-xs text-muted-foreground mt-1">
              Used for distance calculations
            </p>
          </div>
        </div>
        <div className="flex justify-end gap-2 mt-6">
          <button
            onClick={onClose}
            className="px-4 py-2 text-sm hover:bg-accent rounded-md"
          >
            Cancel
          </button>
          <button
            onClick={() => {
              onSubmit({ name, width, height, scale })
              onClose()
            }}
            disabled={!name}
            className="px-4 py-2 text-sm bg-primary text-primary-foreground rounded-md hover:bg-primary/90 disabled:opacity-50"
          >
            Create
          </button>
        </div>
      </div>
    </div>
  )
}

// Camera Sidebar for available cameras
function CameraSidebar({
  cameras,
  placements,
}: {
  cameras: Camera[]
  placements: CameraPlacement[]
}) {
  const placedCameraIds = new Set(placements.map(p => p.camera_id))
  const availableCameras = cameras.filter(c => !placedCameraIds.has(c.id))

  const handleDragStart = useCallback((e: React.DragEvent, camera: Camera) => {
    e.dataTransfer.setData('camera', JSON.stringify(camera))
    e.dataTransfer.effectAllowed = 'move'
  }, [])

  return (
    <div className="space-y-2">
      <h3 className="text-sm font-medium text-muted-foreground mb-3">Available Cameras</h3>
      {availableCameras.length === 0 ? (
        <div className="text-sm text-muted-foreground text-center py-4">
          All cameras placed
        </div>
      ) : (
        availableCameras.map(camera => (
          <div
            key={camera.id}
            draggable
            onDragStart={(e) => handleDragStart(e, camera)}
            className="flex items-center gap-2 p-2 bg-accent/50 rounded-lg cursor-move hover:bg-accent"
          >
            <CameraIcon className="w-4 h-4" />
            <div className="text-sm truncate">{camera.name}</div>
            <div className={`w-2 h-2 rounded-full ml-auto ${
              camera.status === 'online' ? 'bg-green-500' : 'bg-gray-500'
            }`} />
          </div>
        ))
      )}
    </div>
  )
}

// Floor Plan Canvas Component
function FloorPlanCanvas({
  map,
  placements,
  transitions,
  cameras,
  selectedPlacement,
  onSelectPlacement,
  onUpdatePlacement,
  onDropCamera,
  showFOV,
  showTransitions,
}: {
  map: SpatialMap
  placements: CameraPlacement[]
  transitions: CameraTransition[]
  cameras: Camera[]
  selectedPlacement: CameraPlacement | null
  onSelectPlacement: (placement: CameraPlacement | null) => void
  onUpdatePlacement: (id: string, updates: Partial<CameraPlacement>) => void
  onDropCamera: (camera: Camera, position: SpatialPoint) => void
  showFOV: boolean
  showTransitions: boolean
}) {
  const svgRef = useRef<SVGSVGElement>(null)
  const [zoom, setZoom] = useState(1)
  const [pan, setPan] = useState({ x: 0, y: 0 })
  const [isDragging, setIsDragging] = useState(false)
  const [dragStart, setDragStart] = useState({ x: 0, y: 0 })
  const [draggingPlacement, setDraggingPlacement] = useState<string | null>(null)

  const getCameraName = useCallback((cameraId: string) => {
    return cameras.find(c => c.id === cameraId)?.name || cameraId
  }, [cameras])

  const handleWheel = useCallback((e: React.WheelEvent) => {
    e.preventDefault()
    const delta = e.deltaY > 0 ? 0.9 : 1.1
    setZoom(prev => Math.min(Math.max(prev * delta, 0.25), 4))
  }, [])

  const handleMouseDown = useCallback((e: React.MouseEvent) => {
    if (e.button === 1 || (e.button === 0 && e.altKey)) {
      setIsDragging(true)
      setDragStart({ x: e.clientX - pan.x, y: e.clientY - pan.y })
    }
  }, [pan])

  const handleMouseMove = useCallback((e: React.MouseEvent) => {
    if (isDragging) {
      setPan({ x: e.clientX - dragStart.x, y: e.clientY - dragStart.y })
    }
    if (draggingPlacement && svgRef.current) {
      const rect = svgRef.current.getBoundingClientRect()
      const x = (e.clientX - rect.left - pan.x) / zoom
      const y = (e.clientY - rect.top - pan.y) / zoom
      onUpdatePlacement(draggingPlacement, { position: { x, y } })
    }
  }, [isDragging, dragStart, draggingPlacement, pan, zoom, onUpdatePlacement])

  const handleMouseUp = useCallback(() => {
    setIsDragging(false)
    setDraggingPlacement(null)
  }, [])

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    if (!svgRef.current) return

    const rect = svgRef.current.getBoundingClientRect()
    const x = (e.clientX - rect.left - pan.x) / zoom
    const y = (e.clientY - rect.top - pan.y) / zoom

    const cameraData = e.dataTransfer.getData('camera')
    if (cameraData) {
      const camera = JSON.parse(cameraData)
      onDropCamera(camera, { x, y })
    }
  }, [pan, zoom, onDropCamera])

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault()
  }, [])

  return (
    <div className="relative flex-1 bg-accent/20 rounded-lg overflow-hidden">
      {/* Toolbar */}
      <div className="absolute top-4 left-4 z-10 flex gap-2">
        <button
          onClick={() => setZoom(prev => Math.min(prev * 1.2, 4))}
          className="p-2 bg-card border border-border rounded-lg hover:bg-accent"
          title="Zoom in"
        >
          <ZoomIn className="w-4 h-4" />
        </button>
        <button
          onClick={() => setZoom(prev => Math.max(prev * 0.8, 0.25))}
          className="p-2 bg-card border border-border rounded-lg hover:bg-accent"
          title="Zoom out"
        >
          <ZoomOut className="w-4 h-4" />
        </button>
        <button
          onClick={() => { setZoom(1); setPan({ x: 0, y: 0 }) }}
          className="p-2 bg-card border border-border rounded-lg hover:bg-accent"
          title="Reset view"
        >
          <RotateCw className="w-4 h-4" />
        </button>
      </div>

      {/* Zoom indicator */}
      <div className="absolute top-4 right-4 z-10 px-2 py-1 bg-card border border-border rounded text-xs">
        {Math.round(zoom * 100)}%
      </div>

      {/* Canvas */}
      <svg
        ref={svgRef}
        width="100%"
        height="100%"
        onWheel={handleWheel}
        onMouseDown={handleMouseDown}
        onMouseMove={handleMouseMove}
        onMouseUp={handleMouseUp}
        onMouseLeave={handleMouseUp}
        onDrop={handleDrop}
        onDragOver={handleDragOver}
        className="cursor-grab active:cursor-grabbing"
      >
        <g transform={`translate(${pan.x}, ${pan.y}) scale(${zoom})`}>
          {/* Background grid */}
          <defs>
            <pattern id="grid" width="50" height="50" patternUnits="userSpaceOnUse">
              <path d="M 50 0 L 0 0 0 50" fill="none" stroke="currentColor" strokeWidth="0.5" opacity="0.2" />
            </pattern>
          </defs>
          <rect width={map.width} height={map.height} fill="url(#grid)" className="text-muted-foreground" />

          {/* Background image if available */}
          {map.background_image && (
            <image
              href={map.background_image}
              width={map.width}
              height={map.height}
              opacity="0.5"
            />
          )}

          {/* Map boundary */}
          <rect
            width={map.width}
            height={map.height}
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            className="text-border"
          />

          {/* Transitions */}
          {showTransitions && transitions.map(transition => {
            const fromPlacement = placements.find(p => p.camera_id === transition.from_camera_id)
            const toPlacement = placements.find(p => p.camera_id === transition.to_camera_id)
            if (!fromPlacement || !toPlacement) return null

            return (
              <g key={transition.id}>
                <line
                  x1={fromPlacement.position.x}
                  y1={fromPlacement.position.y}
                  x2={toPlacement.position.x}
                  y2={toPlacement.position.y}
                  stroke={getTransitionColor(transition.type)}
                  strokeWidth="2"
                  strokeDasharray={transition.type === 'gap' ? '5,5' : undefined}
                  markerEnd="url(#arrow)"
                />
                {/* Transition label */}
                <text
                  x={(fromPlacement.position.x + toPlacement.position.x) / 2}
                  y={(fromPlacement.position.y + toPlacement.position.y) / 2 - 5}
                  fill={getTransitionColor(transition.type)}
                  fontSize="10"
                  textAnchor="middle"
                >
                  {transition.type}
                </text>
              </g>
            )
          })}

          {/* Arrow marker definition */}
          <defs>
            <marker
              id="arrow"
              markerWidth="10"
              markerHeight="10"
              refX="9"
              refY="3"
              orient="auto"
              markerUnits="strokeWidth"
            >
              <path d="M0,0 L0,6 L9,3 z" fill="currentColor" className="text-muted-foreground" />
            </marker>
          </defs>

          {/* FOV polygons */}
          {showFOV && placements.map(placement => {
            const fovPoints = calculateFOVPolygon(
              placement.position,
              placement.rotation,
              placement.fov_angle,
              placement.fov_depth
            )
            const pointsStr = fovPoints.map(p => `${p.x},${p.y}`).join(' ')
            const isSelected = selectedPlacement?.id === placement.id

            return (
              <polygon
                key={`fov-${placement.id}`}
                points={pointsStr}
                fill={isSelected ? 'rgba(59, 130, 246, 0.2)' : 'rgba(34, 197, 94, 0.15)'}
                stroke={isSelected ? '#3b82f6' : '#22c55e'}
                strokeWidth="1"
              />
            )
          })}

          {/* Camera placements */}
          {placements.map(placement => {
            const isSelected = selectedPlacement?.id === placement.id

            return (
              <g
                key={placement.id}
                transform={`translate(${placement.position.x}, ${placement.position.y})`}
                onClick={() => onSelectPlacement(isSelected ? null : placement)}
                onMouseDown={(e) => {
                  if (e.button === 0 && !e.altKey) {
                    e.stopPropagation()
                    setDraggingPlacement(placement.id)
                  }
                }}
                className="cursor-pointer"
              >
                {/* Camera icon circle */}
                <circle
                  r="15"
                  fill={isSelected ? '#3b82f6' : '#6366f1'}
                  stroke={isSelected ? '#1d4ed8' : '#4f46e5'}
                  strokeWidth="2"
                />
                {/* Direction indicator */}
                <line
                  x1="0"
                  y1="0"
                  x2={Math.cos(placement.rotation * Math.PI / 180) * 20}
                  y2={Math.sin(placement.rotation * Math.PI / 180) * 20}
                  stroke="white"
                  strokeWidth="2"
                  strokeLinecap="round"
                />
                {/* Camera icon */}
                <CameraIcon
                  x="-8"
                  y="-8"
                  width="16"
                  height="16"
                  className="text-white"
                  style={{ pointerEvents: 'none' }}
                />
                {/* Camera name label */}
                <text
                  y="28"
                  textAnchor="middle"
                  fill="currentColor"
                  fontSize="11"
                  className="text-foreground font-medium"
                >
                  {getCameraName(placement.camera_id)}
                </text>
              </g>
            )
          })}
        </g>
      </svg>
    </div>
  )
}

// Camera Placement Editor Panel
function PlacementEditor({
  placement,
  onUpdate,
  onDelete,
  onClose,
}: {
  placement: CameraPlacement
  onUpdate: (updates: Partial<CameraPlacement>) => void
  onDelete: () => void
  onClose: () => void
}) {
  return (
    <div className="w-72 bg-card border-l border-border p-4 space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="font-medium">Camera Settings</h3>
        <button onClick={onClose} className="p-1 hover:bg-accent rounded">
          <X className="w-4 h-4" />
        </button>
      </div>

      <div className="space-y-3">
        <div>
          <label className="block text-sm text-muted-foreground mb-1">Rotation (degrees)</label>
          <input
            type="range"
            min="0"
            max="360"
            value={placement.rotation}
            onChange={e => onUpdate({ rotation: Number(e.target.value) })}
            className="w-full"
          />
          <div className="text-xs text-right">{placement.rotation}°</div>
        </div>

        <div>
          <label className="block text-sm text-muted-foreground mb-1">FOV Angle (degrees)</label>
          <input
            type="range"
            min="10"
            max="180"
            value={placement.fov_angle}
            onChange={e => onUpdate({ fov_angle: Number(e.target.value) })}
            className="w-full"
          />
          <div className="text-xs text-right">{placement.fov_angle}°</div>
        </div>

        <div>
          <label className="block text-sm text-muted-foreground mb-1">FOV Depth (pixels)</label>
          <input
            type="range"
            min="20"
            max="500"
            value={placement.fov_depth}
            onChange={e => onUpdate({ fov_depth: Number(e.target.value) })}
            className="w-full"
          />
          <div className="text-xs text-right">{placement.fov_depth}px</div>
        </div>

        <div className="grid grid-cols-2 gap-2">
          <div>
            <label className="block text-sm text-muted-foreground mb-1">Height (m)</label>
            <input
              type="number"
              step="0.1"
              value={placement.height}
              onChange={e => onUpdate({ height: Number(e.target.value) })}
              className="w-full px-2 py-1 bg-background border border-border rounded text-sm"
            />
          </div>
          <div>
            <label className="block text-sm text-muted-foreground mb-1">Tilt (deg)</label>
            <input
              type="number"
              value={placement.tilt}
              onChange={e => onUpdate({ tilt: Number(e.target.value) })}
              className="w-full px-2 py-1 bg-background border border-border rounded text-sm"
            />
          </div>
        </div>
      </div>

      <button
        onClick={onDelete}
        className="w-full flex items-center justify-center gap-2 px-3 py-2 text-sm text-destructive hover:bg-destructive/10 rounded-md"
      >
        <Trash2 className="w-4 h-4" />
        Remove Camera
      </button>
    </div>
  )
}

// Track State Colors
function getTrackStateColor(state: string): string {
  switch (state) {
    case 'active': return '#22c55e' // green
    case 'transit': return '#f59e0b' // amber
    case 'pending': return '#3b82f6' // blue
    case 'lost': return '#ef4444' // red
    case 'completed': return '#6b7280' // gray
    default: return '#6b7280'
  }
}

// Active Tracks Panel
function ActiveTracksPanel({
  mapId,
  cameras,
}: {
  mapId: string
  cameras: Camera[]
}) {
  const { data: tracks = [], isLoading } = useQuery({
    queryKey: ['spatial-tracks', mapId],
    queryFn: () => spatialApi.listTracks({ map_id: mapId }),
    refetchInterval: 2000, // Refresh every 2 seconds
  })

  const getCameraName = useCallback((cameraId?: string) => {
    if (!cameraId) return 'Unknown'
    return cameras.find(c => c.id === cameraId)?.name || cameraId
  }, [cameras])

  const activeTracks = tracks.filter(t => t.state === 'active' || t.state === 'transit')

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-24">
        <Loader2 className="w-5 h-5 animate-spin" />
      </div>
    )
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium text-muted-foreground">Active Tracks</h3>
        <div className="flex items-center gap-1">
          <Activity className="w-4 h-4 text-green-500" />
          <span className="text-sm">{activeTracks.length}</span>
        </div>
      </div>
      {activeTracks.length === 0 ? (
        <div className="text-sm text-muted-foreground text-center py-4">
          No active tracks
        </div>
      ) : (
        <div className="space-y-2 max-h-48 overflow-auto">
          {activeTracks.map(track => (
            <div
              key={track.id}
              className="p-2 bg-accent/30 rounded-lg text-sm"
            >
              <div className="flex items-center justify-between">
                <span className="font-medium">{track.object_type}</span>
                <span
                  className="px-1.5 py-0.5 rounded text-xs"
                  style={{ backgroundColor: `${getTrackStateColor(track.state)}20`, color: getTrackStateColor(track.state) }}
                >
                  {track.state}
                </span>
              </div>
              <div className="text-xs text-muted-foreground mt-1">
                {track.current_camera_id ? (
                  <>Current: {getCameraName(track.current_camera_id)}</>
                ) : track.expected_camera_id ? (
                  <>Expected: {getCameraName(track.expected_camera_id)}</>
                ) : (
                  <>Cameras visited: {track.total_cameras_visited}</>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// Analytics Panel
function AnalyticsPanel({ mapId }: { mapId: string }) {
  const { data: analytics, isLoading } = useQuery({
    queryKey: ['spatial-analytics', mapId],
    queryFn: () => spatialApi.getAnalytics(mapId),
    refetchInterval: 5000,
  })

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-32">
        <Loader2 className="w-6 h-6 animate-spin" />
      </div>
    )
  }

  if (!analytics) return null

  return (
    <div className="space-y-3">
      <h3 className="text-sm font-medium text-muted-foreground">Analytics</h3>
      <div className="grid grid-cols-2 gap-3">
        <div className="bg-accent/30 rounded-lg p-3">
          <div className="text-2xl font-bold">{analytics.active_tracks}</div>
          <div className="text-xs text-muted-foreground">Active Tracks</div>
        </div>
        <div className="bg-accent/30 rounded-lg p-3">
          <div className="text-2xl font-bold">{analytics.total_tracks}</div>
          <div className="text-xs text-muted-foreground">Total Tracks</div>
        </div>
        <div className="bg-green-500/10 rounded-lg p-3">
          <div className="text-2xl font-bold text-green-500">{analytics.successful_handoffs}</div>
          <div className="text-xs text-muted-foreground">Successful Handoffs</div>
        </div>
        <div className="bg-red-500/10 rounded-lg p-3">
          <div className="text-2xl font-bold text-red-500">{analytics.failed_handoffs}</div>
          <div className="text-xs text-muted-foreground">Failed Handoffs</div>
        </div>
      </div>
      {analytics.average_transit_time > 0 && (
        <div className="text-sm text-muted-foreground">
          Avg transit time: {analytics.average_transit_time.toFixed(1)}s
        </div>
      )}
      {analytics.coverage_gaps.length > 0 && (
        <div className="text-sm text-amber-500">
          Coverage gaps: {analytics.coverage_gaps.join(', ')}
        </div>
      )}
    </div>
  )
}

// Main Component
export function SpatialTracking() {
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const [selectedMapId, setSelectedMapId] = useState<string | null>(null)
  const [selectedPlacement, setSelectedPlacement] = useState<CameraPlacement | null>(null)
  const [showCreateDialog, setShowCreateDialog] = useState(false)
  const [showFOV, setShowFOV] = useState(true)
  const [showTransitions, setShowTransitions] = useState(true)

  // Fetch maps
  const { data: maps = [], isLoading: loadingMaps, error: mapsError } = useQuery({
    queryKey: ['spatial-maps'],
    queryFn: spatialApi.listMaps,
  })

  // Fetch cameras
  const { data: cameras = [] } = useQuery({
    queryKey: ['cameras'],
    queryFn: cameraApi.list,
  })

  // Fetch selected map details
  const { data: selectedMap } = useQuery({
    queryKey: ['spatial-map', selectedMapId],
    queryFn: () => spatialApi.getMap(selectedMapId!),
    enabled: !!selectedMapId,
  })

  // Fetch placements for selected map
  const { data: placements = [] } = useQuery({
    queryKey: ['spatial-placements', selectedMapId],
    queryFn: () => spatialApi.listPlacements(selectedMapId!),
    enabled: !!selectedMapId,
  })

  // Fetch transitions
  const { data: transitions = [] } = useQuery({
    queryKey: ['spatial-transitions', selectedMapId],
    queryFn: () => spatialApi.listTransitions(selectedMapId!),
    enabled: !!selectedMapId,
  })

  // Auto-select first map
  useEffect(() => {
    if (maps.length > 0 && !selectedMapId) {
      setSelectedMapId(maps[0].id)
    }
  }, [maps, selectedMapId])

  // Create map mutation
  const createMap = useMutation({
    mutationFn: (data: { name: string; width: number; height: number; scale: number }) =>
      spatialApi.createMap({
        name: data.name,
        width: data.width,
        height: data.height,
        scale: data.scale,
        metadata: {},
      }),
    onSuccess: (newMap) => {
      addToast('success', `Floor plan "${newMap.name}" created`)
      queryClient.invalidateQueries({ queryKey: ['spatial-maps'] })
      setSelectedMapId(newMap.id)
    },
    onError: (error: Error) => {
      addToast('error', `Failed to create map: ${error.message}`)
    },
  })

  // Delete map mutation
  const deleteMap = useMutation({
    mutationFn: spatialApi.deleteMap,
    onSuccess: () => {
      addToast('success', 'Floor plan deleted')
      queryClient.invalidateQueries({ queryKey: ['spatial-maps'] })
      setSelectedMapId(null)
    },
    onError: (error: Error) => {
      addToast('error', `Failed to delete map: ${error.message}`)
    },
  })

  // Create placement mutation
  const createPlacement = useMutation({
    mutationFn: ({ mapId, placement }: { mapId: string; placement: CameraPlacementCreate }) =>
      spatialApi.createPlacement(mapId, placement),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['spatial-placements', selectedMapId] })
      addToast('success', 'Camera placed')
    },
    onError: (error: Error) => {
      addToast('error', `Failed to place camera: ${error.message}`)
    },
  })

  // Update placement mutation
  const updatePlacement = useMutation({
    mutationFn: ({ placementId, updates }: { placementId: string; updates: Partial<CameraPlacementCreate> }) =>
      spatialApi.updatePlacement(selectedMapId!, placementId, updates),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['spatial-placements', selectedMapId] })
    },
  })

  // Delete placement mutation
  const deletePlacement = useMutation({
    mutationFn: (placementId: string) =>
      spatialApi.deletePlacement(selectedMapId!, placementId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['spatial-placements', selectedMapId] })
      setSelectedPlacement(null)
      addToast('success', 'Camera removed from map')
    },
    onError: (error: Error) => {
      addToast('error', `Failed to remove camera: ${error.message}`)
    },
  })

  // Auto-detect transitions mutation
  const autoDetect = useMutation({
    mutationFn: () => spatialApi.autoDetectTransitions(selectedMapId!),
    onSuccess: (detected) => {
      queryClient.invalidateQueries({ queryKey: ['spatial-transitions', selectedMapId] })
      addToast('success', `Detected ${detected.length} transitions`)
    },
    onError: (error: Error) => {
      addToast('error', `Failed to detect transitions: ${error.message}`)
    },
  })

  const handleDropCamera = useCallback((camera: Camera, position: SpatialPoint) => {
    if (!selectedMapId) return
    createPlacement.mutate({
      mapId: selectedMapId,
      placement: {
        camera_id: camera.id,
        position,
        rotation: 0,
        fov_angle: 90,
        fov_depth: 150,
        height: 3.0,
        tilt: 0,
      },
    })
  }, [selectedMapId, createPlacement])

  if (loadingMaps) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="w-8 h-8 animate-spin text-primary" />
      </div>
    )
  }

  if (mapsError) {
    return (
      <div className="flex flex-col items-center justify-center h-64 gap-4">
        <AlertCircle className="w-12 h-12 text-destructive" />
        <p className="text-muted-foreground">Failed to load spatial tracking data</p>
        <p className="text-sm text-muted-foreground">
          Make sure the Spatial Tracking plugin is running on port 5010
        </p>
      </div>
    )
  }

  return (
    <div className="h-full flex flex-col">
      {/* Header */}
      <div className="flex items-center justify-between mb-4">
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <Target className="w-6 h-6" />
            Spatial Tracking
          </h1>
          <p className="text-muted-foreground mt-1">
            Configure camera positions and track objects across cameras
          </p>
        </div>
        <div className="flex items-center gap-2">
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={showFOV}
              onChange={e => setShowFOV(e.target.checked)}
              className="rounded"
            />
            Show FOV
          </label>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={showTransitions}
              onChange={e => setShowTransitions(e.target.checked)}
              className="rounded"
            />
            Show Transitions
          </label>
          {selectedMapId && (
            <button
              onClick={() => autoDetect.mutate()}
              disabled={autoDetect.isPending}
              className="flex items-center gap-2 px-3 py-1.5 bg-primary text-primary-foreground rounded-md text-sm hover:bg-primary/90 disabled:opacity-50"
            >
              {autoDetect.isPending ? (
                <Loader2 className="w-4 h-4 animate-spin" />
              ) : (
                <Link2 className="w-4 h-4" />
              )}
              Auto-Detect Transitions
            </button>
          )}
        </div>
      </div>

      {/* Main content */}
      <div className="flex-1 flex gap-4 min-h-0">
        {/* Left sidebar - Maps and Cameras */}
        <div className="w-64 flex flex-col gap-4 shrink-0">
          <div className="bg-card border border-border rounded-lg p-4">
            <MapList
              maps={maps}
              selectedMapId={selectedMapId}
              onSelectMap={setSelectedMapId}
              onCreateMap={() => setShowCreateDialog(true)}
              onDeleteMap={(id) => deleteMap.mutate(id)}
            />
          </div>
          {selectedMapId && (
            <div className="bg-card border border-border rounded-lg p-4 flex-1 overflow-auto">
              <CameraSidebar
                cameras={cameras}
                placements={placements}
              />
            </div>
          )}
        </div>

        {/* Center - Floor plan canvas */}
        <div className="flex-1 flex flex-col min-w-0">
          {selectedMap ? (
            <FloorPlanCanvas
              map={selectedMap}
              placements={placements}
              transitions={transitions}
              cameras={cameras}
              selectedPlacement={selectedPlacement}
              onSelectPlacement={setSelectedPlacement}
              onUpdatePlacement={(id, updates) => updatePlacement.mutate({ placementId: id, updates })}
              onDropCamera={handleDropCamera}
              showFOV={showFOV}
              showTransitions={showTransitions}
            />
          ) : (
            <div className="flex-1 flex flex-col items-center justify-center bg-accent/20 rounded-lg">
              <Map className="w-16 h-16 text-muted-foreground mb-4" />
              <p className="text-lg font-medium">No floor plan selected</p>
              <p className="text-muted-foreground mb-4">
                Create a floor plan to get started
              </p>
              <button
                onClick={() => setShowCreateDialog(true)}
                className="flex items-center gap-2 px-4 py-2 bg-primary text-primary-foreground rounded-md"
              >
                <Plus className="w-4 h-4" />
                Create Floor Plan
              </button>
            </div>
          )}
        </div>

        {/* Right sidebar - Placement editor or Analytics */}
        {selectedPlacement ? (
          <PlacementEditor
            placement={selectedPlacement}
            onUpdate={(updates) => {
              updatePlacement.mutate({
                placementId: selectedPlacement.id,
                updates,
              })
              setSelectedPlacement({ ...selectedPlacement, ...updates } as CameraPlacement)
            }}
            onDelete={() => deletePlacement.mutate(selectedPlacement.id)}
            onClose={() => setSelectedPlacement(null)}
          />
        ) : selectedMapId ? (
          <div className="w-72 flex flex-col gap-4 shrink-0">
            <div className="bg-card border border-border rounded-lg p-4">
              <AnalyticsPanel mapId={selectedMapId} />
            </div>
            <div className="bg-card border border-border rounded-lg p-4 flex-1 overflow-auto">
              <ActiveTracksPanel
                mapId={selectedMapId}
                cameras={cameras}
              />
            </div>
          </div>
        ) : null}
      </div>

      {/* Create Map Dialog */}
      <CreateMapDialog
        open={showCreateDialog}
        onClose={() => setShowCreateDialog(false)}
        onSubmit={(data) => createMap.mutate(data)}
      />
    </div>
  )
}
