package recording

import (
	"sync"
	"time"
)

// FrameData represents a single frame with timestamp
type FrameData struct {
	Data      []byte
	Timestamp time.Time
}

// MemoryRingBuffer implements a ring buffer for pre-event frame storage
type MemoryRingBuffer struct {
	mu       sync.RWMutex
	frames   []FrameData
	head     int
	tail     int
	count    int
	capacity int
	maxAge   time.Duration
	closed   bool
}

// NewMemoryRingBuffer creates a new memory-based ring buffer
func NewMemoryRingBuffer(capacity int, maxAge time.Duration) *MemoryRingBuffer {
	return &MemoryRingBuffer{
		frames:   make([]FrameData, capacity),
		capacity: capacity,
		maxAge:   maxAge,
	}
}

// Write adds data to the buffer
func (b *MemoryRingBuffer) Write(data []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return ErrBufferClosed
	}

	// Create a copy of the data
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	frame := FrameData{
		Data:      dataCopy,
		Timestamp: time.Now(),
	}

	// Add to buffer
	b.frames[b.head] = frame
	b.head = (b.head + 1) % b.capacity

	if b.count < b.capacity {
		b.count++
	} else {
		// Overwrite oldest - move tail forward
		b.tail = (b.tail + 1) % b.capacity
	}

	// Clean up old frames beyond maxAge
	b.evictOld()

	return nil
}

// WriteFrame adds a frame with timestamp to the buffer
func (b *MemoryRingBuffer) WriteFrame(frame FrameData) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return ErrBufferClosed
	}

	// Create a copy of the data
	dataCopy := make([]byte, len(frame.Data))
	copy(dataCopy, frame.Data)

	frameCopy := FrameData{
		Data:      dataCopy,
		Timestamp: frame.Timestamp,
	}

	b.frames[b.head] = frameCopy
	b.head = (b.head + 1) % b.capacity

	if b.count < b.capacity {
		b.count++
	} else {
		b.tail = (b.tail + 1) % b.capacity
	}

	b.evictOld()
	return nil
}

// Read reads all buffered data in order
func (b *MemoryRingBuffer) Read() []byte {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.count == 0 {
		return nil
	}

	// Calculate total size
	totalSize := 0
	idx := b.tail
	for i := 0; i < b.count; i++ {
		totalSize += len(b.frames[idx].Data)
		idx = (idx + 1) % b.capacity
	}

	// Concatenate all frames
	result := make([]byte, 0, totalSize)
	idx = b.tail
	for i := 0; i < b.count; i++ {
		result = append(result, b.frames[idx].Data...)
		idx = (idx + 1) % b.capacity
	}

	return result
}

// ReadFrames reads all buffered frames in order
func (b *MemoryRingBuffer) ReadFrames() []FrameData {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.count == 0 {
		return nil
	}

	frames := make([]FrameData, b.count)
	idx := b.tail
	for i := 0; i < b.count; i++ {
		frames[i] = b.frames[idx]
		idx = (idx + 1) % b.capacity
	}

	return frames
}

// ReadSince reads all frames since a given timestamp
func (b *MemoryRingBuffer) ReadSince(since time.Time) []FrameData {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.count == 0 {
		return nil
	}

	var frames []FrameData
	idx := b.tail
	for i := 0; i < b.count; i++ {
		if !b.frames[idx].Timestamp.Before(since) {
			frames = append(frames, b.frames[idx])
		}
		idx = (idx + 1) % b.capacity
	}

	return frames
}

// Duration returns the current buffer duration
func (b *MemoryRingBuffer) Duration() time.Duration {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.count < 2 {
		return 0
	}

	oldest := b.frames[b.tail]
	newestIdx := (b.head - 1 + b.capacity) % b.capacity
	newest := b.frames[newestIdx]

	return newest.Timestamp.Sub(oldest.Timestamp)
}

// Size returns the current buffer size in bytes
func (b *MemoryRingBuffer) Size() int64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var size int64
	idx := b.tail
	for i := 0; i < b.count; i++ {
		size += int64(len(b.frames[idx].Data))
		idx = (idx + 1) % b.capacity
	}
	return size
}

// Count returns the number of frames in the buffer
func (b *MemoryRingBuffer) Count() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.count
}

// Clear clears the buffer
func (b *MemoryRingBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.frames = make([]FrameData, b.capacity)
	b.head = 0
	b.tail = 0
	b.count = 0
}

// Close closes the buffer
func (b *MemoryRingBuffer) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.closed = true
	b.frames = nil
	return nil
}

// evictOld removes frames older than maxAge (must be called with lock held)
func (b *MemoryRingBuffer) evictOld() {
	if b.maxAge == 0 || b.count == 0 {
		return
	}

	cutoff := time.Now().Add(-b.maxAge)

	for b.count > 0 {
		if b.frames[b.tail].Timestamp.After(cutoff) {
			break
		}
		b.frames[b.tail] = FrameData{} // Clear reference
		b.tail = (b.tail + 1) % b.capacity
		b.count--
	}
}

// BufferError represents a ring buffer error
type BufferError string

func (e BufferError) Error() string { return string(e) }

// ErrBufferClosed is returned when writing to a closed buffer
const ErrBufferClosed = BufferError("ring buffer is closed")

// FileRingBuffer implements a file-based ring buffer for larger pre-event storage
type FileRingBuffer struct {
	mu       sync.RWMutex
	path     string
	maxSize  int64
	maxAge   time.Duration
	segments []string
	closed   bool
}

// NewFileRingBuffer creates a file-based ring buffer
func NewFileRingBuffer(path string, maxSize int64, maxAge time.Duration) *FileRingBuffer {
	return &FileRingBuffer{
		path:    path,
		maxSize: maxSize,
		maxAge:  maxAge,
	}
}

// Write adds a segment file to the buffer
func (b *FileRingBuffer) Write(data []byte) error {
	// For file-based buffer, this would write to a temp segment file
	// Implementation depends on specific use case
	return nil
}

// AddSegment adds an existing segment file to the buffer
func (b *FileRingBuffer) AddSegment(path string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.segments = append(b.segments, path)
}

// Read returns paths to all buffered segments
func (b *FileRingBuffer) Read() []byte {
	// For file-based buffer, this returns concatenated data or paths
	return nil
}

// GetSegments returns all segment paths in the buffer
func (b *FileRingBuffer) GetSegments() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	result := make([]string, len(b.segments))
	copy(result, b.segments)
	return result
}

// Duration returns the total duration of buffered segments
func (b *FileRingBuffer) Duration() time.Duration {
	// Would need to read segment metadata
	return 0
}

// Clear clears the buffer (does not delete files)
func (b *FileRingBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.segments = nil
}

// Close closes the buffer
func (b *FileRingBuffer) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	return nil
}
