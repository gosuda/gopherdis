package rdb

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/gosuda/nedis/datastruct/quicklist"
	"github.com/gosuda/nedis/datastruct/skiplist"
	"github.com/gosuda/nedis/db"
	"github.com/gosuda/nedis/object"
)

func TestRDB_RoundtripAllDataTypes(t *testing.T) {
	tempDir := t.TempDir()
	rdbPath := filepath.Join(tempDir, "dump.rdb")

	originDB := db.NewShardedDB()

	// 1. String
	_ = originDB.Set("str_key", object.CreateStringObject("hello world"))

	// 1b. Large Compressible String (triggers LZF compression in RDB)
	largeString := "Redis RDB LZF compression verification string with repetition: Redis RDB LZF compression verification string!"
	_ = originDB.Set("lzf_str_key", object.CreateStringObject(largeString))

	// 2. String with TTL
	_ = originDB.SetWithExpire("str_exp", object.CreateStringObject("exp_val"), 10*time.Second)

	// 3. List
	ql := quicklist.NewQuicklist()
	ql.RPush([]byte("item1"))
	ql.RPush([]byte("item2"))
	ql.RPush([]byte("item3"))
	_ = originDB.Set("list_key", object.CreateObject(object.OBJ_LIST, ql))

	// 4. Set
	smap := map[string]struct{}{
		"alpha": {},
		"beta":  {},
		"gamma": {},
	}
	_ = originDB.Set("set_key", object.CreateObject(object.OBJ_SET, smap))

	// 5. Hash
	hmap := map[string][]byte{
		"field1": []byte("val1"),
		"field2": []byte("val2"),
	}
	_ = originDB.Set("hash_key", object.CreateObject(object.OBJ_HASH, hmap))

	// 6. ZSet
	zs := skiplist.NewZSet()
	zs.Add("alice", 10.5)
	zs.Add("bob", 20.0)
	_ = originDB.Set("zset_key", object.CreateObject(object.OBJ_ZSET, zs))

	// Save to RDB
	mgr := NewManager(rdbPath)
	if err := mgr.Save(originDB); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load into fresh DB
	restoredDB := db.NewShardedDB()
	if err := mgr.Load(restoredDB); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify String
	val, ok := restoredDB.Get("str_key")
	if !ok || val.String() != "hello world" {
		t.Fatalf("string mismatch: %v", val)
	}

	// Verify LZF Compressed String
	val, ok = restoredDB.Get("lzf_str_key")
	if !ok || val.String() != largeString {
		t.Fatalf("lzf string mismatch: got %v", val)
	}

	// Verify String TTL
	val, ok = restoredDB.Get("str_exp")
	if !ok || val.String() != "exp_val" {
		t.Fatalf("str_exp missing")
	}
	ttl, code := restoredDB.TTL("str_exp")
	if code != 0 || ttl <= 0 {
		t.Fatalf("expected positive ttl for str_exp, got code %d, ttl %v", code, ttl)
	}

	// Verify List
	val, ok = restoredDB.Get("list_key")
	if !ok || val.Type != object.OBJ_LIST {
		t.Fatalf("list missing or wrong type")
	}
	restoredQL := val.Ptr.(*quicklist.Quicklist)
	if restoredQL.Len() != 3 {
		t.Fatalf("expected list len 3, got %d", restoredQL.Len())
	}

	// Verify Set
	val, ok = restoredDB.Get("set_key")
	if !ok || val.Type != object.OBJ_SET {
		t.Fatalf("set missing or wrong type")
	}
	restoredSet := val.Ptr.(map[string]struct{})
	if len(restoredSet) != 3 {
		t.Fatalf("expected set len 3, got %d", len(restoredSet))
	}

	// Verify Hash
	val, ok = restoredDB.Get("hash_key")
	if !ok || val.Type != object.OBJ_HASH {
		t.Fatalf("hash missing or wrong type")
	}
	restoredHash := val.Ptr.(map[string][]byte)
	if string(restoredHash["field1"]) != "val1" {
		t.Fatalf("expected hash field1=val1")
	}

	// Verify ZSet
	val, ok = restoredDB.Get("zset_key")
	if !ok || val.Type != object.OBJ_ZSET {
		t.Fatalf("zset missing or wrong type")
	}
	restoredZS := val.Ptr.(*skiplist.ZSet)
	score, ok := restoredZS.Score("bob")
	if !ok || score != 20.0 {
		t.Fatalf("expected bob score 20.0, got %v", score)
	}
}

func TestRDB_BGSave(t *testing.T) {
	tempDir := t.TempDir()
	rdbPath := filepath.Join(tempDir, "bgsave.rdb")

	originDB := db.NewShardedDB()
	for i := 0; i < 500; i++ {
		_ = originDB.Set(fmt.Sprintf("k:%d", i), object.CreateStringObject(fmt.Sprintf("v:%d", i)))
	}

	mgr := NewManager(rdbPath)
	doneCh := make(chan error, 1)

	err := mgr.BGSave(originDB, func(err error) {
		doneCh <- err
	})
	if err != nil {
		t.Fatalf("BGSave failed: %v", err)
	}

	select {
	case err := <-doneCh:
		if err != nil {
			t.Fatalf("BGSave completion returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("BGSave timed out")
	}

	// Verify file is readable
	freshDB := db.NewShardedDB()
	if err := mgr.Load(freshDB); err != nil {
		t.Fatalf("Load after BGSave failed: %v", err)
	}

	for i := 0; i < 500; i++ {
		k := fmt.Sprintf("k:%d", i)
		v, ok := freshDB.Get(k)
		if !ok || v.String() != fmt.Sprintf("v:%d", i) {
			t.Fatalf("mismatch on key %s", k)
		}
	}
}

func TestRDB_ChecksumValidation(t *testing.T) {
	crc1 := CRC64(0, []byte("123456789"))
	if crc1 == 0 {
		t.Fatalf("CRC64 calculated 0 for test string")
	}
}
