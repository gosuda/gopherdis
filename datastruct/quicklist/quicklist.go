package quicklist

import (
	"bytes"
	"sync"
)

// DefaultNodeCapacity defines the maximum number of items in a single node chunk.
const DefaultNodeCapacity = 128

// QuicklistNode is a doubly-linked node containing a contiguous slice of byte slices.
type QuicklistNode struct {
	prev  *QuicklistNode
	next  *QuicklistNode
	items [][]byte
}

// Quicklist is an unrolled linked list optimized for Redis List operations.
type Quicklist struct {
	mu        sync.RWMutex
	head      *QuicklistNode
	tail      *QuicklistNode
	count     int // Total number of elements across all nodes
	nodeCount int // Total number of nodes
}

// NewQuicklist creates and returns an empty Quicklist.
func NewQuicklist() *Quicklist {
	return &Quicklist{}
}

// Len returns the total number of elements in the quicklist.
func (ql *Quicklist) Len() int {
	if ql == nil {
		return 0
	}
	ql.mu.RLock()
	defer ql.mu.RUnlock()
	return ql.count
}

// LPush inserts elements at the head of the quicklist.
func (ql *Quicklist) LPush(val []byte) {
	ql.mu.Lock()
	defer ql.mu.Unlock()

	cloned := bytes.Clone(val)
	if ql.head == nil {
		node := &QuicklistNode{
			items: [][]byte{cloned},
		}
		ql.head = node
		ql.tail = node
		ql.nodeCount = 1
		ql.count = 1
		return
	}

	if len(ql.head.items) < DefaultNodeCapacity {
		ql.head.items = append([][]byte{cloned}, ql.head.items...)
	} else {
		node := &QuicklistNode{
			items: [][]byte{cloned},
			next:  ql.head,
		}
		ql.head.prev = node
		ql.head = node
		ql.nodeCount++
	}
	ql.count++
}

// RPush appends elements at the tail of the quicklist.
func (ql *Quicklist) RPush(val []byte) {
	ql.mu.Lock()
	defer ql.mu.Unlock()

	cloned := bytes.Clone(val)
	if ql.tail == nil {
		node := &QuicklistNode{
			items: [][]byte{cloned},
		}
		ql.head = node
		ql.tail = node
		ql.nodeCount = 1
		ql.count = 1
		return
	}

	if len(ql.tail.items) < DefaultNodeCapacity {
		ql.tail.items = append(ql.tail.items, cloned)
	} else {
		node := &QuicklistNode{
			items: [][]byte{cloned},
			prev:  ql.tail,
		}
		ql.tail.next = node
		ql.tail = node
		ql.nodeCount++
	}
	ql.count++
}

// LPop removes and returns the first element from the head.
func (ql *Quicklist) LPop() ([]byte, bool) {
	ql.mu.Lock()
	defer ql.mu.Unlock()

	if ql == nil || ql.count == 0 || ql.head == nil {
		return nil, false
	}

	val := ql.head.items[0]
	ql.head.items = ql.head.items[1:]
	ql.count--

	if len(ql.head.items) == 0 {
		ql.head = ql.head.next
		if ql.head != nil {
			ql.head.prev = nil
		} else {
			ql.tail = nil
		}
		ql.nodeCount--
	}
	return val, true
}

// RPop removes and returns the last element from the tail.
func (ql *Quicklist) RPop() ([]byte, bool) {
	ql.mu.Lock()
	defer ql.mu.Unlock()

	if ql == nil || ql.count == 0 || ql.tail == nil {
		return nil, false
	}

	lastIdx := len(ql.tail.items) - 1
	val := ql.tail.items[lastIdx]
	ql.tail.items = ql.tail.items[:lastIdx]
	ql.count--

	if len(ql.tail.items) == 0 {
		ql.tail = ql.tail.prev
		if ql.tail != nil {
			ql.tail.next = nil
		} else {
			ql.head = nil
		}
		ql.nodeCount--
	}
	return val, true
}

// normalizeIndex adjusts positive and negative indices to 0-based bounds.
func (ql *Quicklist) normalizeIndex(idx int) int {
	if idx < 0 {
		idx = ql.count + idx
	}
	return idx
}

// LRange returns a slice of elements between start and stop (inclusive).
func (ql *Quicklist) LRange(start, stop int) [][]byte {
	if ql == nil {
		return nil
	}
	ql.mu.RLock()
	defer ql.mu.RUnlock()

	if ql.count == 0 {
		return nil
	}

	start = ql.normalizeIndex(start)
	stop = ql.normalizeIndex(stop)

	if start < 0 {
		start = 0
	}
	if stop >= ql.count {
		stop = ql.count - 1
	}
	if start > stop || start >= ql.count {
		return nil
	}

	result := make([][]byte, 0, stop-start+1)
	currIdx := 0
	currNode := ql.head

	for currNode != nil {
		nodeLen := len(currNode.items)
		nodeEnd := currIdx + nodeLen - 1

		if nodeEnd >= start {
			// Node intersects with requested range
			s := 0
			if start > currIdx {
				s = start - currIdx
			}
			e := nodeLen - 1
			if stop < nodeEnd {
				e = stop - currIdx
			}
			for i := s; i <= e; i++ {
				result = append(result, currNode.items[i])
			}
		}

		if nodeEnd >= stop {
			break
		}
		currIdx += nodeLen
		currNode = currNode.next
	}

	return result
}

// LIndex returns the element at the specified index.
func (ql *Quicklist) LIndex(idx int) ([]byte, bool) {
	if ql == nil {
		return nil, false
	}
	ql.mu.RLock()
	defer ql.mu.RUnlock()

	if ql.count == 0 {
		return nil, false
	}
	idx = ql.normalizeIndex(idx)
	if idx < 0 || idx >= ql.count {
		return nil, false
	}

	currIdx := 0
	currNode := ql.head
	for currNode != nil {
		nodeLen := len(currNode.items)
		if idx < currIdx+nodeLen {
			return currNode.items[idx-currIdx], true
		}
		currIdx += nodeLen
		currNode = currNode.next
	}
	return nil, false
}

// LSet updates the element at the specified index.
func (ql *Quicklist) LSet(idx int, val []byte) bool {
	if ql == nil {
		return false
	}
	ql.mu.Lock()
	defer ql.mu.Unlock()

	if ql.count == 0 {
		return false
	}
	idx = ql.normalizeIndex(idx)
	if idx < 0 || idx >= ql.count {
		return false
	}

	currIdx := 0
	currNode := ql.head
	for currNode != nil {
		nodeLen := len(currNode.items)
		if idx < currIdx+nodeLen {
			currNode.items[idx-currIdx] = bytes.Clone(val)
			return true
		}
		currIdx += nodeLen
		currNode = currNode.next
	}
	return false
}
