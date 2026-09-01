package aof

import (
	"os"
	"sync"
	"time"
)

const (
	// DefaultBufferSize is 4MB per buffer partition for double-buffering.
	DefaultBufferSize = 4 * 1024 * 1024
)

// DoubleBuffer implements a zero-allocation batching buffer for maximum AOF write throughput.
type DoubleBuffer struct {
	bufA     []byte
	bufB     []byte
	active   []byte
	flushing []byte

	activeLen int64
	maxSize   int64

	mu   sync.Mutex
	file *os.File

	stopCh  chan struct{}
	running bool
}

// NewDoubleBuffer creates a new DoubleBuffer bound to an underlying file.
func NewDoubleBuffer(file *os.File, bufSize int) *DoubleBuffer {
	if bufSize <= 0 {
		bufSize = DefaultBufferSize
	}

	bufA := make([]byte, bufSize)
	bufB := make([]byte, bufSize)

	db := &DoubleBuffer{
		bufA:      bufA,
		bufB:      bufB,
		active:    bufA,
		flushing:  bufB,
		activeLen: 0,
		maxSize:   int64(bufSize),
		file:      file,
		stopCh:    make(chan struct{}),
		running:   true,
	}

	// Auto-flush ticker for micro-batching (flushes every 5ms or when buffer is full)
	go db.flusherLoop(5 * time.Millisecond)

	return db
}

// Write appends raw bytes to the active buffer. If the buffer is full, it triggers an immediate swap & flush.
func (db *DoubleBuffer) Write(data []byte) (int, error) {
	n := len(data)
	if n == 0 {
		return 0, nil
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	// If single write exceeds capacity, flush current buffer and write directly
	if int64(n) > db.maxSize {
		if err := db.flushLocked(); err != nil {
			return 0, err
		}
		if db.file != nil {
			return db.file.Write(data)
		}
		return n, nil
	}

	if db.activeLen+int64(n) > db.maxSize {
		if err := db.flushLocked(); err != nil {
			return 0, err
		}
	}

	copy(db.active[db.activeLen:db.activeLen+int64(n)], data)
	db.activeLen += int64(n)
	return n, nil
}

// Flush swaps active and flushing buffers, then performs a single batch I/O write.
func (db *DoubleBuffer) Flush() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.flushLocked()
}

func (db *DoubleBuffer) flushLocked() error {
	if db.activeLen == 0 {
		return nil
	}

	dataToFlush := db.active[:db.activeLen]
	db.active, db.flushing = db.flushing, db.active
	db.activeLen = 0

	if db.file != nil && len(dataToFlush) > 0 {
		_, err := db.file.Write(dataToFlush)
		return err
	}
	return nil
}

func (db *DoubleBuffer) flusherLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			_ = db.Flush()
		case <-db.stopCh:
			return
		}
	}
}

// Close flushes remaining bytes and stops background flusher.
func (db *DoubleBuffer) Close() error {
	if !db.running {
		return nil
	}
	close(db.stopCh)
	db.running = false
	return db.Flush()
}
