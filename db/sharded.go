package db

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/gosuda/gopherdis/object"
)

// NumShards is the number of internal partitions for reducing lock contention.
const NumShards = 64

// shard represents a single locked partition of keys and their expiration times.
type shard struct {
	sync.RWMutex
	entries  map[string]*object.Robj
	expires  map[string]int64  // Key -> expiration timestamp in Unix milliseconds (0 = no expiry)
	versions map[string]uint64 // Key -> modification version counter for WATCH CAS
}

// ShardedDB is an in-memory key-value database partitioned into multiple shards.
type ShardedDB struct {
	shards         [NumShards]shard
	maxMemory      int64  // Maximum memory in bytes (0 = unlimited)
	evictionPolicy int32  // EvictionPolicy enum
	usedMemory     int64  // Approximate used memory in bytes
	watchers       int64  // Number of keys currently under WATCH (gates version tracking)
	cronStopCh     chan struct{}
	cronRunning    bool
	cronMu         sync.Mutex
	txMu           sync.RWMutex // Synchronizes transactions and single operations
}

// NewShardedDB creates and initializes a new ShardedDB instance.
func NewShardedDB() *ShardedDB {
	db := &ShardedDB{
		evictionPolicy: int32(NoEviction),
	}
	for i := 0; i < NumShards; i++ {
		db.shards[i].entries = make(map[string]*object.Robj)
		db.shards[i].expires = make(map[string]int64)
		db.shards[i].versions = make(map[string]uint64)
	}
	return db
}

// SetMaxMemory sets the maximum memory limit in bytes.
func (db *ShardedDB) SetMaxMemory(maxMem int64) {
	atomic.StoreInt64(&db.maxMemory, maxMem)
}

// MaxMemory returns the configured max memory limit.
func (db *ShardedDB) MaxMemory() int64 {
	return atomic.LoadInt64(&db.maxMemory)
}

// SetEvictionPolicy sets the eviction policy when maxmemory is exceeded.
func (db *ShardedDB) SetEvictionPolicy(policy EvictionPolicy) {
	atomic.StoreInt32(&db.evictionPolicy, int32(policy))
}

// UsedMemory returns the approximate memory bytes used by stored keys and values.
func (db *ShardedDB) UsedMemory() int64 {
	return atomic.LoadInt64(&db.usedMemory)
}

func (db *ShardedDB) addMem(delta int64) {
	atomic.AddInt64(&db.usedMemory, delta)
}

func (db *ShardedDB) subMem(delta int64) {
	atomic.AddInt64(&db.usedMemory, -delta)
}

// StartCron launches a background goroutine for active expiration and eviction.
func (db *ShardedDB) StartCron(interval time.Duration) {
	db.cronMu.Lock()
	defer db.cronMu.Unlock()

	if db.cronRunning {
		return
	}
	db.cronStopCh = make(chan struct{})
	db.cronRunning = true

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				db.ActiveExpireCycle()
				_ = db.FreeMemoryIfNeeded()
			case <-db.cronStopCh:
				return
			}
		}
	}()
}

// StopCron terminates the background cron goroutine.
func (db *ShardedDB) StopCron() {
	db.cronMu.Lock()
	defer db.cronMu.Unlock()

	if !db.cronRunning {
		return
	}
	close(db.cronStopCh)
	db.cronRunning = false
}

// fnv32 hashes a string key to a 32-bit integer.
func fnv32(key string) uint32 {
	var hash uint32 = 2166136261
	for i := 0; i < len(key); i++ {
		hash ^= uint32(key[i])
		hash *= 16777619
	}
	return hash
}

// getShard returns the shard responsible for the given key.
func (db *ShardedDB) getShard(key string) *shard {
	idx := fnv32(key) % NumShards
	return &db.shards[idx]
}

// isExpired checks if a key has expired. Caller must hold at least read lock.
func (s *shard) isExpired(key string, now int64) bool {
	exp, ok := s.expires[key]
	if !ok || exp == 0 {
		return false
	}
	return now >= exp
}

