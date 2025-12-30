// Global Stream Manager - Maintains pre-buffered video streams for instant playback
// This keeps WebSocket connections and MediaSource buffers active for all cameras

import { getGo2RTCWsUrl } from './ports'

// Get the appropriate MediaSource constructor for the platform
// iOS Safari uses ManagedMediaSource for better PWA support
function getMediaSourceConstructor(): typeof MediaSource | null {
  if ('ManagedMediaSource' in window) {
    return (window as unknown as { ManagedMediaSource: typeof MediaSource }).ManagedMediaSource
  }
  if ('MediaSource' in window) {
    return MediaSource
  }
  return null
}

// Check if a codec is supported by the current MediaSource implementation
function isCodecSupported(codec: string): boolean {
  const MSConstructor = getMediaSourceConstructor()
  if (!MSConstructor) return false
  return MSConstructor.isTypeSupported(codec)
}

interface StreamBuffer {
  ws: WebSocket | null
  mediaSource: MediaSource | null
  sourceBuffer: SourceBuffer | null
  objectUrl: string | null
  queue: Uint8Array[]
  isReady: boolean
  codec: string | null
  lastActivity: number
  subscribers: Set<(url: string) => void>
  reconnectCount: number // Track consecutive reconnection attempts
  lastConnectTime: number // When was the last successful connection
}

class StreamManager {
  private streams: Map<string, StreamBuffer> = new Map()
  private reconnectTimers: Map<string, NodeJS.Timeout> = new Map()

  // Get or create a stream buffer for a camera
  getStream(cameraId: string): StreamBuffer {
    const streamName = this.getStreamName(cameraId)

    if (!this.streams.has(streamName)) {
      this.createStream(streamName)
    }

    return this.streams.get(streamName)!
  }

  // Subscribe to stream ready events
  subscribe(cameraId: string, callback: (url: string) => void): () => void {
    const stream = this.getStream(cameraId)

    stream.subscribers.add(callback)

    // If already ready, call immediately
    if (stream.isReady && stream.objectUrl) {
      callback(stream.objectUrl)
    }

    // Return unsubscribe function
    return () => {
      stream.subscribers.delete(callback)
    }
  }

  // Get the object URL for immediate playback (if available)
  getObjectUrl(cameraId: string): string | null {
    const streamName = this.getStreamName(cameraId)
    const stream = this.streams.get(streamName)
    return stream?.objectUrl || null
  }

  // Check if stream is ready for instant playback
  isReady(cameraId: string): boolean {
    const streamName = this.getStreamName(cameraId)
    const stream = this.streams.get(streamName)
    return stream?.isReady || false
  }

  // Pre-connect to multiple cameras
  preconnect(cameraIds: string[]): void {
    for (const id of cameraIds) {
      this.getStream(id) // This will create and connect if not exists
    }
  }

  // Disconnect a specific camera stream
  disconnect(cameraId: string): void {
    const streamName = this.getStreamName(cameraId)
    const stream = this.streams.get(streamName)

    if (stream) {
      this.cleanupStream(stream)
      this.streams.delete(streamName)
    }

    const timer = this.reconnectTimers.get(streamName)
    if (timer) {
      clearTimeout(timer)
      this.reconnectTimers.delete(streamName)
    }
  }

  // Disconnect all streams
  disconnectAll(): void {
    for (const stream of this.streams.values()) {
      this.cleanupStream(stream)
    }
    this.streams.clear()

    for (const timer of this.reconnectTimers.values()) {
      clearTimeout(timer)
    }
    this.reconnectTimers.clear()
  }

  private getStreamName(cameraId: string): string {
    return cameraId.toLowerCase().replace(/\s+/g, '_')
  }

  private createStream(streamName: string): void {
    const buffer: StreamBuffer = {
      ws: null,
      mediaSource: null,
      sourceBuffer: null,
      objectUrl: null,
      queue: [],
      isReady: false,
      codec: null,
      lastActivity: Date.now(),
      subscribers: new Set(),
      reconnectCount: 0,
      lastConnectTime: 0
    }

    this.streams.set(streamName, buffer)
    this.connectStream(streamName, buffer)
  }

