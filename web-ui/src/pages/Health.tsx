import { useQuery } from '@tanstack/react-query'
import {
  Camera, AlertTriangle, HardDrive, Activity, CheckCircle, XCircle,
  RefreshCw, Terminal, Pause, Play, Cpu, MemoryStick, Thermometer,
  Clock, Server, Puzzle
} from 'lucide-react'
import { useState, useEffect, useRef } from 'react'
import { statsApi, pluginsApi } from '../lib/api'
import { usePorts } from '../hooks/usePorts'

interface GPUInfo {
  available: boolean
  name?: string
  type?: string
  memory_total?: number
  memory_used?: number
  memory_free?: number
  utilization?: number
  temperature?: number
  index?: number
}

interface NPUInfo {
  available: boolean
  name?: string
  type?: string
  utilization?: number
}

interface SystemMetrics {
  cpu: {
    percent: number
    load_avg: [number, number, number]
  }
  memory: {
    total: number
    used: number
    free: number
    percent: number
  }
  disk: {
    total: number
    used: number
    free: number
    percent: number
    path: string
  }
  // Support both single GPU (legacy) and multiple GPUs
  gpu?: GPUInfo
  gpus?: GPUInfo[]
  npu?: NPUInfo
  uptime: number
}

// Color palette for multiple GPUs
const GPU_COLORS = [
  { bg: 'bg-orange-500', text: 'text-orange-500', hex: '#f97316' },
  { bg: 'bg-cyan-500', text: 'text-cyan-500', hex: '#06b6d4' },
  { bg: 'bg-pink-500', text: 'text-pink-500', hex: '#ec4899' },
  { bg: 'bg-lime-500', text: 'text-lime-500', hex: '#84cc16' },
  { bg: 'bg-violet-500', text: 'text-violet-500', hex: '#8b5cf6' },
  { bg: 'bg-amber-500', text: 'text-amber-500', hex: '#f59e0b' },
  { bg: 'bg-teal-500', text: 'text-teal-500', hex: '#14b8a6' },
  { bg: 'bg-rose-500', text: 'text-rose-500', hex: '#f43f5e' },
]

interface HistoryPoint {
  time: number
  value: number
}

