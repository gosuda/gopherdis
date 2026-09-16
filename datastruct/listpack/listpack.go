// Package listpack implements a compact, contiguous representation for small
// collections.
//
// Redis stores small hashes, sets, sorted sets and lists in a listpack: one
// flat byte buffer holding every element back to back, instead of a hash table
// with per-entry allocations. The saving is real and is why OBJECT ENCODING
// reports it: a 10 field hash costs one allocation here rather than a map
// header plus a string header and a byte slice header per entry.
//
// Lookups are linear. That is the intended trade: the encoding is only used
// below the configured entry and value thresholds, above which callers convert
// to a hash table.
package listpack

import (
	"bytes"
	"encoding/binary"
)

// Listpack is a sequence of byte-string elements packed into one buffer.
// Each element is stored as a uvarint length followed by its bytes.
type Listpack struct {
	buf []byte
	n   int
}

// New returns an empty listpack.
func New() *Listpack { return &Listpack{} }

// Len returns the number of elements.
func (lp *Listpack) Len() int {
	if lp == nil {
		return 0
	}
	return lp.n
}

// Bytes returns the size of the backing buffer, for memory accounting.
func (lp *Listpack) Bytes() int {
	if lp == nil {
		return 0
	}
	return len(lp.buf)
}

// Append adds an element to the end.
func (lp *Listpack) Append(val []byte) {
	var hdr [binary.MaxVarintLen64]byte
	k := binary.PutUvarint(hdr[:], uint64(len(val)))
	lp.buf = append(lp.buf, hdr[:k]...)
	lp.buf = append(lp.buf, val...)
	lp.n++
}

// offsets walks the buffer and returns the start offset of every element's
// payload along with its length.
func (lp *Listpack) offsets() (starts []int, lens []int) {
	starts = make([]int, 0, lp.n)
	lens = make([]int, 0, lp.n)
	for i := 0; i < len(lp.buf); {
		l, k := binary.Uvarint(lp.buf[i:])
		if k <= 0 {
			break
		}
		starts = append(starts, i+k)
		lens = append(lens, int(l))
		i += k + int(l)
	}
	return starts, lens
}

// Get returns the element at index i, or nil when out of range. The result
// aliases the buffer and must not be retained across a mutation.
func (lp *Listpack) Get(i int) []byte {
	if lp == nil || i < 0 || i >= lp.n {
		return nil
	}
	starts, lens := lp.offsets()
	if i >= len(starts) {
		return nil
	}
	return lp.buf[starts[i] : starts[i]+lens[i]]
}

// ForEach calls fn with each element in order. Returning false stops the walk.
func (lp *Listpack) ForEach(fn func(i int, val []byte) bool) {
	if lp == nil {
		return
	}
	idx := 0
	for i := 0; i < len(lp.buf); {
		l, k := binary.Uvarint(lp.buf[i:])
		if k <= 0 {
			return
		}
		start := i + k
		if !fn(idx, lp.buf[start:start+int(l)]) {
			return
		}
		idx++
		i = start + int(l)
	}
}

// Find returns the index of the first element equal to val, stepping by stride
// so callers can search only keys in a key/value listpack. Returns -1 if absent.
func (lp *Listpack) Find(val []byte, start, stride int) int {
	if lp == nil {
		return -1
	}
	found := -1
	lp.ForEach(func(i int, cur []byte) bool {
		if i < start || (i-start)%stride != 0 {
			return true
		}
		if bytes.Equal(cur, val) {
			found = i
			return false
		}
		return true
	})
	return found
}

// ReplaceAt overwrites the element at index i.
func (lp *Listpack) ReplaceAt(i int, val []byte) bool {
	if lp == nil || i < 0 || i >= lp.n {
		return false
	}
	starts, lens := lp.offsets()
	if i >= len(starts) {
		return false
	}
	hdrStart := starts[i] - uvarintLen(uint64(lens[i]))
	tail := starts[i] + lens[i]

	var hdr [binary.MaxVarintLen64]byte
	k := binary.PutUvarint(hdr[:], uint64(len(val)))

	next := make([]byte, 0, len(lp.buf)-(tail-hdrStart)+k+len(val))
	next = append(next, lp.buf[:hdrStart]...)
	next = append(next, hdr[:k]...)
	next = append(next, val...)
	next = append(next, lp.buf[tail:]...)
	lp.buf = next
	return true
}

// DeleteRange removes count elements starting at index i.
func (lp *Listpack) DeleteRange(i, count int) bool {
	if lp == nil || i < 0 || count <= 0 || i+count > lp.n {
		return false
	}
	starts, lens := lp.offsets()
	if i+count > len(starts) {
		return false
	}
	from := starts[i] - uvarintLen(uint64(lens[i]))
	last := i + count - 1
	to := starts[last] + lens[last]

	next := make([]byte, 0, len(lp.buf)-(to-from))
	next = append(next, lp.buf[:from]...)
	next = append(next, lp.buf[to:]...)
	lp.buf = next
	lp.n -= count
	return true
}

// InsertAt inserts val before index i. i == Len() appends.
func (lp *Listpack) InsertAt(i int, val []byte) bool {
	if lp == nil || i < 0 || i > lp.n {
		return false
	}
	if i == lp.n {
		lp.Append(val)
		return true
	}
	starts, lens := lp.offsets()
	at := starts[i] - uvarintLen(uint64(lens[i]))

	var hdr [binary.MaxVarintLen64]byte
	k := binary.PutUvarint(hdr[:], uint64(len(val)))

	next := make([]byte, 0, len(lp.buf)+k+len(val))
	next = append(next, lp.buf[:at]...)
	next = append(next, hdr[:k]...)
	next = append(next, val...)
	next = append(next, lp.buf[at:]...)
	lp.buf = next
	lp.n++
	return true
}

// All returns a copy of every element.
func (lp *Listpack) All() [][]byte {
	if lp == nil {
		return nil
	}
	out := make([][]byte, 0, lp.n)
	lp.ForEach(func(_ int, v []byte) bool {
		out = append(out, bytes.Clone(v))
		return true
	})
	return out
}

func uvarintLen(v uint64) int {
	n := 1
	for v >= 0x80 {
		v >>= 7
		n++
	}
	return n
}
