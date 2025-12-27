package recording

import (
	"testing"
	"time"
)

func TestNewMemoryRingBuffer(t *testing.T) {
	buf := NewMemoryRingBuffer(10, time.Minute)
	if buf == nil {
		t.Fatal("NewMemoryRingBuffer returned nil")
	}
	if buf.capacity != 10 {
		t.Errorf("Expected capacity 10, got %d", buf.capacity)
	}
	if buf.maxAge != time.Minute {
		t.Errorf("Expected maxAge 1 minute, got %v", buf.maxAge)
	}
}

func TestMemoryRingBuffer_Write(t *testing.T) {
	buf := NewMemoryRingBuffer(5, 0)

	// Write some data
	err := buf.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if buf.Count() != 1 {
		t.Errorf("Expected count 1, got %d", buf.Count())
	}
}

func TestMemoryRingBuffer_Write_Overflow(t *testing.T) {
	buf := NewMemoryRingBuffer(3, 0)

	// Fill buffer
	for i := 0; i < 5; i++ {
		err := buf.Write([]byte{byte(i)})
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
	}

	// Should only have last 3 entries
	if buf.Count() != 3 {
		t.Errorf("Expected count 3, got %d", buf.Count())
	}

	// Read and verify we have the last 3 values
	data := buf.Read()
	if len(data) != 3 {
		t.Errorf("Expected 3 bytes, got %d", len(data))
	}
	for i, b := range data {
		expected := byte(i + 2) // Should be 2, 3, 4
		if b != expected {
			t.Errorf("Expected byte %d, got %d at index %d", expected, b, i)
		}
	}
}

func TestMemoryRingBuffer_WriteFrame(t *testing.T) {
	buf := NewMemoryRingBuffer(5, 0)

	ts := time.Now()
	err := buf.WriteFrame(FrameData{Data: []byte("test"), Timestamp: ts})
	if err != nil {
		t.Fatalf("WriteFrame failed: %v", err)
	}

	if buf.Count() != 1 {
		t.Errorf("Expected count 1, got %d", buf.Count())
	}
}

func TestMemoryRingBuffer_Read_Empty(t *testing.T) {
	buf := NewMemoryRingBuffer(5, 0)

	data := buf.Read()
	if data != nil {
		t.Errorf("Expected nil for empty buffer, got %v", data)
	}
}

func TestMemoryRingBuffer_ReadFrames(t *testing.T) {
	buf := NewMemoryRingBuffer(5, 0)

	// Add some frames
	now := time.Now()
	for i := 0; i < 3; i++ {
		_ = buf.WriteFrame(FrameData{
			Data:      []byte{byte(i)},
			Timestamp: now.Add(time.Duration(i) * time.Second),
		})
	}

	frames := buf.ReadFrames()
	if len(frames) != 3 {
		t.Errorf("Expected 3 frames, got %d", len(frames))
	}

	// Verify order
	for i, f := range frames {
		if f.Data[0] != byte(i) {
			t.Errorf("Expected byte %d at index %d, got %d", i, i, f.Data[0])
		}
	}
}

func TestMemoryRingBuffer_ReadFrames_Empty(t *testing.T) {
	buf := NewMemoryRingBuffer(5, 0)

	frames := buf.ReadFrames()
	if frames != nil {
		t.Errorf("Expected nil for empty buffer, got %v", frames)
	}
}

func TestMemoryRingBuffer_ReadSince(t *testing.T) {
	buf := NewMemoryRingBuffer(10, 0)

	now := time.Now()
	// Add frames at different times
	for i := 0; i < 5; i++ {
		_ = buf.WriteFrame(FrameData{
			Data:      []byte{byte(i)},
			Timestamp: now.Add(time.Duration(i) * time.Second),
		})
	}

	// Read frames since 2 seconds after start
	since := now.Add(2 * time.Second)
	frames := buf.ReadSince(since)

	// Should get frames at t+2, t+3, t+4
	if len(frames) != 3 {
		t.Errorf("Expected 3 frames, got %d", len(frames))
	}
}

func TestMemoryRingBuffer_ReadSince_Empty(t *testing.T) {
	buf := NewMemoryRingBuffer(5, 0)

	frames := buf.ReadSince(time.Now())
	if frames != nil {
		t.Errorf("Expected nil for empty buffer, got %v", frames)
	}
}