export function Health() {
  const { apiUrl } = usePorts()
  const [cpuHistory, setCpuHistory] = useState<HistoryPoint[]>([])

  const { data: stats, isLoading, refetch, dataUpdatedAt } = useQuery({
    queryKey: ['stats'],
    queryFn: statsApi.get,
    refetchInterval: 30000,
  })

  const { data: health, isLoading: healthLoading } = useQuery({
    queryKey: ['health'],
    queryFn: async () => {
      const res = await fetch(`${apiUrl}/health`)
      return res.json()
    },
    refetchInterval: 10000,
  })

  // Fetch detailed system health (database, event bus status)
  const { data: systemHealth } = useQuery({
    queryKey: ['system-health'],
    queryFn: async () => {
      const res = await fetch(`${apiUrl}/api/v1/system/health`)
      return res.json()
    },
    refetchInterval: 10000,
  })

  const { data: metrics } = useQuery<SystemMetrics>({
    queryKey: ['system-metrics'],
    queryFn: async () => {
      const res = await fetch(`${apiUrl}/api/v1/system/metrics`)
      const data = await res.json()
      return data.data
    },
    refetchInterval: 2000, // Update every 2 seconds
  })

  // Fetch plugin status
  const { data: plugins } = useQuery({
    queryKey: ['plugin-status'],
    queryFn: async () => {
      try {
        return await pluginsApi.list()
      } catch {
        return []
      }
    },
    refetchInterval: 10000,
  })

  // Update history when metrics change
  useEffect(() => {
    if (metrics) {
      const now = Date.now()
      setCpuHistory(prev => {
        const updated = [...prev, { time: now, value: metrics.cpu.percent }]
        return updated.slice(-60) // Keep last 60 data points (2 minutes)
      })
    }
  }, [metrics])

  if (isLoading || healthLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
      </div>
    )
  }

  // Build services list from health check and plugins
  const baseServices = [
    {
      name: 'API Server',
      status: health?.status === 'healthy' || health?.status === 'degraded' ? 'healthy' : 'error',
      details: `v${health?.version || '0.2.0'}`,
    },
    {
      name: 'Database',
      status: systemHealth?.database === 'ok' ? 'healthy' : 'error',
      details: 'SQLite',
    },
    {
      name: 'Event Bus (NATS)',
      status: systemHealth?.event_bus === 'connected' ? 'healthy' : 'error',
      details: systemHealth?.event_bus === 'connected' ? 'Connected' : 'Disconnected',
    },
  ]

  // Add plugins as services
  // Backend returns 'state' (running/stopped/error) and 'health' (healthy/degraded/unhealthy)
  const pluginServices = (plugins || []).map(plugin => ({
    name: plugin.name,
    status: plugin.state === 'running' && plugin.health === 'healthy' ? 'healthy' :
            plugin.state === 'running' && plugin.health === 'degraded' ? 'warning' :
            plugin.state === 'error' || plugin.health === 'unhealthy' ? 'error' : 'stopped',
    details: `v${plugin.version}${plugin.health ? ` - ${plugin.health}` : ''}`,
    isPlugin: true,
  }))

  const services = [...baseServices, ...pluginServices]

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold">System Health</h1>
          <p className="text-muted-foreground">
            Last updated: {new Date(dataUpdatedAt).toLocaleTimeString()}
          </p>
        </div>
        <button
          onClick={() => refetch()}
          className="inline-flex items-center gap-2 px-4 py-2 bg-secondary text-secondary-foreground rounded-lg hover:bg-secondary/80 transition-colors"
        >
          <RefreshCw size={18} />
          Refresh
        </button>
      </div>

      {/* Quick Stats Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard
          title="Cameras"
          value={`${stats?.cameras.online ?? 0}/${stats?.cameras.total ?? 0}`}
          subtitle="Online"
          icon={Camera}
          color="text-green-500"
        />
        <StatCard
          title="Events Today"
          value={stats?.events.today ?? 0}
          subtitle={`${stats?.events.unacknowledged ?? 0} unacknowledged`}
          icon={AlertTriangle}
          color="text-yellow-500"
        />
        <StatCard
          title="System Status"
          value={health?.status === 'healthy' ? 'Healthy' : 'Degraded'}
          subtitle={`${services.filter(s => s.status === 'healthy').length}/${services.length} services running`}
          icon={Activity}
          color={health?.status === 'healthy' ? 'text-green-500' : 'text-yellow-500'}
        />
        <StatCard
          title="Uptime"
          value={formatUptime(metrics?.uptime || 0)}
          subtitle="System uptime"
          icon={Clock}
          color="text-blue-500"
        />
      </div>

      {/* System Resources */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* CPU Usage */}
        <div className="bg-card rounded-lg border p-6">
          <div className="flex items-center justify-between mb-4">
            <div className="flex items-center gap-2">
              <Cpu className="h-5 w-5 text-blue-500" />
              <h2 className="font-semibold">CPU Usage</h2>
            </div>
            <span className="text-2xl font-bold">{(metrics?.cpu.percent || 0).toFixed(1)}%</span>
          </div>
          <div className="h-24 mb-4">
            <MiniLineChart data={cpuHistory} color="#3b82f6" />
          </div>
          <div className="flex justify-between text-sm text-muted-foreground">
            <span>Load: {metrics?.cpu.load_avg?.[0].toFixed(2) || '0.00'}</span>
            <span>{metrics?.cpu.load_avg?.[1].toFixed(2) || '0.00'}</span>
            <span>{metrics?.cpu.load_avg?.[2].toFixed(2) || '0.00'}</span>
          </div>
        </div>

        {/* Memory Usage */}
        <div className="bg-card rounded-lg border p-6">
          <div className="flex items-center justify-between mb-4">
            <div className="flex items-center gap-2">
              <MemoryStick className="h-5 w-5 text-purple-500" />
              <h2 className="font-semibold">Memory Usage</h2>
            </div>
            <span className="text-2xl font-bold">{(metrics?.memory.percent || 0).toFixed(1)}%</span>
          </div>
          <ProgressBar
            value={metrics?.memory.percent || 0}
            color="bg-purple-500"
            className="h-4 mb-4"
          />
          <div className="flex justify-between text-sm text-muted-foreground">
            <span>Used: {formatBytes(metrics?.memory.used || 0)}</span>
            <span>Free: {formatBytes(metrics?.memory.free || 0)}</span>
            <span>Total: {formatBytes(metrics?.memory.total || 0)}</span>
          </div>
        </div>

        {/* Storage */}
        <div className="bg-card rounded-lg border p-6">
          <div className="flex items-center justify-between mb-4">
            <div className="flex items-center gap-2">
              <HardDrive className="h-5 w-5 text-green-500" />
              <h2 className="font-semibold">Storage</h2>
            </div>
            <span className="text-2xl font-bold">{(metrics?.disk.percent || 0).toFixed(1)}%</span>
          </div>
          <ProgressBar
            value={metrics?.disk.percent || 0}
            color={metrics?.disk.percent && metrics.disk.percent > 90 ? 'bg-red-500' : 'bg-green-500'}
            className="h-4 mb-4"
          />
          <div className="flex justify-between text-sm text-muted-foreground">
            <span>Used: {formatBytes(metrics?.disk.used || 0)}</span>
            <span>Free: {formatBytes(metrics?.disk.free || 0)}</span>
            <span>Total: {formatBytes(metrics?.disk.total || 0)}</span>
          </div>
          <p className="text-xs text-muted-foreground mt-2 font-mono">{metrics?.disk.path}</p>
        </div>

        {/* GPU(s) - only show if at least one GPU is available */}
        {(() => {
          // Normalize to array of GPUs
          const gpuList: GPUInfo[] = metrics?.gpus?.filter(g => g.available) ||
            (metrics?.gpu?.available ? [metrics.gpu] : [])

          if (gpuList.length === 0) return null

          return (
            <div className="bg-card rounded-lg border p-6">
              <div className="flex items-center justify-between mb-4">
                <div className="flex items-center gap-2">
                  <Server className="h-5 w-5 text-orange-500" />
                  <h2 className="font-semibold">
                    {gpuList.length > 1 ? `GPUs (${gpuList.length})` : 'GPU'}
                  </h2>
                </div>
                <span className="text-sm text-green-500">Available</span>
              </div>

              <div className="space-y-4">
                {gpuList.map((gpu, idx) => {
                  const colorScheme = GPU_COLORS[idx % GPU_COLORS.length]
                  return (
                    <div key={gpu.index ?? idx} className={gpuList.length > 1 ? 'pb-4 border-b border-border last:border-0 last:pb-0' : ''}>
                      <div className="flex items-center gap-2 mb-2">
                        {gpuList.length > 1 && (
                          <div className={`w-3 h-3 rounded-full ${colorScheme.bg}`} />
                        )}
                        <p className="text-lg font-medium">{gpu.name || `GPU ${idx}`}</p>
                      </div>
                      {gpu.utilization !== undefined && (
                        <div className="space-y-3">
                          <div>
                            <div className="flex justify-between text-sm mb-1">
                              <span>Utilization</span>
                              <span>{gpu.utilization}%</span>
                            </div>
                            <ProgressBar value={gpu.utilization} color={colorScheme.bg} />
                          </div>
                          {gpu.memory_total && (
                            <div>
                              <div className="flex justify-between text-sm mb-1">
                                <span>VRAM</span>
                                <span>{formatBytes(gpu.memory_used || 0)} / {formatBytes(gpu.memory_total)}</span>
                              </div>
                              <ProgressBar
                                value={(gpu.memory_used || 0) / gpu.memory_total * 100}
                                color={colorScheme.bg}
                              />
                            </div>
                          )}
                          {gpu.temperature !== undefined && (
                            <div className="flex items-center gap-2 text-sm">
                              <Thermometer size={14} />
                              <span>{gpu.temperature}°C</span>
                            </div>
                          )}
                        </div>
                      )}
                      {gpu.type === 'apple' && (
                        <p className="text-sm text-muted-foreground mt-2">
                          Unified memory architecture - shares system RAM
                        </p>
                      )}
                    </div>
                  )
                })}
              </div>

              {/* Color legend for multiple GPUs */}
              {gpuList.length > 1 && (
                <div className="mt-4 pt-4 border-t border-border">
                  <div className="flex flex-wrap gap-4 text-xs text-muted-foreground">
                    {gpuList.map((gpu, idx) => {
                      const colorScheme = GPU_COLORS[idx % GPU_COLORS.length]
                      return (
                        <div key={gpu.index ?? idx} className="flex items-center gap-1.5">
                          <div className={`w-2.5 h-2.5 rounded-full ${colorScheme.bg}`} />
                          <span>{gpu.name || `GPU ${idx}`}</span>
                        </div>
                      )
                    })}
                  </div>
                </div>
              )}
            </div>
          )
        })()}

        {/* NPU - only show if available */}
        {metrics?.npu?.available && (
          <div className="bg-card rounded-lg border p-6">
            <div className="flex items-center justify-between mb-4">
              <div className="flex items-center gap-2">
                <Cpu className="h-5 w-5 text-purple-500" />
                <h2 className="font-semibold">NPU (Neural Processing Unit)</h2>
              </div>
              <span className="text-sm text-green-500">Available</span>
            </div>

            <p className="text-lg font-medium mb-3">{metrics.npu.name}</p>
            {metrics.npu.utilization !== undefined && (
              <div className="space-y-3">
                <div>
                  <div className="flex justify-between text-sm mb-1">
                    <span>Utilization</span>
                    <span>{metrics.npu.utilization}%</span>
                  </div>
                  <ProgressBar value={metrics.npu.utilization} color="bg-purple-500" />
                </div>
              </div>
            )}
            {metrics.npu.type && (
              <p className="text-sm text-muted-foreground mt-2">
                Type: {metrics.npu.type}
              </p>
            )}
          </div>
        )}
      </div>

      {/* Services Status - split into Core and Plugins */}
      <div className="bg-card rounded-lg border">
        <div className="p-4 border-b flex items-center gap-2">
          <Server className="h-5 w-5 text-muted-foreground" />
          <h2 className="font-semibold">Core Services</h2>
        </div>
        <div className="divide-y">
          {baseServices.map((service) => (
            <div key={service.name} className="p-4 flex items-center justify-between">
              <div className="flex items-center gap-3">
                {service.status === 'healthy' ? (
                  <CheckCircle className="h-5 w-5 text-green-500" />
                ) : service.status === 'error' ? (
                  <XCircle className="h-5 w-5 text-red-500" />
                ) : (
                  <div className="h-5 w-5 rounded-full border-2 border-gray-400" />
                )}
                <div>
                  <div className="font-medium">{service.name}</div>
                  <div className="text-sm text-muted-foreground">{service.details}</div>
                </div>
              </div>
              <span className={`text-sm capitalize ${
                service.status === 'healthy' ? 'text-green-500' :
                service.status === 'error' ? 'text-red-500' :
                'text-muted-foreground'
              }`}>
                {service.status}
              </span>
            </div>
          ))}
        </div>
      </div>

      {/* Plugin Status */}
      <div className="bg-card rounded-lg border">
        <div className="p-4 border-b flex items-center gap-2">
          <Puzzle className="h-5 w-5 text-muted-foreground" />
          <h2 className="font-semibold">Plugins ({pluginServices.length})</h2>
        </div>
        {pluginServices.length > 0 ? (
          <div className="divide-y">
            {pluginServices.map((service) => (
              <div key={service.name} className="p-4 flex items-center justify-between">
                <div className="flex items-center gap-3">
                  {service.status === 'healthy' ? (
                    <CheckCircle className="h-5 w-5 text-green-500" />
                  ) : service.status === 'error' ? (
                    <XCircle className="h-5 w-5 text-red-500" />
                  ) : (
                    <div className="h-5 w-5 rounded-full border-2 border-gray-400" />
                  )}
                  <div>
                    <div className="font-medium">{service.name}</div>
                    <div className="text-sm text-muted-foreground">{service.details}</div>
                  </div>
                </div>
                <span className={`text-sm capitalize ${
                  service.status === 'healthy' ? 'text-green-500' :
                  service.status === 'error' ? 'text-red-500' :
                  'text-muted-foreground'
                }`}>
                  {service.status}
                </span>
              </div>
            ))}
          </div>
        ) : (
          <div className="p-8 text-center text-muted-foreground">
            <Puzzle className="h-8 w-8 mx-auto mb-2 opacity-50" />
            <p className="text-sm">No plugins loaded</p>
            <p className="text-xs">Visit Plugins page to install extensions</p>
          </div>
        )}
      </div>

      {/* Recording Storage */}
      <div className="bg-card rounded-lg border p-6">
        <div className="flex items-center gap-2 mb-4">
          <HardDrive className="h-5 w-5 text-muted-foreground" />
          <h2 className="font-semibold">Recording Storage</h2>
        </div>
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
          <div>
            <span className="text-muted-foreground">Database Size</span>
            <p className="text-lg font-semibold">{formatBytes(stats?.storage.database_size || 0)}</p>
          </div>
          <div>
            <span className="text-muted-foreground">Storage Capacity</span>
            <p className="text-lg font-semibold">{formatBytes(metrics?.disk.total || 0)}</p>
          </div>
          <div>
            <span className="text-muted-foreground">Available</span>
            <p className="text-lg font-semibold text-green-500">{formatBytes(metrics?.disk.free || 0)}</p>
          </div>
          <div>
            <span className="text-muted-foreground">Retention</span>
            <p className="text-lg font-semibold">30 days</p>
          </div>
        </div>
      </div>

      {/* System Info */}
      <div className="bg-card rounded-lg border p-6">
        <h2 className="font-semibold mb-4">System Information</h2>
        <dl className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
          <div>
            <dt className="text-muted-foreground">Version</dt>
            <dd className="font-medium">{health?.version || '0.1.0'}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">go2rtc</dt>
            <dd className="font-medium">v1.9.13</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">AI Model</dt>
            <dd className="font-medium">YOLOv12</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">Database</dt>
            <dd className="font-medium">SQLite</dd>
          </div>
        </dl>
      </div>

      {/* Live Logs */}
      <LogViewer />
    </div>
  )
}

