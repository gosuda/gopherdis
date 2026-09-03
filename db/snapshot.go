package db

import (
	"time"

	"github.com/gosuda/gopherdis/object"
)

// DBEntry represents a single key-value snapshot entry along with its expiration timestamp.
type DBEntry struct {
	Key      string
	Val      *object.Robj
	ExpireAt int64 // Expiration timestamp in Unix milliseconds (0 = no expiry)
}

// ForEachShardSnapshot iterates through all database shards one by one.
// For each shard, it acquires RLock, creates a fast shallow-copy snapshot of active
// (non-expired) entries, releases RLock immediately (minimizing lock hold time),
// and then executes the callback function outside the lock.
func (db *ShardedDB) ForEachShardSnapshot(fn func(entries []DBEntry) error) error {
	now := time.Now().UnixMilli()

	for i := 0; i < NumShards; i++ {
		s := &db.shards[i]

		// 1. Acquire RLock for minimum duration
		s.RLock()
		count := len(s.entries)
		if count == 0 {
			s.RUnlock()
			continue
		}

		// Fast shallow-copy of key pointers and expiration timestamps
		entries := make([]DBEntry, 0, count)
		for k, v := range s.entries {
			if s.isExpired(k, now) {
				continue
			}
			entries = append(entries, DBEntry{
				Key:      k,
				Val:      v,
				ExpireAt: s.expires[k],
			})
		}
		s.RUnlock() // 2. Release RLock immediately

		// 3. Process entries outside the lock (disk I/O, serialization, etc.)
		if len(entries) > 0 {
			if err := fn(entries); err != nil {
				return err
			}
		}
	}

	return nil
}
