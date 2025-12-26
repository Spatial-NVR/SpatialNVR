import { useEffect, useRef, useCallback } from 'react'

export interface Detection {
  object_type: string
  label: string
  confidence: number
  bounding_box: {
    x: number
    y: number
    width: number
    height: number
  }
  track_id?: string
}

interface DetectionOverlayProps {
  detections: Detection[]
  visible: boolean
  showLabels?: boolean
  showConfidence?: boolean
  minConfidence?: number
}

// Color mapping for different object types
const OBJECT_COLORS: Record<string, string> = {
  person: '#3B82F6',      // Blue
  vehicle: '#EF4444',     // Red
  car: '#EF4444',
  truck: '#EF4444',
  motorcycle: '#EF4444',
  bicycle: '#F97316',     // Orange
  animal: '#22C55E',      // Green
  dog: '#22C55E',
  cat: '#22C55E',
  bird: '#22C55E',
  face: '#A855F7',        // Purple
  license_plate: '#EAB308', // Yellow
  package: '#14B8A6',     // Teal
  unknown: '#6B7280',     // Gray
}

function getColorForType(objectType: string): string {
  return OBJECT_COLORS[objectType.toLowerCase()] || OBJECT_COLORS.unknown
}

export function DetectionOverlay({
  detections,
  visible,
  showLabels = true,
  showConfidence = true,
  minConfidence = 0,
}: DetectionOverlayProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  const drawDetections = useCallback(() => {
    const canvas = canvasRef.current
    const container = containerRef.current
    if (!canvas || !container) return

    const ctx = canvas.getContext('2d')
    if (!ctx) return

    // Get container dimensions
    const rect = container.getBoundingClientRect()
    const width = rect.width
    const height = rect.height

    // Set canvas size to match container
    if (canvas.width !== width || canvas.height !== height) {
      canvas.width = width
      canvas.height = height
    }

    // Clear canvas
    ctx.clearRect(0, 0, width, height)

    if (!visible) return

    // Filter and draw detections
    const filteredDetections = detections.filter(d => d.confidence >= minConfidence)

    for (const detection of filteredDetections) {
      const { bounding_box: bbox, object_type, label, confidence, track_id } = detection
      const color = getColorForType(object_type)

      // Convert normalized coordinates to pixels
      const x = bbox.x * width
      const y = bbox.y * height
      const w = bbox.width * width
      const h = bbox.height * height

      // Draw bounding box
      ctx.strokeStyle = color
      ctx.lineWidth = 2
      ctx.strokeRect(x, y, w, h)

      // Draw semi-transparent fill
      ctx.fillStyle = color + '20' // 12% opacity
      ctx.fillRect(x, y, w, h)

      const padding = 4
      const textHeight = 14

      // Draw object label at top-left of box
      if (showLabels) {
        const labelText = label || object_type
        ctx.font = 'bold 12px system-ui, -apple-system, sans-serif'
        const textMetrics = ctx.measureText(labelText)
        const labelWidth = textMetrics.width + padding * 2
        const labelHeight = textHeight + padding * 2

        // Position label above box, or inside if no room above
        const labelX = x
        let labelY = y - labelHeight - 2
        if (labelY < 0) {
          labelY = y + 2
        }

        // Draw label background
        ctx.fillStyle = color
        ctx.fillRect(labelX, labelY, labelWidth, labelHeight)

        // Draw label text
        ctx.fillStyle = '#FFFFFF'
        ctx.textBaseline = 'middle'
        ctx.fillText(labelText, labelX + padding, labelY + labelHeight / 2)
      }

      // Draw confidence percentage in bottom-right corner of box
      if (showConfidence) {
        const confText = `${Math.round(confidence * 100)}%`
        ctx.font = 'bold 10px system-ui, -apple-system, sans-serif'
        const confMetrics = ctx.measureText(confText)
        const confWidth = confMetrics.width + padding * 2
        const confHeight = 16

        // Position in bottom-right corner, inside the box
        const confX = x + w - confWidth
        const confY = y + h - confHeight

        // Draw confidence background (semi-transparent)
        ctx.fillStyle = 'rgba(0, 0, 0, 0.6)'
        ctx.fillRect(confX, confY, confWidth, confHeight)

        // Draw confidence text
        ctx.fillStyle = '#FFFFFF'
        ctx.textBaseline = 'middle'
        ctx.fillText(confText, confX + padding, confY + confHeight / 2)
      }

      // Draw track ID at top-right if available
      if (track_id) {
        const trackText = `#${track_id.slice(0, 6)}`
        ctx.font = '10px system-ui, -apple-system, sans-serif'
        const trackMetrics = ctx.measureText(trackText)
        const trackWidth = trackMetrics.width + padding * 2
        const trackHeight = 16

        // Position at top-right corner
        const trackX = x + w - trackWidth
        const trackY = y

        ctx.fillStyle = color + 'CC' // 80% opacity
        ctx.fillRect(trackX, trackY, trackWidth, trackHeight)
        ctx.fillStyle = '#FFFFFF'
        ctx.textBaseline = 'middle'
        ctx.fillText(trackText, trackX + padding, trackY + trackHeight / 2)
      }
    }
  }, [detections, visible, showLabels, showConfidence, minConfidence])

  // Redraw on any changes
  useEffect(() => {
    drawDetections()
  }, [drawDetections])

  // Handle resize
  useEffect(() => {
    const handleResize = () => {
      drawDetections()
    }

    window.addEventListener('resize', handleResize)
    return () => window.removeEventListener('resize', handleResize)
  }, [drawDetections])

  // Use ResizeObserver for container size changes
  useEffect(() => {
    const container = containerRef.current
    if (!container) return

    const resizeObserver = new ResizeObserver(() => {
      drawDetections()
    })

    resizeObserver.observe(container)
    return () => resizeObserver.disconnect()
  }, [drawDetections])

  return (
    <div
      ref={containerRef}
      className="absolute inset-0 pointer-events-none"
      style={{ zIndex: 10 }}
    >
      <canvas
        ref={canvasRef}
        className="w-full h-full"
      />
    </div>
  )
}

export default DetectionOverlay
