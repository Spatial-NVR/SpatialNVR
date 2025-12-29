import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Camera,
  Search,
  Plus,
  Loader2,
  AlertCircle,
  Wifi,
  Video,
  Mic,
  Eye,
  ChevronDown,
  ChevronRight,
  X
} from 'lucide-react'
import { useToast } from '../Toast'

interface StreamConfig {
  width: number
  height: number
  frame_rate: number
  bit_rate: number
  codec: string
}

interface ChannelInfo {
  channel: number
  name?: string
  codec: string
  main_stream: StreamConfig
  sub_stream: StreamConfig
  rtmp_main: string
  rtmp_sub: string
  rtsp_main: string
  rtsp_sub: string
}

interface ProbeResult {
  host: string
  port: number
  model: string
  name: string
  serial: string
  firmware_version: string
  device_type: string
  is_doorbell: boolean
  is_nvr: boolean
  is_battery: boolean
  has_ptz: boolean
  has_two_way_audio: boolean
  has_audio_alarm: boolean
  has_ai_detection: boolean
  channel_count: number
  channels: ChannelInfo[]
}

interface ReolinkSetupProps {
  pluginId: string
  onCameraAdded?: () => void
}

export function ReolinkSetup({ pluginId, onCameraAdded }: ReolinkSetupProps) {
  const { addToast } = useToast()
  const queryClient = useQueryClient()

  // Form state
  const [host, setHost] = useState('')
  const [port, setPort] = useState(80)
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const [cameraName, setCameraName] = useState('')

  // Probe result
  const [probeResult, setProbeResult] = useState<ProbeResult | null>(null)
  const [selectedChannels, setSelectedChannels] = useState<Set<number>>(new Set())
  const [expandedChannels, setExpandedChannels] = useState<Set<number>>(new Set())

  // Probe mutation
  const probeMutation = useMutation({
    mutationFn: async () => {
      const response = await fetch(`/api/v1/plugins/${pluginId}/rpc`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          jsonrpc: '2.0',
          id: Date.now(),
          method: 'probe_camera',
          params: { host, port, username, password }
        })
      })
      const data = await response.json()
      if (data.error) {
        throw new Error(data.error.message)
      }
      return data.result as ProbeResult
    },
    onSuccess: (result) => {
      setProbeResult(result)
      // Select all channels by default
      setSelectedChannels(new Set(result.channels.map(ch => ch.channel)))
      setCameraName(result.name || '')
      addToast('success', `Found ${result.model} with ${result.channel_count} channel(s)`)
    },
    onError: (err: Error) => {
      addToast('error', `Probe failed: ${err.message}`)
    }
  })

  // Add camera mutation
  const addMutation = useMutation({
    mutationFn: async (channels: number[]) => {
      const results = []
      for (const channel of channels) {
        const channelInfo = probeResult?.channels.find(ch => ch.channel === channel)
        const displayName = channels.length === 1 ? cameraName : `${cameraName} Ch${channel + 1}`

        // First add to plugin
        const response = await fetch(`/api/v1/plugins/${pluginId}/rpc`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            jsonrpc: '2.0',
            id: Date.now(),
            method: 'add_camera',
            params: {
              host,
              port,
              username,
              password,
              channel,
              name: displayName
            }
          })
        })
        const data = await response.json()
        if (data.error) {
          throw new Error(data.error.message)
        }

        // Also register with NVR camera service for go2rtc
        const pluginCamera = data.result

        // Use the plugin camera's stream URLs (which respect the protocol setting - HLS by default)
        // Fall back to probe result RTSP URLs only if plugin doesn't provide URLs
        const streamUrl = pluginCamera?.main_stream || channelInfo?.rtsp_main
        const subStreamUrl = pluginCamera?.sub_stream || channelInfo?.rtsp_sub

        // Format the camera configuration with proper structure expected by the API
        const cameraConfig = {
          name: displayName,
          stream: {
            url: streamUrl,
            sub_url: subStreamUrl,
            username: username,
            password: password,
            // Use main stream for recording, sub stream for detection/motion
            roles: {
              detect: 'sub' as const,
              record: 'main' as const,
              audio: 'main' as const,
              motion: 'sub' as const,
            },
          },
          manufacturer: 'Reolink',
          model: probeResult?.model || pluginCamera?.model,
          // Include device capabilities
          audio: {
            enabled: probeResult?.has_two_way_audio || false,
            two_way: probeResult?.has_two_way_audio || false,
          },
          detection: {
            enabled: probeResult?.has_ai_detection || false,
            fps: 5,
            show_overlay: true,
            min_confidence: 0.5,
          },
          // Store extra metadata
          plugin_id: pluginId,
          plugin_camera_id: pluginCamera?.id || `${host}_ch${channel}`
        }

        try {
          const createResponse = await fetch('/api/v1/cameras', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(cameraConfig)
          })
          if (createResponse.ok) {
            results.push(await createResponse.json())
          } else {
            // If NVR registration fails, still count the plugin add as success
            results.push(pluginCamera)
            console.warn(`Failed to register camera with NVR:`, await createResponse.text())
          }
        } catch (err) {
          results.push(pluginCamera)
          console.warn(`Failed to register camera with NVR:`, err)
        }
      }
      return results
    },
    onSuccess: (results) => {
      addToast('success', `Added ${results.length} camera(s)`)
      queryClient.invalidateQueries({ queryKey: ['cameras'] })
      // Reset form
      setProbeResult(null)
      setHost('')
      setPassword('')
      setCameraName('')
      onCameraAdded?.()
    },
    onError: (err: Error) => {
      addToast('error', `Failed to add camera: ${err.message}`)
    }
  })

  const toggleChannel = (channel: number) => {
    setSelectedChannels(prev => {
      const next = new Set(prev)
      if (next.has(channel)) {
        next.delete(channel)
      } else {
        next.add(channel)
      }
      return next
    })
  }

  const toggleChannelExpand = (channel: number) => {
    setExpandedChannels(prev => {
      const next = new Set(prev)
      if (next.has(channel)) {
        next.delete(channel)
      } else {
        next.add(channel)
      }
      return next
    })
  }

  const handleProbe = (e: React.FormEvent) => {
    e.preventDefault()
    if (!host || !password) {
      addToast('error', 'Please enter IP address and password')
      return
    }
    probeMutation.mutate()
  }

  const handleAdd = () => {
    const channels = Array.from(selectedChannels)
    if (channels.length === 0) {
      addToast('error', 'Please select at least one channel')
      return
    }
    addMutation.mutate(channels)
  }

  return (
    <div className="space-y-6">
      {/* Discovery Form */}
      <form onSubmit={handleProbe} className="space-y-4">
        <div className="grid gap-4 md:grid-cols-2">
          <div className="space-y-2">
            <label className="text-sm font-medium">IP Address *</label>
            <input
              type="text"
              value={host}
              onChange={e => setHost(e.target.value)}
              placeholder="192.168.1.100"
              className="w-full px-3 py-2 bg-background border border-border rounded-md"
              disabled={probeMutation.isPending}
            />
          </div>
          <div className="space-y-2">
            <label className="text-sm font-medium">Port</label>
            <input
              type="number"
              value={port}
              onChange={e => setPort(parseInt(e.target.value) || 80)}
              className="w-full px-3 py-2 bg-background border border-border rounded-md"
              disabled={probeMutation.isPending}
            />
          </div>
          <div className="space-y-2">
            <label className="text-sm font-medium">Username</label>
            <input
              type="text"
              value={username}
              onChange={e => setUsername(e.target.value)}
              className="w-full px-3 py-2 bg-background border border-border rounded-md"
              disabled={probeMutation.isPending}
            />
          </div>
          <div className="space-y-2">
            <label className="text-sm font-medium">Password *</label>
            <input
              type="password"
              value={password}
              onChange={e => setPassword(e.target.value)}
              className="w-full px-3 py-2 bg-background border border-border rounded-md"
              disabled={probeMutation.isPending}
            />
          </div>
        </div>

        <button
          type="submit"
          disabled={probeMutation.isPending || !host || !password}
          className="flex items-center gap-2 px-4 py-2 bg-primary text-primary-foreground rounded-md hover:bg-primary/90 disabled:opacity-50"
        >
          {probeMutation.isPending ? (
            <Loader2 className="w-4 h-4 animate-spin" />
          ) : (
            <Search className="w-4 h-4" />
          )}
          Discover Camera
        </button>
      </form>

      {/* Probe Results */}
      {probeResult && (
        <div className="border border-border rounded-lg overflow-hidden">
          {/* Device Info Header */}
          <div className="bg-accent/50 p-4 border-b border-border">
            <div className="flex items-start justify-between">
              <div className="flex items-center gap-3">
                <div className="p-2 bg-primary/10 rounded-lg">
                  <Camera className="w-6 h-6 text-primary" />
                </div>
                <div>
                  <h3 className="font-semibold">{probeResult.model}</h3>
                  <p className="text-sm text-muted-foreground">
                    {probeResult.device_type} • {probeResult.host}:{probeResult.port}
                  </p>
                </div>
              </div>
              <button
                onClick={() => setProbeResult(null)}
                className="p-1 hover:bg-accent rounded"
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            {/* Capabilities */}
            <div className="flex flex-wrap gap-2 mt-3">
              {probeResult.has_ptz && (
                <span className="inline-flex items-center gap-1 px-2 py-0.5 text-xs bg-blue-500/10 text-blue-500 rounded">
                  <Eye className="w-3 h-3" /> PTZ
                </span>
              )}
              {probeResult.has_two_way_audio && (
                <span className="inline-flex items-center gap-1 px-2 py-0.5 text-xs bg-green-500/10 text-green-500 rounded">
                  <Mic className="w-3 h-3" /> Two-Way Audio
                </span>
              )}
              {probeResult.has_ai_detection && (
                <span className="inline-flex items-center gap-1 px-2 py-0.5 text-xs bg-purple-500/10 text-purple-500 rounded">
                  AI Detection
                </span>
              )}
              {probeResult.is_nvr && (
                <span className="inline-flex items-center gap-1 px-2 py-0.5 text-xs bg-orange-500/10 text-orange-500 rounded">
                  NVR
                </span>
              )}
              {probeResult.is_doorbell && (
                <span className="inline-flex items-center gap-1 px-2 py-0.5 text-xs bg-yellow-500/10 text-yellow-500 rounded">
                  Doorbell
                </span>
              )}
              {probeResult.is_battery && (
                <span className="inline-flex items-center gap-1 px-2 py-0.5 text-xs bg-cyan-500/10 text-cyan-500 rounded">
                  Battery
                </span>
              )}
            </div>

            {/* Device details */}
            <div className="grid grid-cols-3 gap-4 mt-4 text-sm">
              <div>
                <span className="text-muted-foreground">Firmware:</span>{' '}
                <span>{probeResult.firmware_version || 'Unknown'}</span>
              </div>
              <div>
                <span className="text-muted-foreground">Serial:</span>{' '}
                <span className="font-mono text-xs">{probeResult.serial || 'Unknown'}</span>
              </div>
              <div>
                <span className="text-muted-foreground">Channels:</span>{' '}
                <span>{probeResult.channel_count}</span>
              </div>
            </div>
          </div>

          {/* Camera Name */}
          <div className="p-4 border-b border-border">
            <label className="block text-sm font-medium mb-2">Camera Name</label>
            <input
              type="text"
              value={cameraName}
              onChange={e => setCameraName(e.target.value)}
              placeholder="Enter a name for this camera"
              className="w-full px-3 py-2 bg-background border border-border rounded-md"
            />
          </div>

          {/* Channels */}
          <div className="divide-y divide-border">
            {probeResult.channels.map(ch => (
              <div key={ch.channel} className="p-4">
                <div className="flex items-center gap-3">
                  <input
                    type="checkbox"
                    checked={selectedChannels.has(ch.channel)}
                    onChange={() => toggleChannel(ch.channel)}
                    className="w-4 h-4 rounded border-border"
                  />
                  <button
                    onClick={() => toggleChannelExpand(ch.channel)}
                    className="flex items-center gap-2 flex-1 text-left"
                  >
                    {expandedChannels.has(ch.channel) ? (
                      <ChevronDown className="w-4 h-4 text-muted-foreground" />
                    ) : (
                      <ChevronRight className="w-4 h-4 text-muted-foreground" />
                    )}
                    <Video className="w-4 h-4" />
                    <span className="font-medium">
                      Channel {ch.channel + 1}
                      {ch.name && ` - ${ch.name}`}
                    </span>
                    <span className="text-sm text-muted-foreground ml-2">
                      {ch.main_stream.width}x{ch.main_stream.height} @ {ch.main_stream.frame_rate}fps
                    </span>
                  </button>
                </div>

                {expandedChannels.has(ch.channel) && (
                  <div className="mt-3 ml-9 grid gap-3 md:grid-cols-2">
                    {/* Main Stream */}
                    <div className="p-3 bg-accent/30 rounded-md">
                      <div className="text-sm font-medium mb-2">Main Stream</div>
                      <div className="text-xs space-y-1 text-muted-foreground">
                        <div>Resolution: {ch.main_stream.width}x{ch.main_stream.height}</div>
                        <div>Frame Rate: {ch.main_stream.frame_rate} fps</div>
                        <div>Bit Rate: {Math.round(ch.main_stream.bit_rate / 1000)} kbps</div>
                        <div>Codec: {ch.main_stream.codec || 'H.264'}</div>
                      </div>
                    </div>
                    {/* Sub Stream */}
                    <div className="p-3 bg-accent/30 rounded-md">
                      <div className="text-sm font-medium mb-2">Sub Stream</div>
                      <div className="text-xs space-y-1 text-muted-foreground">
                        <div>Resolution: {ch.sub_stream.width}x{ch.sub_stream.height}</div>
                        <div>Frame Rate: {ch.sub_stream.frame_rate} fps</div>
                        <div>Bit Rate: {Math.round(ch.sub_stream.bit_rate / 1000)} kbps</div>
                        <div>Codec: {ch.sub_stream.codec || 'H.264'}</div>
                      </div>
                    </div>
                    {/* Stream Info */}
                    <div className="md:col-span-2 p-3 bg-gray-900 rounded-md font-mono text-xs">
                      <div className="flex items-center gap-2 text-muted-foreground mb-1">
                        <Wifi className="w-3 h-3" /> Stream Protocol: <span className="text-green-400">RTSP</span>
                      </div>
                      <div className="text-muted-foreground text-xs mt-1">
                        Streams use RTSP for better audio support with go2rtc
                      </div>
                    </div>
                  </div>
                )}
              </div>
            ))}
          </div>

          {/* Add Button */}
          <div className="p-4 bg-accent/30 border-t border-border">
            <div className="flex items-center justify-between">
              <div className="text-sm text-muted-foreground">
                {selectedChannels.size} of {probeResult.channel_count} channel(s) selected
              </div>
              <button
                onClick={handleAdd}
                disabled={addMutation.isPending || selectedChannels.size === 0}
                className="flex items-center gap-2 px-4 py-2 bg-green-600 text-white rounded-md hover:bg-green-700 disabled:opacity-50"
              >
                {addMutation.isPending ? (
                  <Loader2 className="w-4 h-4 animate-spin" />
                ) : (
                  <Plus className="w-4 h-4" />
                )}
                Add {selectedChannels.size > 1 ? `${selectedChannels.size} Cameras` : 'Camera'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Help text */}
      {!probeResult && (
        <div className="flex items-start gap-3 p-4 bg-blue-500/10 border border-blue-500/20 rounded-lg">
          <AlertCircle className="w-5 h-5 text-blue-500 shrink-0 mt-0.5" />
          <div className="text-sm">
            <p className="font-medium text-blue-500">How to add a Reolink camera</p>
            <ol className="mt-2 space-y-1 text-muted-foreground list-decimal list-inside">
              <li>Enter the camera's IP address (find it in your router or Reolink app)</li>
              <li>Enter the admin password you set during camera setup</li>
              <li>Click "Discover Camera" to probe the device</li>
              <li>Review the detected settings and select which channels to add</li>
              <li>Click "Add Camera" to add it to your NVR</li>
            </ol>
          </div>
        </div>
      )}
    </div>
  )
}
