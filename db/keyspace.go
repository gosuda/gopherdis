package db

import (
	"math/rand"
	"time"

	"github.com/gosuda/gopherdis/glob"
	"github.com/gosuda/gopherdis/object"
)

// Keys returns every live key matching the glob pattern.
//
// Like Redis' KEYS this walks the whole keyspace; shards are snapshotted one at
// a time so a large keyspace does not hold every lock at once.
func (db *ShardedDB) Keys(pattern string) []string {
	now := time.Now().UnixMilli()
	matchAll := pattern == "*"
	var out []string

	for i := 0; i < NumShards; i++ {
		s := &db.shards[i]
		s.RLock()
		for k := range s.entries {
			if s.isExpired(k, now) {
				continue
			}
			if matchAll || glob.Match(pattern, k) {
				out = append(out, k)
			}
		}
		s.RUnlock()
	}
	return out
}

// Scan implements the SCAN cursor over the keyspace.
//
// The cursor is a shard index: one call returns the live keys of a single
// shard, snapshotted under its read lock, and the next cursor points at the
// following shard. A key's shard is a pure function of the key, so a key that
// exists for the whole scan cannot be missed, which is the guarantee SCAN
// makes. count is advisory and bounds how many shards one call will visit.
func (db *ShardedDB) Scan(cursor uint64, pattern string, count int) (uint64, []string) {
	if count <= 0 {
		count = 10
	}
	now := time.Now().UnixMilli()
	matchAll := pattern == "" || pattern == "*"

	var out []string
	idx := cursor
	for idx < uint64(NumShards) {
		s := &db.shards[idx]
		s.RLock()
		for k := range s.entries {
			if s.isExpired(k, now) {
				continue
			}
			if matchAll || glob.Match(pattern, k) {
				out = append(out, k)
			}
		}
		s.RUnlock()
		idx++

		// Stop once this call has produced enough, but always finish the shard
		// it is on so the cursor stays on a shard boundary.
		if len(out) >= count {
			break
		}
	}
	if idx >= uint64(NumShards) {
		return 0, out
	}
	return idx, out
}

// RandomKey returns a random live key, or "" when the keyspace is empty.
func (db *ShardedDB) RandomKey() string {
	now := time.Now().UnixMilli()
	start := rand.Intn(NumShards)
	for n := 0; n < NumShards; n++ {
		s := &db.shards[(start+n)%NumShards]
		s.RLock()
		for k := range s.entries {
			if !s.isExpired(k, now) {
				s.RUnlock()
				return k
			}
		}
		s.RUnlock()
	}
	return ""
}

// GetWithTTL returns the object together with its absolute expiry in unix
// milliseconds (0 when the key has no TTL), so a copy can carry the TTL over.
func (db *ShardedDB) GetWithTTL(key string) (*object.Robj, int64, bool) {
	s := db.getShard(key)
	now := time.Now().UnixMilli()

	s.RLock()
	defer s.RUnlock()

	if s.isExpired(key, now) {
		return nil, 0, false
	}
	val, ok := s.entries[key]
	if !ok {
		return nil, 0, false
	}
	return val, s.expires[key], true
}

// Persist removes a key's TTL. Returns true when a TTL was actually cleared.
func (db *ShardedDB) Persist(key string) bool {
	s := db.getShard(key)
	now := time.Now().UnixMilli()

	s.Lock()
	defer s.Unlock()

	if s.isExpired(key, now) {
		return false
	}
	if _, ok := s.entries[key]; !ok {
		return false
	}
	if exp, has := s.expires[key]; !has || exp == 0 {
		return false
	}
	delete(s.expires, key)
	s.bumpVersion(db, key)
	return true
}
