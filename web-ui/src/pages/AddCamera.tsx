import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, AlertCircle } from 'lucide-react'
import { Link } from 'react-router-dom'
import { cameraApi, ApiError, type CameraConfig } from '../lib/api'

interface FormErrors {
  [key: string]: string
}

export function AddCamera() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [errors, setErrors] = useState<FormErrors>({})
  const [formData, setFormData] = useState({
    name: '',
    streamUrl: '',
    subStreamUrl: '',
    username: '',
    password: '',
    manufacturer: '',
    model: '',
    detectionEnabled: true,
    detectionFps: 5,
    recordingEnabled: true,
    preBuffer: 5,
    postBuffer: 5,
  })

  const createMutation = useMutation({
    mutationFn: (config: CameraConfig) => cameraApi.create(config),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['cameras'] })
      navigate('/cameras')
    },
    onError: (error: ApiError) => {
      if (error.details) {
        const fieldErrors: FormErrors = {}
        error.details.forEach((detail) => {
          // Map API field names to form field names
          const fieldMap: Record<string, string> = {
            name: 'name',
            'stream.url': 'streamUrl',
            'stream.sub_url': 'subStreamUrl',
            'stream.username': 'username',
            'detection.fps': 'detectionFps',
            'recording.pre_buffer_seconds': 'preBuffer',
            'recording.post_buffer_seconds': 'postBuffer',
          }
          const formField = fieldMap[detail.field] || detail.field
          fieldErrors[formField] = detail.message
        })
        setErrors(fieldErrors)
      } else {
        setErrors({ _form: error.message })
      }
    },
  })

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setErrors({})

    const config: CameraConfig = {
      name: formData.name,
      stream: {
        url: formData.streamUrl,
        sub_url: formData.subStreamUrl || undefined,
        username: formData.username || undefined,
        password: formData.password || undefined,
      },
      manufacturer: formData.manufacturer || undefined,
      model: formData.model || undefined,
      detection: {
        enabled: formData.detectionEnabled,
        fps: formData.detectionFps,
      },
      recording: {
        enabled: formData.recordingEnabled,
        pre_buffer_seconds: formData.preBuffer,
        post_buffer_seconds: formData.postBuffer,
      },
    }

    createMutation.mutate(config)
  }

  const inputClass = (field: string) =>
    `w-full px-3 py-2 bg-background border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary ${
      errors[field] ? 'border-destructive' : ''
    }`

  return (
    <div className="max-w-2xl mx-auto space-y-6">
      <div className="flex items-center gap-4">
        <Link
          to="/cameras"
          className="p-2 rounded-lg hover:bg-accent transition-colors"
        >
          <ArrowLeft size={24} />
        </Link>
        <div>
          <h1 className="text-3xl font-bold">Add Camera</h1>
          <p className="text-muted-foreground">Configure a new camera source</p>
        </div>
      </div>

      {errors._form && (
        <div className="bg-destructive/10 text-destructive rounded-lg border border-destructive p-4 flex items-center gap-2">
          <AlertCircle size={20} />
          <p>{errors._form}</p>
        </div>
      )}

      <form onSubmit={handleSubmit} className="bg-card rounded-lg border p-6 space-y-6">
        {/* Basic Info */}
        <div className="space-y-4">
          <h2 className="text-lg font-semibold">Basic Information</h2>

          <div>
            <label htmlFor="name" className="block text-sm font-medium mb-2">
              Camera Name *
            </label>
            <input
              type="text"
              id="name"
              required
              value={formData.name}
              onChange={(e) => setFormData({ ...formData, name: e.target.value })}
              className={inputClass('name')}
              placeholder="Front Door"
            />
            {errors.name && <p className="text-sm text-destructive mt-1">{errors.name}</p>}
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label htmlFor="manufacturer" className="block text-sm font-medium mb-2">
                Manufacturer
              </label>
              <input
                type="text"
                id="manufacturer"
                value={formData.manufacturer}
                onChange={(e) => setFormData({ ...formData, manufacturer: e.target.value })}
                className={inputClass('manufacturer')}
                placeholder="Reolink"
              />
            </div>
            <div>
              <label htmlFor="model" className="block text-sm font-medium mb-2">
                Model
              </label>
              <input
                type="text"
                id="model"
                value={formData.model}
                onChange={(e) => setFormData({ ...formData, model: e.target.value })}
                className={inputClass('model')}
                placeholder="RLC-811A"
              />
            </div>
          </div>
        </div>

        {/* Stream Settings */}
        <div className="space-y-4">
          <h2 className="text-lg font-semibold">Stream Settings</h2>

          <div>
            <label htmlFor="streamUrl" className="block text-sm font-medium mb-2">
              Main Stream URL *
            </label>
            <input
              type="text"
              id="streamUrl"
              required
              value={formData.streamUrl}
              onChange={(e) => setFormData({ ...formData, streamUrl: e.target.value })}
              className={inputClass('streamUrl')}
              placeholder="rtsp://192.168.1.100:554/stream"
            />
            {errors.streamUrl && <p className="text-sm text-destructive mt-1">{errors.streamUrl}</p>}
            <p className="text-sm text-muted-foreground mt-1">
              RTSP, RTMP, or HTTP stream URL for high-quality stream
            </p>
          </div>

          <div>
            <label htmlFor="subStreamUrl" className="block text-sm font-medium mb-2">
              Sub-stream URL (Optional)
            </label>
            <input
              type="text"
              id="subStreamUrl"
              value={formData.subStreamUrl}
              onChange={(e) => setFormData({ ...formData, subStreamUrl: e.target.value })}
              className={inputClass('subStreamUrl')}
              placeholder="rtsp://192.168.1.100:554/substream"
            />
            <p className="text-sm text-muted-foreground mt-1">
              Lower resolution stream for detection (reduces CPU usage)
            </p>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label htmlFor="username" className="block text-sm font-medium mb-2">
                Username
              </label>
              <input
                type="text"
                id="username"
                value={formData.username}
                onChange={(e) => setFormData({ ...formData, username: e.target.value })}
                className={inputClass('username')}
                placeholder="admin"
              />
            </div>
            <div>
              <label htmlFor="password" className="block text-sm font-medium mb-2">
                Password
              </label>
              <input
                type="password"
                id="password"
                value={formData.password}
                onChange={(e) => setFormData({ ...formData, password: e.target.value })}
                className={inputClass('password')}
              />
            </div>
          </div>
        </div>

        {/* Detection Settings */}
        <div className="space-y-4">
          <h2 className="text-lg font-semibold">Detection Settings</h2>

          <div className="flex items-center gap-2">
            <input
              type="checkbox"
              id="detectionEnabled"
              checked={formData.detectionEnabled}
              onChange={(e) => setFormData({ ...formData, detectionEnabled: e.target.checked })}
              className="rounded"
            />
            <label htmlFor="detectionEnabled" className="text-sm font-medium">
              Enable object detection
            </label>
          </div>

          {formData.detectionEnabled && (
            <div>
              <label htmlFor="detectionFps" className="block text-sm font-medium mb-2">
                Detection FPS
              </label>
              <input
                type="number"
                id="detectionFps"
                min="1"
                max="30"
                value={formData.detectionFps}
                onChange={(e) => setFormData({ ...formData, detectionFps: parseInt(e.target.value) || 5 })}
                className={inputClass('detectionFps')}
              />
              {errors.detectionFps && <p className="text-sm text-destructive mt-1">{errors.detectionFps}</p>}
              <p className="text-sm text-muted-foreground mt-1">
                Frames per second to analyze (5 recommended)
              </p>
            </div>
          )}
        </div>

        {/* Recording Settings */}
        <div className="space-y-4">
          <h2 className="text-lg font-semibold">Recording Settings</h2>

          <div className="flex items-center gap-2">
            <input
              type="checkbox"
              id="recordingEnabled"
              checked={formData.recordingEnabled}
              onChange={(e) => setFormData({ ...formData, recordingEnabled: e.target.checked })}
              className="rounded"
            />
            <label htmlFor="recordingEnabled" className="text-sm font-medium">
              Enable recording
            </label>
          </div>

          {formData.recordingEnabled && (
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label htmlFor="preBuffer" className="block text-sm font-medium mb-2">
                  Pre-event buffer (seconds)
                </label>
                <input
                  type="number"
                  id="preBuffer"
                  min="0"
                  max="60"
                  value={formData.preBuffer}
                  onChange={(e) => setFormData({ ...formData, preBuffer: parseInt(e.target.value) || 0 })}
                  className={inputClass('preBuffer')}
                />
                {errors.preBuffer && <p className="text-sm text-destructive mt-1">{errors.preBuffer}</p>}
              </div>
              <div>
                <label htmlFor="postBuffer" className="block text-sm font-medium mb-2">
                  Post-event buffer (seconds)
                </label>
                <input
                  type="number"
                  id="postBuffer"
                  min="0"
                  max="300"
                  value={formData.postBuffer}
                  onChange={(e) => setFormData({ ...formData, postBuffer: parseInt(e.target.value) || 0 })}
                  className={inputClass('postBuffer')}
                />
                {errors.postBuffer && <p className="text-sm text-destructive mt-1">{errors.postBuffer}</p>}
              </div>
            </div>
          )}
        </div>

        <div className="flex gap-4 pt-4">
          <button
            type="submit"
            disabled={createMutation.isPending}
            className="flex-1 bg-primary text-primary-foreground py-2 rounded-lg hover:bg-primary/90 transition-colors disabled:opacity-50"
          >
            {createMutation.isPending ? 'Adding...' : 'Add Camera'}
          </button>
          <Link
            to="/cameras"
            className="px-6 py-2 border rounded-lg hover:bg-accent transition-colors text-center"
          >
            Cancel
          </Link>
        </div>
      </form>
    </div>
  )
}
