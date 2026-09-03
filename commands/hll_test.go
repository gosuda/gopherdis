package commands

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gosuda/gopherdis/db"
)

func TestHLLCommands(t *testing.T) {
	database := db.NewShardedDB()
	ctx := &Context{DB: database}

	// 1. PFADD hll1 a b c -> returns :1
	res := DefaultTable.Execute(ctx, [][]byte{[]byte("PFADD"), []byte("hll1"), []byte("a"), []byte("b"), []byte("c")})
	if string(res) != ":1\r\n" {
		t.Fatalf("expected :1 on first pfadd, got %q", res)
	}

	// 2. PFADD hll1 a b c -> returns :0 (no cardinality change)
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("PFADD"), []byte("hll1"), []byte("a"), []byte("b"), []byte("c")})
	if string(res) != ":0\r\n" {
		t.Fatalf("expected :0 on duplicate pfadd, got %q", res)
	}

	// 3. PFCOUNT hll1 -> returns :3
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("PFCOUNT"), []byte("hll1")})
	if string(res) != ":3\r\n" {
		t.Fatalf("expected :3 on pfcount, got %q", res)
	}

	// 4. Populate hll2 with distinct items
	for i := 0; i < 1000; i++ {
		DefaultTable.Execute(ctx, [][]byte{[]byte("PFADD"), []byte("hll2"), []byte(fmt.Sprintf("item_%d", i))})
	}

	// PFCOUNT hll2
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("PFCOUNT"), []byte("hll2")})
	if !strings.HasPrefix(string(res), ":") {
		t.Fatalf("expected integer reply, got %q", res)
	}

	// 5. Multi-key PFCOUNT hll1 hll2
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("PFCOUNT"), []byte("hll1"), []byte("hll2")})
	if !strings.HasPrefix(string(res), ":") {
		t.Fatalf("expected integer reply from multi pfcount, got %q", res)
	}

	// 6. PFMERGE hll_dest hll1 hll2
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("PFMERGE"), []byte("hll_dest"), []byte("hll1"), []byte("hll2")})
	if string(res) != "+OK\r\n" {
		t.Fatalf("expected +OK on pfmerge, got %q", res)
	}

	res = DefaultTable.Execute(ctx, [][]byte{[]byte("PFCOUNT"), []byte("hll_dest")})
	if !strings.HasPrefix(string(res), ":") {
		t.Fatalf("expected integer reply on merged hll, got %q", res)
	}
}
