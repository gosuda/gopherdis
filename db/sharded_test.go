package db

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gosuda/gopherdis/object"
)

func TestShardedDBBasic(t *testing.T) {
	db := NewShardedDB()

	obj := object.CreateStringObject("world")
	db.Set("hello", obj)

	val, ok := db.Get("hello")
	if !ok || val.String() != "world" {
		t.Fatalf("expected 'world', got %v", val)
	}

	if !db.Exists("hello") {
		t.Fatalf("expected key to exist")
	}

	deleted := db.Del("hello")
	if !deleted {
		t.Fatalf("expected key to be deleted")
	}

	if db.Exists("hello") {
		t.Fatalf("expected key to not exist")
	}
}

func TestShardedDBTTL(t *testing.T) {
	db := NewShardedDB()

	obj := object.CreateStringObject("temp")
	db.SetWithExpire("k_temp", obj, 50*time.Millisecond)

	val, ok := db.Get("k_temp")
	if !ok || val.String() != "temp" {
		t.Fatalf("expected key to exist before expiration")
	}

	time.Sleep(60 * time.Millisecond)

	val, ok = db.Get("k_temp")
	if ok || val != nil {
		t.Fatalf("expected key to expire")
	}
}

func TestShardedDBConcurrent(t *testing.T) {
	db := NewShardedDB()
	var wg sync.WaitGroup

	numGoroutines := 50
	numOps := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				k := fmt.Sprintf("key:%d:%d", id, j)
				v := fmt.Sprintf("val:%d:%d", id, j)
				db.Set(k, object.CreateStringObject(v))
				ret, ok := db.Get(k)
				if !ok || ret.String() != v {
					t.Errorf("mismatch on key %s", k)
				}
			}
		}(i)
	}

	wg.Wait()
}

func TestActiveExpireCycle(t *testing.T) {
	db := NewShardedDB()

	// Insert 100 keys with 30ms expiration
	for i := 0; i < 100; i++ {
		db.SetWithExpire(fmt.Sprintf("exp_%d", i), object.CreateStringObject("val"), 30*time.Millisecond)
	}

	time.Sleep(50 * time.Millisecond)

	// Run ActiveExpireCycle
	deleted := db.ActiveExpireCycle()
	if deleted != 100 {
		t.Fatalf("expected 100 keys deleted by active expire, got %d", deleted)
	}
}

func TestLRUEviction(t *testing.T) {
	db := NewShardedDB()
	// Set small memory limit (enough for ~3 items)
	db.SetMaxMemory(300)
	db.SetEvictionPolicy(AllKeysLRU)

	_ = db.Set("key1", object.CreateStringObject("val1"))
	time.Sleep(10 * time.Millisecond)
	_ = db.Set("key2", object.CreateStringObject("val2"))
	time.Sleep(10 * time.Millisecond)
	_ = db.Set("key3", object.CreateStringObject("val3"))

	// Touch key1 to update its LRU timestamp
	_, _ = db.Get("key1")
	time.Sleep(10 * time.Millisecond)

	// Adding key4 should evict key2 (oldest LRU)
	_ = db.Set("key4", object.CreateStringObject("val4"))

	if !db.Exists("key1") {
		t.Fatalf("expected key1 to still exist (recently accessed)")
	}
	if !db.Exists("key4") {
		t.Fatalf("expected key4 to exist")
	}
}

func TestNoEvictionOOM(t *testing.T) {
	db := NewShardedDB()
	db.SetMaxMemory(150)
	db.SetEvictionPolicy(NoEviction)

	err1 := db.Set("k1", object.CreateStringObject("v1"))
	if err1 != nil {
		t.Fatalf("unexpected error: %v", err1)
	}

	err2 := db.Set("k2", object.CreateStringObject("v2"))
	if err2 != nil {
		t.Fatalf("unexpected error: %v", err2)
	}

	// Should trigger ErrOOM
	err3 := db.Set("k3_long_key_name", object.CreateStringObject("v3_long_value_data"))
	if err3 != ErrOOM {
		t.Fatalf("expected ErrOOM, got %v", err3)
	}
}

func TestShardedDB_TTLEdgeCases(t *testing.T) {
	database := NewShardedDB()

	// 1. Non-existent key -> code -2
	_, code := database.TTL("non_existing")
	if code != -2 {
		t.Fatalf("expected -2 for non-existent key, got %d", code)
	}

	// 2. Key without expiration -> code -1
	_ = database.Set("persistent_k", object.CreateStringObject("v"))
	_, code = database.TTL("persistent_k")
	if code != -1 {
		t.Fatalf("expected -1 for persistent key, got %d", code)
	}

	// 3. Key with SetExpireAt
	absMs := time.Now().UnixMilli() + 5000
	if !database.SetExpireAt("persistent_k", absMs) {
		t.Fatalf("expected SetExpireAt to succeed")
	}

	ttl, code := database.TTL("persistent_k")
	if code != 0 || ttl <= 0 || ttl > 6*time.Second {
		t.Fatalf("expected valid TTL, got code %d, ttl %v", code, ttl)
	}
}

func TestShardedDB_ConcurrentExpireAndAccess(t *testing.T) {
	database := NewShardedDB()
	database.StartCron(10 * time.Millisecond)
	defer database.StopCron()

	const numKeys = 200
	for i := 0; i < numKeys; i++ {
		key := fmt.Sprintf("short_lived_%d", i)
		_ = database.SetWithExpire(key, object.CreateStringObject("temp"), 20*time.Millisecond)
	}

	var wg sync.WaitGroup
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				key := fmt.Sprintf("short_lived_%d", j)
				_, _ = database.Get(key)
				time.Sleep(1 * time.Millisecond)
			}
		}()
	}
	wg.Wait()
}