// Get retrieves an object by key. Returns nil, false if key does not exist or has expired.
func (db *ShardedDB) Get(key string) (*object.Robj, bool) {
	s := db.getShard(key)
	now := time.Now().UnixMilli()

	s.RLock()
	if s.isExpired(key, now) {
		s.RUnlock()
		// Lazy deletion of expired key
		s.Lock()
		if s.isExpired(key, now) {
			if oldObj, exists := s.entries[key]; exists {
				db.subMem(estimateObjectSize(key, oldObj))
			}
			delete(s.entries, key)
			delete(s.expires, key)
		}
		s.Unlock()
		return nil, false
	}
	val, ok := s.entries[key]
	if ok && val != nil {
		// Update LRU clock
		atomic.StoreUint32(&val.Lru, uint32(time.Now().Unix()))
	}
	s.RUnlock()
	return val, ok
}

// Set stores a key-value pair without expiration.
func (db *ShardedDB) Set(key string, val *object.Robj) error {
	if val != nil {
		atomic.StoreUint32(&val.Lru, uint32(time.Now().Unix()))
	}
	newSize := estimateObjectSize(key, val)
	maxMem := atomic.LoadInt64(&db.maxMemory)
	if maxMem > 0 && db.UsedMemory()+newSize > maxMem {
		if err := db.FreeMemoryIfNeeded(); err != nil {
			return err
		}
		if db.UsedMemory()+newSize > maxMem && EvictionPolicy(atomic.LoadInt32(&db.evictionPolicy)) == NoEviction {
			return ErrOOM
		}
	}

	s := db.getShard(key)
	s.Lock()
	defer s.Unlock()

	if old, exists := s.entries[key]; exists {
		db.subMem(estimateObjectSize(key, old))
	}
	s.entries[key] = val
	delete(s.expires, key)
	s.bumpVersion(db, key)
	db.addMem(newSize)
	return nil
}

// SetWithExpire stores a key-value pair with a specific TTL duration.
func (db *ShardedDB) SetWithExpire(key string, val *object.Robj, ttl time.Duration) error {
	if val != nil {
		atomic.StoreUint32(&val.Lru, uint32(time.Now().Unix()))
	}
	newSize := estimateObjectSize(key, val)
	maxMem := atomic.LoadInt64(&db.maxMemory)
	if maxMem > 0 && db.UsedMemory()+newSize > maxMem {
		if err := db.FreeMemoryIfNeeded(); err != nil {
			return err
		}
		if db.UsedMemory()+newSize > maxMem && EvictionPolicy(atomic.LoadInt32(&db.evictionPolicy)) == NoEviction {
			return ErrOOM
		}
	}

	s := db.getShard(key)
	exp := time.Now().Add(ttl).UnixMilli()

	s.Lock()
	defer s.Unlock()

	if old, exists := s.entries[key]; exists {
		db.subMem(estimateObjectSize(key, old))
	}
	s.entries[key] = val
	s.expires[key] = exp
	s.bumpVersion(db, key)
	db.addMem(newSize)
	return nil
}


// SetExpire sets or updates the TTL for an existing key.
func (db *ShardedDB) SetExpire(key string, ttl time.Duration) bool {
	return db.SetExpireAt(key, time.Now().UnixMilli()+ttl.Milliseconds())
}

// SetExpireAt sets or updates the absolute expiration timestamp (Unix milliseconds) for an existing key.
func (db *ShardedDB) SetExpireAt(key string, exp int64) bool {
	s := db.getShard(key)
	now := time.Now().UnixMilli()

	s.Lock()
	defer s.Unlock()

	if s.isExpired(key, now) {
		if old, exists := s.entries[key]; exists {
			db.subMem(estimateObjectSize(key, old))
		}
		delete(s.entries, key)
		delete(s.expires, key)
		s.bumpVersion(db, key)
		return false
	}
	if _, ok := s.entries[key]; !ok {
		return false
	}
	s.expires[key] = exp
	s.bumpVersion(db, key)
	return true
}

// Del removes one or more keys from the database. Returns true if the key existed.
func (db *ShardedDB) Del(key string) bool {
	s := db.getShard(key)
	now := time.Now().UnixMilli()

	s.Lock()
	defer s.Unlock()

	if s.isExpired(key, now) {
		if old, exists := s.entries[key]; exists {
			db.subMem(estimateObjectSize(key, old))
		}
		delete(s.entries, key)
		delete(s.expires, key)
		s.bumpVersion(db, key)
		return false
	}
	old, ok := s.entries[key]
	if ok {
		db.subMem(estimateObjectSize(key, old))
		delete(s.entries, key)
		delete(s.expires, key)
		s.bumpVersion(db, key)
	}
	return ok
}

