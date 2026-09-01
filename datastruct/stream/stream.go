package stream

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrGroupNotFound  = errors.New("NOGROUP No such key or consumer group")
	ErrGroupExists    = errors.New("BUSYGROUP Consumer Group name already exists")
)

// WaitClient represents a blocked XREAD / XREADGROUP client connection.
type WaitClient struct {
	ID     uint64
	WakeCh chan struct{}
}

// Stream is a memory-efficient and high-throughput log-structured data structure.
type Stream struct {
	mu         sync.Mutex
	head       *StreamChunk
	tail       *StreamChunk
	length     int64
	firstID    StreamID
	lastID     StreamID
	fields     []string
	fieldMap   map[string]uint16
	cgroups    map[string]*ConsumerGroup
	waiters    map[uint64]*WaitClient
	nextWaitID uint64
}

// NewStream creates an empty Stream instance.
func NewStream() *Stream {
	return &Stream{
		fieldMap: make(map[string]uint16),
		cgroups:  make(map[string]*ConsumerGroup),
		waiters:  make(map[uint64]*WaitClient),
	}
}

// Len returns the number of active entries in the stream.
func (s *Stream) Len() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.length
}

// LastID returns the highest ID in the stream.
func (s *Stream) LastID() StreamID {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastID
}

// FirstID returns the lowest ID in the stream.
func (s *Stream) FirstID() StreamID {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.firstID
}

func (s *Stream) internField(name string) uint16 {
	if id, exists := s.fieldMap[name]; exists {
		return id
	}
	id := uint16(len(s.fields))
	s.fields = append(s.fields, name)
	s.fieldMap[name] = id
	return id
}

// Add appends a new message to the stream.
func (s *Stream) Add(id StreamID, fieldNames []string, fieldVals [][]byte, maxLen int64, approx bool) (StreamID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Verify monotonic ID increment
	if s.length > 0 && id.Compare(s.lastID) <= 0 {
		return ZeroID, ErrInvalidStreamID
	}

	// 2. Intern field names
	var fIDsBuf [16]uint16
	numFields := len(fieldNames)
	var fieldIDs []uint16
	if numFields <= 16 {
		fieldIDs = fIDsBuf[:numFields]
	} else {
		fieldIDs = make([]uint16, numFields)
	}

	for i, fn := range fieldNames {
		fieldIDs[i] = s.internField(fn)
	}

	// 3. Append to tail chunk
	if s.tail == nil {
		chunk := NewChunk()
		s.head = chunk
		s.tail = chunk
		s.firstID = id
	}

	ok, err := s.tail.Append(id, fieldIDs, fieldVals)
	if err != nil {
		return ZeroID, err
	}
	if !ok {
		// Tail chunk is full or exhausted, allocate new chunk
		newChunk := NewChunk()
		s.tail.next = newChunk
		newChunk.prev = s.tail
		s.tail = newChunk
		_, _ = s.tail.Append(id, fieldIDs, fieldVals)
	}

	s.lastID = id
	s.length++

	// 4. Optional MaxLen Trim
	if maxLen > 0 && s.length > maxLen {
		s.trimInternal(maxLen, approx)
	}

	// 5. Notify blocked XREAD clients
	if len(s.waiters) > 0 {
		s.notifyWaitersInternal()
	}

	return id, nil
}

// AddRaw appends a new message directly from raw [name1, val1, name2, val2, ...] byte slices.
func (s *Stream) AddRaw(id StreamID, pairs [][]byte, maxLen int64, approx bool) (StreamID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Verify monotonic ID increment
	if s.length > 0 && id.Compare(s.lastID) <= 0 {
		return ZeroID, ErrInvalidStreamID
	}

	numFields := len(pairs) / 2
	var fIDsBuf [16]uint16
	var fValsBuf [16][]byte
	var fieldIDs []uint16
	var fieldVals [][]byte
	if numFields <= 16 {
		fieldIDs = fIDsBuf[:numFields]
		fieldVals = fValsBuf[:numFields]
	} else {
		fieldIDs = make([]uint16, numFields)
		fieldVals = make([][]byte, numFields)
	}

	for i := 0; i < numFields; i++ {
		fieldIDs[i] = s.internField(string(pairs[i*2]))
		fieldVals[i] = pairs[i*2+1]
	}

	// 3. Append to tail chunk
	if s.tail == nil {
		chunk := NewChunk()
		s.head = chunk
		s.tail = chunk
		s.firstID = id
	}

	ok, err := s.tail.Append(id, fieldIDs, fieldVals)
	if err != nil {
		return ZeroID, err
	}
	if !ok {
		newChunk := NewChunk()
		s.tail.next = newChunk
		newChunk.prev = s.tail
		s.tail = newChunk
		_, _ = s.tail.Append(id, fieldIDs, fieldVals)
	}

	s.lastID = id
	s.length++

	if maxLen > 0 && s.length > maxLen {
		s.trimInternal(maxLen, approx)
	}

	if len(s.waiters) > 0 {
		s.notifyWaitersInternal()
	}

	return id, nil
}

