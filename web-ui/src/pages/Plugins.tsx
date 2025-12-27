import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import {
  Puzzle,
  Download,
  Check,
  RefreshCw,
  Power,
  PowerOff,
  Trash2,
  ExternalLink,
  Star,
  Camera,
  Plug,
  Cpu,
  AlertCircle,
  Loader2,
  Github,
  Settings,
  FolderSearch
} from 'lucide-react'
import { pluginsApi, CatalogPlugin } from '../lib/api'
import { useToast } from '../components/Toast'

// Category icon mapping
const categoryIcons: Record<string, typeof Camera> = {
  cameras: Camera,
  integrations: Plug,
  detectors: Cpu,
}

// Get status color
function getStatusColor(state?: string, enabled?: boolean): string {
  if (!enabled) return 'bg-gray-500'
  switch (state) {
    case 'running': return 'bg-green-500'
    case 'error': return 'bg-red-500'
    case 'starting': return 'bg-yellow-500'
    case 'stopped': return 'bg-gray-500'
    default: return 'bg-gray-500'
  }
}

// Plugin card component
function PluginCard({
  plugin,
  onInstall,
  onEnable,
  onDisable,
  onUpdate,
  onUninstall,
  isInstalling,
}: {
  plugin: CatalogPlugin
  onInstall: (id: string) => void
  onEnable: (id: string) => void
  onDisable: (id: string) => void
  onUpdate: (id: string) => void
  onUninstall: (id: string) => void
  isInstalling: boolean
}) {
  const CategoryIcon = categoryIcons[plugin.category] || Puzzle

  return (
    <div className="bg-card border border-border rounded-lg p-4 hover:border-primary/50 transition-colors">
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-start gap-3 flex-1 min-w-0">
          <div className="p-2 bg-primary/10 rounded-lg shrink-0">
            <CategoryIcon className="w-5 h-5 text-primary" />
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2">
              <h3 className="font-medium truncate">{plugin.name}</h3>
              {plugin.featured && (
                <Star className="w-4 h-4 text-yellow-500 fill-yellow-500 shrink-0" />
              )}
            </div>
            <p className="text-sm text-muted-foreground line-clamp-2 mt-1">
              {plugin.description}
            </p>
            {plugin.capabilities && plugin.capabilities.length > 0 && (
              <div className="flex flex-wrap gap-1 mt-2">
                {plugin.capabilities.slice(0, 4).map(cap => (
                  <span
                    key={cap}
                    className="px-1.5 py-0.5 text-xs bg-accent rounded"
                  >
                    {cap}
                  </span>
                ))}
                {plugin.capabilities.length > 4 && (
                  <span className="px-1.5 py-0.5 text-xs bg-accent rounded">
                    +{plugin.capabilities.length - 4}
                  </span>
                )}
              </div>
            )}
          </div>
        </div>

        {/* Status and actions */}
        <div className="flex flex-col items-end gap-2 shrink-0">
          {plugin.installed ? (
            <>
              <div className="flex items-center gap-2">
                <div className={`w-2 h-2 rounded-full ${getStatusColor(plugin.state, plugin.enabled)}`} />
                <span className="text-xs text-muted-foreground">
                  v{plugin.installed_version}
                </span>
              </div>
              <div className="flex items-center gap-1">
                <Link
                  to={`/plugins/${plugin.id}`}
                  className="p-1.5 text-muted-foreground hover:text-primary hover:bg-primary/10 rounded"
                  title="Settings & Logs"
                >
                  <Settings className="w-4 h-4" />
                </Link>
                {plugin.update_available && (
                  <button
                    onClick={() => onUpdate(plugin.id)}
                    className="p-1.5 text-yellow-500 hover:bg-yellow-500/10 rounded"
                    title={`Update to ${plugin.latest_version}`}
                  >
                    <RefreshCw className="w-4 h-4" />
                  </button>
                )}
                {plugin.enabled ? (
                  <button
                    onClick={() => onDisable(plugin.id)}
                    className="p-1.5 text-muted-foreground hover:text-red-500 hover:bg-red-500/10 rounded"
                    title="Disable"
                  >
                    <PowerOff className="w-4 h-4" />
                  </button>
                ) : (
                  <button
                    onClick={() => onEnable(plugin.id)}
                    className="p-1.5 text-green-500 hover:bg-green-500/10 rounded"
                    title="Enable"
                  >
                    <Power className="w-4 h-4" />
                  </button>
                )}
                <button
                  onClick={() => onUninstall(plugin.id)}
                  className="p-1.5 text-muted-foreground hover:text-red-500 hover:bg-red-500/10 rounded"
                  title="Uninstall"
                >
                  <Trash2 className="w-4 h-4" />
                </button>
              </div>
            </>
          ) : (
            <button
              onClick={() => onInstall(plugin.id)}
              disabled={isInstalling}
              className="flex items-center gap-1.5 px-3 py-1.5 text-sm bg-primary text-primary-foreground rounded-md hover:bg-primary/90 disabled:opacity-50"
            >
              {isInstalling ? (
                <Loader2 className="w-4 h-4 animate-spin" />
              ) : (
                <Download className="w-4 h-4" />
              )}
              Install
            </button>
          )}
        </div>
      </div>
    </div>
  )
}

