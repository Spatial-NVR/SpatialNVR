import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Moon, Sun, ChevronDown, ChevronRight, Cpu, Download, Check, AlertCircle, Loader2, HardDrive, Trash2 } from 'lucide-react'
import { useState, useEffect } from 'react'
import { configApi, modelsApi, storageApi, cameraApi, ModelDownloadStatus } from '../lib/api'
import { useToast } from '../components/Toast'

// Format bytes to human readable
function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

export function Settings() {
  const queryClient = useQueryClient()
  const [theme, setTheme] = useState<'dark' | 'light'>('dark')
  const { addToast } = useToast()

  const { data: config } = useQuery({
    queryKey: ['config'],
    queryFn: configApi.get,
  })

  // Track model download statuses
  const [downloadStatuses, setDownloadStatuses] = useState<Record<string, ModelDownloadStatus>>({})
  const [isPollingDownloads, setIsPollingDownloads] = useState(false)

  // Fetch initial model statuses on mount
  const { data: initialModelStatuses } = useQuery({
    queryKey: ['model-statuses-initial'],
    queryFn: modelsApi.getStatuses,
    staleTime: 60000, // Cache for 1 minute
  })

  // Set initial statuses when loaded
  useEffect(() => {
    if (initialModelStatuses && Object.keys(downloadStatuses).length === 0) {
      setDownloadStatuses(initialModelStatuses)
    }
  }, [initialModelStatuses, downloadStatuses])

  // Poll for download status when downloads are active
  const { data: modelStatuses } = useQuery({
    queryKey: ['model-statuses'],
    queryFn: modelsApi.getStatuses,
    enabled: isPollingDownloads,
    refetchInterval: isPollingDownloads ? 1000 : false, // Poll every second when active
  })

  // Update download statuses when polled
  useEffect(() => {
    if (modelStatuses) {
      setDownloadStatuses(modelStatuses)

      // Check if any downloads are still in progress
      const hasActiveDownloads = Object.values(modelStatuses).some(
        s => s.status === 'downloading' || s.status === 'pending'
      )

      if (!hasActiveDownloads && isPollingDownloads) {
        setIsPollingDownloads(false)
        // Check for completed downloads
        const completedModels = Object.values(modelStatuses).filter(s => s.status === 'completed')
        if (completedModels.length > 0) {
          addToast('success', 'Model download completed')
        }
      }
    }
  }, [modelStatuses, isPollingDownloads, addToast])

  const updateConfig = useMutation({
    mutationFn: configApi.update,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['config'] })
      addToast('success', 'Settings saved - downloading models if needed...')
      // Start polling for download status
      setIsPollingDownloads(true)
    },
    onError: (error: Error) => {
      addToast('error', `Failed to save settings: ${error.message}`)
    },
  })

  // Fetch storage stats
  const { data: storageStats, refetch: refetchStorage } = useQuery({
    queryKey: ['storage'],
    queryFn: () => storageApi.getStats(),
    refetchInterval: 30000,
  })

  // Fetch cameras for storage breakdown
  const { data: cameras } = useQuery({
    queryKey: ['cameras'],
    queryFn: () => cameraApi.list(),
  })

  // Run retention cleanup mutation
  const [isRunningRetention, setIsRunningRetention] = useState(false)
  const runRetention = useMutation({
    mutationFn: storageApi.runRetention,
    onMutate: () => setIsRunningRetention(true),
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: ['storage'] })
      refetchStorage()
      addToast('success', `Cleanup complete: ${result.segments_deleted} segments deleted, ${formatBytes(result.bytes_freed)} freed`)
      setIsRunningRetention(false)
    },
    onError: (error: Error) => {
      addToast('error', `Cleanup failed: ${error.message}`)
      setIsRunningRetention(false)
    },
  })

  // Helper to get status for a specific model
  const getModelStatus = (modelId: string): ModelDownloadStatus | undefined => {
    return downloadStatuses[modelId]
  }

  // Get camera name by ID
  const getCameraName = (cameraId: string): string => {
    return cameras?.find(c => c.id === cameraId)?.name || cameraId
  }

  // General settings
  const [systemName, setSystemName] = useState('')
  const [timezone, setTimezone] = useState('UTC')
  const [gridColumns, setGridColumns] = useState(3)

  // Storage settings
  const [maxStorage, setMaxStorage] = useState(1000)
  const [retentionDays, setRetentionDays] = useState(30)

  // Detection settings
  const [detectionBackend, setDetectionBackend] = useState('onnx')
  const [objectModel, setObjectModel] = useState('yolo12n')
  const [faceModel, setFaceModel] = useState('buffalo_l')
  const [lprModel, setLprModel] = useState('paddleocr')
  const [detectionFps, setDetectionFps] = useState(5)
  const [objectConfidence, setObjectConfidence] = useState(0.5)
  const [faceConfidence, setFaceConfidence] = useState(0.7)
  const [lprConfidence, setLprConfidence] = useState(0.8)
  const [enabledDetectors, setEnabledDetectors] = useState({
    objects: true,
    faces: false,
    lpr: false,
  })
  const [enabledObjects, setEnabledObjects] = useState({
    person: true,
    vehicle: true,
    animal: false,
    package: false,
  })

  useEffect(() => {
    if (config) {
      // General settings
      setSystemName(config.system?.name || 'NVR System')
      setTimezone(config.system?.timezone || 'UTC')
      setMaxStorage(config.system?.max_storage_gb || 1000)
      setRetentionDays(config.storage?.retention?.default_days || 30)
      setGridColumns(config.preferences?.ui?.dashboard?.grid_columns || 3)

      // Detection settings
      if (config.detection) {
        setDetectionBackend(config.detection.backend || 'onnx')
        setDetectionFps(config.detection.fps || 5)

        if (config.detection.objects) {
          setEnabledDetectors(prev => ({ ...prev, objects: config.detection!.objects.enabled }))
          setObjectModel(config.detection.objects.model || 'yolo12n')
          setObjectConfidence(config.detection.objects.confidence || 0.5)
          const classes = config.detection.objects.classes || ['person', 'vehicle']
          setEnabledObjects({
            person: classes.includes('person'),
            vehicle: classes.includes('vehicle'),
            animal: classes.includes('animal'),
            package: classes.includes('package'),
          })
        }

        if (config.detection.faces) {
          setEnabledDetectors(prev => ({ ...prev, faces: config.detection!.faces.enabled }))
          setFaceModel(config.detection.faces.model || 'buffalo_l')
          setFaceConfidence(config.detection.faces.confidence || 0.7)
        }

        if (config.detection.lpr) {
          setEnabledDetectors(prev => ({ ...prev, lpr: config.detection!.lpr.enabled }))
          setLprModel(config.detection.lpr.model || 'paddleocr')
          setLprConfidence(config.detection.lpr.confidence || 0.8)
        }
      }
    }
  }, [config])

  const toggleTheme = () => {
    const newTheme = theme === 'dark' ? 'light' : 'dark'
    setTheme(newTheme)
    document.body.classList.remove('dark', 'light')
    document.body.classList.add(newTheme)
  }

  const handleSave = () => {
    // Build enabled object classes array
    const enabledClasses = Object.entries(enabledObjects)
      .filter(([, enabled]) => enabled)
      .map(([className]) => className)

    updateConfig.mutate({
      system: {
        name: systemName,
        timezone: timezone,
        max_storage_gb: maxStorage,
      },
      storage: {
        retention: {
          default_days: retentionDays,
        },
      },
      detection: {
        backend: detectionBackend,
        fps: detectionFps,
        objects: {
          enabled: enabledDetectors.objects,
          model: enabledDetectors.objects ? objectModel : '',
          confidence: enabledDetectors.objects ? objectConfidence : 0,
          classes: enabledDetectors.objects ? enabledClasses : [],
        },
        faces: {
          enabled: enabledDetectors.faces,
          model: enabledDetectors.faces ? faceModel : '',
          confidence: enabledDetectors.faces ? faceConfidence : 0,
        },
        lpr: {
          enabled: enabledDetectors.lpr,
          model: enabledDetectors.lpr ? lprModel : '',
          confidence: enabledDetectors.lpr ? lprConfidence : 0,
        },
      },
      preferences: {
        ui: {
          theme: theme,
          language: 'en',
          dashboard: {
            grid_columns: gridColumns,
            show_fps: true,
          },
        },
        timeline: {
          default_range_hours: 24,
          thumbnail_interval_seconds: 10,
        },
        events: {
          auto_acknowledge_after_days: 7,
          group_similar_events: true,
          group_window_seconds: 300,
        },
      },
    })
  }

  const timezones = [
    'UTC',
    'America/New_York',
    'America/Chicago',
    'America/Denver',
    'America/Los_Angeles',
    'America/Anchorage',
    'Pacific/Honolulu',
    'Europe/London',
    'Europe/Paris',
    'Asia/Tokyo',
    'Australia/Sydney',
  ]

  const backends = [
    { value: 'onnx', label: 'ONNX Runtime', description: 'CPU/GPU - Most compatible' },
    { value: 'tensorrt', label: 'NVIDIA TensorRT', description: 'NVIDIA GPUs - Fastest' },
    { value: 'openvino', label: 'Intel OpenVINO', description: 'Intel CPUs/GPUs/VPUs' },
    { value: 'coral', label: 'Google Coral TPU', description: 'Edge TPU - Low power' },
    { value: 'coreml', label: 'Apple CoreML', description: 'Apple Silicon - Native' },
  ]

  const objectModels = [
    { value: 'yolo12n', label: 'YOLOv12 Nano', description: '~2ms inference' },
    { value: 'yolo12s', label: 'YOLOv12 Small', description: '~4ms inference' },
    { value: 'yolo12m', label: 'YOLOv12 Medium', description: '~8ms inference' },
    { value: 'yolo12l', label: 'YOLOv12 Large', description: '~15ms inference' },
  ]

  const faceModels = [
    { value: 'buffalo_l', label: 'Buffalo Large', description: 'Best accuracy' },
    { value: 'buffalo_s', label: 'Buffalo Small', description: 'Faster, good accuracy' },
  ]

  const lprModels = [
    { value: 'paddleocr', label: 'PaddleOCR v3', description: 'Multi-language support' },
    { value: 'easyocr', label: 'EasyOCR', description: 'Good for US plates' },
  ]

  return (
    <div className="space-y-6 max-w-3xl">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold">Settings</h1>
          <p className="text-muted-foreground">Configure your NVR system</p>
        </div>
      </div>

      {/* General Settings */}
      <section className="bg-card rounded-lg border">
        <div className="p-4 border-b">
          <h2 className="font-semibold">General</h2>
        </div>
        <div className="p-4 space-y-4">
          <SettingRow label="System Name">
            <input
              type="text"
              value={systemName}
              onChange={(e) => setSystemName(e.target.value)}
              className="w-full px-3 py-2 bg-background border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary"
            />
          </SettingRow>

          <SettingRow label="Timezone">
            <Select value={timezone} onChange={setTimezone} options={timezones.map(tz => ({ value: tz, label: tz }))} />
          </SettingRow>

          <SettingRow label="Theme">
            <button
              onClick={toggleTheme}
              className="inline-flex items-center gap-2 px-3 py-2 bg-background border rounded-lg hover:bg-accent transition-colors"
            >
              {theme === 'dark' ? <Moon size={16} /> : <Sun size={16} />}
              {theme === 'dark' ? 'Dark' : 'Light'}
            </button>
          </SettingRow>

          <SettingRow label="Dashboard Grid">
            <Select
              value={gridColumns.toString()}
              onChange={(v) => setGridColumns(parseInt(v))}
              options={[
                { value: '2', label: '2 columns' },
                { value: '3', label: '3 columns' },
                { value: '4', label: '4 columns' },
              ]}
            />
          </SettingRow>
        </div>
      </section>

      {/* Storage Settings */}
      <section className="bg-card rounded-lg border">
        <div className="p-4 border-b flex items-center gap-2">
          <HardDrive size={18} className="text-muted-foreground" />
          <h2 className="font-semibold">Storage</h2>
        </div>
        <div className="p-4 space-y-4">
          {/* Storage Overview */}
          {storageStats && (
            <div className="p-4 bg-background rounded-lg border space-y-3">
              <div className="flex items-center justify-between">
                <span className="text-sm font-medium">Storage Usage</span>
                <span className="text-sm text-muted-foreground">
                  {formatBytes(storageStats.used_bytes)} / {formatBytes(maxStorage * 1024 * 1024 * 1024)}
                </span>
              </div>
              <div className="w-full h-3 bg-gray-700 rounded-full overflow-hidden">
                <div
                  className={`h-full transition-all duration-300 ${
                    (storageStats.used_bytes / (maxStorage * 1024 * 1024 * 1024)) > 0.9
                      ? 'bg-red-500'
                      : (storageStats.used_bytes / (maxStorage * 1024 * 1024 * 1024)) > 0.75
                        ? 'bg-yellow-500'
                        : 'bg-green-500'
                  }`}
                  style={{ width: `${Math.min(100, (storageStats.used_bytes / (maxStorage * 1024 * 1024 * 1024)) * 100)}%` }}
                />
              </div>
              <div className="flex items-center justify-between text-xs text-muted-foreground">
                <span>{storageStats.segment_count} recording segments</span>
                <span>{formatBytes(storageStats.available_bytes)} available</span>
              </div>

              {/* Per-Camera Breakdown */}
              {storageStats.by_camera && Object.keys(storageStats.by_camera).length > 0 && (
                <div className="pt-3 border-t space-y-2">
                  <span className="text-xs font-medium text-muted-foreground uppercase tracking-wide">Usage by Camera</span>
                  {Object.entries(storageStats.by_camera)
                    .sort(([,a], [,b]) => b - a)
                    .map(([cameraId, bytes]) => (
                      <div key={cameraId} className="flex items-center justify-between text-sm">
                        <span className="truncate max-w-[200px]">{getCameraName(cameraId)}</span>
                        <span className="text-muted-foreground">{formatBytes(bytes)}</span>
                      </div>
                    ))}
                </div>
              )}

              {/* Manual Cleanup Button */}
              <div className="pt-3 border-t">
                <button
                  onClick={() => runRetention.mutate()}
                  disabled={isRunningRetention}
                  className="flex items-center gap-2 px-3 py-2 text-sm bg-secondary text-secondary-foreground rounded-lg hover:bg-secondary/80 transition-colors disabled:opacity-50"
                >
                  {isRunningRetention ? (
                    <>
                      <Loader2 size={14} className="animate-spin" />
                      Running cleanup...
                    </>
                  ) : (
                    <>
                      <Trash2 size={14} />
                      Run Cleanup Now
                    </>
                  )}
                </button>
                <p className="text-xs text-muted-foreground mt-1">
                  Delete recordings older than retention period
                </p>
              </div>
            </div>
          )}

          <SettingRow label="Max Storage">
            <div className="flex items-center gap-2">
              <input
                type="number"
                value={maxStorage}
                onChange={(e) => setMaxStorage(parseInt(e.target.value) || 0)}
                min={10}
                max={100000}
                className="w-28 px-3 py-2 bg-background border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary"
              />
              <span className="text-sm text-muted-foreground">GB</span>
            </div>
            <p className="text-xs text-muted-foreground mt-1 col-span-2 col-start-2">
              When this limit is reached, oldest recordings are deleted automatically
            </p>
          </SettingRow>

          <SettingRow label="Default Retention">
            <div className="flex items-center gap-2">
              <input
                type="number"
                value={retentionDays}
                onChange={(e) => setRetentionDays(parseInt(e.target.value) || 0)}
                min={1}
                max={365}
                className="w-28 px-3 py-2 bg-background border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary"
              />
              <span className="text-sm text-muted-foreground">days</span>
            </div>
            <p className="text-xs text-muted-foreground mt-1 col-span-2 col-start-2">
              Global default. Can be overridden per camera in camera settings.
            </p>
          </SettingRow>

          <SettingRow label="Storage Path">
            <span className="text-sm text-muted-foreground font-mono">/data</span>
          </SettingRow>
        </div>
      </section>

      {/* Detection Settings */}
      <section className="bg-card rounded-lg border">
        <div className="p-4 border-b flex items-center gap-2">
          <Cpu size={18} className="text-muted-foreground" />
          <h2 className="font-semibold">Detection</h2>
        </div>
        <div className="p-4 space-y-6">
          {/* Backend Selection */}
          <SettingRow label="Inference Backend">
            <Select
              value={detectionBackend}
              onChange={setDetectionBackend}
              options={backends.map(b => ({ value: b.value, label: `${b.label} - ${b.description}` }))}
            />
          </SettingRow>

          <hr className="border-border" />

          {/* Detectors - Collapsible */}
          <div className="space-y-3">
            <label className="text-sm font-medium">Detectors</label>

            {/* Object Detection */}
            <CollapsibleDetector
              label="Object Detection"
              description="Detect people, vehicles, animals, packages"
              enabled={enabledDetectors.objects}
              onToggle={(v) => setEnabledDetectors({ ...enabledDetectors, objects: v })}
            >
              <SettingRow label="Model">
                <div className="space-y-2">
                  <Select
                    value={objectModel}
                    onChange={setObjectModel}
                    options={objectModels.map(m => ({ value: m.value, label: `${m.label} (${m.description})` }))}
                  />
                  <ModelStatusBadge status={getModelStatus(objectModel)} />
                </div>
              </SettingRow>

              <SettingRow label="Confidence">
                <div className="flex items-center gap-3">
                  <input
                    type="range"
                    min={0.1}
                    max={0.95}
                    step={0.05}
                    value={objectConfidence}
                    onChange={(e) => setObjectConfidence(parseFloat(e.target.value))}
                    className="flex-1 accent-primary"
                  />
                  <span className="w-12 text-sm text-right">{(objectConfidence * 100).toFixed(0)}%</span>
                </div>
              </SettingRow>

              <div className="space-y-2">
                <label className="text-sm text-muted-foreground">Objects to Detect</label>
                <div className="flex flex-wrap gap-2">
                  {Object.entries(enabledObjects).map(([key, value]) => (
                    <label
                      key={key}
                      className={`px-3 py-1.5 rounded-full text-sm cursor-pointer transition-colors ${
                        value
                          ? 'bg-primary text-primary-foreground'
                          : 'bg-secondary text-secondary-foreground hover:bg-secondary/80'
                      }`}
                    >
                      <input
                        type="checkbox"
                        checked={value}
                        onChange={(e) => setEnabledObjects({ ...enabledObjects, [key]: e.target.checked })}
                        className="sr-only"
                      />
                      {key.charAt(0).toUpperCase() + key.slice(1)}
                    </label>
                  ))}
                </div>
              </div>
            </CollapsibleDetector>

            {/* Face Recognition */}
            <CollapsibleDetector
              label="Face Recognition"
              description="Identify known faces"
              enabled={enabledDetectors.faces}
              onToggle={(v) => setEnabledDetectors({ ...enabledDetectors, faces: v })}
            >
              <SettingRow label="Model">
                <div className="space-y-2">
                  <Select
                    value={faceModel}
                    onChange={setFaceModel}
                    options={faceModels.map(m => ({ value: m.value, label: `${m.label} (${m.description})` }))}
                  />
                  <ModelStatusBadge status={getModelStatus(faceModel)} />
                </div>
              </SettingRow>

              <SettingRow label="Match Confidence">
                <div className="flex items-center gap-3">
                  <input
                    type="range"
                    min={0.5}
                    max={0.99}
                    step={0.01}
                    value={faceConfidence}
                    onChange={(e) => setFaceConfidence(parseFloat(e.target.value))}
                    className="flex-1 accent-primary"
                  />
                  <span className="w-12 text-sm text-right">{(faceConfidence * 100).toFixed(0)}%</span>
                </div>
              </SettingRow>
            </CollapsibleDetector>

            {/* License Plate Recognition */}
            <CollapsibleDetector
              label="License Plate Recognition"
              description="Read vehicle license plates"
              enabled={enabledDetectors.lpr}
              onToggle={(v) => setEnabledDetectors({ ...enabledDetectors, lpr: v })}
            >
              <SettingRow label="Model">
                <div className="space-y-2">
                  <Select
                    value={lprModel}
                    onChange={setLprModel}
                    options={lprModels.map(m => ({ value: m.value, label: `${m.label} (${m.description})` }))}
                  />
                  <ModelStatusBadge status={getModelStatus(lprModel)} />
                </div>
              </SettingRow>

              <SettingRow label="Confidence">
                <div className="flex items-center gap-3">
                  <input
                    type="range"
                    min={0.5}
                    max={0.99}
                    step={0.01}
                    value={lprConfidence}
                    onChange={(e) => setLprConfidence(parseFloat(e.target.value))}
                    className="flex-1 accent-primary"
                  />
                  <span className="w-12 text-sm text-right">{(lprConfidence * 100).toFixed(0)}%</span>
                </div>
              </SettingRow>
            </CollapsibleDetector>
          </div>

          <hr className="border-border" />

          {/* Global Detection Settings */}
          <SettingRow label="Detection FPS">
            <div className="flex items-center gap-2">
              <input
                type="number"
                value={detectionFps}
                onChange={(e) => setDetectionFps(parseInt(e.target.value) || 1)}
                min={1}
                max={30}
                className="w-20 px-3 py-2 bg-background border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary"
              />
              <span className="text-sm text-muted-foreground">fps per camera</span>
            </div>
          </SettingRow>
        </div>
      </section>

      {/* Save Button */}
      <div className="flex justify-end gap-3 pb-8">
        <button
          onClick={handleSave}
          disabled={updateConfig.isPending}
          className="px-6 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors disabled:opacity-50"
        >
          {updateConfig.isPending ? 'Saving...' : 'Save Changes'}
        </button>
      </div>
    </div>
  )
}

