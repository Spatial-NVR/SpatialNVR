import { useState, useEffect, useCallback } from 'react'
import { Routes, Route, Navigate } from 'react-router-dom'
import { Layout } from './components/Layout'
import { Dashboard } from './pages/Dashboard'
import { CameraList } from './pages/CameraList'
import { CameraDetail } from './pages/CameraDetail'
import { AddCamera } from './pages/AddCamera'
import { Events } from './pages/Events'
import { Recordings } from './pages/Recordings'
import { Search } from './pages/Search'
import { Settings } from './pages/Settings'
import { Health } from './pages/Health'
import { Plugins } from './pages/Plugins'
import { PluginDetail } from './pages/PluginDetail'
import { SpatialTracking } from './pages/SpatialTracking'
import { StreamPreloader } from './components/StreamPreloader'
import { PortsProvider } from './hooks/usePorts'

interface StartupState {
  phase: string
  message: string
  ready: boolean
  error?: string
}

function StartupScreen({ state, retryCount }: { state: StartupState; retryCount: number }) {
  const phaseMessages: Record<string, string> = {
    connecting: 'Connecting to server...',
    initializing: 'Initializing...',
    database: 'Setting up database...',
    migrations: 'Running migrations...',
    eventbus: 'Starting event system...',
    detection: 'Initializing detection...',
    plugins: 'Loading plugins...',
    gateway: 'Starting API...',
    router: 'Finalizing...',
    ready: 'Ready!',
    error: 'Startup failed',
  }

  const displayMessage = phaseMessages[state.phase] || state.message || state.phase

  return (
    <div className="min-h-screen bg-gray-900 flex items-center justify-center">
      <div className="text-center">
        <div className="mb-8">
          <svg className="w-16 h-16 mx-auto text-blue-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
          </svg>
        </div>
        <h1 className="text-2xl font-bold text-white mb-2">SpatialNVR</h1>

        {state.error ? (
          <div className="mt-4">
            <p className="text-red-400 mb-2">{displayMessage}</p>
            <p className="text-red-300 text-sm max-w-md">{state.error}</p>
          </div>
        ) : (
          <div className="mt-4">
            <div className="flex items-center justify-center gap-2 text-gray-400">
              <svg className="animate-spin h-5 w-5" viewBox="0 0 24 24">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" fill="none" />
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
              </svg>
              <span>{displayMessage}</span>
            </div>
            {retryCount > 5 && (
              <p className="text-gray-500 text-xs mt-2">
                Still waiting... (attempt {retryCount})
              </p>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

function App() {
  const [startupState, setStartupState] = useState<StartupState>({
    phase: 'connecting',
    message: 'Connecting to server...',
    ready: false
  })
  const [isReady, setIsReady] = useState(false)
  const [retryCount, setRetryCount] = useState(0)

  const checkStartup = useCallback(async () => {
    try {
      // Try the startup endpoint first
      const response = await fetch('/api/v1/startup', {
        headers: { 'Accept': 'application/json' },
      })

      if (response.ok) {
        const contentType = response.headers.get('content-type')
        if (contentType && contentType.includes('application/json')) {
          const state: StartupState = await response.json()
          setStartupState(state)
          if (state.ready) {
            setIsReady(true)
            return true
          }
        }
      }

      // If startup endpoint returns non-200 or non-JSON, try health check
      // This handles the case where router was swapped but startup endpoint changed
      const healthResponse = await fetch('/health')
      if (healthResponse.ok) {
        const healthData = await healthResponse.json()
        if (healthData.ready === true || healthData.status === 'healthy') {
          setStartupState({ phase: 'ready', message: 'Ready', ready: true })
          setIsReady(true)
          return true
        }
      }
    } catch (error) {
      // Network error - server might not be up yet
      console.debug('Startup check failed:', error)
    }

    setRetryCount(prev => prev + 1)
    return false
  }, [])

  useEffect(() => {
    let interval: ReturnType<typeof setInterval>
    let mounted = true

    const startPolling = async () => {
      // Check immediately
      const ready = await checkStartup()
      if (ready || !mounted) return

      // Poll every 300ms until ready (faster polling)
      interval = setInterval(async () => {
        if (!mounted) return
        const ready = await checkStartup()
        if (ready) {
          clearInterval(interval)
        }
      }, 300)
    }

    startPolling()

    return () => {
      mounted = false
      if (interval) clearInterval(interval)
    }
  }, [checkStartup])

  // After 30 seconds of trying, just show the app anyway
  // This handles edge cases where startup endpoint has issues
  useEffect(() => {
    if (isReady) return

    const timeout = setTimeout(() => {
      console.warn('Startup check timed out after 30s, showing app anyway')
      setIsReady(true)
    }, 30000)

    return () => clearTimeout(timeout)
  }, [isReady])

  // Show loading screen until backend is ready
  if (!isReady) {
    return <StartupScreen state={startupState} retryCount={retryCount} />
  }

  return (
    <PortsProvider>
      {/* Pre-connect to all camera streams for instant playback */}
      <StreamPreloader />
      <Routes>
        <Route path="/" element={<Layout />}>
          <Route index element={<Dashboard />} />
          <Route path="cameras" element={<CameraList />} />
          <Route path="cameras/add" element={<AddCamera />} />
          <Route path="cameras/:id" element={<CameraDetail />} />
          <Route path="events" element={<Events />} />
          <Route path="recordings" element={<Recordings />} />
          <Route path="search" element={<Search />} />
          {/* Settings handles its own /settings/plugins redirect internally */}
          <Route path="settings/*" element={<Settings />} />
          <Route path="health" element={<Health />} />
          <Route path="plugins" element={<Plugins />} />
          <Route path="plugins/:id" element={<PluginDetail />} />
          <Route path="spatial" element={<SpatialTracking />} />
          {/* Catch unknown paths */}
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Routes>
    </PortsProvider>
  )
}

export default App
