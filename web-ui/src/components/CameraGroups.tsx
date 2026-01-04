import { useState } from 'react'
import { FolderPlus, Folder, ChevronDown, ChevronRight, Edit2, Trash2, Check, X } from 'lucide-react'
import type { Camera } from '../lib/api'

export interface CameraGroup {
  id: string
  name: string
  cameraIds: string[]
  color?: string
}

interface CameraGroupsProps {
  groups: CameraGroup[]
  cameras: Camera[]
  selectedGroupId: string | null
  onSelectGroup: (groupId: string | null) => void
  onCreateGroup: (name: string, cameraIds: string[]) => void
  onUpdateGroup: (id: string, updates: Partial<CameraGroup>) => void
  onDeleteGroup: (id: string) => void
}

export function CameraGroups({
  groups,
  cameras,
  selectedGroupId,
  onSelectGroup,
  onCreateGroup,
  onUpdateGroup,
  onDeleteGroup,
}: CameraGroupsProps) {
  const [expanded, setExpanded] = useState(true)
  const [showCreateDialog, setShowCreateDialog] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editName, setEditName] = useState('')
  const [newGroupName, setNewGroupName] = useState('')
  const [selectedCameras, setSelectedCameras] = useState<string[]>([])

  const handleCreate = () => {
    if (newGroupName.trim() && selectedCameras.length > 0) {
      onCreateGroup(newGroupName.trim(), selectedCameras)
      setNewGroupName('')
      setSelectedCameras([])
      setShowCreateDialog(false)
    }
  }

  const handleStartEdit = (group: CameraGroup) => {
    setEditingId(group.id)
    setEditName(group.name)
  }

  const handleSaveEdit = (id: string) => {
    if (editName.trim()) {
      onUpdateGroup(id, { name: editName.trim() })
    }
    setEditingId(null)
  }

  const toggleCamera = (cameraId: string) => {
    setSelectedCameras(prev =>
      prev.includes(cameraId)
        ? prev.filter(id => id !== cameraId)
        : [...prev, cameraId]
    )
  }

  return (
    <div className="bg-card border rounded-lg">
      {/* Header */}
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center justify-between p-3 hover:bg-accent/50 transition-colors"
      >
        <div className="flex items-center gap-2">
          {expanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
          <span className="font-medium text-sm">Camera Groups</span>
        </div>
        <button
          onClick={(e) => {
            e.stopPropagation()
            setShowCreateDialog(true)
          }}
          className="p-1 hover:bg-accent rounded"
          title="Create Group"
        >
          <FolderPlus size={16} />
        </button>
      </button>

      {expanded && (
        <div className="border-t">
          {/* All Cameras */}
          <button
            onClick={() => onSelectGroup(null)}
            className={`w-full flex items-center gap-2 px-3 py-2 text-sm hover:bg-accent transition-colors ${
              selectedGroupId === null ? 'bg-primary/10 text-primary' : ''
            }`}
          >
            <Folder size={14} />
            <span>All Cameras</span>
            <span className="ml-auto text-xs text-muted-foreground">
              {cameras.length}
            </span>
          </button>

          {/* Groups */}
          {groups.map((group) => (
            <div
              key={group.id}
              className={`flex items-center gap-2 px-3 py-2 text-sm hover:bg-accent transition-colors group ${
                selectedGroupId === group.id ? 'bg-primary/10 text-primary' : ''
              }`}
            >
              {editingId === group.id ? (
                <>
                  <input
                    type="text"
                    value={editName}
                    onChange={(e) => setEditName(e.target.value)}
                    className="flex-1 px-2 py-0.5 bg-background border rounded text-sm"
                    autoFocus
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') handleSaveEdit(group.id)
                      if (e.key === 'Escape') setEditingId(null)
                    }}
                  />
                  <button
                    onClick={() => handleSaveEdit(group.id)}
                    className="p-1 text-green-500 hover:bg-green-500/10 rounded"
                  >
                    <Check size={14} />
                  </button>
                  <button
                    onClick={() => setEditingId(null)}
                    className="p-1 text-muted-foreground hover:bg-accent rounded"
                  >
                    <X size={14} />
                  </button>
                </>
              ) : (
                <>
                  <button
                    onClick={() => onSelectGroup(group.id)}
                    className="flex items-center gap-2 flex-1"
                  >
                    <Folder
                      size={14}
                      style={{ color: group.color }}
                      className={group.color ? '' : 'text-muted-foreground'}
                    />
                    <span>{group.name}</span>
                    <span className="ml-auto text-xs text-muted-foreground">
                      {group.cameraIds.length}
                    </span>
                  </button>
                  <div className="opacity-0 group-hover:opacity-100 flex items-center gap-1 transition-opacity">
                    <button
                      onClick={() => handleStartEdit(group)}
                      className="p-1 hover:bg-accent rounded"
                    >
                      <Edit2 size={12} />
                    </button>
                    <button
                      onClick={() => onDeleteGroup(group.id)}
                      className="p-1 text-destructive hover:bg-destructive/10 rounded"
                    >
                      <Trash2 size={12} />
                    </button>
                  </div>
                </>
              )}
            </div>
          ))}
        </div>
      )}

      {/* Create Dialog */}
      {showCreateDialog && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-card border rounded-lg p-4 w-96 max-h-[80vh] overflow-y-auto">
            <h3 className="font-medium mb-3">Create Camera Group</h3>

            <input
              type="text"
              value={newGroupName}
              onChange={(e) => setNewGroupName(e.target.value)}
              placeholder="Group name..."
              className="w-full px-3 py-2 bg-background border rounded-lg mb-3"
              autoFocus
            />

            <div className="mb-3">
              <label className="block text-sm font-medium mb-2">
                Select Cameras
              </label>
              <div className="space-y-1 max-h-48 overflow-y-auto border rounded-lg p-2">
                {cameras.map((camera) => (
                  <label
                    key={camera.id}
                    className="flex items-center gap-2 p-2 hover:bg-accent rounded cursor-pointer"
                  >
                    <input
                      type="checkbox"
                      checked={selectedCameras.includes(camera.id)}
                      onChange={() => toggleCamera(camera.id)}
                      className="rounded"
                    />
                    <span className="text-sm">{camera.name}</span>
                  </label>
                ))}
              </div>
            </div>

            <div className="flex justify-end gap-2">
              <button
                onClick={() => {
                  setShowCreateDialog(false)
                  setNewGroupName('')
                  setSelectedCameras([])
                }}
                className="px-3 py-1.5 text-sm text-muted-foreground hover:text-foreground"
              >
                Cancel
              </button>
              <button
                onClick={handleCreate}
                disabled={!newGroupName.trim() || selectedCameras.length === 0}
                className="px-3 py-1.5 text-sm bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 disabled:opacity-50"
              >
                Create
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