// Exists checks if a key exists and is not expired.
func (db *ShardedDB) Exists(key string) bool {
	s := db.getShard(key)
	now := time.Now().UnixMilli()

	s.RLock()
	defer s.RUnlock()

	if s.isExpired(key, now) {
		return false
	}
	_, ok := s.entries[key]
	return ok
}

// TTL returns the remaining time-to-live of a key.
// Returns -2 if key does not exist / expired, -1 if key has no expiration, or remaining duration.
func (db *ShardedDB) TTL(key string) (time.Duration, int) {
	s := db.getShard(key)
	now := time.Now().UnixMilli()

	s.RLock()
	defer s.RUnlock()

	if s.isExpired(key, now) {
		return 0, -2
	}
	if _, ok := s.entries[key]; !ok {
		return 0, -2
	}
	exp, hasExp := s.expires[key]
	if !hasExp || exp == 0 {
		return 0, -1
	}
	remMs := exp - now
	if remMs < 0 {
		return 0, -2
	}
	return time.Duration(remMs) * time.Millisecond, 0
}

// AddWatchers registers n watched keys, enabling version tracking while > 0.
func (db *ShardedDB) AddWatchers(n int64) {
	atomic.AddInt64(&db.watchers, n)
}

// RemoveWatchers unregisters n watched keys.
func (db *ShardedDB) RemoveWatchers(n int64) {
	atomic.AddInt64(&db.watchers, -n)
}

// bumpVersion increments the key's modification version, but only while any
// client has keys under WATCH. Skipping the write when nobody watches avoids
// allocating a versions-map entry for every key in the common case.
// Caller must hold the shard write lock.
func (s *shard) bumpVersion(db *ShardedDB, key string) {
	if atomic.LoadInt64(&db.watchers) > 0 {
		s.versions[key]++
	}
}

// GetVersion returns the modification version counter of a key.
func (db *ShardedDB) GetVersion(key string) uint64 {
	s := db.getShard(key)
	s.RLock()
	defer s.RUnlock()
	return s.versions[key]
}

// GetShardIdx returns the shard index (0 to NumShards-1) for a key.
func (db *ShardedDB) GetShardIdx(key string) int {
	return int(fnv32(key) % NumShards)
}

// LockShards locks shards in strict ascending index order (0 to NumShards-1) to avoid deadlocks.
func (db *ShardedDB) LockShards(indices []int) {
	for _, idx := range indices {
		if idx >= 0 && idx < NumShards {
			db.shards[idx].Lock()
		}
	}
}

// UnlockShards unlocks shards in reverse (descending) order.
func (db *ShardedDB) UnlockShards(indices []int) {
	for i := len(indices) - 1; i >= 0; i-- {
		idx := indices[i]
		if idx >= 0 && idx < NumShards {
			db.shards[idx].Unlock()
		}
	}
}

// BeginTx acquires exclusive lock for transaction execution.
func (db *ShardedDB) BeginTx() {
	db.txMu.Lock()
}

// EndTx releases exclusive transaction lock.
func (db *ShardedDB) EndTx() {
	db.txMu.Unlock()
}

// BeginOp acquires shared lock for single operations.
func (db *ShardedDB) BeginOp() {
	db.txMu.RLock()
}

// EndOp releases shared lock for single operations.
func (db *ShardedDB) EndOp() {
	db.txMu.RUnlock()
}

// FlushAll removes all keys from all shards.
func (db *ShardedDB) FlushAll() {
	for i := 0; i < NumShards; i++ {
		s := &db.shards[i]
		s.Lock()
		s.entries = make(map[string]*object.Robj)
		s.expires = make(map[string]int64)
		s.versions = make(map[string]uint64)
		s.Unlock()
	}
	atomic.StoreInt64(&db.usedMemory, 0)
}

// Len returns the total number of non-expired keys across all shards.
func (db *ShardedDB) Len() int64 {
	var count int64
	for i := 0; i < NumShards; i++ {
		db.shards[i].RLock()
		count += int64(len(db.shards[i].entries))
		db.shards[i].RUnlock()
	}
	return count
}

