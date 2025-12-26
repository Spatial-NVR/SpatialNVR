import { useState, useRef, useEffect, useCallback } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, Trash2, X, Eye, EyeOff } from 'lucide-react'
import { zonesApi, MotionZone, Point, ZoneCreateRequest } from '../lib/api'

// Available colors for zones
const ZONE_COLORS = [
  '#ef4444', // red
  '#f97316', // orange
  '#eab308', // yellow
  '#22c55e', // green
  '#06b6d4', // cyan
  '#3b82f6', // blue
  '#8b5cf6', // purple
  '#ec4899', // pink
]

interface MotionZoneEditorProps {
  cameraId: string
  snapshotUrl: string
}

export function MotionZoneEditor({ cameraId, snapshotUrl }: MotionZoneEditorProps) {
  const queryClient = useQueryClient()
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const imageRef = useRef<HTMLImageElement | null>(null)

  // State
  const [isDrawing, setIsDrawing] = useState(false)
  const [currentPoints, setCurrentPoints] = useState<Point[]>([])
  const [selectedZoneId, setSelectedZoneId] = useState<string | null>(null)
  const [editingZone, setEditingZone] = useState<MotionZone | null>(null)
  const [showZoneSettings, setShowZoneSettings] = useState(false)
  const [newZoneName, setNewZoneName] = useState('')
  const [canvasSize, setCanvasSize] = useState({ width: 640, height: 360 })
  const [imageLoaded, setImageLoaded] = useState(false)

  // Default zone settings for new zones
  const [newZoneSettings, setNewZoneSettings] = useState({
    sensitivity: 5,
    min_confidence: 0.5,
    cooldown_seconds: 30,
    notifications: true,
    recording: true,
    object_types: [] as string[],
  })

  // Fetch zones for this camera
  const { data: zones = [], isLoading } = useQuery({
    queryKey: ['zones', cameraId],
    queryFn: () => zonesApi.list(cameraId),
  })

  // Mutations
  const createMutation = useMutation({
    mutationFn: (zone: ZoneCreateRequest) => zonesApi.create(zone),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['zones', cameraId] })
      setIsDrawing(false)
      setCurrentPoints([])
      setNewZoneName('')
    },
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, zone }: { id: string; zone: Parameters<typeof zonesApi.update>[1] }) =>
      zonesApi.update(id, zone),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['zones', cameraId] })
      setEditingZone(null)
      setSelectedZoneId(null)
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => zonesApi.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['zones', cameraId] })
      setSelectedZoneId(null)
    },
  })

  // Load and resize image
  useEffect(() => {
    const img = new Image()
    img.crossOrigin = 'anonymous'
    img.onload = () => {
      imageRef.current = img
      setImageLoaded(true)

      // Calculate canvas size to fit container while maintaining aspect ratio
      if (containerRef.current) {
        const containerWidth = containerRef.current.clientWidth
        const aspectRatio = img.width / img.height
        const height = containerWidth / aspectRatio
        setCanvasSize({ width: containerWidth, height })
      }
    }
    img.onerror = () => {
      console.error('Failed to load snapshot for zone editor')
      setImageLoaded(false)
    }
    img.src = snapshotUrl
  }, [snapshotUrl])

  // Draw canvas
  const drawCanvas = useCallback(() => {
    const canvas = canvasRef.current
    const ctx = canvas?.getContext('2d')
    const img = imageRef.current

    if (!canvas || !ctx || !img || !imageLoaded) return

    // Clear canvas
    ctx.clearRect(0, 0, canvas.width, canvas.height)

    // Draw snapshot image
    ctx.drawImage(img, 0, 0, canvas.width, canvas.height)

    // Draw existing zones
    zones.forEach((zone) => {
      if (zone.points.length < 3) return

      const color = zone.color || ZONE_COLORS[0]
      const isSelected = zone.id === selectedZoneId
      const isEditing = editingZone?.id === zone.id

      ctx.beginPath()
      zone.points.forEach((point, i) => {
        const x = point.x * canvas.width
        const y = point.y * canvas.height
        if (i === 0) {
          ctx.moveTo(x, y)
        } else {
          ctx.lineTo(x, y)
        }
      })
      ctx.closePath()

      // Fill with transparency
      ctx.fillStyle = isSelected || isEditing
        ? `${color}40`
        : zone.enabled
          ? `${color}20`
          : `${color}10`
      ctx.fill()

      // Stroke
      ctx.strokeStyle = isSelected || isEditing ? color : zone.enabled ? color : '#666'
      ctx.lineWidth = isSelected || isEditing ? 3 : 2
      ctx.setLineDash(zone.enabled ? [] : [5, 5])
      ctx.stroke()
      ctx.setLineDash([])

      // Draw zone name
      if (zone.points.length > 0) {
        const centerX = zone.points.reduce((sum, p) => sum + p.x, 0) / zone.points.length * canvas.width
        const centerY = zone.points.reduce((sum, p) => sum + p.y, 0) / zone.points.length * canvas.height

        ctx.font = '14px sans-serif'
        ctx.textAlign = 'center'
        ctx.textBaseline = 'middle'
        ctx.fillStyle = '#fff'
        ctx.strokeStyle = '#000'
        ctx.lineWidth = 3
        ctx.strokeText(zone.name, centerX, centerY)
        ctx.fillText(zone.name, centerX, centerY)
      }

      // Draw points if editing
      if (isEditing) {
        zone.points.forEach((point) => {
          const x = point.x * canvas.width
          const y = point.y * canvas.height
          ctx.beginPath()
          ctx.arc(x, y, 6, 0, Math.PI * 2)
          ctx.fillStyle = color
          ctx.fill()
          ctx.strokeStyle = '#fff'
          ctx.lineWidth = 2
          ctx.stroke()
        })
      }
    })

    // Draw current points being drawn
    if (currentPoints.length > 0) {
      const color = ZONE_COLORS[zones.length % ZONE_COLORS.length]

      ctx.beginPath()
      currentPoints.forEach((point, i) => {
        const x = point.x * canvas.width
        const y = point.y * canvas.height
        if (i === 0) {
          ctx.moveTo(x, y)
        } else {
          ctx.lineTo(x, y)
        }
      })

      if (currentPoints.length > 2) {
        ctx.closePath()
        ctx.fillStyle = `${color}30`
        ctx.fill()
      }

      ctx.strokeStyle = color
      ctx.lineWidth = 2
      ctx.stroke()

      // Draw points
      currentPoints.forEach((point) => {
        const x = point.x * canvas.width
        const y = point.y * canvas.height
        ctx.beginPath()
        ctx.arc(x, y, 6, 0, Math.PI * 2)
        ctx.fillStyle = color
        ctx.fill()
        ctx.strokeStyle = '#fff'
        ctx.lineWidth = 2
        ctx.stroke()
      })
    }
  }, [zones, currentPoints, selectedZoneId, editingZone, imageLoaded])

  // Redraw on changes
  useEffect(() => {
    drawCanvas()
  }, [drawCanvas, canvasSize])

  // Handle canvas click
  const handleCanvasClick = (e: React.MouseEvent<HTMLCanvasElement>) => {
    const canvas = canvasRef.current
    if (!canvas) return

    const rect = canvas.getBoundingClientRect()
    // Use rect dimensions (display size) for accurate click position
    const x = (e.clientX - rect.left) / rect.width
    const y = (e.clientY - rect.top) / rect.height

    if (isDrawing) {
      // Check if clicking near the first point to close the polygon
      if (currentPoints.length >= 3) {
        const firstPoint = currentPoints[0]
        const dist = Math.sqrt(
          Math.pow((firstPoint.x - x) * rect.width, 2) +
          Math.pow((firstPoint.y - y) * rect.height, 2)
        )
        if (dist < 15) {
          // Close the polygon and prompt for name
          setShowZoneSettings(true)
          return
        }
      }

      // Add new point
      setCurrentPoints([...currentPoints, { x, y }])
    } else {
      // Check if clicking on an existing zone
      const clickedZone = zones.find((zone) => {
        if (zone.points.length < 3) return false
        return isPointInPolygon(
          { x, y },
          zone.points
        )
      })

      if (clickedZone) {
        setSelectedZoneId(clickedZone.id === selectedZoneId ? null : clickedZone.id)
      } else {
        setSelectedZoneId(null)
      }
    }
  }

  // Point in polygon test
  const isPointInPolygon = (point: Point, polygon: Point[]): boolean => {
    let inside = false
    for (let i = 0, j = polygon.length - 1; i < polygon.length; j = i++) {
      const xi = polygon[i].x, yi = polygon[i].y
      const xj = polygon[j].x, yj = polygon[j].y

      if (((yi > point.y) !== (yj > point.y)) &&
          (point.x < (xj - xi) * (point.y - yi) / (yj - yi) + xi)) {
        inside = !inside
      }
    }
    return inside
  }

  // Start drawing mode
  const startDrawing = () => {
    setIsDrawing(true)
    setCurrentPoints([])
    setSelectedZoneId(null)
    setEditingZone(null)
  }

  // Cancel drawing
  const cancelDrawing = () => {
    setIsDrawing(false)
    setCurrentPoints([])
    setShowZoneSettings(false)
    setNewZoneName('')
  }

  // Save new zone
  const saveZone = () => {
    if (currentPoints.length < 3) return
    if (!newZoneName.trim()) return

    const color = ZONE_COLORS[zones.length % ZONE_COLORS.length]

    createMutation.mutate({
      camera_id: cameraId,
      name: newZoneName.trim(),
      enabled: true,
      points: currentPoints,
      color,
      ...newZoneSettings,
    })

    setShowZoneSettings(false)
  }

  // Toggle zone enabled
  const toggleZoneEnabled = (zone: MotionZone) => {
    updateMutation.mutate({
      id: zone.id,
      zone: {
        name: zone.name,
        enabled: !zone.enabled,
        points: zone.points,
        object_types: zone.object_types,
        min_confidence: zone.min_confidence,
        min_size: zone.min_size,
        sensitivity: zone.sensitivity,
        cooldown_seconds: zone.cooldown_seconds,
        notifications: zone.notifications,
        recording: zone.recording,
        color: zone.color,
      },
    })
  }

  // Delete selected zone
  const deleteSelectedZone = () => {
    if (selectedZoneId) {
      deleteMutation.mutate(selectedZoneId)
    }
  }

  const selectedZone = zones.find((z) => z.id === selectedZoneId)

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {/* Toolbar */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          {!isDrawing ? (
            <button
              onClick={startDrawing}
              className="flex items-center gap-2 px-3 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors"
            >
              <Plus size={16} />
              Add Zone
            </button>
          ) : (
            <>
              <button
                onClick={cancelDrawing}
                className="flex items-center gap-2 px-3 py-2 bg-gray-600 text-white rounded-lg hover:bg-gray-700 transition-colors"
              >
                <X size={16} />
                Cancel
              </button>
              <span className="text-sm text-muted-foreground">
                Click to add points. Click first point to close.
              </span>
            </>
          )}
        </div>

        {selectedZone && !isDrawing && (
          <div className="flex items-center gap-2">
            <button
              onClick={() => toggleZoneEnabled(selectedZone)}
              className="flex items-center gap-2 px-3 py-2 rounded-lg hover:bg-accent transition-colors"
              title={selectedZone.enabled ? 'Disable zone' : 'Enable zone'}
            >
              {selectedZone.enabled ? <Eye size={16} /> : <EyeOff size={16} />}
              {selectedZone.enabled ? 'Enabled' : 'Disabled'}
            </button>
            <button
              onClick={deleteSelectedZone}
              disabled={deleteMutation.isPending}
              className="flex items-center gap-2 px-3 py-2 text-destructive hover:bg-destructive/10 rounded-lg transition-colors disabled:opacity-50"
            >
              <Trash2 size={16} />
              Delete
            </button>
          </div>
        )}
      </div>

      {/* Canvas */}
      <div ref={containerRef} className="relative bg-black rounded-lg overflow-hidden">
        <canvas
          ref={canvasRef}
          width={canvasSize.width}
          height={canvasSize.height}
          onClick={handleCanvasClick}
          className={`w-full cursor-${isDrawing ? 'crosshair' : 'pointer'}`}
          style={{ cursor: isDrawing ? 'crosshair' : 'pointer' }}
        />

        {!imageLoaded && (
          <div className="absolute inset-0 flex items-center justify-center bg-gray-900">
            <span className="text-muted-foreground">Loading snapshot...</span>
          </div>
        )}
      </div>

      {/* Zone List */}
      {zones.length > 0 && (
        <div className="space-y-2">
          <h3 className="text-sm font-medium text-muted-foreground">Zones ({zones.length})</h3>
          <div className="grid gap-2">
            {zones.map((zone) => (
              <div
                key={zone.id}
                onClick={() => setSelectedZoneId(zone.id === selectedZoneId ? null : zone.id)}
                className={`flex items-center justify-between p-3 rounded-lg border cursor-pointer transition-colors ${
                  zone.id === selectedZoneId
                    ? 'border-primary bg-primary/5'
                    : 'border-border hover:bg-accent/50'
                }`}
              >
                <div className="flex items-center gap-3">
                  <div
                    className="w-4 h-4 rounded"
                    style={{ backgroundColor: zone.color || ZONE_COLORS[0] }}
                  />
                  <div>
                    <span className="font-medium">{zone.name}</span>
                    <div className="text-xs text-muted-foreground">
                      {zone.points.length} points
                      {!zone.enabled && ' (disabled)'}
                    </div>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  {zone.notifications && (
                    <span className="text-xs px-2 py-0.5 bg-blue-500/20 text-blue-500 rounded">
                      Notify
                    </span>
                  )}
                  {zone.recording && (
                    <span className="text-xs px-2 py-0.5 bg-red-500/20 text-red-500 rounded">
                      Record
                    </span>
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {zones.length === 0 && !isDrawing && (
        <div className="text-center py-8 text-muted-foreground">
          <p>No motion zones defined</p>
          <p className="text-sm mt-1">Click "Add Zone" to create a detection zone</p>
        </div>
      )}

      {/* New Zone Settings Modal */}
      {showZoneSettings && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-card rounded-lg border max-w-md w-full mx-4 p-6">
            <h3 className="text-lg font-semibold mb-4">New Motion Zone</h3>

            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium mb-1">Zone Name</label>
                <input
                  type="text"
                  value={newZoneName}
                  onChange={(e) => setNewZoneName(e.target.value)}
                  placeholder="e.g., Front Porch, Driveway"
                  className="w-full px-3 py-2 bg-background border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary"
                  autoFocus
                />
              </div>

              <div>
                <label className="block text-sm font-medium mb-1">
                  Sensitivity: {newZoneSettings.sensitivity}
                </label>
                <input
                  type="range"
                  min="1"
                  max="10"
                  value={newZoneSettings.sensitivity}
                  onChange={(e) => setNewZoneSettings({ ...newZoneSettings, sensitivity: parseInt(e.target.value) })}
                  className="w-full"
                />
                <p className="text-xs text-muted-foreground">
                  Higher = more sensitive to motion
                </p>
              </div>

              <div>
                <label className="block text-sm font-medium mb-1">
                  Min Confidence: {Math.round(newZoneSettings.min_confidence * 100)}%
                </label>
                <input
                  type="range"
                  min="0.1"
                  max="0.95"
                  step="0.05"
                  value={newZoneSettings.min_confidence}
                  onChange={(e) => setNewZoneSettings({ ...newZoneSettings, min_confidence: parseFloat(e.target.value) })}
                  className="w-full"
                />
              </div>

              <div>
                <label className="block text-sm font-medium mb-1">
                  Cooldown: {newZoneSettings.cooldown_seconds}s
                </label>
                <input
                  type="range"
                  min="5"
                  max="120"
                  step="5"
                  value={newZoneSettings.cooldown_seconds}
                  onChange={(e) => setNewZoneSettings({ ...newZoneSettings, cooldown_seconds: parseInt(e.target.value) })}
                  className="w-full"
                />
                <p className="text-xs text-muted-foreground">
                  Time between events from this zone
                </p>
              </div>

              <div className="flex items-center justify-between">
                <span className="text-sm font-medium">Send Notifications</span>
                <button
                  type="button"
                  onClick={() => setNewZoneSettings({ ...newZoneSettings, notifications: !newZoneSettings.notifications })}
                  className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                    newZoneSettings.notifications ? 'bg-primary' : 'bg-gray-600'
                  }`}
                >
                  <span
                    className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                      newZoneSettings.notifications ? 'translate-x-6' : 'translate-x-1'
                    }`}
                  />
                </button>
              </div>

              <div className="flex items-center justify-between">
                <span className="text-sm font-medium">Trigger Recording</span>
                <button
                  type="button"
                  onClick={() => setNewZoneSettings({ ...newZoneSettings, recording: !newZoneSettings.recording })}
                  className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                    newZoneSettings.recording ? 'bg-primary' : 'bg-gray-600'
                  }`}
                >
                  <span
                    className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                      newZoneSettings.recording ? 'translate-x-6' : 'translate-x-1'
                    }`}
                  />
                </button>
              </div>
            </div>

            <div className="flex gap-3 justify-end mt-6">
              <button
                onClick={cancelDrawing}
                className="px-4 py-2 rounded-lg hover:bg-accent transition-colors"
              >
                Cancel
              </button>
              <button
                onClick={saveZone}
                disabled={!newZoneName.trim() || createMutation.isPending}
                className="px-4 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors disabled:opacity-50"
              >
                {createMutation.isPending ? 'Saving...' : 'Save Zone'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
