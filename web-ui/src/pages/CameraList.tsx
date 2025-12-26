import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { Plus, Camera, Circle } from 'lucide-react'
import { cameraApi, type Camera as CameraType } from '../lib/api'

export function CameraList() {
  const { data: cameras, isLoading, error } = useQuery({
    queryKey: ['cameras'],
    queryFn: cameraApi.list,
  })

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold">Cameras</h1>
          <p className="text-muted-foreground">Manage your camera sources</p>
        </div>
        <Link
          to="/cameras/add"
          className="inline-flex items-center gap-2 bg-primary text-primary-foreground px-4 py-2 rounded-lg hover:bg-primary/90 transition-colors"
        >
          <Plus size={20} />
          Add Camera
        </Link>
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center h-64">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
        </div>
      ) : error ? (
        <div className="bg-destructive/10 text-destructive rounded-lg border border-destructive p-4">
          <p>Failed to load cameras. Please check your connection.</p>
        </div>
      ) : cameras && cameras.length > 0 ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {cameras.map((camera) => (
            <CameraCard key={camera.id} camera={camera} />
          ))}
        </div>
      ) : (
        <div className="bg-card rounded-lg border p-12 text-center">
          <Camera className="h-16 w-16 mx-auto mb-4 opacity-50" />
          <h3 className="text-xl font-semibold mb-2">No cameras configured</h3>
          <p className="text-muted-foreground mb-4">
            Get started by adding your first camera to the system.
          </p>
          <Link
            to="/cameras/add"
            className="inline-flex items-center gap-2 bg-primary text-primary-foreground px-4 py-2 rounded-lg hover:bg-primary/90 transition-colors"
          >
            <Plus size={20} />
            Add Camera
          </Link>
        </div>
      )}
    </div>
  )
}

function CameraCard({ camera }: { camera: CameraType }) {
  const statusConfig = {
    online: { color: 'text-green-500', label: 'Online' },
    offline: { color: 'text-gray-500', label: 'Offline' },
    error: { color: 'text-red-500', label: 'Error' },
    starting: { color: 'text-yellow-500', label: 'Starting' },
  }

  const status = statusConfig[camera.status] || statusConfig.offline

  return (
    <Link
      to={`/cameras/${camera.id}`}
      className="bg-card rounded-lg border overflow-hidden hover:border-primary transition-colors"
    >
      <div className="aspect-video bg-muted flex items-center justify-center relative">
        {camera.status === 'online' ? (
          <img
            src={cameraApi.getSnapshotUrl(camera.id)}
            alt={camera.name}
            className="w-full h-full object-cover"
            onError={(e) => {
              const target = e.target as HTMLImageElement;
              target.style.display = 'none';
              target.nextElementSibling?.classList.remove('hidden');
            }}
          />
        ) : null}
        <div className={`absolute inset-0 flex items-center justify-center ${camera.status === 'online' ? 'hidden' : ''}`}>
          <Camera className="h-12 w-12 opacity-50" />
        </div>
      </div>
      <div className="p-4">
        <div className="flex items-center justify-between">
          <h3 className="font-semibold">{camera.name}</h3>
          <div className="flex items-center gap-1">
            <Circle size={8} className={`fill-current ${status.color}`} />
            <span className="text-sm text-muted-foreground">{status.label}</span>
          </div>
        </div>
        {(camera.manufacturer || camera.model) && (
          <p className="text-sm text-muted-foreground mt-1">
            {[camera.manufacturer, camera.model].filter(Boolean).join(' ')}
          </p>
        )}
        {camera.fps_current && (
          <p className="text-xs text-muted-foreground mt-1">
            {camera.fps_current.toFixed(0)} fps
            {camera.bitrate_current && ` | ${(camera.bitrate_current / 1000000).toFixed(1)} Mbps`}
          </p>
        )}
      </div>
    </Link>
  )
}