// Helper Components
function StatCard({ title, value, subtitle, icon: Icon, color }: {
  title: string
  value: string | number
  subtitle: string
  icon: React.ElementType
  color: string
}) {
  return (
    <div className="bg-card rounded-lg border p-6 space-y-2">
      <div className="flex items-center justify-between">
        <span className="text-sm text-muted-foreground">{title}</span>
        <Icon className={`h-5 w-5 ${color}`} />
      </div>
      <div className="text-2xl font-bold">{value}</div>
      <div className="text-sm text-muted-foreground">{subtitle}</div>
    </div>
  )
}

function ProgressBar({ value, color, className = '' }: {
  value: number
  color: string
  className?: string
}) {
  return (
    <div className={`bg-muted rounded-full overflow-hidden ${className}`}>
      <div
        className={`h-full ${color} transition-all duration-300`}
        style={{ width: `${Math.min(100, Math.max(0, value))}%` }}
      />
    </div>
  )
}

function MiniLineChart({ data, color }: { data: HistoryPoint[], color: string }) {
  const canvasRef = useRef<HTMLCanvasElement>(null)

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas || data.length < 2) return

    const ctx = canvas.getContext('2d')
    if (!ctx) return

    const rect = canvas.getBoundingClientRect()
    canvas.width = rect.width * 2
    canvas.height = rect.height * 2
    ctx.scale(2, 2)

    const width = rect.width
    const height = rect.height
    const padding = 4

    // Clear
    ctx.clearRect(0, 0, width, height)

    // Draw grid lines
    ctx.strokeStyle = 'rgba(255,255,255,0.1)'
    ctx.lineWidth = 1
    for (let i = 0; i <= 4; i++) {
      const y = padding + (height - 2 * padding) * (i / 4)
      ctx.beginPath()
      ctx.moveTo(0, y)
      ctx.lineTo(width, y)
      ctx.stroke()
    }

    // Find min/max
    const values = data.map(d => d.value)
    const max = Math.max(...values, 100)
    const min = 0

    // Draw line
    ctx.beginPath()
    ctx.strokeStyle = color
    ctx.lineWidth = 2
    ctx.lineJoin = 'round'

    data.forEach((point, i) => {
      const x = padding + (width - 2 * padding) * (i / (data.length - 1))
      const y = height - padding - (height - 2 * padding) * ((point.value - min) / (max - min))

      if (i === 0) {
        ctx.moveTo(x, y)
      } else {
        ctx.lineTo(x, y)
      }
    })
    ctx.stroke()

    // Draw fill
    ctx.lineTo(width - padding, height - padding)
    ctx.lineTo(padding, height - padding)
    ctx.closePath()
    ctx.fillStyle = color.replace(')', ', 0.1)').replace('rgb', 'rgba')
    ctx.fill()
  }, [data, color])

  return <canvas ref={canvasRef} className="w-full h-full" />
}

