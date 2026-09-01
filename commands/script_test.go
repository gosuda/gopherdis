package commands

import (
	"strings"
	"testing"

	"github.com/gosuda/nedis/db"
	"github.com/gosuda/nedis/scripting"
)

func TestScriptCommands(t *testing.T) {
	database := db.NewShardedDB()
	eng := scripting.NewEngine()
	ctx := &Context{
		DB:        database,
		Scripting: eng,
	}

	// 1. EVAL with multiple keys and args
	res := DefaultTable.Execute(ctx, [][]byte{
		[]byte("EVAL"),
		[]byte("return {KEYS[1], KEYS[2], ARGV[1], ARGV[2]}"),
		[]byte("2"),
		[]byte("k1"), []byte("k2"),
		[]byte("a1"), []byte("a2"),
	})
	if string(res) != "*4\r\n$2\r\nk1\r\n$2\r\nk2\r\n$2\r\na1\r\n$2\r\na2\r\n" {
		t.Fatalf("EVAL table return mismatch: %q", res)
	}

	// 2. EVAL with redis.call SET
	res = DefaultTable.Execute(ctx, [][]byte{
		[]byte("EVAL"),
		[]byte("return redis.call('SET', KEYS[1], ARGV[1])"),
		[]byte("1"),
		[]byte("lua_key"),
		[]byte("lua_val"),
	})
	if string(res) != "+OK\r\n" {
		t.Fatalf("expected +OK on redis.call SET, got %q", res)
	}

	// 3. Verify key was set in DB
	val, ok := database.Get("lua_key")
	if !ok || val.String() != "lua_val" {
		t.Fatalf("key was not set in DB by Lua: %v", val)
	}

	// 4. SCRIPT LOAD
	scriptGet := "return redis.call('GET', KEYS[1])"
	res = DefaultTable.Execute(ctx, [][]byte{
		[]byte("SCRIPT"), []byte("LOAD"), []byte(scriptGet),
	})
	if !strings.HasPrefix(string(res), "$40\r\n") {
		t.Fatalf("expected 40-char SHA1 from SCRIPT LOAD, got %q", res)
	}
	sha1Str := strings.TrimSpace(strings.Split(string(res), "\r\n")[1])

	// 5. SCRIPT EXISTS
	res = DefaultTable.Execute(ctx, [][]byte{
		[]byte("SCRIPT"), []byte("EXISTS"), []byte(sha1Str),
	})
	if string(res) != "*1\r\n:1\r\n" {
		t.Fatalf("expected [1] from SCRIPT EXISTS, got %q", res)
	}

	// 6. EVALSHA
	res = DefaultTable.Execute(ctx, [][]byte{
		[]byte("EVALSHA"), []byte(sha1Str), []byte("1"), []byte("lua_key"),
	})
	if string(res) != "$7\r\nlua_val\r\n" {
		t.Fatalf("expected lua_val from EVALSHA, got %q", res)
	}

	// 7. SCRIPT FLUSH and verify NOSCRIPT error
	res = DefaultTable.Execute(ctx, [][]byte{
		[]byte("SCRIPT"), []byte("FLUSH"),
	})
	if string(res) != "+OK\r\n" {
		t.Fatalf("expected +OK on SCRIPT FLUSH, got %q", res)
	}

	res = DefaultTable.Execute(ctx, [][]byte{
		[]byte("EVALSHA"), []byte(sha1Str), []byte("1"), []byte("lua_key"),
	})
	if !strings.HasPrefix(string(res), "-NOSCRIPT") {
		t.Fatalf("expected -NOSCRIPT error after SCRIPT FLUSH, got %q", res)
	}
}
