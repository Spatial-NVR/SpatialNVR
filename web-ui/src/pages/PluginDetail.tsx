import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useParams, Link } from 'react-router-dom'
import { useState, useEffect, useRef } from 'react'
import {
  ArrowLeft,
  Power,
  PowerOff,
  RefreshCw,
  Settings,
  FileText,
  Activity,
  AlertCircle,
  CheckCircle,
  Clock,
  Cpu,
  HardDrive,
  Loader2,
  Terminal,
  Save,
  X,
  ChevronDown,
  ChevronRight,
  Camera
} from 'lucide-react'
import { pluginsApi } from '../lib/api'
import { useToast } from '../components/Toast'
import { ReolinkSetup } from '../components/plugins/ReolinkSetup'
import { WyzeSetup } from '../components/plugins/WyzeSetup'

type TabType = 'overview' | 'setup' | 'settings' | 'logs'

// Plugin IDs that have setup components
const PLUGINS_WITH_SETUP = ['reolink', 'wyze', 'nvr-camera-reolink', 'nvr-camera-wyze']

// Log level colors
const logLevelColors: Record<string, string> = {
  DEBUG: 'text-gray-400',
  INFO: 'text-blue-400',
  WARN: 'text-yellow-400',
  ERROR: 'text-red-400',
  FATAL: 'text-red-600 font-bold',
}

// Health state badge
function HealthBadge({ state, message }: { state: string; message?: string }) {
  const colors: Record<string, string> = {
    healthy: 'bg-green-500/10 text-green-500 border-green-500/20',
    degraded: 'bg-yellow-500/10 text-yellow-500 border-yellow-500/20',
    unhealthy: 'bg-red-500/10 text-red-500 border-red-500/20',
    unknown: 'bg-gray-500/10 text-gray-500 border-gray-500/20',
  }

  const icons: Record<string, typeof CheckCircle> = {
    healthy: CheckCircle,
    degraded: AlertCircle,
    unhealthy: AlertCircle,
    unknown: Clock,
  }

  const Icon = icons[state] || Clock
  const color = colors[state] || colors.unknown

  return (
    <div className={`inline-flex items-center gap-1.5 px-2 py-1 rounded-full border ${color}`}>
      <Icon className="w-3.5 h-3.5" />
      <span className="text-xs font-medium capitalize">{state}</span>
      {message && (
        <span className="text-xs opacity-70">- {message}</span>
      )}
    </div>
  )
}

// Config editor component
function ConfigEditor({
  config,
  schema,
  onSave,
  isSaving
}: {
  config: Record<string, unknown>
  schema?: Record<string, { type: string; description?: string; default?: unknown }>
  onSave: (config: Record<string, unknown>) => void
  isSaving: boolean
}) {
  const [editedConfig, setEditedConfig] = useState<Record<string, unknown>>(config)
  const [hasChanges, setHasChanges] = useState(false)

  useEffect(() => {
    setEditedConfig(config)
    setHasChanges(false)
  }, [config])

  const handleChange = (key: string, value: unknown) => {
    setEditedConfig(prev => ({ ...prev, [key]: value }))
    setHasChanges(true)
  }

  const handleSave = () => {
    onSave(editedConfig)
    setHasChanges(false)
  }

  const handleReset = () => {
    setEditedConfig(config)
    setHasChanges(false)
  }

  return (
    <div className="space-y-4">
      {Object.entries(editedConfig).map(([key, value]) => {
        const fieldSchema = schema?.[key]
        const valueType = typeof value

        return (
          <div key={key} className="space-y-1">
            <label className="block text-sm font-medium">
              {key}
            </label>
            {fieldSchema?.description && (
              <p className="text-xs text-muted-foreground">{fieldSchema.description}</p>
            )}
            {valueType === 'boolean' ? (
              <button
                onClick={() => handleChange(key, !value)}
                className={`px-3 py-1.5 rounded text-sm ${
                  value
                    ? 'bg-green-500/10 text-green-500 border border-green-500/20'
                    : 'bg-gray-500/10 text-gray-500 border border-gray-500/20'
                }`}
              >
                {value ? 'Enabled' : 'Disabled'}
              </button>
            ) : valueType === 'number' ? (
              <input
                type="number"
                value={value as number}
                onChange={e => handleChange(key, parseFloat(e.target.value) || 0)}
                className="w-full px-3 py-2 bg-background border border-border rounded-md text-sm"
              />
            ) : valueType === 'object' ? (
              <pre className="px-3 py-2 bg-background border border-border rounded-md text-xs font-mono overflow-x-auto">
                {JSON.stringify(value, null, 2)}
              </pre>
            ) : (
              <input
                type="text"
                value={String(value)}
                onChange={e => handleChange(key, e.target.value)}
                className="w-full px-3 py-2 bg-background border border-border rounded-md text-sm"
              />
            )}
          </div>
        )
      })}

      {Object.keys(editedConfig).length === 0 && (
        <p className="text-muted-foreground text-sm py-4 text-center">
          No configuration options available for this plugin.
        </p>
      )}

      {hasChanges && (
        <div className="flex items-center gap-2 pt-4 border-t border-border">
          <button
            onClick={handleSave}
            disabled={isSaving}
            className="flex items-center gap-2 px-4 py-2 bg-primary text-primary-foreground rounded-md text-sm hover:bg-primary/90 disabled:opacity-50"
          >
            {isSaving ? (
              <Loader2 className="w-4 h-4 animate-spin" />
            ) : (
              <Save className="w-4 h-4" />
            )}
            Save Changes
          </button>
          <button
            onClick={handleReset}
            className="flex items-center gap-2 px-4 py-2 bg-accent text-foreground rounded-md text-sm hover:bg-accent/80"
          >
            <X className="w-4 h-4" />
            Reset
          </button>
        </div>
      )}
    </div>
  )
}

