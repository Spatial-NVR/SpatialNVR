import { useState, useEffect } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Camera,
  LogIn,
  Loader2,
  CheckCircle,
  AlertCircle,
  Eye,
  EyeOff,
  RefreshCw,
  Plus,
  X,
  Shield,
  Key
} from 'lucide-react'
import { useToast } from '../Toast'

interface WyzeCamera {
  id: string
  name: string
  model: string
  manufacturer: string
  capabilities: string[]
  firmware_version?: string
  serial?: string
}

interface WyzeSetupProps {
  pluginId: string
  onCameraAdded?: () => void
}

type SetupStep = 'login' | 'cameras' | 'complete'

export function WyzeSetup({ pluginId, onCameraAdded }: WyzeSetupProps) {
  const { addToast } = useToast()
  const queryClient = useQueryClient()

  // Current step
  const [step, setStep] = useState<SetupStep>('login')

  // Login form
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [keyId, setKeyId] = useState('')
  const [apiKey, setApiKey] = useState('')
  const [totpKey, setTotpKey] = useState('')
  const [useApiKey, setUseApiKey] = useState(false)

  // Camera selection
  const [discoveredCameras, setDiscoveredCameras] = useState<WyzeCamera[]>([])
  const [selectedCameras, setSelectedCameras] = useState<Set<string>>(new Set())
  const [excludedCameras, setExcludedCameras] = useState<Set<string>>(new Set())
  const [cameraNames, setCameraNames] = useState<Record<string, string>>({})

  // Check if already authenticated
  const { data: healthData } = useQuery({
    queryKey: ['plugin-health', pluginId],
    queryFn: async () => {
      const response = await fetch(`/api/plugins/${pluginId}/rpc`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          jsonrpc: '2.0',
          id: Date.now(),
          method: 'health'
        })
      })
      const data = await response.json()
      return data.result
    },
    refetchInterval: 5000
  })

  // Auto-transition to cameras step if authenticated
  useEffect(() => {
    if (healthData?.details?.authenticated && step === 'login') {
      discoverMutation.mutate()
    }
  }, [healthData?.details?.authenticated])

  // Login mutation
  const loginMutation = useMutation({
    mutationFn: async () => {
      const config: Record<string, unknown> = {
        email,
        password
      }
      if (useApiKey) {
        config.key_id = keyId
        config.api_key = apiKey
      }
      if (totpKey) {
        config.totp_key = totpKey
      }

      // Initialize plugin with credentials
      const response = await fetch(`/api/plugins/${pluginId}/rpc`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          jsonrpc: '2.0',
          id: Date.now(),
          method: 'initialize',
          params: config
        })
      })
      const data = await response.json()
      if (data.error) {
        throw new Error(data.error.message)
      }
      return data.result
    },
    onSuccess: () => {
      addToast('success', 'Successfully logged in to Wyze')
      // After login, discover cameras
      discoverMutation.mutate()
    },
    onError: (err: Error) => {
      addToast('error', `Login failed: ${err.message}`)
    }
  })

  // Discover cameras mutation
  const discoverMutation = useMutation({
    mutationFn: async () => {
      const response = await fetch(`/api/plugins/${pluginId}/rpc`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          jsonrpc: '2.0',
          id: Date.now(),
          method: 'discover_cameras'
        })
      })
      const data = await response.json()
      if (data.error) {
        throw new Error(data.error.message)
      }
      return data.result as WyzeCamera[]
    },
    onSuccess: (cameras) => {
      setDiscoveredCameras(cameras)
      // Select all cameras by default
      setSelectedCameras(new Set(cameras.map(c => c.id)))
      // Initialize names
      const names: Record<string, string> = {}
      cameras.forEach(c => {
        names[c.id] = c.name
      })
      setCameraNames(names)
      setStep('cameras')
    },
    onError: (err: Error) => {
      addToast('error', `Failed to discover cameras: ${err.message}`)
    }
  })

  // Add cameras mutation
  const addMutation = useMutation({
    mutationFn: async () => {
      const camerasToAdd = Array.from(selectedCameras).filter(id => !excludedCameras.has(id))
      const results = []

      for (const mac of camerasToAdd) {
        const response = await fetch(`/api/plugins/${pluginId}/rpc`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            jsonrpc: '2.0',
            id: Date.now(),
            method: 'add_camera',
            params: {
              mac,
              name: cameraNames[mac]
            }
          })
        })
        const data = await response.json()
        if (data.error) {
          console.warn(`Failed to add camera ${mac}:`, data.error.message)
        } else {
          results.push(data.result)
        }
      }

      return results
    },
    onSuccess: (results) => {
      addToast('success', `Added ${results.length} camera(s) to NVR`)
      queryClient.invalidateQueries({ queryKey: ['cameras'] })
      setStep('complete')
      onCameraAdded?.()
    },
    onError: (err: Error) => {
      addToast('error', `Failed to add cameras: ${err.message}`)
    }
  })

  const toggleCamera = (id: string) => {
    setSelectedCameras(prev => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  const toggleExclude = (id: string) => {
    setExcludedCameras(prev => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  const handleLogin = (e: React.FormEvent) => {
    e.preventDefault()
    if (!email || !password) {
      addToast('error', 'Please enter email and password')
      return
    }
    loginMutation.mutate()
  }

  const handleAddCameras = () => {
    const activeCount = Array.from(selectedCameras).filter(id => !excludedCameras.has(id)).length
    if (activeCount === 0) {
      addToast('error', 'Please select at least one camera')
      return
    }
    addMutation.mutate()
  }

  const getModelName = (model: string): string => {
    const modelNames: Record<string, string> = {
      'WYZECP1': 'Cam Pan',
      'WYZEC1': 'Cam v1',
      'WYZEC1-JZ': 'Cam v2',
      'WYZE_CAKP2': 'Cam v3',
      'HL_CAM3P': 'Cam v3 Pro',
      'HL_PAN2': 'Cam Pan v2',
      'HL_PAN3': 'Cam Pan v3',
      'HL_PANP': 'Cam Pan Pro',
      'WYZEDB3': 'Video Doorbell v1',
      'GW_BE1': 'Video Doorbell v2',
      'GW_GC1': 'Video Doorbell Pro',
      'AN_RSCW': 'Cam OG',
      'AN_RLT': 'Cam OG Telephoto',
      'HL_WCO2': 'Cam Outdoor v2',
      'WVOD1': 'Cam Outdoor v1',
      'HL_CFL1': 'Cam Floodlight',
      'HL_CFL2': 'Cam Floodlight v2'
    }
    return modelNames[model] || model
  }

  // Login Step
  if (step === 'login') {
    return (
      <div className="space-y-6">
        <form onSubmit={handleLogin} className="space-y-4">
          {/* Email */}
          <div className="space-y-2">
            <label className="text-sm font-medium">Wyze Account Email *</label>
            <input
              type="email"
              value={email}
              onChange={e => setEmail(e.target.value)}
              placeholder="your.email@example.com"
              className="w-full px-3 py-2 bg-background border border-border rounded-md"
              disabled={loginMutation.isPending}
            />
          </div>

          {/* Password */}
          <div className="space-y-2">
            <label className="text-sm font-medium">Password *</label>
            <div className="relative">
              <input
                type={showPassword ? 'text' : 'password'}
                value={password}
                onChange={e => setPassword(e.target.value)}
                className="w-full px-3 py-2 pr-10 bg-background border border-border rounded-md"
                disabled={loginMutation.isPending}
              />
              <button
                type="button"
                onClick={() => setShowPassword(!showPassword)}
                className="absolute right-2 top-1/2 -translate-y-1/2 p-1 text-muted-foreground hover:text-foreground"
              >
                {showPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
              </button>
            </div>
          </div>

          {/* TOTP Key (optional) */}
          <div className="space-y-2">
            <label className="text-sm font-medium flex items-center gap-2">
              <Shield className="w-4 h-4" />
              TOTP Secret (for 2FA)
            </label>
            <input
              type="text"
              value={totpKey}
              onChange={e => setTotpKey(e.target.value)}
              placeholder="Optional - for accounts with 2FA enabled"
              className="w-full px-3 py-2 bg-background border border-border rounded-md font-mono text-sm"
              disabled={loginMutation.isPending}
            />
            <p className="text-xs text-muted-foreground">
              If you have 2FA enabled, enter your TOTP secret key (the code you used to set up your authenticator app)
            </p>
          </div>

          {/* API Key Toggle */}
          <div className="border border-border rounded-lg p-4">
            <label className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={useApiKey}
                onChange={e => setUseApiKey(e.target.checked)}
                className="rounded border-border"
                disabled={loginMutation.isPending}
              />
              <Key className="w-4 h-4" />
              <span className="text-sm font-medium">Use API Key (recommended)</span>
            </label>

            {useApiKey && (
              <div className="mt-4 space-y-3">
                <div className="space-y-2">
                  <label className="text-sm font-medium">Key ID</label>
                  <input
                    type="text"
                    value={keyId}
                    onChange={e => setKeyId(e.target.value)}
                    placeholder="Your Wyze API Key ID"
                    className="w-full px-3 py-2 bg-background border border-border rounded-md font-mono text-sm"
                    disabled={loginMutation.isPending}
                  />
                </div>
                <div className="space-y-2">
                  <label className="text-sm font-medium">API Key</label>
                  <input
                    type="password"
                    value={apiKey}
                    onChange={e => setApiKey(e.target.value)}
                    placeholder="Your Wyze API Key"
                    className="w-full px-3 py-2 bg-background border border-border rounded-md"
                    disabled={loginMutation.isPending}
                  />
                </div>
                <p className="text-xs text-muted-foreground">
                  Get your API key from{' '}
                  <a
                    href="https://developer-api-console.wyze.com/"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-primary hover:underline"
                  >
                    Wyze Developer Console
                  </a>
                </p>
              </div>
            )}
          </div>

          <button
            type="submit"
            disabled={loginMutation.isPending || !email || !password}
            className="flex items-center gap-2 px-4 py-2 bg-primary text-primary-foreground rounded-md hover:bg-primary/90 disabled:opacity-50"
          >
            {loginMutation.isPending ? (
              <Loader2 className="w-4 h-4 animate-spin" />
            ) : (
              <LogIn className="w-4 h-4" />
            )}
            Sign In to Wyze
          </button>
        </form>

        {/* Help */}
        <div className="flex items-start gap-3 p-4 bg-blue-500/10 border border-blue-500/20 rounded-lg">
          <AlertCircle className="w-5 h-5 text-blue-500 shrink-0 mt-0.5" />
          <div className="text-sm">
            <p className="font-medium text-blue-500">Wyze Cloud Integration</p>
            <p className="mt-1 text-muted-foreground">
              This plugin connects to your Wyze cloud account to access your cameras.
              Your credentials are stored locally and used only to authenticate with Wyze.
            </p>
            <p className="mt-2 text-muted-foreground">
              For best results, use a Wyze API key from the Developer Console.
              This avoids issues with 2FA and rate limiting.
            </p>
          </div>
        </div>
      </div>
    )
  }

  // Cameras Step
  if (step === 'cameras') {
    const activeCount = Array.from(selectedCameras).filter(id => !excludedCameras.has(id)).length

    return (
      <div className="space-y-6">
        {/* Header */}
        <div className="flex items-center justify-between">
          <div>
            <h3 className="font-medium">Your Wyze Cameras</h3>
            <p className="text-sm text-muted-foreground">
              Found {discoveredCameras.length} camera(s) in your account
            </p>
          </div>
          <button
            onClick={() => discoverMutation.mutate()}
            disabled={discoverMutation.isPending}
            className="flex items-center gap-2 px-3 py-1.5 text-sm hover:bg-accent rounded-md"
          >
            {discoverMutation.isPending ? (
              <Loader2 className="w-4 h-4 animate-spin" />
            ) : (
              <RefreshCw className="w-4 h-4" />
            )}
            Refresh
          </button>
        </div>

        {/* Camera List */}
        {discoveredCameras.length === 0 ? (
          <div className="text-center py-8 text-muted-foreground">
            No cameras found in your Wyze account
          </div>
        ) : (
          <div className="border border-border rounded-lg divide-y divide-border">
            {discoveredCameras.map(camera => {
              const isSelected = selectedCameras.has(camera.id)
              const isExcluded = excludedCameras.has(camera.id)
              const isActive = isSelected && !isExcluded

              return (
                <div
                  key={camera.id}
                  className={`p-4 transition-colors ${
                    isExcluded ? 'bg-red-500/5 opacity-60' : isActive ? 'bg-green-500/5' : ''
                  }`}
                >
                  <div className="flex items-start gap-4">
                    {/* Selection */}
                    <input
                      type="checkbox"
                      checked={isActive}
                      onChange={() => {
                        if (isExcluded) {
                          toggleExclude(camera.id)
                        } else {
                          toggleCamera(camera.id)
                        }
                      }}
                      className="mt-1 w-4 h-4 rounded border-border"
                    />

                    {/* Camera Info */}
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2">
                        <Camera className="w-4 h-4 text-muted-foreground" />
                        <input
                          type="text"
                          value={cameraNames[camera.id] || ''}
                          onChange={e => setCameraNames(prev => ({
                            ...prev,
                            [camera.id]: e.target.value
                          }))}
                          className="font-medium bg-transparent border-none p-0 focus:outline-none focus:ring-0"
                          disabled={!isActive}
                        />
                      </div>
                      <div className="mt-1 flex items-center gap-3 text-sm text-muted-foreground">
                        <span>{getModelName(camera.model)}</span>
                        <span className="font-mono text-xs">{camera.id}</span>
                      </div>
                      {/* Capabilities */}
                      <div className="mt-2 flex flex-wrap gap-1.5">
                        {camera.capabilities.map(cap => (
                          <span
                            key={cap}
                            className="px-1.5 py-0.5 text-xs bg-accent rounded"
                          >
                            {cap}
                          </span>
                        ))}
                      </div>
                    </div>

                    {/* Exclude Button */}
                    <button
                      onClick={() => toggleExclude(camera.id)}
                      className={`p-1.5 rounded ${
                        isExcluded
                          ? 'bg-red-500/10 text-red-500'
                          : 'hover:bg-accent text-muted-foreground'
                      }`}
                      title={isExcluded ? 'Include camera' : 'Exclude camera'}
                    >
                      {isExcluded ? (
                        <Plus className="w-4 h-4" />
                      ) : (
                        <X className="w-4 h-4" />
                      )}
                    </button>
                  </div>
                </div>
              )
            })}
          </div>
        )}

        {/* Actions */}
        <div className="flex items-center justify-between pt-4 border-t border-border">
          <div className="text-sm text-muted-foreground">
            {activeCount} camera(s) will be added
            {excludedCameras.size > 0 && (
              <span className="text-red-500 ml-2">
                ({excludedCameras.size} excluded)
              </span>
            )}
          </div>
          <button
            onClick={handleAddCameras}
            disabled={addMutation.isPending || activeCount === 0}
            className="flex items-center gap-2 px-4 py-2 bg-green-600 text-white rounded-md hover:bg-green-700 disabled:opacity-50"
          >
            {addMutation.isPending ? (
              <Loader2 className="w-4 h-4 animate-spin" />
            ) : (
              <Plus className="w-4 h-4" />
            )}
            Add {activeCount} Camera{activeCount !== 1 ? 's' : ''}
          </button>
        </div>
      </div>
    )
  }

  // Complete Step
  return (
    <div className="text-center py-12">
      <CheckCircle className="w-16 h-16 text-green-500 mx-auto" />
      <h3 className="mt-4 text-xl font-semibold">Setup Complete!</h3>
      <p className="mt-2 text-muted-foreground">
        Your Wyze cameras have been added to the NVR.
      </p>
      <div className="mt-6 flex items-center justify-center gap-3">
        <button
          onClick={() => {
            setStep('cameras')
            discoverMutation.mutate()
          }}
          className="px-4 py-2 bg-accent hover:bg-accent/80 rounded-md"
        >
          Add More Cameras
        </button>
        <button
          onClick={() => setStep('login')}
          className="px-4 py-2 text-muted-foreground hover:text-foreground"
        >
          Use Different Account
        </button>
      </div>
    </div>
  )
}
