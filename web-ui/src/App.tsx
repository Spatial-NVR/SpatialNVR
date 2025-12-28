import { useState, useEffect } from 'react'
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

function StartupScreen({ state }: { state: StartupState }) {
  const phaseMessages: Record<string, string> = {
    initializing: 'Initializing...',
    database: 'Setting up database...',
    migrations: 'Running migrations...',
    eventbus: 'Starting event system...',
    detection: 'Initializing detection...',
    plugins: 'Loading plugins...',
    gateway: 'Starting API...',
    router: 'Finalizing...',
    error: 'Startup failed',
  }

  const displayMessage = phaseMessages[state.phase] || state.message

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
            <p className="text-red-300 text-sm">{state.error}</p>
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
          </div>
        )}
      </div>
    </div>
  )
}

function App() {
  const [startupState, setStartupState] = useState<StartupState | null>(null)
  const [isReady, setIsReady] = useState(false)

  useEffect(() => {
    let interval: NodeJS.Timeout

    const checkStartup = async () => {
      try {
        const response = await fetch('/api/v1/startup')
        if (response.ok) {
          const state: StartupState = await response.json()
          setStartupState(state)
          if (state.ready) {
            setIsReady(true)
            clearInterval(interval)
          }
        }
      } catch {
        // Server not ready yet, keep polling
      }
    }

    // Check immediately
    checkStartup()

    // Poll every 500ms until ready
    interval = setInterval(checkStartup, 500)

    return () => clearInterval(interval)
  }, [])

  // Show loading screen until backend is ready
  if (!isReady) {
    return <StartupScreen state={startupState || { phase: 'connecting', message: 'Connecting...', ready: false }} />
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