// Log viewer component
function LogViewer({ pluginId }: { pluginId: string }) {
  const [autoScroll, setAutoScroll] = useState(true)
  const [filter, setFilter] = useState('')
  const [level, setLevel] = useState<string>('all')
  const [expandedLines, setExpandedLines] = useState<Set<number>>(new Set())
  const logsEndRef = useRef<HTMLDivElement>(null)

  const { data: logs, isLoading, refetch } = useQuery({
    queryKey: ['plugin-logs', pluginId],
    queryFn: () => pluginsApi.getLogs(pluginId, { lines: 500, level: level === 'all' ? undefined : level }),
    refetchInterval: 2000,
  })

  useEffect(() => {
    if (autoScroll && logsEndRef.current) {
      logsEndRef.current.scrollIntoView({ behavior: 'smooth' })
    }
  }, [logs, autoScroll])

  const filteredLogs = logs?.logs?.filter(log => {
    if (filter && !log.message.toLowerCase().includes(filter.toLowerCase())) {
      return false
    }
    if (level !== 'all' && log.level !== level) {
      return false
    }
    return true
  }) || []

  const toggleLine = (index: number) => {
    setExpandedLines(prev => {
      const next = new Set(prev)
      if (next.has(index)) {
        next.delete(index)
      } else {
        next.add(index)
      }
      return next
    })
  }

  return (
    <div className="space-y-3">
      {/* Controls */}
      <div className="flex items-center gap-3 flex-wrap">
        <input
          type="text"
          value={filter}
          onChange={e => setFilter(e.target.value)}
          placeholder="Filter logs..."
          className="flex-1 min-w-48 px-3 py-1.5 bg-background border border-border rounded-md text-sm"
        />
        <select
          value={level}
          onChange={e => setLevel(e.target.value)}
          className="px-3 py-1.5 bg-background border border-border rounded-md text-sm"
        >
          <option value="all">All Levels</option>
          <option value="DEBUG">Debug</option>
          <option value="INFO">Info</option>
          <option value="WARN">Warning</option>
          <option value="ERROR">Error</option>
        </select>
        <button
          onClick={() => refetch()}
          className="p-1.5 hover:bg-accent rounded"
          title="Refresh"
        >
          <RefreshCw className="w-4 h-4" />
        </button>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={autoScroll}
            onChange={e => setAutoScroll(e.target.checked)}
            className="rounded"
          />
          Auto-scroll
        </label>
      </div>

      {/* Log output */}
      <div className="bg-gray-900 rounded-lg p-3 h-96 overflow-y-auto font-mono text-xs">
        {isLoading ? (
          <div className="flex items-center justify-center h-full">
            <Loader2 className="w-6 h-6 animate-spin text-gray-500" />
          </div>
        ) : filteredLogs.length === 0 ? (
          <div className="flex items-center justify-center h-full text-gray-500">
            No logs available
          </div>
        ) : (
          <div className="space-y-0.5">
            {filteredLogs.map((log, index) => (
              <div key={index} className="hover:bg-gray-800/50 rounded px-1">
                <div
                  className="flex items-start gap-2 cursor-pointer"
                  onClick={() => log.metadata && toggleLine(index)}
                >
                  {log.metadata && (
                    expandedLines.has(index) ? (
                      <ChevronDown className="w-3 h-3 text-gray-500 mt-0.5 shrink-0" />
                    ) : (
                      <ChevronRight className="w-3 h-3 text-gray-500 mt-0.5 shrink-0" />
                    )
                  )}
                  <span className="text-gray-500 shrink-0">
                    {new Date(log.timestamp).toLocaleTimeString()}
                  </span>
                  <span className={`shrink-0 w-12 ${logLevelColors[log.level] || 'text-gray-400'}`}>
                    [{log.level}]
                  </span>
                  <span className="text-gray-200 break-all">{log.message}</span>
                </div>
                {expandedLines.has(index) && log.metadata && (
                  <pre className="ml-20 mt-1 text-gray-400 text-[10px] overflow-x-auto">
                    {JSON.stringify(log.metadata, null, 2)}
                  </pre>
                )}
              </div>
            ))}
            <div ref={logsEndRef} />
          </div>
        )}
      </div>

      {/* Log stats */}
      {logs && (
        <div className="text-xs text-muted-foreground">
          Showing {filteredLogs.length} of {logs.total} log entries
        </div>
      )}
    </div>
  )
}

