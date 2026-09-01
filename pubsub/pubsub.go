package pubsub

import (
	"fmt"
	"sync"
	"sync/atomic"
)

const (
	NumPubSubShards = 64
	DefaultMsgQueueSize = 512
)

// Subscriber represents a client connection subscribed to channels or patterns.
type Subscriber struct {
	ID       uint64
	MsgCh    chan []byte
	closed   int32
	mu       sync.Mutex
	channels map[string]struct{}
	patterns map[string]struct{}
}

// NewSubscriber creates a new subscriber session with an ID.
func NewSubscriber(id uint64) *Subscriber {
	return &Subscriber{
		ID:       id,
		MsgCh:    make(chan []byte, DefaultMsgQueueSize),
		channels: make(map[string]struct{}),
		patterns: make(map[string]struct{}),
	}
}

// SubCount returns the total number of channels and patterns this subscriber is listening to.
func (s *Subscriber) SubCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.channels) + len(s.patterns)
}

// HasSubscriptions returns true if subscriber has at least one active subscription.
func (s *Subscriber) HasSubscriptions() bool {
	return s.SubCount() > 0
}

// TrySend attempts a non-blocking push to the subscriber's outbound message queue.
func (s *Subscriber) TrySend(msg []byte) bool {
	if atomic.LoadInt32(&s.closed) == 1 {
		return false
	}
	select {
	case s.MsgCh <- msg:
		return true
	default:
		// Queue full (slow consumer) - drop message to protect publisher
		return false
	}
}

// Close marks the subscriber closed and drains the message channel.
func (s *Subscriber) Close() {
	if atomic.CompareAndSwapInt32(&s.closed, 0, 1) {
		s.mu.Lock()
		s.channels = make(map[string]struct{})
		s.patterns = make(map[string]struct{})
		s.mu.Unlock()
		close(s.MsgCh)
	}
}

// PatternEntry represents a pattern subscription pair.
type PatternEntry struct {
	Pattern string
	Sub     *Subscriber
}

// channelShard isolates lock contention for a partition of channels.
type channelShard struct {
	sync.RWMutex
	channels map[string]map[uint64]*Subscriber
}

// ShardedHub manages lock-sharded channel routing and lock-free COW pattern routing.
type ShardedHub struct {
	shards     [NumPubSubShards]channelShard
	patterns   atomic.Pointer[[]PatternEntry]
	patMu      sync.Mutex
	nextSubID  uint64
}

// NewShardedHub initializes a new high-throughput ShardedHub.
func NewShardedHub() *ShardedHub {
	hub := &ShardedHub{}
	for i := 0; i < NumPubSubShards; i++ {
		hub.shards[i].channels = make(map[string]map[uint64]*Subscriber)
	}
	emptyPatterns := make([]PatternEntry, 0)
	hub.patterns.Store(&emptyPatterns)
	return hub
}

// NextSubscriberID generates a unique 64-bit ID for a new subscriber.
func (h *ShardedHub) NextSubscriberID() uint64 {
	return atomic.AddUint64(&h.nextSubID, 1)
}

func fnv32(key string) uint32 {
	var hash uint32 = 2166136261
	for i := 0; i < len(key); i++ {
		hash ^= uint32(key[i])
		hash *= 16777619
	}
	return hash
}

func (h *ShardedHub) getShard(channel string) *channelShard {
	idx := fnv32(channel) % NumPubSubShards
	return &h.shards[idx]
}

// Subscribe registers a subscriber to an exact channel. Returns the subscriber's new total subscription count.
func (h *ShardedHub) Subscribe(sub *Subscriber, channel string) int {
	shard := h.getShard(channel)

	shard.Lock()
	subs, exists := shard.channels[channel]
	if !exists {
		subs = make(map[uint64]*Subscriber)
		shard.channels[channel] = subs
	}
	subs[sub.ID] = sub
	shard.Unlock()

	sub.mu.Lock()
	sub.channels[channel] = struct{}{}
	count := len(sub.channels) + len(sub.patterns)
	sub.mu.Unlock()

	return count
}

// Unsubscribe unregisters a subscriber from a channel. Returns the subscriber's new total subscription count.
func (h *ShardedHub) Unsubscribe(sub *Subscriber, channel string) int {
	shard := h.getShard(channel)

	shard.Lock()
	if subs, exists := shard.channels[channel]; exists {
		delete(subs, sub.ID)
		if len(subs) == 0 {
			delete(shard.channels, channel)
		}
	}
	shard.Unlock()

	sub.mu.Lock()
	delete(sub.channels, channel)
	count := len(sub.channels) + len(sub.patterns)
	sub.mu.Unlock()

	return count
}