  private connectStream(streamName: string, buffer: StreamBuffer): void {
    // Clean up any existing connection
    if (buffer.ws) {
      buffer.ws.close()
    }

    // Check if MediaSource is available
    const MSConstructor = getMediaSourceConstructor()
    if (!MSConstructor) {
      console.warn(`[StreamManager] MediaSource not available, skipping ${streamName}`)
      return
    }

    const ws = new WebSocket(`${getGo2RTCWsUrl()}/api/ws?src=${streamName}`)
    buffer.ws = ws
    ws.binaryType = 'arraybuffer'

    // Create new MediaSource using the appropriate constructor for the platform
    const mediaSource = new MSConstructor()
    buffer.mediaSource = mediaSource
    buffer.objectUrl = URL.createObjectURL(mediaSource)

    ws.onopen = () => {
      console.log(`[StreamManager] Connected to ${streamName}`)
      // Reset reconnect count on successful connection
      buffer.reconnectCount = 0
      buffer.lastConnectTime = Date.now()
      // Request MSE stream
      ws.send(JSON.stringify({ type: 'mse' }))
    }

    ws.onmessage = (event) => {
      buffer.lastActivity = Date.now()

      if (typeof event.data === 'string') {
        const msg = JSON.parse(event.data)

        if (msg.type === 'mse') {
          const codec = msg.value
          console.log(`[StreamManager] ${streamName} codec: ${codec}`)
          buffer.codec = codec

          // Wait for mediaSource to be open
          if (mediaSource.readyState === 'open') {
            this.setupSourceBuffer(streamName, buffer, codec)
          } else {
            mediaSource.addEventListener('sourceopen', () => {
              this.setupSourceBuffer(streamName, buffer, codec)
            }, { once: true })
          }
        }
      } else if (event.data instanceof ArrayBuffer) {
        // Queue binary data
        buffer.queue.push(new Uint8Array(event.data))
        this.processQueue(buffer)
      }
    }

    ws.onerror = (e) => {
      console.error(`[StreamManager] WebSocket error for ${streamName}:`, e)
    }

    ws.onclose = () => {
      console.log(`[StreamManager] Disconnected from ${streamName}`)
      buffer.isReady = false

      // Exponential backoff: 2s, 4s, 8s, 16s, max 30s
      // Reset count if we were connected for more than 60 seconds (stable connection)
      const wasStableConnection = buffer.lastConnectTime > 0 &&
        (Date.now() - buffer.lastConnectTime) > 60000

      if (wasStableConnection) {
        buffer.reconnectCount = 0
      }

      buffer.reconnectCount++
      const baseDelay = 2000
      const maxDelay = 30000
      const delay = Math.min(baseDelay * Math.pow(2, buffer.reconnectCount - 1), maxDelay)

      console.log(`[StreamManager] Will reconnect to ${streamName} in ${delay}ms (attempt ${buffer.reconnectCount})`)

      const timer = setTimeout(() => {
        if (this.streams.has(streamName)) {
          console.log(`[StreamManager] Reconnecting to ${streamName} (attempt ${buffer.reconnectCount})`)
          this.connectStream(streamName, buffer)
        }
      }, delay)

      this.reconnectTimers.set(streamName, timer)
    }
  }

  private setupSourceBuffer(streamName: string, buffer: StreamBuffer, codec: string): void {
    if (!buffer.mediaSource || buffer.mediaSource.readyState !== 'open') {
      return
    }

    if (!isCodecSupported(codec)) {
      console.error(`[StreamManager] Unsupported codec: ${codec}`)
      return
    }

    try {
      const sourceBuffer = buffer.mediaSource.addSourceBuffer(codec)
      sourceBuffer.mode = 'segments'
      buffer.sourceBuffer = sourceBuffer

      sourceBuffer.addEventListener('updateend', () => {
        this.processQueue(buffer)

        // Keep buffer small for live viewing (max 5 seconds)
        if (sourceBuffer.buffered.length > 0) {
          const start = sourceBuffer.buffered.start(0)
          const end = sourceBuffer.buffered.end(sourceBuffer.buffered.length - 1)
          if (end - start > 5 && !sourceBuffer.updating) {
            try {
              sourceBuffer.remove(start, end - 3)
            } catch {
              // Ignore removal errors
            }
          }
        }
      })

      // Mark as ready after first data is buffered
      sourceBuffer.addEventListener('updateend', () => {
        if (!buffer.isReady && sourceBuffer.buffered.length > 0) {
          buffer.isReady = true
          console.log(`[StreamManager] ${streamName} ready for instant playback`)

          // Notify all subscribers
          if (buffer.objectUrl) {
            for (const callback of buffer.subscribers) {
              callback(buffer.objectUrl)
            }
          }
        }
      }, { once: true })

      // Process any queued data
      this.processQueue(buffer)

    } catch (e) {
      console.error(`[StreamManager] Failed to setup source buffer:`, e)
    }
  }

  private processQueue(buffer: StreamBuffer): void {
    if (!buffer.sourceBuffer || buffer.sourceBuffer.updating || buffer.queue.length === 0) {
      return
    }

    try {
      const data = buffer.queue.shift()!
      buffer.sourceBuffer.appendBuffer(data)
    } catch (e) {
      console.error('[StreamManager] Buffer append error:', e)
      // Clear some queue if we're getting backed up
      if (buffer.queue.length > 10) {
        buffer.queue = buffer.queue.slice(-5)
      }
    }
  }

  private cleanupStream(stream: StreamBuffer): void {
    if (stream.ws) {
      stream.ws.close()
      stream.ws = null
    }

    if (stream.objectUrl) {
      URL.revokeObjectURL(stream.objectUrl)
      stream.objectUrl = null
    }

    if (stream.mediaSource && stream.mediaSource.readyState === 'open') {
      try {
        stream.mediaSource.endOfStream()
      } catch {
        // Ignore
      }
    }

    stream.mediaSource = null
    stream.sourceBuffer = null
    stream.queue = []
    stream.isReady = false
    stream.subscribers.clear()
  }
}

// Export singleton instance
export const streamManager = new StreamManager()

// Export for direct access in components
export type { StreamBuffer }
