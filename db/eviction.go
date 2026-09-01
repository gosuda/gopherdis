package db

import (
	"errors"
	"math/rand"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gosuda/nedis/object"
)

// ErrOOM is returned when memory limit is exceeded under NoEviction policy.
var ErrOOM = errors.New("OOM command not allowed when used memory > 'maxmemory'")

// EvictionPolicy defines how keys are selected for deletion when maxmemory is reached.
type EvictionPolicy int

const (
	NoEviction EvictionPolicy = iota
	AllKeysLRU
	VolatileLRU
	AllKeysRandom
	VolatileRandom
	VolatileTTL
)

// ParseEvictionPolicy parses policy name string into EvictionPolicy enum.
func ParseEvictionPolicy(name string) EvictionPolicy {
	switch strings.ToLower(name) {
	case "allkeys-lru":
		return AllKeysLRU
	case "volatile-lru":
		return VolatileLRU
	case "allkeys-random":
		return AllKeysRandom
	case "volatile-random":
		return VolatileRandom
	case "volatile-ttl":
		return VolatileTTL
	default:
		return NoEviction
	}
}

// ActiveExpireCycle samples keys from all shards and deletes expired ones.
// Returns the total number of expired keys removed.
func (db *ShardedDB) ActiveExpireCycle() int {
	now := time.Now().UnixMilli()
	totalDeleted := 0

	for i := 0; i < NumShards; i++ {
		s := &db.shards[i]

		for iter := 0; iter < 16; iter++ {
			s.Lock()
			numExpires := len(s.expires)
			if numExpires == 0 {
				s.Unlock()
				break
			}

			sampleSize := 20
			if sampleSize > numExpires {
				sampleSize = numExpires
			}

			// Sample keys
			sampled := 0
			expired := 0
			for k, exp := range s.expires {
				sampled++
				if exp > 0 && now >= exp {
					if obj, exists := s.entries[k]; exists {
						db.subMem(estimateObjectSize(k, obj))
					}
					delete(s.entries, k)
					delete(s.expires, k)
					expired++
					totalDeleted++
				}
				if sampled >= sampleSize {
					break
				}
			}
			s.Unlock()

			// If less than 25% were expired, move to next shard
			if sampleSize == 0 || (expired*100)/sampleSize < 25 {
				break
			}
		}
	}
	return totalDeleted
}

// FreeMemoryIfNeeded evicts keys when used memory exceeds MaxMemory limit.
func (db *ShardedDB) FreeMemoryIfNeeded() error {
	maxMem := atomic.LoadInt64(&db.maxMemory)
	if maxMem <= 0 {
		return nil
	}

	used := db.UsedMemory()
	if used <= maxMem {
		return nil
	}

	policy := EvictionPolicy(atomic.LoadInt32(&db.evictionPolicy))
	if policy == NoEviction {
		return ErrOOM
	}

	const samples = 5

	for db.UsedMemory() > maxMem {
		bestKey := ""
		bestShardIdx := -1
		var bestScore int64 // For LRU: smallest timestamp; For TTL: smallest expire time

		// Search across multiple random shards
		for attempt := 0; attempt < NumShards*2; attempt++ {
			shardIdx := rand.Intn(NumShards)
			s := &db.shards[shardIdx]

			s.RLock()
			candidateMap := s.entries
			if policy == VolatileLRU || policy == VolatileRandom || policy == VolatileTTL {
				// Only consider keys with TTL
				if len(s.expires) == 0 {
					s.RUnlock()
					continue
				}
			}
			if len(candidateMap) == 0 {
				s.RUnlock()
				continue
			}

			sampled := 0
			for k, obj := range candidateMap {
				if policy == VolatileLRU || policy == VolatileTTL || policy == VolatileRandom {
					if _, hasExp := s.expires[k]; !hasExp {
						continue
					}
				}

				switch policy {
				case AllKeysRandom, VolatileRandom:
					bestKey = k
					bestShardIdx = shardIdx
					s.RUnlock()
					goto EVICT_FOUND

				case AllKeysLRU, VolatileLRU:
					lru := int64(atomic.LoadUint32(&obj.Lru))
					if bestKey == "" || lru < bestScore {
						bestScore = lru
						bestKey = k
						bestShardIdx = shardIdx
					}

				case VolatileTTL:
					exp := s.expires[k]
					if bestKey == "" || exp < bestScore {
						bestScore = exp
						bestKey = k
						bestShardIdx = shardIdx
					}
				}

				sampled++
				if sampled >= samples {
					break
				}
			}
			s.RUnlock()

			if bestKey != "" {
				break
			}
		}

	EVICT_FOUND:
		if bestKey == "" || bestShardIdx < 0 {
			// No evictable keys found
			return ErrOOM
		}

		// Evict selected key
		s := &db.shards[bestShardIdx]
		s.Lock()
		if obj, exists := s.entries[bestKey]; exists {
			db.subMem(estimateObjectSize(bestKey, obj))
			delete(s.entries, bestKey)
			delete(s.expires, bestKey)
		}
		s.Unlock()
	}

	return nil
}

// estimateObjectSize provides an approximate byte size of a key and its Robj.
func estimateObjectSize(key string, obj *object.Robj) int64 {
	size := int64(len(key)) + 64 // key length + struct overhead
	if obj == nil {
		return size
	}
	switch v := obj.Ptr.(type) {
	case []byte:
		size += int64(len(v))
	case string:
		size += int64(len(v))
	case int64:
		size += 8
	case map[string][]byte:
		for k, val := range v {
			size += int64(len(k) + len(val) + 32)
		}
	case map[string]struct{}:
		for k := range v {
			size += int64(len(k) + 16)
		}
	default:
		size += 64
	}
	return size
}
