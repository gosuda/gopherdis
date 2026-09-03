package commands

import (
	"strings"
	"testing"

	"github.com/gosuda/gopherdis/db"
	"github.com/gosuda/gopherdis/object"
)

func TestAdminCommands(t *testing.T) {
	database := db.NewShardedDB()
	ctx := &Context{
		DB: database,
	}

	// 1. HELLO 2
	res := DefaultTable.Execute(ctx, [][]byte{[]byte("HELLO"), []byte("2")})
	if !strings.HasPrefix(string(res), "*14\r\n") || !strings.Contains(string(res), "gopherdis") {
		t.Fatalf("expected RESP2 HELLO array, got %q", res)
	}

	// 2. HELLO 3
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("HELLO"), []byte("3")})
	if !strings.HasPrefix(string(res), "%7\r\n") || !strings.Contains(string(res), "proto\r\n:3\r\n") {
		t.Fatalf("expected RESP3 HELLO map, got %q", res)
	}

	// 3. TIME
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("TIME")})
	if !strings.HasPrefix(string(res), "*2\r\n") {
		t.Fatalf("expected 2-element array from TIME, got %q", res)
	}

	// 4. DBSIZE
	database.Set("k1", object.CreateStringObject("v1"))
	database.Set("k2", object.CreateStringObject("v2"))
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("DBSIZE")})
	if string(res) != ":2\r\n" {
		t.Fatalf("expected :2 from DBSIZE, got %q", res)
	}

	// 5. CLIENT LIST
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("CLIENT"), []byte("LIST")})
	if !strings.Contains(string(res), "addr=") {
		t.Fatalf("expected client list string, got %q", res)
	}

	// 6. SLOWLOG GET
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("SLOWLOG"), []byte("GET")})
	if string(res) != "*0\r\n" {
		t.Fatalf("expected empty slowlog array, got %q", res)
	}

	// 7. CONFIG GET & SET
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("CONFIG"), []byte("SET"), []byte("maxmemory"), []byte("1048576")})
	if string(res) != "+OK\r\n" {
		t.Fatalf("expected +OK on CONFIG SET, got %q", res)
	}

	res = DefaultTable.Execute(ctx, [][]byte{[]byte("CONFIG"), []byte("GET"), []byte("maxmemory")})
	if !strings.Contains(string(res), "1048576") {
		t.Fatalf("expected 1048576 from CONFIG GET, got %q", res)
	}

	// 8. INFO
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("INFO")})
	if !strings.Contains(string(res), "# Server") || !strings.Contains(string(res), "redis_version:7.2.0") {
		t.Fatalf("expected INFO output, got %q", res)
	}

	// 9. COMMAND & COMMAND COUNT
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("COMMAND"), []byte("COUNT")})
	if !strings.HasPrefix(string(res), ":") {
		t.Fatalf("expected integer count from COMMAND COUNT, got %q", res)
	}

	// 10. FLUSHDB
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("FLUSHDB")})
	if string(res) != "+OK\r\n" {
		t.Fatalf("expected +OK from FLUSHDB, got %q", res)
	}
	if database.Len() != 0 {
		t.Fatalf("expected 0 keys after FLUSHDB, got %d", database.Len())
	}
}