// Helper Components
function SettingRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="grid grid-cols-3 gap-4 items-center">
      <label className="text-sm font-medium">{label}</label>
      <div className="col-span-2">{children}</div>
    </div>
  )
}

function Select({
  value,
  onChange,
  options,
}: {
  value: string
  onChange: (value: string) => void
  options: { value: string; label: string }[]
}) {
  return (
    <div className="relative">
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="w-full px-3 py-2 bg-background border rounded-lg appearance-none focus:outline-none focus:ring-2 focus:ring-primary pr-10"
      >
        {options.map((opt) => (
          <option key={opt.value} value={opt.value}>
            {opt.label}
          </option>
        ))}
      </select>
      <ChevronDown className="absolute right-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground pointer-events-none" />
    </div>
  )
}

function Toggle({
  checked,
  onChange,
}: {
  checked: boolean
  onChange: (checked: boolean) => void
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      onClick={(e) => {
        e.stopPropagation()
        onChange(!checked)
      }}
      className={`relative w-11 h-6 rounded-full transition-colors flex-shrink-0 ${
        checked ? 'bg-green-600' : 'bg-gray-600'
      }`}
    >
      <span
        className={`absolute top-1 left-1 w-4 h-4 bg-white rounded-full shadow transition-transform ${
          checked ? 'translate-x-5' : 'translate-x-0'
        }`}
      />
    </button>
  )
}

