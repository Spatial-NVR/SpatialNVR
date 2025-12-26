import { useState } from 'react'
import { Search as SearchIcon, Calendar, Camera, Filter, Image, User, Car } from 'lucide-react'

export function Search() {
  const [query, setQuery] = useState('')
  const [searchType, setSearchType] = useState<'all' | 'faces' | 'plates' | 'objects'>('all')

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">Search</h1>
        <p className="text-muted-foreground">Search through events, faces, and license plates</p>
      </div>

      {/* Search bar */}
      <div className="flex gap-4">
        <div className="flex-1 relative">
          <SearchIcon size={18} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
          <input
            type="text"
            placeholder="Search events, descriptions, or use natural language..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className="w-full bg-card border border-border rounded-lg pl-10 pr-4 py-3 focus:outline-none focus:ring-2 focus:ring-primary"
          />
        </div>
        <button
          disabled
          className="px-6 py-3 bg-primary text-primary-foreground rounded-lg font-medium hover:bg-primary/90 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          title="AI Search coming in Week 7"
        >
          Search
        </button>
      </div>

      {/* Search type tabs */}
      <div className="flex gap-2">
        {[
          { id: 'all', label: 'All', icon: Filter },
          { id: 'faces', label: 'Faces', icon: User },
          { id: 'plates', label: 'License Plates', icon: Car },
          { id: 'objects', label: 'Objects', icon: Image },
        ].map(({ id, label, icon: Icon }) => (
          <button
            key={id}
            onClick={() => setSearchType(id as typeof searchType)}
            className={`flex items-center gap-2 px-4 py-2 rounded-lg transition-colors ${
              searchType === id
                ? 'bg-primary text-primary-foreground'
                : 'bg-card border border-border hover:bg-accent'
            }`}
          >
            <Icon size={16} />
            <span className="text-sm font-medium">{label}</span>
          </button>
        ))}
      </div>

      {/* Filters */}
      <div className="flex flex-wrap gap-4 p-4 bg-card rounded-lg border border-border">
        <div className="flex items-center gap-2">
          <Calendar size={18} className="text-muted-foreground" />
          <input
            type="date"
            className="bg-background border border-border rounded-md px-3 py-2 text-sm"
          />
          <span className="text-muted-foreground">to</span>
          <input
            type="date"
            className="bg-background border border-border rounded-md px-3 py-2 text-sm"
          />
        </div>

        <div className="flex items-center gap-2">
          <Camera size={18} className="text-muted-foreground" />
          <select className="bg-background border border-border rounded-md px-3 py-2 text-sm">
            <option value="">All Cameras</option>
          </select>
        </div>
      </div>

      {/* Coming soon message */}
      <div className="flex flex-col items-center justify-center py-16 text-center bg-card rounded-lg border border-border">
        <SearchIcon size={48} className="text-muted-foreground mb-4" />
        <h2 className="text-lg font-medium mb-2">AI Search Coming Soon</h2>
        <p className="text-muted-foreground max-w-md">
          Semantic search with natural language queries, face recognition, and license plate
          detection will be available in Week 7.
        </p>
        <div className="mt-6 flex flex-wrap gap-2 justify-center">
          <span className="px-3 py-1 bg-muted rounded-full text-xs">Natural Language</span>
          <span className="px-3 py-1 bg-muted rounded-full text-xs">Face Recognition</span>
          <span className="px-3 py-1 bg-muted rounded-full text-xs">License Plates</span>
          <span className="px-3 py-1 bg-muted rounded-full text-xs">Object Detection</span>
        </div>
      </div>
    </div>
  )
}
