import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { cameraApi, type ProtocolOption as ProtocolOptionApiType } from '../lib/api'
import { Link } from 'react-router-dom'
import { ExternalLink, Loader2, CheckCircle, AlertCircle, Video, Mic, Camera, Wifi, Cpu, Settings, RefreshCw } from 'lucide-react'
import { useState } from 'react'

interface PluginCameraSettingsProps {
  cameraId: string
  pluginId: string
  pluginCameraId: string
  onToast?: (message: string, type?: 'success' | 'error') => void
}

export function PluginCameraSettings({ cameraId, pluginId, pluginCameraId, onToast }: PluginCameraSettingsProps) {
  const queryClient = useQueryClient()
  const [selectedProtocol, setSelectedProtocol] = useState<string | null>(null)

  // Fetch capabilities
  const { data: capabilities, isLoading: capabilitiesLoading, refetch: refetchCapabilities } = useQuery({
    queryKey: ['camera-capabilities', cameraId],
    queryFn: () => cameraApi.getCapabilities(cameraId),
  })

  // Fetch protocols
  const { data: protocols, isLoading: protocolsLoading } = useQuery({
    queryKey: ['camera-protocols', cameraId],
    queryFn: () => cameraApi.getProtocols(cameraId),
  })

  // Fetch device info
  const { data: deviceInfo, isLoading: deviceInfoLoading } = useQuery({
    queryKey: ['camera-device-info', cameraId],
    queryFn: () => cameraApi.getDeviceInfo(cameraId),
  })

  // Set protocol mutation
  const setProtocolMutation = useMutation({
    mutationFn: (protocolId: string) => cameraApi.setProtocol(cameraId, protocolId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['camera-capabilities', cameraId] })
      queryClient.invalidateQueries({ queryKey: ['camera-protocols', cameraId] })
      queryClient.invalidateQueries({ queryKey: ['camera', cameraId] })
      onToast?.('Protocol changed successfully', 'success')
    },
    onError: (error) => {
      onToast?.(`Failed to change protocol: ${error}`, 'error')
    },
  })

  const isLoading = capabilitiesLoading || protocolsLoading || deviceInfoLoading

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-8">
        <Loader2 className="w-6 h-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  const currentProtocol = capabilities?.current_protocol || protocols?.find(p => p.stream_url)?.id

  return (
    <div className="space-y-6">
      {/* Plugin Info */}
      <div className="bg-muted/30 rounded-lg p-4">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Settings className="w-4 h-4 text-muted-foreground" />
            <span className="text-sm">
              Managed by plugin: <span className="font-medium">{pluginId}</span>
            </span>
          </div>
          <Link
            to={`/plugins/${pluginId}`}
            className="flex items-center gap-1 text-sm text-primary hover:underline"
          >
            View plugin
            <ExternalLink className="w-3 h-3" />
          </Link>
        </div>
        {pluginCameraId && (
          <p className="text-xs text-muted-foreground mt-1">
            Plugin Camera ID: {pluginCameraId}
          </p>
        )}
      </div>

      {/* Device Info */}
      {deviceInfo && (
        <div className="space-y-3">
          <h4 className="text-sm font-medium flex items-center gap-2">
            <Cpu className="w-4 h-4" />
            Device Information
          </h4>
          <div className="grid grid-cols-2 gap-4 text-sm">
            <div>
              <span className="text-muted-foreground">Model:</span>
              <span className="ml-2 font-medium">{deviceInfo.model || 'Unknown'}</span>
            </div>
            <div>
              <span className="text-muted-foreground">Manufacturer:</span>
              <span className="ml-2 font-medium">{deviceInfo.manufacturer || 'Unknown'}</span>
            </div>
            {deviceInfo.serial && (
              <div>
                <span className="text-muted-foreground">Serial:</span>
                <span className="ml-2 font-mono text-xs">{deviceInfo.serial}</span>
              </div>
            )}
            {deviceInfo.firmware_version && (
              <div>
                <span className="text-muted-foreground">Firmware:</span>
                <span className="ml-2">{deviceInfo.firmware_version}</span>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Capabilities */}
      {capabilities && (
        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <h4 className="text-sm font-medium flex items-center gap-2">
              <Camera className="w-4 h-4" />
              Capabilities
            </h4>
            <button
              onClick={() => refetchCapabilities()}
              className="p-1 hover:bg-muted rounded"
              title="Refresh capabilities"
            >
              <RefreshCw className="w-4 h-4 text-muted-foreground" />
            </button>
          </div>
          <div className="flex flex-wrap gap-2">
            {capabilities.has_ptz && (
              <CapabilityBadge label="PTZ" enabled />
            )}
            {capabilities.has_audio && (
              <CapabilityBadge label="Audio" enabled icon={<Mic className="w-3 h-3" />} />
            )}
            {capabilities.has_two_way_audio && (
              <CapabilityBadge label="Two-Way Audio" enabled />
            )}
            {capabilities.has_snapshot && (
              <CapabilityBadge label="Snapshot" enabled />
            )}
            {!!(capabilities.features as Record<string, unknown>)?.night_vision && (
              <CapabilityBadge label="Night Vision" enabled />
            )}
            {capabilities.has_ai_detection && (
              <CapabilityBadge label="AI Detection" enabled />
            )}
            {capabilities.is_battery && (
              <CapabilityBadge label="Battery" enabled />
            )}
            {capabilities.device_type && (
              <span className="px-2 py-1 text-xs bg-muted rounded-full">
                {capabilities.device_type}
              </span>
            )}
          </div>
          {capabilities.ai_types && capabilities.ai_types.length > 0 && (
            <div className="text-xs text-muted-foreground">
              AI Types: {capabilities.ai_types.join(', ')}
            </div>
          )}
        </div>
      )}

      {/* Protocol Selector */}
      {protocols && protocols.length > 0 && (
        <div className="space-y-3">
          <h4 className="text-sm font-medium flex items-center gap-2">
            <Wifi className="w-4 h-4" />
            Streaming Protocol
          </h4>
          <div className="space-y-2">
            {protocols.map((protocol) => (
              <ProtocolOptionButton
                key={protocol.id}
                protocol={protocol}
                isSelected={currentProtocol === protocol.id}
                isPending={setProtocolMutation.isPending && selectedProtocol === protocol.id}
                onSelect={() => {
                  setSelectedProtocol(protocol.id)
                  setProtocolMutation.mutate(protocol.id)
                }}
              />
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

function CapabilityBadge({
  label,
  enabled,
  icon
}: {
  label: string
  enabled: boolean
  icon?: React.ReactNode
}) {
  return (
    <span className={`
      inline-flex items-center gap-1 px-2 py-1 text-xs rounded-full
      ${enabled ? 'bg-green-500/10 text-green-600 dark:text-green-400' : 'bg-muted text-muted-foreground'}
    `}>
      {icon || (enabled ? <CheckCircle className="w-3 h-3" /> : <AlertCircle className="w-3 h-3" />)}
      {label}
    </span>
  )
}

function ProtocolOptionButton({
  protocol,
  isSelected,
  isPending,
  onSelect
}: {
  protocol: ProtocolOptionApiType
  isSelected: boolean
  isPending: boolean
  onSelect: () => void
}) {
  return (
    <button
      onClick={onSelect}
      disabled={isSelected || isPending}
      className={`
        w-full text-left p-3 rounded-lg border transition-colors
        ${isSelected
          ? 'border-primary bg-primary/5'
          : 'border-border hover:border-primary/50 hover:bg-muted/50'
        }
        ${isPending ? 'opacity-50' : ''}
      `}
    >
      <div className="flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2">
            <Video className="w-4 h-4 text-muted-foreground" />
            <span className="font-medium">{protocol.name}</span>
            {isSelected && <CheckCircle className="w-4 h-4 text-primary" />}
            {isPending && <Loader2 className="w-4 h-4 animate-spin" />}
          </div>
          {protocol.description && (
            <p className="text-xs text-muted-foreground mt-1">{protocol.description}</p>
          )}
        </div>
      </div>
      {protocol.stream_url && isSelected && (
        <div className="mt-2 text-xs font-mono text-muted-foreground bg-muted/50 px-2 py-1 rounded overflow-hidden text-ellipsis">
          {protocol.stream_url}
        </div>
      )}
    </button>
  )
}
