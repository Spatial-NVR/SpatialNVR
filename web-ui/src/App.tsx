import { Routes, Route } from 'react-router-dom'
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

function App() {
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
          <Route path="settings" element={<Settings />} />
          <Route path="health" element={<Health />} />
          <Route path="plugins" element={<Plugins />} />
          <Route path="plugins/:id" element={<PluginDetail />} />
          <Route path="spatial" element={<SpatialTracking />} />
        </Route>
      </Routes>
    </PortsProvider>
  )
}

export default App
