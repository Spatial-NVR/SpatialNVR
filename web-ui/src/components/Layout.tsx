import { Outlet, NavLink } from 'react-router-dom'
import { Home, Camera, Bell, Settings, Menu, X, ChevronLeft, ChevronRight, Video, Search, Target, Puzzle, Activity } from 'lucide-react'
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { DoorbellNotification } from './DoorbellNotification'
import { usePorts } from '../hooks/usePorts'

export function Layout() {
  const [mobileOpen, setMobileOpen] = useState(false)
  const [expanded, setExpanded] = useState(false)
  const { apiUrl } = usePorts()

  const { data: health } = useQuery({
    queryKey: ['health'],
    queryFn: async () => {
      const res = await fetch(`${apiUrl}/health`)
      return res.json()
    },
    refetchInterval: 30000,
  })

  const mainNavItems = [
    { to: '/', icon: Home, label: 'Live View' },
    { to: '/cameras', icon: Camera, label: 'Cameras' },
    { to: '/recordings', icon: Video, label: 'Recordings' },
    { to: '/events', icon: Bell, label: 'Events' },
    { to: '/search', icon: Search, label: 'Search' },
    { to: '/spatial', icon: Target, label: 'Spatial Tracking' },
  ]

  const isHealthy = health?.status === 'healthy'

  return (
    <div className="min-h-screen bg-background">
      {/* Doorbell notifications (global) */}
      <DoorbellNotification />

      {/* Mobile header */}
      <header className="lg:hidden flex items-center justify-between p-4 border-b border-border">
        <h1 className="text-xl font-bold">NVR</h1>
        <button
          onClick={() => setMobileOpen(!mobileOpen)}
          className="p-2 rounded-md hover:bg-accent"
        >
          {mobileOpen ? <X size={24} /> : <Menu size={24} />}
        </button>
      </header>

      <div className="flex h-[calc(100vh-65px)] lg:h-screen">
        {/* Sidebar - Desktop: icon-only by default, expandable */}
        <aside
          className={`
            hidden lg:flex flex-col h-full bg-card border-r border-border transition-all duration-300
            ${expanded ? 'w-56' : 'w-16'}
          `}
        >
          {/* Logo / Brand */}
          <div className={`p-4 flex items-center ${expanded ? 'justify-between' : 'justify-center'} border-b border-border`}>
            {expanded ? (
              <>
                <div>
                  <h1 className="text-lg font-bold">NVR</h1>
                  <p className="text-xs text-muted-foreground">Video Recorder</p>
                </div>
                <button
                  onClick={() => setExpanded(false)}
                  className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
                  title="Collapse"
                >
                  <ChevronLeft size={18} />
                </button>
              </>
            ) : (
              <button
                onClick={() => setExpanded(true)}
                className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
                title="Expand"
              >
                <ChevronRight size={18} />
              </button>
            )}
          </div>

          {/* Main nav */}
          <nav className={`flex-1 py-4 ${expanded ? 'px-3' : 'px-2'} space-y-1`}>
            {mainNavItems.map(({ to, icon: Icon, label }) => (
              <NavLink
                key={to}
                to={to}
                title={!expanded ? label : undefined}
                className={({ isActive }) =>
                  `flex items-center gap-3 rounded-lg transition-colors ${
                    expanded ? 'px-3 py-2.5' : 'p-2.5 justify-center'
                  } ${
                    isActive
                      ? 'bg-primary text-primary-foreground'
                      : 'hover:bg-accent text-muted-foreground hover:text-foreground'
                  }`
                }
              >
                <Icon size={20} />
                {expanded && <span className="text-sm font-medium">{label}</span>}
              </NavLink>
            ))}
          </nav>

          {/* Bottom section - Plugins, Health, Settings */}
          <div className={`py-4 ${expanded ? 'px-3' : 'px-2'} border-t border-border space-y-1`}>
            <NavLink
              to="/plugins"
              title={!expanded ? 'Plugins' : undefined}
              className={({ isActive }) =>
                `flex items-center gap-3 rounded-lg transition-colors ${
                  expanded ? 'px-3 py-2.5' : 'p-2.5 justify-center'
                } ${
                  isActive
                    ? 'bg-primary text-primary-foreground'
                    : 'hover:bg-accent text-muted-foreground hover:text-foreground'
                }`
              }
            >
              <Puzzle size={20} />
              {expanded && <span className="text-sm font-medium">Plugins</span>}
            </NavLink>

            <NavLink
              to="/health"
              title={!expanded ? 'System Health' : undefined}
              className={({ isActive }) =>
                `flex items-center gap-3 rounded-lg transition-colors ${
                  expanded ? 'px-3 py-2.5' : 'p-2.5 justify-center'
                } ${
                  isActive
                    ? 'bg-primary text-primary-foreground'
                    : 'hover:bg-accent text-muted-foreground hover:text-foreground'
                }`
              }
            >
              <Activity size={20} />
              {expanded && <span className="text-sm font-medium">System Health</span>}
            </NavLink>

            <NavLink
              to="/settings"
              title={!expanded ? 'Settings' : undefined}
              className={({ isActive }) =>
                `flex items-center gap-3 rounded-lg transition-colors ${
                  expanded ? 'px-3 py-2.5' : 'p-2.5 justify-center'
                } ${
                  isActive
                    ? 'bg-primary text-primary-foreground'
                    : 'hover:bg-accent text-muted-foreground hover:text-foreground'
                }`
              }
            >
              <Settings size={20} />
              {expanded && <span className="text-sm font-medium">Settings</span>}
            </NavLink>

            {/* Health indicator */}
            <div
              title={isHealthy ? 'System Healthy' : 'System Degraded'}
              className={`flex items-center gap-3 rounded-lg ${
                expanded ? 'px-3 py-2' : 'p-2.5 justify-center'
              }`}
            >
              <div className={`w-2.5 h-2.5 rounded-full ${isHealthy ? 'bg-green-500' : 'bg-yellow-500'}`} />
              {expanded && (
                <span className="text-xs text-muted-foreground">
                  {isHealthy ? 'System Healthy' : 'System Degraded'}
                </span>
              )}
            </div>
          </div>
        </aside>

        {/* Mobile Sidebar */}
        <aside
          className={`
            fixed inset-y-0 left-0 z-50 w-64 bg-card border-r border-border transform transition-transform lg:hidden flex flex-col h-full
            ${mobileOpen ? 'translate-x-0' : '-translate-x-full'}
          `}
        >
          <div className="p-6">
            <h1 className="text-2xl font-bold">NVR System</h1>
            <p className="text-sm text-muted-foreground">Network Video Recorder</p>
          </div>

          <nav className="px-4 space-y-1 flex-1">
            {mainNavItems.map(({ to, icon: Icon, label }) => (
              <NavLink
                key={to}
                to={to}
                onClick={() => setMobileOpen(false)}
                className={({ isActive }) =>
                  `flex items-center gap-3 px-4 py-3 rounded-lg transition-colors ${
                    isActive
                      ? 'bg-primary text-primary-foreground'
                      : 'hover:bg-accent'
                  }`
                }
              >
                <Icon size={20} />
                <span>{label}</span>
              </NavLink>
            ))}
          </nav>

          <div className="p-4 border-t border-border space-y-1">
            <NavLink
              to="/plugins"
              onClick={() => setMobileOpen(false)}
              className={({ isActive }) =>
                `flex items-center gap-3 px-4 py-3 rounded-lg transition-colors ${
                  isActive
                    ? 'bg-primary text-primary-foreground'
                    : 'hover:bg-accent'
                }`
              }
            >
              <Puzzle size={20} />
              <span>Plugins</span>
            </NavLink>

            <NavLink
              to="/health"
              onClick={() => setMobileOpen(false)}
              className={({ isActive }) =>
                `flex items-center gap-3 px-4 py-3 rounded-lg transition-colors ${
                  isActive
                    ? 'bg-primary text-primary-foreground'
                    : 'hover:bg-accent'
                }`
              }
            >
              <Activity size={20} />
              <span>System Health</span>
            </NavLink>

            <NavLink
              to="/settings"
              onClick={() => setMobileOpen(false)}
              className={({ isActive }) =>
                `flex items-center gap-3 px-4 py-3 rounded-lg transition-colors ${
                  isActive
                    ? 'bg-primary text-primary-foreground'
                    : 'hover:bg-accent'
                }`
              }
            >
              <Settings size={20} />
              <span>Settings</span>
            </NavLink>

            <div className="flex items-center gap-3 px-4 py-2">
              <div className={`w-2 h-2 rounded-full ${isHealthy ? 'bg-green-500' : 'bg-yellow-500'}`} />
              <span className="text-sm text-muted-foreground">
                {isHealthy ? 'System Healthy' : 'System Degraded'}
              </span>
            </div>
          </div>
        </aside>

        {/* Overlay for mobile */}
        {mobileOpen && (
          <div
            className="fixed inset-0 bg-black/50 z-40 lg:hidden"
            onClick={() => setMobileOpen(false)}
          />
        )}

        {/* Main content */}
        <main className="flex-1 p-4 lg:p-6 overflow-auto">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
