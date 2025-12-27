import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Bell, Filter, Check, X, Clock, List } from 'lucide-react'
import { eventApi, Event, cameraApi } from '../lib/api'

// Extended event with camera name for display
interface DisplayEvent extends Event {
  camera_name?: string
}

export function Events() {
  const [showFilters, setShowFilters] = useState(false)
  const [filterType, setFilterType] = useState<string>('all')
  const [viewMode, setViewMode] = useState<'list' | 'timeline'>('list')
  const queryClient = useQueryClient()

  // Fetch cameras for name lookup
  const { data: cameras } = useQuery({
    queryKey: ['cameras'],
    queryFn: cameraApi.list,
  })

  // Create camera name lookup map
  const cameraNameMap = cameras?.reduce((acc, cam) => {
    acc[cam.id] = cam.name
    return acc
  }, {} as Record<string, string>) || {}

  const { data: eventsData, isLoading, error } = useQuery({
    queryKey: ['events', filterType],
    queryFn: async () => {
      const params = filterType !== 'all' ? { type: filterType } : undefined
      return eventApi.list(params)
    },
  })

  // Filter out state_change events and map events to include camera names
  const events: DisplayEvent[] = (eventsData?.data || [])
    .filter(event => event.event_type !== 'state_change')
    .map(event => ({
      ...event,
      camera_name: cameraNameMap[event.camera_id] || event.camera_id,
    }))

  const acknowledgeMutation = useMutation({
    mutationFn: eventApi.acknowledge,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['events'] })
    },
  })

  const formatTime = (timestamp: number) => {
    return new Date(timestamp * 1000).toLocaleString()
  }

  const formatRelativeTime = (timestamp: number) => {
    const now = Date.now() / 1000
    const diff = now - timestamp
    if (diff < 60) return 'Just now'
    if (diff < 3600) return `${Math.floor(diff / 60)}m ago`
    if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`
    return `${Math.floor(diff / 86400)}d ago`
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold">Events</h1>
          <p className="text-muted-foreground">Detection events from your cameras</p>
        </div>
        <div className="flex items-center gap-2">
          {/* View mode toggle */}
          <div className="flex border rounded-lg overflow-hidden">
            <button
              onClick={() => setViewMode('list')}
              className={`px-3 py-2 flex items-center gap-1 ${viewMode === 'list' ? 'bg-primary text-primary-foreground' : 'hover:bg-accent'}`}
              title="List view"
            >
              <List size={18} />
            </button>
            <button
              onClick={() => setViewMode('timeline')}
              className={`px-3 py-2 flex items-center gap-1 ${viewMode === 'timeline' ? 'bg-primary text-primary-foreground' : 'hover:bg-accent'}`}
              title="Timeline view"
            >
              <Clock size={18} />
            </button>
          </div>
          <button
            onClick={() => setShowFilters(!showFilters)}
            className={`inline-flex items-center gap-2 px-4 py-2 border rounded-lg hover:bg-accent transition-colors ${showFilters ? 'bg-accent' : ''}`}
          >
            <Filter size={20} />
            Filter
          </button>
        </div>
      </div>

      {/* Filter panel */}
      {showFilters && (
        <div className="bg-card rounded-lg border p-4">
          <div className="flex items-center justify-between mb-3">
            <h3 className="font-medium">Filter Events</h3>
            <button onClick={() => setShowFilters(false)} className="p-1 rounded hover:bg-accent">
              <X size={16} />
            </button>
          </div>
          <div className="flex flex-wrap gap-2">
            {['all', 'motion', 'person', 'vehicle', 'animal', 'doorbell'].map((type) => (
              <button
                key={type}
                onClick={() => setFilterType(type)}
                className={`px-3 py-1.5 rounded-lg text-sm capitalize transition-colors ${
                  filterType === type
                    ? 'bg-primary text-primary-foreground'
                    : 'bg-muted hover:bg-accent'
                }`}
              >
                {type === 'all' ? 'All Events' : type}
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Error state */}
      {error && (
        <div className="bg-destructive/10 border border-destructive/20 rounded-lg p-4 text-center">
          <p className="text-destructive">Failed to load events. Please try again.</p>
        </div>
      )}

      {isLoading ? (
        <div className="flex items-center justify-center h-64">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
        </div>
      ) : events && events.length > 0 ? (
        viewMode === 'list' ? (
          <div className="space-y-4">
            {events.map((event) => (
              <div
                key={event.id}
                className="bg-card rounded-lg border p-4 flex gap-4"
              >
                <div className="w-32 h-20 bg-muted rounded flex items-center justify-center overflow-hidden">
                  {event.thumbnail_path ? (
                    <img
                      src={event.thumbnail_path}
                      alt="Event thumbnail"
                      className="w-full h-full object-cover"
                    />
                  ) : (
                    <Bell className="opacity-50" />
                  )}
                </div>
                <div className="flex-1">
                  <div className="flex items-start justify-between">
                    <div>
                      <h3 className="font-semibold capitalize">
                        {event.label || event.event_type}
                      </h3>
                      <p className="text-sm text-muted-foreground">
                        {event.camera_name} • {formatTime(event.timestamp)}
                      </p>
                      <p className="text-xs text-muted-foreground">
                        {formatRelativeTime(event.timestamp)}
                      </p>
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="text-sm text-muted-foreground">
                        {Math.round(event.confidence * 100)}%
                      </span>
                      {!event.acknowledged && (
                        <button
                          onClick={() => acknowledgeMutation.mutate(event.id)}
                          disabled={acknowledgeMutation.isPending}
                          className="p-1 rounded hover:bg-accent transition-colors disabled:opacity-50"
                          title="Acknowledge event"
                        >
                          <Check size={16} />
                        </button>
                      )}
                    </div>
                  </div>
                </div>
              </div>
            ))}
          </div>
        ) : (
          /* Timeline view */
          <div className="bg-card rounded-lg border p-4">
            <div className="relative">
              {/* Timeline bar */}
              <div className="absolute left-4 top-0 bottom-0 w-0.5 bg-border" />

              <div className="space-y-6">
                {events.map((event) => (
                  <div key={event.id} className="relative pl-10">
                    {/* Timeline dot */}
                    <div className={`absolute left-2.5 top-2 w-3 h-3 rounded-full border-2 ${
                      event.acknowledged
                        ? 'bg-muted border-muted-foreground'
                        : 'bg-primary border-primary'
                    }`} />

                    <div className="flex items-start gap-4">
                      {/* Thumbnail */}
                      <div className="w-20 h-14 bg-muted rounded flex-shrink-0 overflow-hidden">
                        {event.thumbnail_path ? (
                          <img
                            src={event.thumbnail_path}
                            alt="Event"
                            className="w-full h-full object-cover"
                          />
                        ) : (
                          <div className="w-full h-full flex items-center justify-center">
                            <Bell size={16} className="opacity-50" />
                          </div>
                        )}
                      </div>

                      {/* Event details */}
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center justify-between">
                          <span className="font-medium capitalize">
                            {event.label || event.event_type}
                          </span>
                          <span className="text-xs text-muted-foreground">
                            {formatRelativeTime(event.timestamp)}
                          </span>
                        </div>
                        <p className="text-sm text-muted-foreground truncate">
                          {event.camera_name}
                        </p>
                      </div>

                      {/* Actions */}
                      {!event.acknowledged && (
                        <button
                          onClick={() => acknowledgeMutation.mutate(event.id)}
                          disabled={acknowledgeMutation.isPending}
                          className="p-1 rounded hover:bg-accent transition-colors disabled:opacity-50 flex-shrink-0"
                          title="Acknowledge"
                        >
                          <Check size={16} />
                        </button>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </div>
        )
      ) : (
        <div className="bg-card rounded-lg border p-12 text-center">
          <Bell className="h-16 w-16 mx-auto mb-4 opacity-50" />
          <h3 className="text-xl font-semibold mb-2">No events yet</h3>
          <p className="text-muted-foreground">
            Events will appear here when your cameras detect activity.
          </p>
        </div>
      )}
    </div>
  )
}