func (s *Stream) trimInternal(maxLen int64, approx bool) int64 {
	var deleted int64 = 0

	for s.length > maxLen && s.head != nil {
		activeInHead := int64(s.head.NumActive())
		if approx && (s.length-activeInHead) >= maxLen {
			// Fast Drop entire head chunk (O(1) segment pruning)
			deleted += activeInHead
			s.length -= activeInHead
			nextHead := s.head.next
			s.head.Release()
			s.head = nextHead
			if s.head != nil {
				s.head.prev = nil
			} else {
				s.tail = nil
			}
		} else {
			// Mark oldest entries deleted
			for i := range s.head.entries {
				if !s.head.entries[i].Deleted {
					s.head.entries[i].Deleted = true
					s.head.deletedCount++
					deleted++
					s.length--
					if s.length <= maxLen {
						break
					}
				}
			}
			if s.head.NumActive() == 0 {
				nextHead := s.head.next
				s.head.Release()
				s.head = nextHead
				if s.head != nil {
					s.head.prev = nil
				} else {
					s.tail = nil
				}
			}
		}
	}

	return deleted
}

// Trim removes old messages exceeding maxLen.
func (s *Stream) Trim(maxLen int64, approx bool) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.trimInternal(maxLen, approx)
}

// Range queries a range of entries between start and end.
func (s *Stream) Range(start, end StreamID, count int, reverse bool) []StreamEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	cap := 16
	if count > 0 && count < 1024 {
		cap = count
	}
	result := make([]StreamEntry, 0, cap)

	if !reverse {
		for chunk := s.head; chunk != nil; chunk = chunk.next {
			if len(chunk.entries) > 0 && chunk.entries[len(chunk.entries)-1].ID.Compare(start) < 0 {
				continue
			}
			for _, rec := range chunk.entries {
				if rec.Deleted {
					continue
				}
				if rec.ID.Compare(start) < 0 {
					continue
				}
				if rec.ID.Compare(end) > 0 {
					return result
				}

				entry := s.unpackEntry(rec)
				result = append(result, entry)
				if count > 0 && len(result) >= count {
					return result
				}
			}
		}
	} else {
		for chunk := s.tail; chunk != nil; chunk = chunk.prev {
			if len(chunk.entries) > 0 && chunk.entries[0].ID.Compare(start) > 0 {
				continue
			}
			for i := len(chunk.entries) - 1; i >= 0; i-- {
				rec := chunk.entries[i]
				if rec.Deleted {
					continue
				}
				if rec.ID.Compare(start) > 0 {
					continue
				}
				if rec.ID.Compare(end) < 0 {
					return result
				}

				entry := s.unpackEntry(rec)
				result = append(result, entry)
				if count > 0 && len(result) >= count {
					return result
				}
			}
		}
	}

	return result
}

func (s *Stream) unpackEntry(rec EntryRecord) StreamEntry {
	numFields := len(rec.FieldIDs)
	fieldNames := make([]string, numFields)
	fieldVals := make([][]byte, numFields)

	for i, fID := range rec.FieldIDs {
		if int(fID) < len(s.fields) {
			fieldNames[i] = s.fields[fID]
		}
		fieldVals[i] = rec.Values[i]
	}

	return StreamEntry{
		ID:     rec.ID,
		Fields: fieldNames,
		Values: fieldVals,
	}
}

// Delete removes entries by ID.
func (s *Stream) Delete(ids []StreamID) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	idMap := make(map[StreamID]struct{}, len(ids))
	for _, id := range ids {
		idMap[id] = struct{}{}
	}

	var deleted int64 = 0
	for chunk := s.head; chunk != nil; chunk = chunk.next {
		for i := range chunk.entries {
			if !chunk.entries[i].Deleted {
				if _, ok := idMap[chunk.entries[i].ID]; ok {
					chunk.entries[i].Deleted = true
					chunk.deletedCount++
					deleted++
					s.length--
				}
			}
		}
	}

	return deleted
}

// CreateGroup creates a new consumer group starting from startID.
func (s *Stream) CreateGroup(name string, startID StreamID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.cgroups[name]; exists {
		return ErrGroupExists
	}

	s.cgroups[name] = NewConsumerGroup(name, startID)
	return nil
}

// DestroyGroup deletes a consumer group.
func (s *Stream) DestroyGroup(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.cgroups[name]; exists {
		delete(s.cgroups, name)
		return true
	}
	return false
}