export function Plugins() {
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const [activeCategory, setActiveCategory] = useState<string | null>(null)
  const [customRepoUrl, setCustomRepoUrl] = useState('')
  const [installingPlugins, setInstallingPlugins] = useState<Set<string>>(new Set())

  // Fetch catalog with status
  const { data: catalog, isLoading, error, refetch } = useQuery({
    queryKey: ['plugin-catalog'],
    queryFn: pluginsApi.getCatalog,
  })

  // Install from catalog mutation - looks up repo URL from catalog and uses generic install
  const installFromCatalog = useMutation({
    mutationFn: async (pluginId: string) => {
      // Find the plugin in the catalog to get its repository URL
      const plugin = catalog?.plugins?.find((p: CatalogPlugin) => p.id === pluginId)
      if (!plugin?.repo) {
        throw new Error(`Plugin ${pluginId} not found in catalog or has no repository URL`)
      }
      return pluginsApi.install(plugin.repo)
    },
    onMutate: (pluginId) => {
      setInstallingPlugins(prev => new Set(prev).add(pluginId))
    },
    onSuccess: (result) => {
      addToast('success', `${result.plugin.name} installed successfully (hot-reload)`)
      queryClient.invalidateQueries({ queryKey: ['plugin-catalog'] })
      queryClient.invalidateQueries({ queryKey: ['plugins'] })
    },
    onError: (error: Error) => {
      addToast('error', `Install failed: ${error.message}`)
    },
    onSettled: (_, __, pluginId) => {
      setInstallingPlugins(prev => {
        const next = new Set(prev)
        next.delete(pluginId)
        return next
      })
    },
  })

  // Install from custom URL mutation
  const installFromUrl = useMutation({
    mutationFn: pluginsApi.install,
    onSuccess: (result) => {
      addToast('success', `${result.plugin.name} installed successfully (hot-reload)`)
      setCustomRepoUrl('')
      queryClient.invalidateQueries({ queryKey: ['plugin-catalog'] })
      queryClient.invalidateQueries({ queryKey: ['plugins'] })
    },
    onError: (error: Error) => {
      addToast('error', `Install failed: ${error.message}`)
    },
  })

  // Enable plugin mutation
  const enablePlugin = useMutation({
    mutationFn: pluginsApi.enable,
    onSuccess: (result) => {
      addToast('success', `Plugin ${result.id} enabled`)
      queryClient.invalidateQueries({ queryKey: ['plugin-catalog'] })
    },
    onError: (error: Error) => {
      addToast('error', `Failed to enable: ${error.message}`)
    },
  })

  // Disable plugin mutation
  const disablePlugin = useMutation({
    mutationFn: pluginsApi.disable,
    onSuccess: (result) => {
      addToast('success', `Plugin ${result.id} disabled`)
      queryClient.invalidateQueries({ queryKey: ['plugin-catalog'] })
    },
    onError: (error: Error) => {
      addToast('error', `Failed to disable: ${error.message}`)
    },
  })

  // Update plugin mutation
  const updatePlugin = useMutation({
    mutationFn: pluginsApi.update,
    onSuccess: (result) => {
      addToast('success', `${result.plugin.name} updated to v${result.plugin.version}`)
      queryClient.invalidateQueries({ queryKey: ['plugin-catalog'] })
    },
    onError: (error: Error) => {
      addToast('error', `Update failed: ${error.message}`)
    },
  })

  // Uninstall plugin mutation
  const uninstallPlugin = useMutation({
    mutationFn: pluginsApi.uninstall,
    onSuccess: (result) => {
      addToast('success', `Plugin ${result.id} uninstalled`)
      queryClient.invalidateQueries({ queryKey: ['plugin-catalog'] })
    },
    onError: (error: Error) => {
      addToast('error', `Uninstall failed: ${error.message}`)
    },
  })

  // Rescan plugins mutation
  const rescanPlugins = useMutation({
    mutationFn: pluginsApi.rescan,
    onSuccess: (result) => {
      addToast('success', result.message)
      queryClient.invalidateQueries({ queryKey: ['plugin-catalog'] })
      queryClient.invalidateQueries({ queryKey: ['plugins'] })
    },
    onError: (error: Error) => {
      addToast('error', `Rescan failed: ${error.message}`)
    },
  })

  // Filter plugins by category
  const filteredPlugins = catalog?.plugins?.filter(p =>
    !activeCategory || p.category === activeCategory
  ) || []

  // Separate installed and available plugins
  const installedPlugins = filteredPlugins.filter(p => p.installed)
  const availablePlugins = filteredPlugins.filter(p => !p.installed)

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="w-8 h-8 animate-spin text-primary" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center h-64 gap-4">
        <AlertCircle className="w-12 h-12 text-destructive" />
        <p className="text-muted-foreground">Failed to load plugin catalog</p>
        <button
          onClick={() => refetch()}
          className="px-4 py-2 bg-primary text-primary-foreground rounded-md"
        >
          Retry
        </button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <Puzzle className="w-6 h-6" />
            Plugins
          </h1>
          <p className="text-muted-foreground mt-1">
            Extend your NVR with camera integrations and features
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => rescanPlugins.mutate()}
            disabled={rescanPlugins.isPending}
            className="flex items-center gap-2 px-3 py-2 text-sm bg-accent hover:bg-accent/80 rounded-md disabled:opacity-50"
            title="Rescan plugins directory to discover newly installed plugins"
          >
            {rescanPlugins.isPending ? (
              <Loader2 className="w-4 h-4 animate-spin" />
            ) : (
              <FolderSearch className="w-4 h-4" />
            )}
            Rescan
          </button>
          <button
            onClick={() => refetch()}
            className="p-2 hover:bg-accent rounded-md"
            title="Refresh catalog"
          >
            <RefreshCw className="w-5 h-5" />
          </button>
        </div>
      </div>

      {/* Category filter */}
      {catalog?.categories && Object.keys(catalog.categories).length > 0 && (
        <div className="flex gap-2 flex-wrap">
          <button
            onClick={() => setActiveCategory(null)}
            className={`px-3 py-1.5 rounded-full text-sm transition-colors ${
              !activeCategory
                ? 'bg-primary text-primary-foreground'
                : 'bg-accent hover:bg-accent/80'
            }`}
          >
            All
          </button>
          {Object.entries(catalog.categories).map(([key, cat]) => {
            const Icon = categoryIcons[key] || Puzzle
            return (
              <button
                key={key}
                onClick={() => setActiveCategory(key)}
                className={`px-3 py-1.5 rounded-full text-sm flex items-center gap-1.5 transition-colors ${
                  activeCategory === key
                    ? 'bg-primary text-primary-foreground'
                    : 'bg-accent hover:bg-accent/80'
                }`}
              >
                <Icon className="w-4 h-4" />
                {cat.name}
              </button>
            )
          })}
        </div>
      )}

      {/* Custom GitHub install */}
      <div className="bg-card border border-border rounded-lg p-4">
        <h2 className="text-sm font-medium mb-3 flex items-center gap-2">
          <Github className="w-4 h-4" />
          Install from GitHub
        </h2>
        <div className="flex gap-2">
          <input
            type="text"
            value={customRepoUrl}
            onChange={e => setCustomRepoUrl(e.target.value)}
            placeholder="github.com/owner/plugin-repo"
            className="flex-1 px-3 py-2 bg-background border border-border rounded-md text-sm"
          />
          <button
            onClick={() => customRepoUrl && installFromUrl.mutate(customRepoUrl)}
            disabled={!customRepoUrl || installFromUrl.isPending}
            className="px-4 py-2 bg-primary text-primary-foreground rounded-md text-sm hover:bg-primary/90 disabled:opacity-50 flex items-center gap-2"
          >
            {installFromUrl.isPending ? (
              <Loader2 className="w-4 h-4 animate-spin" />
            ) : (
              <Download className="w-4 h-4" />
            )}
            Install
          </button>
        </div>
        <p className="text-xs text-muted-foreground mt-2">
          Install plugins from any GitHub repository. No restart required.
        </p>
      </div>

      {/* Installed plugins */}
      {installedPlugins.length > 0 && (
        <section>
          <h2 className="text-lg font-semibold mb-3 flex items-center gap-2">
            <Check className="w-5 h-5 text-green-500" />
            Installed ({installedPlugins.length})
          </h2>
          <div className="grid gap-3 md:grid-cols-2">
            {installedPlugins.map(plugin => (
              <PluginCard
                key={plugin.id}
                plugin={plugin}
                onInstall={(id) => installFromCatalog.mutate(id)}
                onEnable={(id) => enablePlugin.mutate(id)}
                onDisable={(id) => disablePlugin.mutate(id)}
                onUpdate={(id) => updatePlugin.mutate(id)}
                onUninstall={(id) => uninstallPlugin.mutate(id)}
                isInstalling={installingPlugins.has(plugin.id)}
              />
            ))}
          </div>
        </section>
      )}

      {/* Available plugins */}
      {availablePlugins.length > 0 && (
        <section>
          <h2 className="text-lg font-semibold mb-3 flex items-center gap-2">
            <Download className="w-5 h-5" />
            Available ({availablePlugins.length})
          </h2>
          <div className="grid gap-3 md:grid-cols-2">
            {availablePlugins.map(plugin => (
              <PluginCard
                key={plugin.id}
                plugin={plugin}
                onInstall={(id) => installFromCatalog.mutate(id)}
                onEnable={(id) => enablePlugin.mutate(id)}
                onDisable={(id) => disablePlugin.mutate(id)}
                onUpdate={(id) => updatePlugin.mutate(id)}
                onUninstall={(id) => uninstallPlugin.mutate(id)}
                isInstalling={installingPlugins.has(plugin.id)}
              />
            ))}
          </div>
        </section>
      )}

      {/* Empty state */}
      {filteredPlugins.length === 0 && (
        <div className="flex flex-col items-center justify-center py-12 text-center">
          <Puzzle className="w-12 h-12 text-muted-foreground mb-4" />
          <p className="text-muted-foreground">
            {activeCategory
              ? `No plugins found in this category`
              : 'No plugins available'}
          </p>
        </div>
      )}

      {/* Footer info */}
      <div className="text-center text-xs text-muted-foreground pt-4 border-t border-border">
        Catalog version {catalog?.version}
        {catalog?.updated && ` • Updated ${catalog.updated}`}
        <a
          href="https://github.com/nvr-plugins"
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-1 ml-2 hover:text-foreground"
        >
          Browse all plugins <ExternalLink className="w-3 h-3" />
        </a>
      </div>
    </div>
  )
}
