package commands

import (
	"strings"
	"testing"

	"github.com/gosuda/nedis/db"
	"github.com/gosuda/nedis/object"
)

func TestDebugAndMemoryCommands(t *testing.T) {
	database := db.NewShardedDB()
	ctx := &Context{
		DB: database,
	}

	database.Set("test_key", object.CreateStringObject("hello_world"))

	// 1. MEMORY USAGE
	res := DefaultTable.Execute(ctx, [][]byte{[]byte("MEMORY"), []byte("USAGE"), []byte("test_key")})
	if !strings.HasPrefix(string(res), ":") {
		t.Fatalf("expected integer from MEMORY USAGE, got %q", res)
	}

	// 2. MEMORY STATS & PURGE & DOCTOR
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("MEMORY"), []byte("STATS")})
	if !strings.Contains(string(res), "total.allocated") {
		t.Fatalf("expected stats array from MEMORY STATS, got %q", res)
	}

	res = DefaultTable.Execute(ctx, [][]byte{[]byte("MEMORY"), []byte("PURGE")})
	if string(res) != "+OK\r\n" {
		t.Fatalf("expected +OK from MEMORY PURGE, got %q", res)
	}

	res = DefaultTable.Execute(ctx, [][]byte{[]byte("MEMORY"), []byte("DOCTOR")})
	if !strings.Contains(string(res), "Hi Sam") {
		t.Fatalf("expected doctor report from MEMORY DOCTOR, got %q", res)
	}

	// 3. LATENCY DOCTOR & GRAPH
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("LATENCY"), []byte("DOCTOR")})
	if !strings.Contains(string(res), "Dave") {
		t.Fatalf("expected latency doctor report, got %q", res)
	}

	res = DefaultTable.Execute(ctx, [][]byte{[]byte("LATENCY"), []byte("GRAPH"), []byte("command")})
	if !strings.Contains(string(res), "Nedis Latency Graph") {
		t.Fatalf("expected latency graph, got %q", res)
	}

	// 4. DEBUG OBJECT & DIGEST
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("DEBUG"), []byte("OBJECT"), []byte("test_key")})
	if !strings.HasPrefix(string(res), "+Value at:") {
		t.Fatalf("expected debug object string, got %q", res)
	}

	res = DefaultTable.Execute(ctx, [][]byte{[]byte("DEBUG"), []byte("DIGEST")})
	if !strings.HasPrefix(string(res), "+") || len(string(res)) < 40 {
		t.Fatalf("expected 40-char SHA1 digest, got %q", res)
	}

	// 5. MODULE LIST
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("MODULE"), []byte("LIST")})
	if string(res) != "*0\r\n" {
		t.Fatalf("expected empty module list, got %q", res)
	}
}
