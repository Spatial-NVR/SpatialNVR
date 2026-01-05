import { useState, useCallback, useMemo } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Loader2,
  AlertCircle,
  Eye,
  EyeOff,
  Settings,
  ChevronRight,
} from 'lucide-react'
import { pluginsApi, Setting } from '../../lib/api'

interface PluginSettingsProps {
  pluginId: string
  onToast?: (message: string, type?: 'success' | 'error') => void
}

/**
 * PluginSettings renders a plugin's settings declaratively.
 * Plugins return an array of Setting objects describing their UI,
 * and this component renders them generically - no plugin-specific code needed.
 *
 * This enables a Scrypted-style architecture where plugins describe their
 * configuration UI rather than providing custom components.
 */
export function PluginSettings({ pluginId, onToast }: PluginSettingsProps) {
  const queryClient = useQueryClient()
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set(['Connection', 'Setup']))
  const [pendingValues, setPendingValues] = useState<Record<string, unknown>>({})

  // Fetch settings from the plugin
  const { data: settings, isLoading, error, refetch } = useQuery({
    queryKey: ['plugin-settings', pluginId],
    queryFn: () => pluginsApi.getSettings(pluginId),
    refetchInterval: 5000, // Refresh to get updated values (e.g., after discovery)
  })

  // Mutation to update a setting
  const putSettingMutation = useMutation({
    mutationFn: ({ key, value }: { key: string; value: unknown }) =>
      pluginsApi.putSetting(pluginId, key, value),
    onSuccess: () => {
      // Refetch settings to get updated state
      queryClient.invalidateQueries({ queryKey: ['plugin-settings', pluginId] })
      // Also invalidate cameras in case discovery added new ones
      queryClient.invalidateQueries({ queryKey: ['cameras'] })
      queryClient.invalidateQueries({ queryKey: ['plugin-cameras', pluginId] })
    },
    onError: (error) => {
      onToast?.(`Failed to update setting: ${error}`, 'error')
    },
  })

  // Handle setting value change
  const handleChange = useCallback((setting: Setting, value: unknown) => {
    if (setting.immediate || setting.type === 'button') {
      // Apply immediately
      putSettingMutation.mutate({ key: setting.key, value })
    } else {
      // Store in pending values for batch save
      setPendingValues(prev => ({ ...prev, [setting.key]: value }))
    }
  }, [putSettingMutation])

  // Group settings by their group property
  const groupedSettings = useMemo(() => {
    if (!settings) return new Map<string, Setting[]>()

    const groups = new Map<string, Setting[]>()
    for (const setting of settings) {
      const group = setting.group || 'General'
      if (!groups.has(group)) {
        groups.set(group, [])
      }
      groups.get(group)!.push(setting)
    }
    return groups
  }, [settings])

  const toggleGroup = (group: string) => {
    setExpandedGroups(prev => {
      const next = new Set(prev)
      if (next.has(group)) {
        next.delete(group)
      } else {
        next.add(group)
      }
      return next
    })
  }

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="w-6 h-6 animate-spin text-primary" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center py-12 gap-3">
        <AlertCircle className="w-8 h-8 text-destructive" />
        <p className="text-muted-foreground">Failed to load plugin settings</p>
        <button
          onClick={() => refetch()}
          className="text-sm text-primary hover:underline"
        >
          Try again
        </button>
      </div>
    )
  }

  if (!settings || settings.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-12 gap-4">
        <Settings className="w-12 h-12 text-muted-foreground/30" />
        <p className="text-muted-foreground">No settings available for this plugin</p>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {Array.from(groupedSettings.entries()).map(([group, groupSettings]) => (
        <div key={group} className="border border-border rounded-lg overflow-hidden">
          {/* Group header */}
          <button
            onClick={() => toggleGroup(group)}
            className="w-full flex items-center justify-between p-4 bg-muted/30 hover:bg-muted/50 transition-colors"
          >
            <span className="font-medium">{group}</span>
            <ChevronRight
              className={`w-5 h-5 transition-transform ${
                expandedGroups.has(group) ? 'rotate-90' : ''
              }`}
            />
          </button>

          {/* Group content */}
          {expandedGroups.has(group) && (
            <div className="p-4 space-y-4">
              {groupSettings.map(setting => (
                <SettingInput
                  key={setting.key}
                  setting={setting}
                  value={pendingValues[setting.key] ?? setting.value}
                  onChange={(value) => handleChange(setting, value)}
                  isLoading={putSettingMutation.isPending}
                />
              ))}
            </div>
          )}
        </div>
      ))}
    </div>
  )
}

// Individual setting input component
interface SettingInputProps {
  setting: Setting
  value: unknown
  onChange: (value: unknown) => void
  isLoading?: boolean
}