export function PluginDetail() {
  const { id } = useParams<{ id: string }>()
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const [activeTab, setActiveTab] = useState<TabType>('overview')

  // Fetch plugin status
  const { data: plugin, isLoading: isLoadingPlugin, error } = useQuery({
    queryKey: ['plugin', id],
    queryFn: () => pluginsApi.getStatus(id!),
    enabled: !!id,
    refetchInterval: 5000,
  })

  // Fetch plugin config
  const { data: configData, isLoading: isLoadingConfig } = useQuery({
    queryKey: ['plugin-config', id],
    queryFn: () => pluginsApi.getConfig(id!),
    enabled: !!id && activeTab === 'settings',
  })

  // Enable mutation
  const enableMutation = useMutation({
    mutationFn: () => pluginsApi.enable(id!),
    onSuccess: () => {
      addToast('success', 'Plugin enabled')
      queryClient.invalidateQueries({ queryKey: ['plugin', id] })
    },
    onError: (err: Error) => {
      addToast('error', `Failed to enable: ${err.message}`)
    },
  })

  // Disable mutation
  const disableMutation = useMutation({
    mutationFn: () => pluginsApi.disable(id!),
    onSuccess: () => {
      addToast('success', 'Plugin disabled')
      queryClient.invalidateQueries({ queryKey: ['plugin', id] })
    },
    onError: (err: Error) => {
      addToast('error', `Failed to disable: ${err.message}`)
    },
  })

  // Restart mutation
  const restartMutation = useMutation({
    mutationFn: () => pluginsApi.restart(id!),
    onSuccess: () => {
      addToast('success', 'Plugin restarting...')
      queryClient.invalidateQueries({ queryKey: ['plugin', id] })
    },
    onError: (err: Error) => {
      addToast('error', `Failed to restart: ${err.message}`)
    },
  })

  // Save config mutation
  const saveConfigMutation = useMutation({
    mutationFn: (config: Record<string, unknown>) => pluginsApi.setConfig(id!, config),
    onSuccess: () => {
      addToast('success', 'Configuration saved')
      queryClient.invalidateQueries({ queryKey: ['plugin-config', id] })
    },
    onError: (err: Error) => {
      addToast('error', `Failed to save: ${err.message}`)
    },
  })

  if (isLoadingPlugin) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="w-8 h-8 animate-spin text-primary" />
      </div>
    )
  }

  if (error || !plugin) {
    return (
      <div className="flex flex-col items-center justify-center h-64 gap-4">
        <AlertCircle className="w-12 h-12 text-destructive" />
        <p className="text-muted-foreground">Plugin not found</p>
        <Link
          to="/plugins"
          className="px-4 py-2 bg-primary text-primary-foreground rounded-md"
        >
          Back to Plugins
        </Link>
      </div>
    )
  }

  const isRunning = plugin.state === 'running'
  const isBusy = enableMutation.isPending || disableMutation.isPending || restartMutation.isPending

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-start gap-4">
          <Link
            to="/plugins"
            className="p-2 hover:bg-accent rounded-md mt-1"
          >
            <ArrowLeft className="w-5 h-5" />
          </Link>
          <div>
            <h1 className="text-2xl font-bold">{plugin.name}</h1>
            <p className="text-muted-foreground mt-1">{plugin.description}</p>
            <div className="flex items-center gap-3 mt-2">
              <span className="text-sm text-muted-foreground">v{plugin.version}</span>
              <HealthBadge state={plugin.health.state} message={plugin.health.message} />
              {plugin.critical && (
                <span className="px-2 py-0.5 text-xs bg-red-500/10 text-red-500 rounded">
                  Critical
                </span>
              )}
              {plugin.builtin && (
                <span className="px-2 py-0.5 text-xs bg-blue-500/10 text-blue-500 rounded">
                  Built-in
                </span>
              )}
            </div>
          </div>
        </div>

        {/* Actions */}
        <div className="flex items-center gap-2">
          <button
            onClick={() => restartMutation.mutate()}
            disabled={isBusy || !isRunning}
            className="flex items-center gap-2 px-3 py-2 bg-accent hover:bg-accent/80 rounded-md text-sm disabled:opacity-50"
          >
            {restartMutation.isPending ? (
              <Loader2 className="w-4 h-4 animate-spin" />
            ) : (
              <RefreshCw className="w-4 h-4" />
            )}
            Restart
          </button>
          {isRunning ? (
            <button
              onClick={() => disableMutation.mutate()}
              disabled={isBusy || plugin.critical}
              className="flex items-center gap-2 px-3 py-2 bg-red-500/10 text-red-500 hover:bg-red-500/20 rounded-md text-sm disabled:opacity-50"
              title={plugin.critical ? 'Cannot disable critical plugin' : 'Disable plugin'}
            >
              {disableMutation.isPending ? (
                <Loader2 className="w-4 h-4 animate-spin" />
              ) : (
                <PowerOff className="w-4 h-4" />
              )}
              Disable
            </button>
          ) : (
            <button
              onClick={() => enableMutation.mutate()}
              disabled={isBusy}
              className="flex items-center gap-2 px-3 py-2 bg-green-500/10 text-green-500 hover:bg-green-500/20 rounded-md text-sm disabled:opacity-50"
            >
              {enableMutation.isPending ? (
                <Loader2 className="w-4 h-4 animate-spin" />
              ) : (
                <Power className="w-4 h-4" />
              )}
              Enable
            </button>
          )}
        </div>
      </div>

      {/* Tabs */}
      <div className="border-b border-border">
        <nav className="flex gap-4">
          {[
            { id: 'overview', label: 'Overview', icon: Activity },
            ...(PLUGINS_WITH_SETUP.some(p => plugin.id.includes(p))
              ? [{ id: 'setup', label: 'Setup', icon: Camera }]
              : []),
            { id: 'settings', label: 'Settings', icon: Settings },
            { id: 'logs', label: 'Logs', icon: Terminal },
          ].map(tab => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id as TabType)}
              className={`flex items-center gap-2 px-3 py-2 border-b-2 -mb-px transition-colors ${
                activeTab === tab.id
                  ? 'border-primary text-primary'
                  : 'border-transparent text-muted-foreground hover:text-foreground'
              }`}
            >
              <tab.icon className="w-4 h-4" />
              {tab.label}
            </button>
          ))}
        </nav>
      </div>

      {/* Tab content */}
      <div className="min-h-[400px]">
        {activeTab === 'overview' && (
          <div className="grid gap-6 md:grid-cols-2">
            {/* Status card */}
            <div className="bg-card border border-border rounded-lg p-4">
              <h3 className="font-medium mb-4 flex items-center gap-2">
                <Activity className="w-4 h-4" />
                Status
              </h3>
              <div className="space-y-3">
                <div className="flex justify-between">
                  <span className="text-muted-foreground">State</span>
                  <span className={`capitalize ${
                    plugin.state === 'running' ? 'text-green-500' :
                    plugin.state === 'error' ? 'text-red-500' :
                    plugin.state === 'starting' ? 'text-yellow-500' :
                    'text-gray-500'
                  }`}>
                    {plugin.state}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Started</span>
                  <span>
                    {plugin.startedAt
                      ? new Date(plugin.startedAt).toLocaleString()
                      : 'Not running'}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Category</span>
                  <span className="capitalize">{plugin.category}</span>
                </div>
                {plugin.lastError && (
                  <div className="mt-3 p-3 bg-red-500/10 border border-red-500/20 rounded">
                    <p className="text-sm text-red-500 font-medium">Last Error</p>
                    <p className="text-xs text-red-400 mt-1">{plugin.lastError}</p>
                  </div>
                )}
              </div>
            </div>

            {/* Capabilities card */}
            <div className="bg-card border border-border rounded-lg p-4">
              <h3 className="font-medium mb-4 flex items-center gap-2">
                <Cpu className="w-4 h-4" />
                Capabilities
              </h3>
              {plugin.capabilities && plugin.capabilities.length > 0 ? (
                <div className="flex flex-wrap gap-2">
                  {plugin.capabilities.map(cap => (
                    <span
                      key={cap}
                      className="px-2 py-1 text-sm bg-accent rounded"
                    >
                      {cap}
                    </span>
                  ))}
                </div>
              ) : (
                <p className="text-muted-foreground text-sm">No capabilities declared</p>
              )}
            </div>

            {/* Dependencies card */}
            <div className="bg-card border border-border rounded-lg p-4">
              <h3 className="font-medium mb-4 flex items-center gap-2">
                <FileText className="w-4 h-4" />
                Dependencies
              </h3>
              {plugin.dependencies && plugin.dependencies.length > 0 ? (
                <div className="space-y-2">
                  {plugin.dependencies.map(dep => (
                    <Link
                      key={dep}
                      to={`/plugins/${dep}`}
                      className="flex items-center gap-2 text-sm hover:text-primary"
                    >
                      <ChevronRight className="w-4 h-4" />
                      {dep}
                    </Link>
                  ))}
                </div>
              ) : (
                <p className="text-muted-foreground text-sm">No dependencies</p>
              )}
            </div>

            {/* Metrics card */}
            <div className="bg-card border border-border rounded-lg p-4">
              <h3 className="font-medium mb-4 flex items-center gap-2">
                <HardDrive className="w-4 h-4" />
                Metrics
              </h3>
              {plugin.metrics ? (
                <div className="space-y-3">
                  {plugin.metrics.memory && (
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">Memory</span>
                      <span>{Math.round(plugin.metrics.memory / 1024 / 1024)} MB</span>
                    </div>
                  )}
                  {plugin.metrics.cpu && (
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">CPU</span>
                      <span>{plugin.metrics.cpu.toFixed(1)}%</span>
                    </div>
                  )}
                  {plugin.metrics.uptime && (
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">Uptime</span>
                      <span>{formatUptime(plugin.metrics.uptime)}</span>
                    </div>
                  )}
                </div>
              ) : (
                <p className="text-muted-foreground text-sm">Metrics not available</p>
              )}
            </div>
          </div>
        )}

        {activeTab === 'setup' && (
          <div className="bg-card border border-border rounded-lg p-6">
            <h3 className="font-medium mb-4 flex items-center gap-2">
              <Camera className="w-4 h-4" />
              Camera Setup
            </h3>
            {plugin.id.includes('reolink') && (
              <ReolinkSetup
                pluginId={plugin.id}
                onCameraAdded={() => {
                  queryClient.invalidateQueries({ queryKey: ['cameras'] })
                }}
              />
            )}
            {plugin.id.includes('wyze') && (
              <WyzeSetup
                pluginId={plugin.id}
                onCameraAdded={() => {
                  queryClient.invalidateQueries({ queryKey: ['cameras'] })
                }}
              />
            )}
          </div>
        )}

        {activeTab === 'settings' && (
          <div className="bg-card border border-border rounded-lg p-6">
            <h3 className="font-medium mb-4 flex items-center gap-2">
              <Settings className="w-4 h-4" />
              Configuration
            </h3>
            {isLoadingConfig ? (
              <div className="flex items-center justify-center py-8">
                <Loader2 className="w-6 h-6 animate-spin text-primary" />
              </div>
            ) : (
              <ConfigEditor
                config={configData?.config || {}}
                schema={configData?.schema}
                onSave={(config) => saveConfigMutation.mutate(config)}
                isSaving={saveConfigMutation.isPending}
              />
            )}
          </div>
        )}

        {activeTab === 'logs' && (
          <div className="bg-card border border-border rounded-lg p-4">
            <h3 className="font-medium mb-4 flex items-center gap-2">
              <Terminal className="w-4 h-4" />
              Plugin Logs
            </h3>
            <LogViewer pluginId={id!} />
          </div>
        )}
      </div>
    </div>
  )
}

// Helper to format uptime
function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)

  if (days > 0) {
    return `${days}d ${hours}h ${minutes}m`
  }
  if (hours > 0) {
    return `${hours}h ${minutes}m`
  }
  return `${minutes}m`
}
