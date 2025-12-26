import { useState, useEffect, useCallback, useRef } from 'react'
import { Bell, Phone, PhoneOff, X, Volume2, VolumeX } from 'lucide-react'
import { nvrWebSocket, doorbellApi, cameraApi, AudioSessionResponse } from '../lib/api'

interface DoorbellRing {
  id: string
  camera_id: string
  camera_name: string
  snapshot_url: string
  timestamp: Date
}

interface DoorbellNotificationProps {
  onAnswer?: (cameraId: string, session: AudioSessionResponse) => void
  onDismiss?: (cameraId: string) => void
}

export function DoorbellNotification({ onAnswer, onDismiss }: DoorbellNotificationProps) {
  const [rings, setRings] = useState<DoorbellRing[]>([])
  const [answeredSession, setAnsweredSession] = useState<AudioSessionResponse | null>(null)
  const [isAnswering, setIsAnswering] = useState(false)
  const [isMuted, setIsMuted] = useState(false)
  const audioRef = useRef<HTMLAudioElement | null>(null)
  const ringtoneRef = useRef<HTMLAudioElement | null>(null)

  // Play doorbell sound when ring comes in
  const playRingtone = useCallback(() => {
    if (ringtoneRef.current) {
      ringtoneRef.current.currentTime = 0
      ringtoneRef.current.play().catch(() => {
        // Audio autoplay blocked
      })
    }
  }, [])

  const stopRingtone = useCallback(() => {
    if (ringtoneRef.current) {
      ringtoneRef.current.pause()
      ringtoneRef.current.currentTime = 0
    }
  }, [])

  // Listen for doorbell events via WebSocket
  useEffect(() => {
    const handleDoorbell = async (data: unknown) => {
      const message = data as { camera_id: string; action: string; snapshot_url?: string }

      if (message.action === 'ring') {
        // Fetch camera name
        let cameraName = message.camera_id
        try {
          const camera = await cameraApi.get(message.camera_id)
          cameraName = camera.name
        } catch {
          // Use camera ID if fetch fails
        }

        const ring: DoorbellRing = {
          id: `${message.camera_id}-${Date.now()}`,
          camera_id: message.camera_id,
          camera_name: cameraName,
          snapshot_url: message.snapshot_url || cameraApi.getSnapshotUrl(message.camera_id),
          timestamp: new Date(),
        }

        setRings(prev => {
          // Don't add duplicate rings from same camera within 30 seconds
          const recentRing = prev.find(
            r => r.camera_id === ring.camera_id &&
            Date.now() - r.timestamp.getTime() < 30000
          )
          if (recentRing) return prev
          return [...prev, ring]
        })

        playRingtone()

        // Auto-dismiss after 60 seconds
        setTimeout(() => {
          setRings(prev => prev.filter(r => r.id !== ring.id))
        }, 60000)
      }
    }

    nvrWebSocket.connect()
    const unsubscribe = nvrWebSocket.on('doorbell', handleDoorbell)

    return () => {
      unsubscribe()
    }
  }, [playRingtone])

  const handleAnswer = async (ring: DoorbellRing) => {
    setIsAnswering(true)
    stopRingtone()

    try {
      const session = await doorbellApi.answer(ring.camera_id)
      setAnsweredSession(session)
      setRings(prev => prev.filter(r => r.id !== ring.id))
      onAnswer?.(ring.camera_id, session)
    } catch (error) {
      console.error('Failed to answer doorbell:', error)
    } finally {
      setIsAnswering(false)
    }
  }

  const handleDismiss = (ring: DoorbellRing) => {
    stopRingtone()
    setRings(prev => prev.filter(r => r.id !== ring.id))
    onDismiss?.(ring.camera_id)
  }

  const handleHangup = () => {
    setAnsweredSession(null)
    setIsMuted(false)
  }

  if (rings.length === 0 && !answeredSession) {
    return null
  }

  return (
    <>
      {/* Hidden audio elements */}
      <audio
        ref={ringtoneRef}
        src="/sounds/doorbell.mp3"
        loop
        preload="auto"
      />
      <audio
        ref={audioRef}
        autoPlay
        muted={isMuted}
      />

      {/* Doorbell ring notifications */}
      <div className="fixed top-4 right-4 z-50 space-y-3">
        {rings.map(ring => (
          <div
            key={ring.id}
            className="bg-card border border-border rounded-lg shadow-lg overflow-hidden animate-in slide-in-from-right duration-300 w-80"
          >
            {/* Header */}
            <div className="bg-primary/10 px-4 py-2 flex items-center justify-between">
              <div className="flex items-center gap-2">
                <Bell className="w-5 h-5 text-primary animate-bounce" />
                <span className="font-semibold">Doorbell</span>
              </div>
              <button
                onClick={() => handleDismiss(ring)}
                className="p-1 hover:bg-background/50 rounded transition-colors"
              >
                <X size={16} />
              </button>
            </div>

            {/* Snapshot */}
            <div className="relative aspect-video bg-black">
              <img
                src={ring.snapshot_url}
                alt="Doorbell snapshot"
                className="w-full h-full object-cover"
                onError={(e) => {
                  // Hide broken image
                  (e.target as HTMLImageElement).style.display = 'none'
                }}
              />
              <div className="absolute bottom-2 left-2 bg-black/70 px-2 py-1 rounded text-sm text-white">
                {ring.camera_name}
              </div>
            </div>

            {/* Actions */}
            <div className="p-3 flex gap-2">
              <button
                onClick={() => handleAnswer(ring)}
                disabled={isAnswering}
                className="flex-1 flex items-center justify-center gap-2 bg-green-600 hover:bg-green-700 text-white py-2 px-4 rounded-lg transition-colors disabled:opacity-50"
              >
                <Phone size={18} />
                {isAnswering ? 'Connecting...' : 'Answer'}
              </button>
              <button
                onClick={() => handleDismiss(ring)}
                className="flex items-center justify-center gap-2 bg-gray-600 hover:bg-gray-700 text-white py-2 px-4 rounded-lg transition-colors"
              >
                <X size={18} />
                Ignore
              </button>
            </div>
          </div>
        ))}

        {/* Active call overlay */}
        {answeredSession && (
          <div className="bg-card border border-green-500 rounded-lg shadow-lg overflow-hidden w-80">
            <div className="bg-green-500/20 px-4 py-2 flex items-center justify-between">
              <div className="flex items-center gap-2">
                <Phone className="w-5 h-5 text-green-500" />
                <span className="font-semibold text-green-700 dark:text-green-400">Connected</span>
              </div>
              <span className="text-sm text-muted-foreground">
                Two-way audio active
              </span>
            </div>

            <div className="p-3 flex gap-2">
              <button
                onClick={() => setIsMuted(!isMuted)}
                className={`flex-1 flex items-center justify-center gap-2 py-2 px-4 rounded-lg transition-colors ${
                  isMuted
                    ? 'bg-yellow-600 hover:bg-yellow-700 text-white'
                    : 'bg-gray-600 hover:bg-gray-700 text-white'
                }`}
              >
                {isMuted ? <VolumeX size={18} /> : <Volume2 size={18} />}
                {isMuted ? 'Unmute' : 'Mute'}
              </button>
              <button
                onClick={handleHangup}
                className="flex items-center justify-center gap-2 bg-red-600 hover:bg-red-700 text-white py-2 px-4 rounded-lg transition-colors"
              >
                <PhoneOff size={18} />
                End
              </button>
            </div>
          </div>
        )}
      </div>
    </>
  )
}