// ReadGroup reads entries for a consumer within a group.
func (s *Stream) ReadGroup(groupName, consumerName string, startID StreamID, isNew bool, count int, noAck bool) ([]StreamEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cg, exists := s.cgroups[groupName]
	if !exists {
		return nil, ErrGroupNotFound
	}

	var result []StreamEntry

	if isNew {
		// Read new undelivered messages strictly > cg.LastDeliveredID
		for chunk := s.head; chunk != nil; chunk = chunk.next {
			for _, rec := range chunk.entries {
				if rec.Deleted {
					continue
				}
				if rec.ID.Compare(cg.LastDeliveredID) <= 0 {
					continue
				}

				entry := s.unpackEntry(rec)
				result = append(result, entry)

				if !noAck {
					cg.AddPending(consumerName, rec.ID)
				}
				cg.LastDeliveredID = rec.ID

				if count > 0 && len(result) >= count {
					return result, nil
				}
			}
		}
	} else {
		// Read historical PEL messages for consumer > startID
		consumer := cg.GetOrCreateConsumer(consumerName)
		for chunk := s.head; chunk != nil; chunk = chunk.next {
			for _, rec := range chunk.entries {
				if rec.Deleted {
					continue
				}
				if rec.ID.Compare(startID) <= 0 {
					continue
				}
				if _, inPel := consumer.Pel[rec.ID]; inPel {
					entry := s.unpackEntry(rec)
					result = append(result, entry)
					if count > 0 && len(result) >= count {
						return result, nil
					}
				}
			}
		}
	}

	return result, nil
}

// Ack confirms processing of messages in a consumer group.
func (s *Stream) Ack(groupName string, ids []StreamID) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	cg, exists := s.cgroups[groupName]
	if !exists {
		return 0
	}

	var acked int64 = 0
	for _, id := range ids {
		if cg.Ack(id) {
			acked++
		}
	}

	return acked
}

// Pending returns summary of pending entries for XPENDING.
func (s *Stream) Pending(groupName string, start, end StreamID, count int, consumerFilter string) []PendingReportEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	cg, exists := s.cgroups[groupName]
	if !exists {
		return nil
	}

	now := time.Now().UnixMilli()
	var result []PendingReportEntry

	for chunk := s.head; chunk != nil; chunk = chunk.next {
		for _, rec := range chunk.entries {
			if rec.Deleted {
				continue
			}
			if rec.ID.Compare(start) < 0 {
				continue
			}
			if rec.ID.Compare(end) > 0 {
				return result
			}

			if nack, inPel := cg.Pel[rec.ID]; inPel {
				if consumerFilter == "" || nack.ConsumerName == consumerFilter {
					result = append(result, PendingReportEntry{
						ID:            rec.ID,
						ConsumerName:  nack.ConsumerName,
						IdleTimeMs:    now - nack.DeliveryTime,
						DeliveryCount: nack.DeliveryCount,
					})
					if count > 0 && len(result) >= count {
						return result
					}
				}
			}
		}
	}

	return result
}

// Claim transfers ownership of pending messages to a new consumer.
func (s *Stream) Claim(groupName, consumerName string, minIdleMs int64, ids []StreamID) []StreamEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	cg, exists := s.cgroups[groupName]
	if !exists {
		return nil
	}

	now := time.Now().UnixMilli()
	claimedIDs := make(map[StreamID]struct{})

	for _, id := range ids {
		nack, inPel := cg.Pel[id]
		if inPel && (now-nack.DeliveryTime >= minIdleMs) {
			cg.AddPending(consumerName, id)
			claimedIDs[id] = struct{}{}
		}
	}

	var result []StreamEntry
	for chunk := s.head; chunk != nil; chunk = chunk.next {
		for _, rec := range chunk.entries {
			if rec.Deleted {
				continue
			}
			if _, claimed := claimedIDs[rec.ID]; claimed {
				result = append(result, s.unpackEntry(rec))
			}
		}
	}

	return result
}

// RegisterWaiter adds a blocking client to the wait list.
func (s *Stream) RegisterWaiter() *WaitClient {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := atomic.AddUint64(&s.nextWaitID, 1)
	wc := &WaitClient{
		ID:     id,
		WakeCh: make(chan struct{}, 1),
	}
	s.waiters[id] = wc
	return wc
}

// UnregisterWaiter removes a blocking client.
func (s *Stream) UnregisterWaiter(wc *WaitClient) {
	if wc == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.waiters, wc.ID)
}

func (s *Stream) notifyWaitersInternal() {
	for _, wc := range s.waiters {
		select {
		case wc.WakeCh <- struct{}{}:
		default:
		}
	}
}
