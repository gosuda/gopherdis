package stream

import (
	"sync"

	"github.com/gosuda/beaver/pure"
)

const (
	ChunkArenaSize = 8192 // 8KB per chunk slab
	MaxChunkEntries = 512
)

var chunkArenaPool = pure.NewPool(ChunkArenaSize)

// EntryRecord holds metadata and value slices pointing into the chunk's pure.Arena.
type EntryRecord struct {
	ID       StreamID
	FieldIDs []uint16
	Values   [][]byte
	Deleted  bool
}

// StreamEntry represents an unpacked stream message for external consumption.
type StreamEntry struct {
	ID     StreamID
	Fields []string
	Values [][]byte
}

// StreamChunk represents a segment of up to 512 stream entries backed by a beaver pure.Arena.
type StreamChunk struct {
	arena        *pure.Arena
	entries      []EntryRecord
	deletedCount int
	prev         *StreamChunk
	next         *StreamChunk
}

var chunkPool = sync.Pool{
	New: func() any {
		return &StreamChunk{
			entries: make([]EntryRecord, 0, 64),
		}
	},
}

// NewChunk allocates a new StreamChunk using beaver/pure.Pool.
func NewChunk() *StreamChunk {
	c := chunkPool.Get().(*StreamChunk)
	c.arena = chunkArenaPool.Get()
	c.entries = c.entries[:0]
	c.deletedCount = 0
	c.prev = nil
	c.next = nil
	return c
}

// Release returns the chunk's arena back to beaver pool.
func (c *StreamChunk) Release() {
	if c.arena != nil {
		c.arena.Reset()
		chunkArenaPool.Put(c.arena)
		c.arena = nil
	}
	c.entries = c.entries[:0]
	c.prev = nil
	c.next = nil
	chunkPool.Put(c)
}

// IsFull checks if chunk has reached maximum entries capacity.
func (c *StreamChunk) IsFull() bool {
	return len(c.entries) >= MaxChunkEntries
}

// NumActive returns the count of non-deleted entries in this chunk.
func (c *StreamChunk) NumActive() int {
	return len(c.entries) - c.deletedCount
}

// Append writes field values into the chunk arena and records entry metadata.
func (c *StreamChunk) Append(id StreamID, fieldIDs []uint16, values [][]byte) (bool, error) {
	if c.IsFull() {
		return false, nil
	}

	valSlices := make([][]byte, len(values))

	for i, v := range values {
		if len(v) > 0 {
			buf, err := c.arena.Alloc(len(v))
			if err != nil {
				// Arena exhausted, caller will roll over to new chunk
				return false, nil
			}
			copy(buf, v)
			valSlices[i] = buf
		} else {
			valSlices[i] = []byte{}
		}
	}

	c.entries = append(c.entries, EntryRecord{
		ID:       id,
		FieldIDs: fieldIDs,
		Values:   valSlices,
		Deleted:  false,
	})

	return true, nil
}
