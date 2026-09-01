package aof

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gosuda/nedis/db"
	"github.com/gosuda/nedis/object"
)

func TestAOF_FeedAndLoad(t *testing.T) {
	tempDir := t.TempDir()
	aofPath := filepath.Join(tempDir, "test.aof")

	aofEngine, err := OpenAOF(aofPath, FsyncAlways)
	if err != nil {
		t.Fatalf("OpenAOF failed: %v", err)
	}

	// Feed commands
	_ = aofEngine.Feed([][]byte{[]byte("SET"), []byte("foo"), []byte("bar")})
	_ = aofEngine.Feed([][]byte{[]byte("LPUSH"), []byte("mylist"), []byte("v1"), []byte("v2")})
	_ = aofEngine.Feed([][]byte{[]byte("HSET"), []byte("myhash"), []byte("f1"), []byte("v1")})
	_ = aofEngine.Feed([][]byte{[]byte("SADD"), []byte("myset"), []byte("m1"), []byte("m2")})
	_ = aofEngine.Feed([][]byte{[]byte("ZADD"), []byte("myzset"), []byte("10.5"), []byte("zm1")})

	_ = aofEngine.Close()

	// Load into fresh DB
	newDB := db.NewShardedDB()
	aofReader, err := OpenAOF(aofPath, FsyncNo)
	if err != nil {
		t.Fatalf("OpenAOF for load failed: %v", err)
	}
	defer aofReader.Close()

	if err := aofReader.Load(newDB); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Validate loaded state
	val, ok := newDB.Get("foo")
	if !ok || val.String() != "bar" {
		t.Fatalf("expected foo=bar, got %v", val)
	}

	val, ok = newDB.Get("mylist")
	if !ok || val.Type != object.OBJ_LIST {
		t.Fatalf("expected mylist list object")
	}

	val, ok = newDB.Get("myhash")
	if !ok || val.Type != object.OBJ_HASH {
		t.Fatalf("expected myhash hash object")
	}

	val, ok = newDB.Get("myset")
	if !ok || val.Type != object.OBJ_SET {
		t.Fatalf("expected myset set object")
	}

	val, ok = newDB.Get("myzset")
	if !ok || val.Type != object.OBJ_ZSET {
		t.Fatalf("expected myzset zset object")
	}
}

func TestAOF_Rewrite(t *testing.T) {
	tempDir := t.TempDir()
	aofPath := filepath.Join(tempDir, "rewrite.aof")

	aofEngine, err := OpenAOF(aofPath, FsyncEverySec)
	if err != nil {
		t.Fatalf("OpenAOF failed: %v", err)
	}
	defer aofEngine.Close()

	memDB := db.NewShardedDB()

	// Populate DB with 500 keys
	for i := 0; i < 500; i++ {
		key := fmt.Sprintf("k:%d", i)
		val := object.CreateStringObject(fmt.Sprintf("v:%d", i))
		_ = memDB.Set(key, val)
	}

	// Add key with TTL
	_ = memDB.SetWithExpire("exp_key", object.CreateStringObject("exp_val"), 10*time.Second)

	// Perform rewrite
	if err := aofEngine.Rewrite(memDB); err != nil {
		t.Fatalf("Rewrite failed: %v", err)
	}

	if aofEngine.CurrentSize() == 0 {
		t.Fatalf("AOF file size is 0 after rewrite")
	}

	// Load rewritten AOF into a clean DB
	freshDB := db.NewShardedDB()
	if err := aofEngine.Load(freshDB); err != nil {
		t.Fatalf("Load after rewrite failed: %v", err)
	}

	for i := 0; i < 500; i++ {
		key := fmt.Sprintf("k:%d", i)
		val, ok := freshDB.Get(key)
		if !ok || val.String() != fmt.Sprintf("v:%d", i) {
			t.Fatalf("key %s missing or mismatch after rewrite load", key)
		}
	}

	val, ok := freshDB.Get("exp_key")
	if !ok || val.String() != "exp_val" {
		t.Fatalf("exp_key missing after rewrite load")
	}

	ttl, code := freshDB.TTL("exp_key")
	if code != 0 || ttl <= 0 {
		t.Fatalf("expected valid TTL for exp_key, got code %d, ttl %v", code, ttl)
	}
}

func TestDoubleBuffer_ConcurrentAndOverflow(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "double_buf.bin")

	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	defer file.Close()

	// Small buffer of 512 bytes to frequently force buffer overflow swaps
	dbuf := NewDoubleBuffer(file, 512)
	defer dbuf.Close()

	const numGoroutines = 30
	const numWritesPerGoroutine = 50
	msg := []byte("*3\r\n$3\r\nSET\r\n$1\r\na\r\n$1\r\nb\r\n")

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numWritesPerGoroutine; j++ {
				_, err := dbuf.Write(msg)
				if err != nil {
					t.Errorf("write error: %v", err)
				}
			}
		}()
	}

	wg.Wait()
	_ = dbuf.Flush()

	info, err := file.Stat()
	if err != nil {
		t.Fatalf("file stat error: %v", err)
	}

	expectedBytes := int64(numGoroutines * numWritesPerGoroutine * len(msg))
	if info.Size() != expectedBytes {
		t.Fatalf("expected file size %d bytes, got %d bytes", expectedBytes, info.Size())
	}
}

func TestAOF_CorruptedOrTruncatedLoad(t *testing.T) {
	tempDir := t.TempDir()
	aofPath := filepath.Join(tempDir, "corrupted.aof")

	// Write valid command followed by half-written/truncated command
	content := []byte("*3\r\n$3\r\nSET\r\n$5\r\nvalid\r\n$3\r\n123\r\n*3\r\n$3\r\nSET\r\n$7\r\ntruncat")
	if err := os.WriteFile(aofPath, content, 0644); err != nil {
		t.Fatalf("failed to write corrupted file: %v", err)
	}

	aofEngine, err := OpenAOF(aofPath, FsyncNo)
	if err != nil {
		t.Fatalf("failed to open aof: %v", err)
	}
	defer aofEngine.Close()

	targetDB := db.NewShardedDB()
	// Load should safely handle unexpected EOF without crashing
	_ = aofEngine.Load(targetDB)

	val, ok := targetDB.Get("valid")
	if !ok || val.String() != "123" {
		t.Fatalf("expected valid key to be loaded before truncation, got %v", val)
	}
}