func TestMemoryRingBuffer_Duration(t *testing.T) {
	buf := NewMemoryRingBuffer(10, 0)

	// Empty buffer should return 0 duration
	if buf.Duration() != 0 {
		t.Errorf("Expected 0 duration for empty buffer, got %v", buf.Duration())
	}

	// Single frame should return 0 duration
	now := time.Now()
	_ = buf.WriteFrame(FrameData{Data: []byte{1}, Timestamp: now})
	if buf.Duration() != 0 {
		t.Errorf("Expected 0 duration for single frame, got %v", buf.Duration())
	}

	// Multiple frames
	_ = buf.WriteFrame(FrameData{Data: []byte{2}, Timestamp: now.Add(5 * time.Second)})
	duration := buf.Duration()
	if duration != 5*time.Second {
		t.Errorf("Expected 5 seconds, got %v", duration)
	}
}

func TestMemoryRingBuffer_Size(t *testing.T) {
	buf := NewMemoryRingBuffer(10, 0)

	if buf.Size() != 0 {
		t.Errorf("Expected size 0 for empty buffer, got %d", buf.Size())
	}

	_ = buf.Write([]byte("hello"))
	_ = buf.Write([]byte("world"))

	expectedSize := int64(len("hello") + len("world"))
	if buf.Size() != expectedSize {
		t.Errorf("Expected size %d, got %d", expectedSize, buf.Size())
	}
}

func TestMemoryRingBuffer_Clear(t *testing.T) {
	buf := NewMemoryRingBuffer(10, 0)

	_ = buf.Write([]byte("test"))
	buf.Clear()

	if buf.Count() != 0 {
		t.Errorf("Expected count 0 after clear, got %d", buf.Count())
	}
	if buf.Size() != 0 {
		t.Errorf("Expected size 0 after clear, got %d", buf.Size())
	}
}

func TestMemoryRingBuffer_Close(t *testing.T) {
	buf := NewMemoryRingBuffer(10, 0)

	err := buf.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Write after close should fail
	err = buf.Write([]byte("test"))
	if err != ErrBufferClosed {
		t.Errorf("Expected ErrBufferClosed, got %v", err)
	}

	err = buf.WriteFrame(FrameData{Data: []byte("test"), Timestamp: time.Now()})
	if err != ErrBufferClosed {
		t.Errorf("Expected ErrBufferClosed for WriteFrame, got %v", err)
	}
}

func TestMemoryRingBuffer_EvictOld(t *testing.T) {
	maxAge := 100 * time.Millisecond
	buf := NewMemoryRingBuffer(10, maxAge)

	// Write some old frames
	oldTime := time.Now().Add(-200 * time.Millisecond)
	buf.WriteFrame(FrameData{Data: []byte{1}, Timestamp: oldTime})
	buf.WriteFrame(FrameData{Data: []byte{2}, Timestamp: oldTime})

	// Write a new frame - this should trigger eviction
	buf.WriteFrame(FrameData{Data: []byte{3}, Timestamp: time.Now()})

	// Old frames should be evicted
	if buf.Count() != 1 {
		t.Errorf("Expected count 1 after eviction, got %d", buf.Count())
	}
}

func TestMemoryRingBuffer_ConcurrentAccess(t *testing.T) {
	buf := NewMemoryRingBuffer(100, 0)

	// Run concurrent writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				buf.Write([]byte{byte(id), byte(j)})
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have some data without panicking
	if buf.Count() == 0 {
		t.Error("Expected some data in buffer after concurrent writes")
	}
}

func TestFileRingBuffer_New(t *testing.T) {
	buf := NewFileRingBuffer("/tmp/test", 1024*1024, time.Hour)
	if buf == nil {
		t.Fatal("NewFileRingBuffer returned nil")
	}
	if buf.path != "/tmp/test" {
		t.Errorf("Expected path /tmp/test, got %s", buf.path)
	}
}

func TestFileRingBuffer_AddSegment(t *testing.T) {
	buf := NewFileRingBuffer("/tmp/test", 1024*1024, time.Hour)

	buf.AddSegment("/tmp/segment1.ts")
	buf.AddSegment("/tmp/segment2.ts")

	segments := buf.GetSegments()
	if len(segments) != 2 {
		t.Errorf("Expected 2 segments, got %d", len(segments))
	}
}

func TestFileRingBuffer_Clear(t *testing.T) {
	buf := NewFileRingBuffer("/tmp/test", 1024*1024, time.Hour)

	buf.AddSegment("/tmp/segment1.ts")
	buf.Clear()

	segments := buf.GetSegments()
	if len(segments) != 0 {
		t.Errorf("Expected 0 segments after clear, got %d", len(segments))
	}
}

func TestFileRingBuffer_Close(t *testing.T) {
	buf := NewFileRingBuffer("/tmp/test", 1024*1024, time.Hour)

	err := buf.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestBufferError(t *testing.T) {
	err := BufferError("test error")
	if err.Error() != "test error" {
		t.Errorf("Expected 'test error', got '%s'", err.Error())
	}
}