// PSubscribe registers a subscriber to a glob pattern using atomic Copy-On-Write.
func (h *ShardedHub) PSubscribe(sub *Subscriber, pattern string) int {
	h.patMu.Lock()
	defer h.patMu.Unlock()

	oldList := *h.patterns.Load()
	newList := make([]PatternEntry, len(oldList), len(oldList)+1)
	copy(newList, oldList)

	// Avoid duplicate pattern subscription for same subscriber
	alreadyExists := false
	for _, pe := range newList {
		if pe.Pattern == pattern && pe.Sub.ID == sub.ID {
			alreadyExists = true
			break
		}
	}
	if !alreadyExists {
		newList = append(newList, PatternEntry{Pattern: pattern, Sub: sub})
		h.patterns.Store(&newList)
	}

	sub.mu.Lock()
	sub.patterns[pattern] = struct{}{}
	count := len(sub.channels) + len(sub.patterns)
	sub.mu.Unlock()

	return count
}

// PUnsubscribe unregisters a subscriber from a glob pattern using atomic Copy-On-Write.
func (h *ShardedHub) PUnsubscribe(sub *Subscriber, pattern string) int {
	h.patMu.Lock()
	defer h.patMu.Unlock()

	oldList := *h.patterns.Load()
	newList := make([]PatternEntry, 0, len(oldList))
	for _, pe := range oldList {
		if pe.Pattern == pattern && pe.Sub.ID == sub.ID {
			continue
		}
		newList = append(newList, pe)
	}
	h.patterns.Store(&newList)

	sub.mu.Lock()
	delete(sub.patterns, pattern)
	count := len(sub.channels) + len(sub.patterns)
	sub.mu.Unlock()

	return count
}

// UnsubscribeAll unregisters a subscriber from all its channels and patterns.
func (h *ShardedHub) UnsubscribeAll(sub *Subscriber) {
	sub.mu.Lock()
	chs := make([]string, 0, len(sub.channels))
	for ch := range sub.channels {
		chs = append(chs, ch)
	}
	pats := make([]string, 0, len(sub.patterns))
	for pat := range sub.patterns {
		pats = append(pats, pat)
	}
	sub.mu.Unlock()

	for _, ch := range chs {
		h.Unsubscribe(sub, ch)
	}
	for _, pat := range pats {
		h.PUnsubscribe(sub, pat)
	}
}

// Publish delivers a message to all exact channel and pattern subscribers. Returns total receivers count.
func (h *ShardedHub) Publish(channel string, message []byte) int {
	receivers := 0
	msgPayload := formatMessage(channel, message)

	// 1. Sharded Exact Channel Matching (Shard RLock)
	shard := h.getShard(channel)
	shard.RLock()
	if subs, exists := shard.channels[channel]; exists && len(subs) > 0 {
		for _, sub := range subs {
			if sub.TrySend(msgPayload) {
				receivers++
			}
		}
	}
	shard.RUnlock()

	// 2. Lock-free Atomic Pattern Matching (0 Locks)
	patternsPtr := h.patterns.Load()
	if patternsPtr != nil {
		patternList := *patternsPtr
		for _, pe := range patternList {
			if matchPattern(pe.Pattern, channel) {
				pmsgPayload := formatPMessage(pe.Pattern, channel, message)
				if pe.Sub.TrySend(pmsgPayload) {
					receivers++
				}
			}
		}
	}

	return receivers
}

// PubSubChannels lists active channels optionally matching a glob pattern.
func (h *ShardedHub) PubSubChannels(pattern string) []string {
	var result []string
	for i := 0; i < NumPubSubShards; i++ {
		shard := &h.shards[i]
		shard.RLock()
		for ch := range shard.channels {
			if pattern == "" || matchPattern(pattern, ch) {
				result = append(result, ch)
			}
		}
		shard.RUnlock()
	}
	return result
}

// PubSubNumSub returns subscriber counts for specified channels.
func (h *ShardedHub) PubSubNumSub(channels []string) map[string]int {
	result := make(map[string]int, len(channels))
	for _, ch := range channels {
		shard := h.getShard(ch)
		shard.RLock()
		count := len(shard.channels[ch])
		shard.RUnlock()
		result[ch] = count
	}
	return result
}

// PubSubNumPat returns the total count of pattern subscriptions.
func (h *ShardedHub) PubSubNumPat() int {
	patternsPtr := h.patterns.Load()
	if patternsPtr == nil {
		return 0
	}
	return len(*patternsPtr)
}

func formatMessage(channel string, message []byte) []byte {
	return []byte(fmt.Sprintf("*3\r\n$7\r\nmessage\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n",
		len(channel), channel, len(message), message))
}

func formatPMessage(pattern, channel string, message []byte) []byte {
	return []byte(fmt.Sprintf("*4\r\n$8\r\npmessage\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n",
		len(pattern), pattern, len(channel), channel, len(message), message))
}

func FormatSubscribeReply(action, name string, count int) []byte {
	return []byte(fmt.Sprintf("*3\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n:%d\r\n",
		len(action), action, len(name), name, count))
}