function SettingInput({ setting, value, onChange, isLoading }: SettingInputProps) {
  const [showPassword, setShowPassword] = useState(false)

  const renderInput = () => {
    switch (setting.type) {
      case 'string':
        return (
          <input
            type="text"
            value={(value as string) || ''}
            onChange={(e) => onChange(e.target.value)}
            placeholder={setting.placeholder}
            disabled={setting.readonly || isLoading}
            className="w-full px-3 py-2 bg-background border border-input rounded-md focus:outline-none focus:ring-2 focus:ring-primary disabled:opacity-50"
          />
        )

      case 'password':
        return (
          <div className="relative">
            <input
              type={showPassword ? 'text' : 'password'}
              value={(value as string) || ''}
              onChange={(e) => onChange(e.target.value)}
              placeholder={setting.placeholder}
              disabled={setting.readonly || isLoading}
              className="w-full px-3 py-2 pr-10 bg-background border border-input rounded-md focus:outline-none focus:ring-2 focus:ring-primary disabled:opacity-50"
            />
            <button
              type="button"
              onClick={() => setShowPassword(!showPassword)}
              className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
            >
              {showPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
            </button>
          </div>
        )

      case 'number':
      case 'integer':
        return (
          <input
            type="number"
            value={(value as number) ?? ''}
            onChange={(e) => onChange(setting.type === 'integer' ? parseInt(e.target.value) : parseFloat(e.target.value))}
            placeholder={setting.placeholder}
            min={setting.range?.[0]}
            max={setting.range?.[1]}
            step={setting.type === 'integer' ? 1 : 'any'}
            disabled={setting.readonly || isLoading}
            className="w-full px-3 py-2 bg-background border border-input rounded-md focus:outline-none focus:ring-2 focus:ring-primary disabled:opacity-50"
          />
        )

      case 'boolean':
        return (
          <label className="flex items-center gap-3 cursor-pointer">
            <input
              type="checkbox"
              checked={!!value}
              onChange={(e) => onChange(e.target.checked)}
              disabled={setting.readonly || isLoading}
              className="w-5 h-5 rounded border-input focus:ring-2 focus:ring-primary"
            />
            <span className="text-sm text-muted-foreground">{setting.description}</span>
          </label>
        )

      case 'textarea':
        return (
          <textarea
            value={(value as string) || ''}
            onChange={(e) => onChange(e.target.value)}
            placeholder={setting.placeholder}
            disabled={setting.readonly || isLoading}
            rows={4}
            className="w-full px-3 py-2 bg-background border border-input rounded-md focus:outline-none focus:ring-2 focus:ring-primary disabled:opacity-50 resize-y"
          />
        )

      case 'button':
        return (
          <button
            onClick={() => onChange(true)}
            disabled={isLoading}
            className="px-4 py-2 bg-primary text-primary-foreground rounded-md hover:bg-primary/90 disabled:opacity-50 flex items-center gap-2"
          >
            {isLoading && <Loader2 className="w-4 h-4 animate-spin" />}
            {setting.title}
          </button>
        )

      case 'device':
        // For device selection, choices should be populated with available devices
        if (!setting.choices || setting.choices.length === 0) {
          return (
            <p className="text-sm text-muted-foreground italic">No devices available</p>
          )
        }
        return setting.multiple ? (
          <div className="space-y-2 max-h-48 overflow-y-auto">
            {setting.choices.map((choice) => (
              <label key={String(choice.value)} className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={Array.isArray(value) && value.includes(choice.value)}
                  onChange={(e) => {
                    const current = Array.isArray(value) ? value : []
                    if (e.target.checked) {
                      onChange([...current, choice.value])
                    } else {
                      onChange(current.filter(v => v !== choice.value))
                    }
                  }}
                  disabled={setting.readonly || isLoading}
                  className="w-4 h-4 rounded border-input"
                />
                <span className="text-sm">{choice.title}</span>
              </label>
            ))}
          </div>
        ) : (
          <select
            value={String(value ?? '')}
            onChange={(e) => onChange(e.target.value)}
            disabled={setting.readonly || isLoading}
            className="w-full px-3 py-2 bg-background border border-input rounded-md focus:outline-none focus:ring-2 focus:ring-primary disabled:opacity-50"
          >
            <option value="">Select...</option>
            {setting.choices.map((choice) => (
              <option key={String(choice.value)} value={String(choice.value)}>
                {choice.title}
              </option>
            ))}
          </select>
        )

      default:
        // Default to dropdown if choices exist, otherwise text input
        if (setting.choices && setting.choices.length > 0) {
          return (
            <select
              value={String(value ?? '')}
              onChange={(e) => onChange(e.target.value)}
              disabled={setting.readonly || isLoading}
              className="w-full px-3 py-2 bg-background border border-input rounded-md focus:outline-none focus:ring-2 focus:ring-primary disabled:opacity-50"
            >
              <option value="">Select...</option>
              {setting.choices.map((choice) => (
                <option key={String(choice.value)} value={String(choice.value)}>
                  {choice.title}
                </option>
              ))}
            </select>
          )
        }
        return (
          <input
            type="text"
            value={(value as string) || ''}
            onChange={(e) => onChange(e.target.value)}
            placeholder={setting.placeholder}
            disabled={setting.readonly || isLoading}
            className="w-full px-3 py-2 bg-background border border-input rounded-md focus:outline-none focus:ring-2 focus:ring-primary disabled:opacity-50"
          />
        )
    }
  }

  return (
    <div className="space-y-1.5">
      {setting.type !== 'boolean' && setting.type !== 'button' && (
        <label className="block text-sm font-medium">
          {setting.title}
        </label>
      )}
      {renderInput()}
      {setting.type !== 'boolean' && setting.description && (
        <p className="text-xs text-muted-foreground">{setting.description}</p>
      )}
    </div>
  )
}
