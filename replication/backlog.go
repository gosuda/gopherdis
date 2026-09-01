package replication

import (
	"sync"
)

// Backlog is a fixed-size circular ring buffer holding recent raw RESP command bytes.
type Backlog struct {
	mu          sync.RWMutex
	buf         []byte
	size        int64
	firstOffset int64 // Global offset of the oldest byte currently in buffer
	lastOffset  int64 // Global offset of the newest byte currently in buffer
	head        int64 // Write pointer in circular buffer
}

// NewBacklog creates a new circular backlog buffer with specified capacity in bytes.
func NewBacklog(capacity int64) *Backlog {
	if capacity <= 0 {
		capacity = 1024 * 1024 // Default 1MB
	}
	return &Backlog{
		buf:         make([]byte, capacity),
		size:        capacity,
		firstOffset: 0,
		lastOffset:  0,
		head:        0,
	}
}

// Feed writes data into the circular ring buffer and advances offsets.
func (b *Backlog) Feed(data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	n := int64(len(data))
	if n == 0 {
		return
	}

	for i := int64(0); i < n; i++ {
		b.buf[b.head] = data[i]
		b.head = (b.head + 1) % b.size
	}

	b.lastOffset += n
	if b.lastOffset-b.firstOffset > b.size {
		b.firstOffset = b.lastOffset - b.size
	}
}

// CanPartialSync checks if the requested offset falls within the valid range in the backlog.
func (b *Backlog) CanPartialSync(offset int64) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.lastOffset > 0 && offset >= b.firstOffset && offset <= b.lastOffset
}

// ReadFromOffset returns the continuous slice of backlog bytes starting from requested offset to lastOffset.
func (b *Backlog) ReadFromOffset(offset int64) []byte {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if offset < b.firstOffset || offset > b.lastOffset {
		return nil
	}

	bytesToRead := b.lastOffset - offset
	if bytesToRead <= 0 {
		return []byte{}
	}

	result := make([]byte, bytesToRead)
	// Calculate starting index in circular buffer
	// Distance from lastOffset is (lastOffset - offset)
	// Current write head is b.head (points to lastOffset position)
	startIdx := (b.head - bytesToRead) % b.size
	if startIdx < 0 {
		startIdx += b.size
	}

	for i := int64(0); i < bytesToRead; i++ {
		result[i] = b.buf[(startIdx+i)%b.size]
	}

	return result
}

// Offsets returns current first and last global offsets.
func (b *Backlog) Offsets() (int64, int64) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.firstOffset, b.lastOffset
}
