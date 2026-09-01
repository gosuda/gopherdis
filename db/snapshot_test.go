package db

import (
	"fmt"
	"testing"
	"time"

	"github.com/gosuda/nedis/object"
)

func TestForEachShardSnapshot(t *testing.T) {
	db := NewShardedDB()

	// Insert items across shards
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key:%d", i)
		val := object.CreateStringObject(fmt.Sprintf("val:%d", i))
		_ = db.Set(key, val)
	}

	// Insert an expired item
	_ = db.SetWithExpire("expired_key", object.CreateStringObject("temp"), 1*time.Millisecond)
	time.Sleep(5 * time.Millisecond)

	var collected []DBEntry
	err := db.ForEachShardSnapshot(func(entries []DBEntry) error {
		collected = append(collected, entries...)
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(collected) != 100 {
		t.Fatalf("expected 100 active entries, got %d", len(collected))
	}

	for _, entry := range collected {
		if entry.Key == "expired_key" {
			t.Fatalf("expired key was included in snapshot")
		}
	}
}
