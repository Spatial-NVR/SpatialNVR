import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import {
  Camera,
  Video,
  VideoOff,
  ExternalLink,
  Loader2,
  AlertCircle,
  CheckCircle,
  Info,
  Settings,
  Plug
} from 'lucide-react'
import { cameraApi, pluginsApi, Camera as CameraType } from '../../lib/api'

interface PluginCamera {
  id: string
  name: string
  model?: string
  host?: string
  channel?: number
  online: boolean
  main_stream?: string
  sub_stream?: string
}

interface PluginCamerasTabProps {
  pluginId: string
}

export function PluginCamerasTab({ pluginId }: PluginCamerasTabProps) {
  // Fetch cameras from the plugin via RPC
  const { data: pluginCameras, isLoading: isLoadingPlugin, error: pluginError } = useQuery({
    queryKey: ['plugin-cameras', pluginId],
    queryFn: async () => {
      try {
        const result = await pluginsApi.rpc<PluginCamera[] | { cameras?: PluginCamera[] }>(pluginId, 'list_cameras', {})
        // Handle both array response and object with cameras property
        if (Array.isArray(result)) {
          return result
        }
        if (result && typeof result === 'object' && 'cameras' in result && Array.isArray(result.cameras)) {
          return result.cameras
        }
        return []
      } catch {
        return []
      }
    },
    refetchInterval: 10000,
  })

  // Fetch NVR cameras to match with plugin cameras
  const { data: nvrCameras, isLoading: isLoadingNvr } = useQuery({
    queryKey: ['cameras'],
    queryFn: cameraApi.list,
    refetchInterval: 10000,
  })

  // Match plugin cameras with NVR cameras
  const getCameraMapping = (pluginCameraId: string): CameraType | undefined => {
    return nvrCameras?.find(
      cam => cam.plugin_id === pluginId && cam.plugin_camera_id === pluginCameraId
    )
  }

  const isLoading = isLoadingPlugin || isLoadingNvr

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="w-6 h-6 animate-spin text-primary" />
      </div>
    )
  }

  if (pluginError) {
    return (
      <div className="flex flex-col items-center justify-center py-12 gap-3">
        <AlertCircle className="w-8 h-8 text-destructive" />
        <p className="text-muted-foreground">Failed to load plugin cameras</p>
      </div>
    )
  }

  if (!pluginCameras || pluginCameras.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-12 gap-4">
        <Camera className="w-12 h-12 text-muted-foreground/30" />
        <div className="text-center">
          <p className="text-muted-foreground">No cameras managed by this plugin</p>
          <p className="text-sm text-muted-foreground/70 mt-1">
            Use the Setup tab to discover and add cameras
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {/* Summary */}
      <div className="flex items-center justify-between p-4 bg-accent/30 rounded-lg">
        <div className="flex items-center gap-3">
          <Plug className="w-5 h-5 text-primary" />
          <span className="font-medium">{pluginCameras.length} camera{pluginCameras.length !== 1 ? 's' : ''} managed</span>
        </div>
        <div className="flex items-center gap-4 text-sm">
          <span className="flex items-center gap-1.5">
            <span className="w-2 h-2 rounded-full bg-green-500" />
            {pluginCameras.filter(c => c.online).length} online
          </span>
          <span className="flex items-center gap-1.5">
            <span className="w-2 h-2 rounded-full bg-gray-400" />
            {pluginCameras.filter(c => !c.online).length} offline
          </span>
        </div>
      </div>

      {/* Camera List */}
      <div className="divide-y divide-border rounded-lg border border-border overflow-hidden">
        {pluginCameras.map((pluginCamera) => {
          const nvrCamera = getCameraMapping(pluginCamera.id)
          const isRegistered = !!nvrCamera

          return (
            <div
              key={pluginCamera.id}
              className="flex items-center gap-4 p-4 hover:bg-accent/30 transition-colors"
            >
              {/* Status indicator */}
              <div className={`relative flex-shrink-0`}>
                {pluginCamera.online ? (
                  <Video className="w-8 h-8 text-green-500" />
                ) : (
                  <VideoOff className="w-8 h-8 text-gray-400" />
                )}
                {isRegistered && (
                  <CheckCircle className="w-4 h-4 text-primary absolute -bottom-1 -right-1 bg-background rounded-full" />
                )}
              </div>

              {/* Camera info */}
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2">
                  <span className="font-medium truncate">{pluginCamera.name}</span>
                  {!isRegistered && (
                    <span className="text-xs px-1.5 py-0.5 bg-yellow-500/10 text-yellow-500 rounded">
                      Not in NVR
                    </span>
                  )}
                </div>
                <div className="flex items-center gap-3 mt-1 text-sm text-muted-foreground">
                  {pluginCamera.model && (
                    <span>{pluginCamera.model}</span>
                  )}
                  {pluginCamera.host && (
                    <span className="font-mono text-xs">{pluginCamera.host}</span>
                  )}
                  {pluginCamera.channel !== undefined && (
                    <span>Ch {pluginCamera.channel + 1}</span>
                  )}
                </div>
              </div>

              {/* NVR status / Actions */}
              <div className="flex items-center gap-2">
                {isRegistered ? (
                  <>
                    <span className={`text-xs px-2 py-1 rounded ${
                      nvrCamera.status === 'online'
                        ? 'bg-green-500/10 text-green-500'
                        : nvrCamera.status === 'error'
                        ? 'bg-red-500/10 text-red-500'
                        : 'bg-gray-500/10 text-gray-500'
                    }`}>
                      {nvrCamera.status}
                    </span>
                    <Link
                      to={`/cameras/${nvrCamera.id}`}
                      className="p-2 hover:bg-accent rounded-md transition-colors"
                      title="View camera"
                    >
                      <ExternalLink className="w-4 h-4" />
                    </Link>
                    <Link
                      to={`/cameras/${nvrCamera.id}`}
                      className="p-2 hover:bg-accent rounded-md transition-colors"
                      title="Camera settings"
                    >
                      <Settings className="w-4 h-4" />
                    </Link>
                  </>
                ) : (
                  <div className="flex items-center gap-2 text-muted-foreground">
                    <Info className="w-4 h-4" />
                    <span className="text-xs">Re-add from Setup tab</span>
                  </div>
                )}
              </div>
            </div>
          )
        })}
      </div>

      {/* Help text */}
      <div className="flex items-start gap-3 p-4 bg-blue-500/10 border border-blue-500/20 rounded-lg">
        <Info className="w-5 h-5 text-blue-500 shrink-0 mt-0.5" />
        <div className="text-sm text-muted-foreground">
          <p className="font-medium text-blue-500 mb-1">About Plugin Cameras</p>
          <p>
            Cameras shown here are managed by this plugin. Cameras with a checkmark are registered
            in the NVR and can be viewed in the Live View. Click the external link icon to view
            camera details including plugin-specific controls like PTZ.
          </p>
        </div>
      </div>
    </div>
  )
}
