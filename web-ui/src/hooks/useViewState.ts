import { useState, useEffect, useCallback, useRef } from 'react'
import type { GridLayout, ViewPreset } from '../components/GridLayoutSelector'
import type { CameraGroup } from '../components/CameraGroups'

const STORAGE_KEY_LAYOUT = 'nvr-grid-layout'
const STORAGE_KEY_PRESETS = 'nvr-view-presets'
const STORAGE_KEY_GROUPS = 'nvr-camera-groups'
const STORAGE_KEY_TOUR = 'nvr-tour-settings'

interface TourSettings {
  interval: number
  lastCameraIndex: number
}

export function useViewState(cameraIds: string[]) {
  // Layout state
  const [layout, setLayout] = useState<GridLayout>(() => {
    const saved = localStorage.getItem(STORAGE_KEY_LAYOUT)
    return (saved as GridLayout) || 'auto'
  })

  // Presets
  const [presets, setPresets] = useState<ViewPreset[]>(() => {
    try {
      const saved = localStorage.getItem(STORAGE_KEY_PRESETS)
      return saved ? JSON.parse(saved) : []
    } catch {
      return []
    }
  })

  // Groups
  const [groups, setGroups] = useState<CameraGroup[]>(() => {
    try {
      const saved = localStorage.getItem(STORAGE_KEY_GROUPS)
      return saved ? JSON.parse(saved) : []
    } catch {
      return []
    }
  })

  const [selectedGroupId, setSelectedGroupId] = useState<string | null>(null)

  // Tour mode
  const [tourActive, setTourActive] = useState(false)
  const [tourSettings, setTourSettings] = useState<TourSettings>(() => {
    try {
      const saved = localStorage.getItem(STORAGE_KEY_TOUR)
      return saved ? JSON.parse(saved) : { interval: 5, lastCameraIndex: 0 }
    } catch {
      return { interval: 5, lastCameraIndex: 0 }
    }
  })
  const [tourCameraIndex, setTourCameraIndex] = useState(0)
  const tourTimerRef = useRef<number | null>(null)

  // Persist layout
  useEffect(() => {
    localStorage.setItem(STORAGE_KEY_LAYOUT, layout)
  }, [layout])

  // Persist presets
  useEffect(() => {
    localStorage.setItem(STORAGE_KEY_PRESETS, JSON.stringify(presets))
  }, [presets])

  // Persist groups
  useEffect(() => {
    localStorage.setItem(STORAGE_KEY_GROUPS, JSON.stringify(groups))
  }, [groups])

  // Persist tour settings
  useEffect(() => {
    localStorage.setItem(STORAGE_KEY_TOUR, JSON.stringify(tourSettings))
  }, [tourSettings])

  // Tour timer
  useEffect(() => {
    if (tourActive && cameraIds.length > 1) {
      tourTimerRef.current = window.setInterval(() => {
        setTourCameraIndex((prev) => (prev + 1) % cameraIds.length)
      }, tourSettings.interval * 1000)

      return () => {
        if (tourTimerRef.current) {
          clearInterval(tourTimerRef.current)
        }
      }
    }
  }, [tourActive, tourSettings.interval, cameraIds.length])

  // Get visible cameras based on layout, group, and tour mode
  const getVisibleCameras = useCallback((): string[] => {
    let cameras = cameraIds

    // Filter by group
    if (selectedGroupId) {
      const group = groups.find(g => g.id === selectedGroupId)
      if (group) {
        cameras = cameras.filter(id => group.cameraIds.includes(id))
      }
    }

    // If tour mode, show only current camera
    if (tourActive && cameras.length > 0) {
      const safeIndex = tourCameraIndex % cameras.length
      return [cameras[safeIndex]]
    }

    // Limit by layout
    const layoutLimits: Record<GridLayout, number> = {
      '1x1': 1,
      '2x2': 4,
      '3x3': 9,
      '4x4': 16,
      'auto': Infinity,
    }

    const limit = layoutLimits[layout]
    return cameras.slice(0, limit)
  }, [cameraIds, selectedGroupId, groups, tourActive, tourCameraIndex, layout])

  // Get grid CSS classes based on layout
  const getGridClasses = useCallback((): string => {
    if (tourActive) {
      return 'grid-cols-1'
    }

    switch (layout) {
      case '1x1':
        return 'grid-cols-1'
      case '2x2':
        return 'grid-cols-1 md:grid-cols-2'
      case '3x3':
        return 'grid-cols-1 md:grid-cols-2 lg:grid-cols-3'
      case '4x4':
        return 'grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4'
      case 'auto':
      default:
        return 'grid-cols-1 md:grid-cols-2 xl:grid-cols-3'
    }
  }, [layout, tourActive])

  // Preset management
  const savePreset = useCallback((name: string) => {
    const preset: ViewPreset = {
      id: crypto.randomUUID(),
      name,
      layout,
      cameraIds: getVisibleCameras(),
      groupId: selectedGroupId || undefined,
      createdAt: Date.now(),
    }
    setPresets((prev) => [...prev, preset])
  }, [layout, getVisibleCameras, selectedGroupId])

  const loadPreset = useCallback((preset: ViewPreset) => {
    setLayout(preset.layout)
    if (preset.groupId) {
      setSelectedGroupId(preset.groupId)
    } else {
      setSelectedGroupId(null)
    }
  }, [])

  const deletePreset = useCallback((id: string) => {
    setPresets((prev) => prev.filter((p) => p.id !== id))
  }, [])

  // Group management
  const createGroup = useCallback((name: string, cameraIds: string[]) => {
    const group: CameraGroup = {
      id: crypto.randomUUID(),
      name,
      cameraIds,
    }
    setGroups((prev) => [...prev, group])
  }, [])

  const updateGroup = useCallback((id: string, updates: Partial<CameraGroup>) => {
    setGroups((prev) =>
      prev.map((g) => (g.id === id ? { ...g, ...updates } : g))
    )
  }, [])

  const deleteGroup = useCallback((id: string) => {
    setGroups((prev) => prev.filter((g) => g.id !== id))
    if (selectedGroupId === id) {
      setSelectedGroupId(null)
    }
  }, [selectedGroupId])

  // Tour controls
  const toggleTour = useCallback(() => {
    setTourActive((prev) => !prev)
    if (!tourActive) {
      setTourCameraIndex(0)
    }
  }, [tourActive])

  const setTourInterval = useCallback((seconds: number) => {
    setTourSettings((prev) => ({ ...prev, interval: seconds }))
  }, [])

  return {
    // Layout
    layout,
    setLayout,
    getGridClasses,

    // Visible cameras
    getVisibleCameras,

    // Presets
    presets,
    savePreset,
    loadPreset,
    deletePreset,

    // Groups
    groups,
    selectedGroupId,
    setSelectedGroupId,
    createGroup,
    updateGroup,
    deleteGroup,

    // Tour
    tourActive,
    toggleTour,
    tourInterval: tourSettings.interval,
    setTourInterval,
    tourCameraIndex,
  }
}
