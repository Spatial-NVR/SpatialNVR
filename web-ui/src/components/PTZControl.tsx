import { useState, useCallback, useRef, useEffect } from 'react'
import {
  ChevronUp,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  ZoomIn,
  ZoomOut,
  Home,
  Crosshair,
  Square,
  Bookmark,
  Save,
  Trash2,
  Keyboard,
} from 'lucide-react'
import { pluginsApi } from '../lib/api'

interface PTZControlProps {
  cameraId: string
  pluginId: string
  presets?: { id: string; name: string }[]
  onPresetSaved?: () => void
  onPresetDeleted?: () => void
}

type PTZAction = 'pan' | 'tilt' | 'zoom' | 'stop' | 'preset'

interface PTZCommand {
  action: PTZAction
  direction?: number
  speed?: number
  preset?: string
}

export function PTZControl({ cameraId, pluginId, presets = [], onPresetSaved, onPresetDeleted }: PTZControlProps) {
  const [isMoving, setIsMoving] = useState(false)
  const [activeDirection, setActiveDirection] = useState<string | null>(null)
  const [speed, setSpeed] = useState(0.5)
  const [showPresetSave, setShowPresetSave] = useState(false)
  const [newPresetName, setNewPresetName] = useState('')
  const [keyboardEnabled, setKeyboardEnabled] = useState(false)
  const [presetToDelete, setPresetToDelete] = useState<string | null>(null)
  const moveTimerRef = useRef<number | null>(null)

  const sendPTZCommand = useCallback(async (cmd: PTZCommand) => {
    try {
      await pluginsApi.rpc(pluginId, 'ptz_control', {
        camera_id: cameraId,
        command: cmd,
      })
    } catch (error) {
      console.error('PTZ command failed:', error)
    }
  }, [cameraId, pluginId])

  const startMovement = useCallback((direction: string, action: PTZAction, dir: number) => {
    setIsMoving(true)
    setActiveDirection(direction)
    sendPTZCommand({ action, direction: dir, speed })

    // Send continuous commands while button is held
    moveTimerRef.current = window.setInterval(() => {
      sendPTZCommand({ action, direction: dir, speed })
    }, 200)
  }, [sendPTZCommand, speed])

  const stopMovement = useCallback(() => {
    if (moveTimerRef.current) {
      clearInterval(moveTimerRef.current)
      moveTimerRef.current = null
    }
    setIsMoving(false)
    setActiveDirection(null)
    sendPTZCommand({ action: 'stop' })
  }, [sendPTZCommand])

  const handleZoom = useCallback((direction: number) => {
    sendPTZCommand({ action: 'zoom', direction, speed })
  }, [sendPTZCommand, speed])

  const goToPreset = useCallback((presetId: string) => {
    sendPTZCommand({ action: 'preset', preset: presetId })
  }, [sendPTZCommand])

  const savePreset = useCallback(async () => {
    if (!newPresetName.trim()) return

    try {
      await pluginsApi.rpc(pluginId, 'ptz_save_preset', {
        camera_id: cameraId,
        name: newPresetName.trim(),
      })
      setNewPresetName('')
      setShowPresetSave(false)
      onPresetSaved?.()
    } catch (error) {
      console.error('Failed to save preset:', error)
    }
  }, [cameraId, pluginId, newPresetName, onPresetSaved])

  const deletePreset = useCallback(async (presetId: string) => {
    try {
      await pluginsApi.rpc(pluginId, 'ptz_delete_preset', {
        camera_id: cameraId,
        preset_id: presetId,
      })
      setPresetToDelete(null)
      onPresetDeleted?.()
    } catch (error) {
      console.error('Failed to delete preset:', error)
    }
  }, [cameraId, pluginId, onPresetDeleted])

  // Keyboard controls
  useEffect(() => {
    if (!keyboardEnabled) return

    const handleKeyDown = (e: KeyboardEvent) => {
      // Only handle if container has focus or keyboard is enabled
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) {
        return
      }

      switch (e.key) {
        case 'ArrowUp':
          e.preventDefault()
          startMovement('up', 'tilt', 1)
          break
        case 'ArrowDown':
          e.preventDefault()
          startMovement('down', 'tilt', -1)
          break
        case 'ArrowLeft':
          e.preventDefault()
          startMovement('left', 'pan', -1)
          break
        case 'ArrowRight':
          e.preventDefault()
          startMovement('right', 'pan', 1)
          break
        case '+':
        case '=':
          e.preventDefault()
          handleZoom(1)
          break
        case '-':
          e.preventDefault()
          handleZoom(-1)
          break
        case 'Escape':
        case ' ':
          e.preventDefault()
          stopMovement()
          break
        case 'h':
        case 'H':
          e.preventDefault()
          goToPreset('home')
          break
      }
    }

    const handleKeyUp = (e: KeyboardEvent) => {
      if (['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight', '+', '=', '-'].includes(e.key)) {
        stopMovement()
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    window.addEventListener('keyup', handleKeyUp)

    return () => {
      window.removeEventListener('keydown', handleKeyDown)
      window.removeEventListener('keyup', handleKeyUp)
    }
  }, [keyboardEnabled, startMovement, stopMovement, handleZoom, goToPreset])

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (moveTimerRef.current) {
        clearInterval(moveTimerRef.current)
      }
    }
  }, [])

  const DirectionButton = ({
    direction,
    action,
    dir,
    icon: Icon,
    className = '',
  }: {
    direction: string
    action: PTZAction
    dir: number
    icon: typeof ChevronUp
    className?: string
  }) => (
    <button
      onMouseDown={() => startMovement(direction, action, dir)}
      onMouseUp={stopMovement}
      onMouseLeave={stopMovement}
      onTouchStart={() => startMovement(direction, action, dir)}
      onTouchEnd={stopMovement}
      className={`p-3 bg-card border rounded-lg hover:bg-accent active:bg-primary active:text-primary-foreground transition-colors ${
        activeDirection === direction ? 'bg-primary text-primary-foreground' : ''
      } ${className}`}
    >
      <Icon size={20} />
    </button>
  )

  return (
    <div className="bg-card border rounded-lg p-4">
      <div className="flex items-center justify-between mb-4">
        <h3 className="font-medium flex items-center gap-2">
          <Crosshair size={16} />
          PTZ Control
        </h3>
        <div className="flex items-center gap-3">
          <button
            onClick={() => setKeyboardEnabled(!keyboardEnabled)}
            className={`p-1.5 rounded transition-colors flex items-center gap-1 text-xs ${
              keyboardEnabled
                ? 'bg-primary text-primary-foreground'
                : 'hover:bg-accent text-muted-foreground'
            }`}
            title={keyboardEnabled ? 'Disable keyboard shortcuts' : 'Enable keyboard shortcuts (arrows, +/-, H for home)'}
          >
            <Keyboard size={14} />
            {keyboardEnabled && <span>On</span>}
          </button>
          <div className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground">Speed</span>
            <input
              type="range"
              min="0.1"
              max="1"
              step="0.1"
              value={speed}
              onChange={(e) => setSpeed(parseFloat(e.target.value))}
              className="w-20"
            />
            <span className="text-xs w-8">{Math.round(speed * 100)}%</span>
          </div>
        </div>
      </div>

      <div className="flex gap-6">
        {/* Directional Pad */}
        <div className="flex flex-col items-center gap-1">
          <DirectionButton direction="up" action="tilt" dir={1} icon={ChevronUp} />
          <div className="flex gap-1">
            <DirectionButton direction="left" action="pan" dir={-1} icon={ChevronLeft} />
            <button
              onClick={stopMovement}
              className="p-3 bg-card border rounded-lg hover:bg-accent transition-colors"
              title="Stop"
            >
              <Square size={20} />
            </button>
            <DirectionButton direction="right" action="pan" dir={1} icon={ChevronRight} />
          </div>
          <DirectionButton direction="down" action="tilt" dir={-1} icon={ChevronDown} />
        </div>

        {/* Zoom Controls */}
        <div className="flex flex-col gap-1">
          <span className="text-xs text-muted-foreground text-center mb-1">Zoom</span>
          <button
            onMouseDown={() => handleZoom(1)}
            onMouseUp={stopMovement}
            onMouseLeave={stopMovement}
            className="p-3 bg-card border rounded-lg hover:bg-accent active:bg-primary active:text-primary-foreground transition-colors"
          >
            <ZoomIn size={20} />
          </button>
          <button
            onMouseDown={() => handleZoom(-1)}
            onMouseUp={stopMovement}
            onMouseLeave={stopMovement}
            className="p-3 bg-card border rounded-lg hover:bg-accent active:bg-primary active:text-primary-foreground transition-colors"
          >
            <ZoomOut size={20} />
          </button>
        </div>

        {/* Presets */}
        <div className="flex-1">
          <div className="flex items-center justify-between mb-2">
            <span className="text-xs text-muted-foreground flex items-center gap-1">
              <Bookmark size={12} />
              Presets
            </span>
            <button
              onClick={() => setShowPresetSave(!showPresetSave)}
              className="p-1 hover:bg-accent rounded transition-colors"
              title="Save current position"
            >
              <Save size={14} />
            </button>
          </div>

          {showPresetSave && (
            <div className="flex gap-2 mb-2">
              <input
                type="text"
                value={newPresetName}
                onChange={(e) => setNewPresetName(e.target.value)}
                placeholder="Preset name..."
                className="flex-1 px-2 py-1 text-sm bg-background border rounded"
                onKeyDown={(e) => e.key === 'Enter' && savePreset()}
              />
              <button
                onClick={savePreset}
                className="px-2 py-1 text-sm bg-primary text-primary-foreground rounded hover:bg-primary/90"
              >
                Save
              </button>
            </div>
          )}

          <div className="flex flex-wrap gap-1">
            <button
              onClick={() => goToPreset('home')}
              className="px-2 py-1 text-xs bg-card border rounded hover:bg-accent transition-colors flex items-center gap-1"
            >
              <Home size={12} />
              Home
            </button>
            {presets.map((preset) => (
              <div key={preset.id} className="group relative">
                <button
                  onClick={() => goToPreset(preset.id)}
                  className="px-2 py-1 text-xs bg-card border rounded hover:bg-accent transition-colors pr-6"
                >
                  {preset.name}
                </button>
                <button
                  onClick={(e) => {
                    e.stopPropagation()
                    setPresetToDelete(preset.id)
                  }}
                  className="absolute right-1 top-1/2 -translate-y-1/2 opacity-0 group-hover:opacity-100 p-0.5 hover:bg-destructive/20 rounded transition-all"
                  title="Delete preset"
                >
                  <Trash2 size={10} className="text-destructive" />
                </button>
              </div>
            ))}
          </div>
        </div>
      </div>

      {/* Delete confirmation dialog */}
      {presetToDelete && (
        <div className="mt-3 p-3 bg-destructive/10 border border-destructive/20 rounded-lg">
          <p className="text-sm text-destructive mb-2">
            Delete preset "{presets.find(p => p.id === presetToDelete)?.name}"?
          </p>
          <div className="flex gap-2">
            <button
              onClick={() => deletePreset(presetToDelete)}
              className="px-2 py-1 text-xs bg-destructive text-destructive-foreground rounded hover:bg-destructive/90"
            >
              Delete
            </button>
            <button
              onClick={() => setPresetToDelete(null)}
              className="px-2 py-1 text-xs border rounded hover:bg-accent"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {isMoving && (
        <div className="mt-2 text-xs text-center text-muted-foreground">
          Moving {activeDirection}...
        </div>
      )}

      {keyboardEnabled && (
        <div className="mt-3 text-xs text-muted-foreground text-center bg-muted/30 py-2 rounded">
          Keyboard: Arrow keys to move, +/- to zoom, H for home, Space/Esc to stop
        </div>
      )}
    </div>
  )
}