function LogViewer() {
  const { apiUrl } = usePorts()
  const [logs, setLogs] = useState<string[]>([])
  const [isStreaming, setIsStreaming] = useState(true)
  const [autoScroll, setAutoScroll] = useState(true)
  const logContainerRef = useRef<HTMLDivElement>(null)
  const eventSourceRef = useRef<EventSource | null>(null)

  useEffect(() => {
    if (!isStreaming) {
      if (eventSourceRef.current) {
        eventSourceRef.current.close()
        eventSourceRef.current = null
      }
      return
    }

    const eventSource = new EventSource(`${apiUrl}/api/v1/logs/stream`)
    eventSourceRef.current = eventSource

    eventSource.onmessage = (event) => {
      setLogs(prev => {
        const newLogs = [...prev, event.data]
        if (newLogs.length > 200) {
          return newLogs.slice(-200)
        }
        return newLogs
      })
    }

    eventSource.onerror = () => {
      setTimeout(() => {
        if (isStreaming && eventSourceRef.current === eventSource) {
          eventSource.close()
          setIsStreaming(false)
          setTimeout(() => setIsStreaming(true), 1000)
        }
      }, 2000)
    }

    return () => {
      eventSource.close()
    }
  }, [isStreaming])

  useEffect(() => {
    if (autoScroll && logContainerRef.current) {
      logContainerRef.current.scrollTop = logContainerRef.current.scrollHeight
    }
  }, [logs, autoScroll])

  const getLogColor = (log: string) => {
    if (log.includes('ERROR')) return 'text-red-400'
    if (log.includes('WARN')) return 'text-yellow-400'
    if (log.includes('DEBUG')) return 'text-gray-500'
    return 'text-gray-300'
  }

  return (
    <div className="bg-card rounded-lg border">
      <div className="p-4 border-b flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Terminal size={18} className="text-muted-foreground" />
          <h2 className="font-semibold">Live Logs</h2>
          {isStreaming && (
            <span className="flex items-center gap-1 text-xs text-green-500">
              <span className="w-2 h-2 bg-green-500 rounded-full animate-pulse" />
              Streaming
            </span>
          )}
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setAutoScroll(!autoScroll)}
            className={`p-2 rounded-lg transition-colors ${
              autoScroll ? 'bg-primary/20 text-primary' : 'hover:bg-accent'
            }`}
            title={autoScroll ? 'Auto-scroll enabled' : 'Auto-scroll disabled'}
          >
            <Activity size={16} />
          </button>
          <button
            onClick={() => setIsStreaming(!isStreaming)}
            className="p-2 rounded-lg hover:bg-accent transition-colors"
            title={isStreaming ? 'Pause' : 'Resume'}
          >
            {isStreaming ? <Pause size={16} /> : <Play size={16} />}
          </button>
          <button
            onClick={() => setLogs([])}
            className="p-2 rounded-lg hover:bg-accent transition-colors"
            title="Clear logs"
          >
            <XCircle size={16} />
          </button>
        </div>
      </div>
      <div
        ref={logContainerRef}
        className="h-64 overflow-y-auto bg-black/50 p-4 font-mono text-xs"
        onScroll={(e) => {
          const target = e.target as HTMLDivElement
          const isAtBottom = target.scrollHeight - target.scrollTop <= target.clientHeight + 50
          if (!isAtBottom && autoScroll) {
            setAutoScroll(false)
          }
        }}
      >
        {logs.length === 0 ? (
          <div className="text-gray-500 text-center py-8">
            {isStreaming ? 'Waiting for logs...' : 'Log streaming paused'}
          </div>
        ) : (
          logs.map((log, i) => (
            <div key={i} className={`py-0.5 ${getLogColor(log)}`}>
              {log}
            </div>
          ))
        )}
      </div>
    </div>
  )
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`
}

function formatUptime(seconds: number): string {
  if (seconds === 0) return '0s'
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)

  if (days > 0) return `${days}d ${hours}h`
  if (hours > 0) return `${hours}h ${minutes}m`
  return `${minutes}m`
}
