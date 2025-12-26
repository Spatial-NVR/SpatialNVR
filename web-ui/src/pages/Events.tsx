import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Bell, Filter, Check, X } from 'lucide-react'

interface Event {
  id: string
  cameraId: string
  cameraName: string
  eventType: string
  timestamp: number
  confidence: number
  acknowledged: boolean
  thumbnailPath?: string
}

async function fetchEvents(): Promise<Event[]> {
  const response = await fetch('http://localhost:5000/api/v1/events')
  const data = await response.json()
  return data.events || []
}

async function acknowledgeEvent(id: string): Promise<void> {
  await fetch(`http://localhost:5000/api/v1/events/${id}/acknowledge`, {
    method: 'POST',
  })
}

export function Events() {
  const [showFilters, setShowFilters] = useState(false)
  const [filterType, setFilterType] = useState<string>('all')
  const queryClient = useQueryClient()

  const { data: events, isLoading } = useQuery({
    queryKey: ['events'],
    queryFn: fetchEvents,
  })

  const acknowledgeMutation = useMutation({
    mutationFn: acknowledgeEvent,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['events'] })
    },
  })

  const filteredEvents = events?.filter((event) => {
    if (filterType === 'all') return true
    return event.eventType === filterType
  })

  const formatTime = (timestamp: number) => {
    return new Date(timestamp * 1000).toLocaleString()
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold">Events</h1>
          <p className="text-muted-foreground">Detection events from your cameras</p>
        </div>
        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`inline-flex items-center gap-2 px-4 py-2 border rounded-lg hover:bg-accent transition-colors ${showFilters ? 'bg-accent' : ''}`}
        >
          <Filter size={20} />
          Filter
        </button>
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

      {isLoading ? (
        <div className="flex items-center justify-center h-64">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
        </div>
      ) : filteredEvents && filteredEvents.length > 0 ? (
        <div className="space-y-4">
          {filteredEvents.map((event) => (
            <div
              key={event.id}
              className="bg-card rounded-lg border p-4 flex gap-4"
            >
              <div className="w-32 h-20 bg-muted rounded flex items-center justify-center">
                {event.thumbnailPath ? (
                  <img
                    src={event.thumbnailPath}
                    alt="Event thumbnail"
                    className="w-full h-full object-cover rounded"
                  />
                ) : (
                  <Bell className="opacity-50" />
                )}
              </div>
              <div className="flex-1">
                <div className="flex items-start justify-between">
                  <div>
                    <h3 className="font-semibold capitalize">{event.eventType}</h3>
                    <p className="text-sm text-muted-foreground">
                      {event.cameraName} • {formatTime(event.timestamp)}
                    </p>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-sm text-muted-foreground">
                      {Math.round(event.confidence * 100)}% confidence
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
