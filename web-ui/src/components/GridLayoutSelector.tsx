import { useState } from 'react'
import { Grid2X2, Grid3X3, LayoutGrid, Square, Maximize2, Save, FolderOpen, Trash2, Play, Pause, Settings2 } from 'lucide-react'

export type GridLayout = '1x1' | '2x2' | '3x3' | '4x4' | 'auto'

export interface ViewPreset {
  id: string
  name: string
  layout: GridLayout
  cameraIds: string[]  // Specific cameras to show, empty = all
  groupId?: string
  createdAt: number
}

interface GridLayoutSelectorProps {
  layout: GridLayout
  onLayoutChange: (layout: GridLayout) => void
  presets: ViewPreset[]
  onSavePreset: (name: string) => void
  onLoadPreset: (preset: ViewPreset) => void
  onDeletePreset: (id: string) => void
  tourActive: boolean
  onTourToggle: () => void
  tourInterval: number
  onTourIntervalChange: (seconds: number) => void
}

export function GridLayoutSelector({
  layout,
  onLayoutChange,
  presets,
  onSavePreset,
  onLoadPreset,
  onDeletePreset,
  tourActive,
  onTourToggle,
  tourInterval,
  onTourIntervalChange,
}: GridLayoutSelectorProps) {
  const [showPresetMenu, setShowPresetMenu] = useState(false)
  const [showSaveDialog, setShowSaveDialog] = useState(false)
  const [presetName, setPresetName] = useState('')
  const [showTourSettings, setShowTourSettings] = useState(false)

  const layouts: { value: GridLayout; icon: typeof Grid2X2; label: string }[] = [
    { value: '1x1', icon: Square, label: 'Single' },
    { value: '2x2', icon: Grid2X2, label: '2×2' },
    { value: '3x3', icon: Grid3X3, label: '3×3' },
    { value: '4x4', icon: LayoutGrid, label: '4×4' },
    { value: 'auto', icon: Maximize2, label: 'Auto' },
  ]

  const handleSave = () => {
    if (presetName.trim()) {
      onSavePreset(presetName.trim())
      setPresetName('')
      setShowSaveDialog(false)
    }
  }

  return (
    <div className="flex items-center gap-2">
      {/* Layout buttons */}
      <div className="flex bg-card border rounded-lg p-1 gap-1">
        {layouts.map(({ value, icon: Icon, label }) => (
          <button
            key={value}
            onClick={() => onLayoutChange(value)}
            className={`p-2 rounded transition-colors ${
              layout === value
                ? 'bg-primary text-primary-foreground'
                : 'hover:bg-accent text-muted-foreground hover:text-foreground'
            }`}
            title={label}
          >
            <Icon size={18} />
          </button>
        ))}
      </div>

      {/* Tour mode */}
      <div className="relative">
        <div className="flex bg-card border rounded-lg p-1 gap-1">
          <button
            onClick={onTourToggle}
            className={`p-2 rounded transition-colors ${
              tourActive
                ? 'bg-green-500 text-white'
                : 'hover:bg-accent text-muted-foreground hover:text-foreground'
            }`}
            title={tourActive ? 'Stop Tour' : 'Start Tour'}
          >
            {tourActive ? <Pause size={18} /> : <Play size={18} />}
          </button>
          <button
            onClick={() => setShowTourSettings(!showTourSettings)}
            className="p-2 rounded hover:bg-accent text-muted-foreground hover:text-foreground transition-colors"
            title="Tour Settings"
          >
            <Settings2 size={18} />
          </button>
        </div>

        {showTourSettings && (
          <div className="absolute right-0 mt-2 w-48 bg-card border rounded-lg shadow-lg p-3 z-50">
            <label className="block text-sm font-medium mb-2">
              Interval (seconds)
            </label>
            <input
              type="range"
              min="3"
              max="30"
              value={tourInterval}
              onChange={(e) => onTourIntervalChange(Number(e.target.value))}
              className="w-full"
            />
            <div className="text-center text-sm text-muted-foreground mt-1">
              {tourInterval}s
            </div>
          </div>
        )}
      </div>

      {/* Presets */}
      <div className="relative">
        <div className="flex bg-card border rounded-lg p-1 gap-1">
          <button
            onClick={() => setShowPresetMenu(!showPresetMenu)}
            className="p-2 rounded hover:bg-accent text-muted-foreground hover:text-foreground transition-colors"
            title="Load Preset"
          >
            <FolderOpen size={18} />
          </button>
          <button
            onClick={() => setShowSaveDialog(true)}
            className="p-2 rounded hover:bg-accent text-muted-foreground hover:text-foreground transition-colors"
            title="Save Preset"
          >
            <Save size={18} />
          </button>
        </div>

        {showPresetMenu && (
          <div className="absolute right-0 mt-2 w-56 bg-card border rounded-lg shadow-lg overflow-hidden z-50">
            <div className="p-2 border-b text-sm font-medium text-muted-foreground">
              View Presets
            </div>
            {presets.length === 0 ? (
              <div className="p-4 text-center text-sm text-muted-foreground">
                No saved presets
              </div>
            ) : (
              <div className="max-h-64 overflow-y-auto">
                {presets.map((preset) => (
                  <div
                    key={preset.id}
                    className="flex items-center justify-between p-2 hover:bg-accent group"
                  >
                    <button
                      onClick={() => {
                        onLoadPreset(preset)
                        setShowPresetMenu(false)
                      }}
                      className="flex-1 text-left text-sm"
                    >
                      {preset.name}
                      <span className="text-xs text-muted-foreground ml-2">
                        ({preset.layout})
                      </span>
                    </button>
                    <button
                      onClick={() => onDeletePreset(preset.id)}
                      className="p-1 opacity-0 group-hover:opacity-100 text-destructive hover:bg-destructive/10 rounded transition-opacity"
                    >
                      <Trash2 size={14} />
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}
      </div>

      {/* Save Dialog */}
      {showSaveDialog && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-card border rounded-lg p-4 w-80">
            <h3 className="font-medium mb-3">Save View Preset</h3>
            <input
              type="text"
              value={presetName}
              onChange={(e) => setPresetName(e.target.value)}
              placeholder="Preset name..."
              className="w-full px-3 py-2 bg-background border rounded-lg mb-3"
              autoFocus
              onKeyDown={(e) => e.key === 'Enter' && handleSave()}
            />
            <div className="flex justify-end gap-2">
              <button
                onClick={() => setShowSaveDialog(false)}
                className="px-3 py-1.5 text-sm text-muted-foreground hover:text-foreground"
              >
                Cancel
              </button>
              <button
                onClick={handleSave}
                className="px-3 py-1.5 text-sm bg-primary text-primary-foreground rounded-lg hover:bg-primary/90"
              >
                Save
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