function CollapsibleDetector({
  label,
  description,
  enabled,
  onToggle,
  children,
}: {
  label: string
  description: string
  enabled: boolean
  onToggle: (enabled: boolean) => void
  children: React.ReactNode
}) {
  const [isOpen, setIsOpen] = useState(false)

  return (
    <div className="border rounded-lg overflow-hidden">
      <div
        className="flex items-center justify-between p-3 cursor-pointer hover:bg-accent/50 transition-colors"
        onClick={() => setIsOpen(!isOpen)}
      >
        <div className="flex items-center gap-3">
          {isOpen ? (
            <ChevronDown size={16} className="text-muted-foreground" />
          ) : (
            <ChevronRight size={16} className="text-muted-foreground" />
          )}
          <div>
            <div className="font-medium text-sm">{label}</div>
            <div className="text-xs text-muted-foreground">{description}</div>
          </div>
        </div>
        <Toggle checked={enabled} onChange={onToggle} />
      </div>
      {isOpen && (
        <div className="p-4 pt-2 border-t bg-background/50 space-y-4">
          {children}
        </div>
      )}
    </div>
  )
}

function ModelStatusBadge({ status }: { status?: ModelDownloadStatus }) {
  if (!status) return null

  switch (status.status) {
    case 'downloading':
      return (
        <div className="flex items-center gap-2 text-xs text-blue-400">
          <Loader2 size={12} className="animate-spin" />
          <span>Downloading {status.progress.toFixed(0)}%</span>
          <div className="w-16 h-1.5 bg-gray-700 rounded-full overflow-hidden">
            <div
              className="h-full bg-blue-500 transition-all duration-300"
              style={{ width: `${status.progress}%` }}
            />
          </div>
        </div>
      )
    case 'pending':
      return (
        <div className="flex items-center gap-1 text-xs text-yellow-400">
          <Download size={12} />
          <span>Pending</span>
        </div>
      )
    case 'completed':
      return (
        <div className="flex items-center gap-1 text-xs text-green-400">
          <Check size={12} />
          <span>Downloaded</span>
        </div>
      )
    case 'failed':
      return (
        <div className="flex items-center gap-1 text-xs text-red-400" title={status.error}>
          <AlertCircle size={12} />
          <span>Failed</span>
        </div>
      )
    default:
      return null
  }
}
