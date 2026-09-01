package commands

import (
	"strings"
	"testing"

	"github.com/gosuda/nedis/db"
)

func TestBitmapCommands(t *testing.T) {
	database := db.NewShardedDB()
	ctx := &Context{DB: database}

	// 1. SETBIT bm 7 1 -> returns :0
	res := DefaultTable.Execute(ctx, [][]byte{[]byte("SETBIT"), []byte("bm"), []byte("7"), []byte("1")})
	if string(res) != ":0\r\n" {
		t.Fatalf("expected :0 on first setbit, got %q", res)
	}

	// 2. GETBIT bm 7 -> returns :1, GETBIT bm 6 -> returns :0
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("GETBIT"), []byte("bm"), []byte("7")})
	if string(res) != ":1\r\n" {
		t.Fatalf("expected :1, got %q", res)
	}
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("GETBIT"), []byte("bm"), []byte("6")})
	if string(res) != ":0\r\n" {
		t.Fatalf("expected :0, got %q", res)
	}

	// 3. SETBIT bm 0 1 -> returns :0 (now byte 0 is 1000 0001 = 0x81)
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("SETBIT"), []byte("bm"), []byte("0"), []byte("1")})
	if string(res) != ":0\r\n" {
		t.Fatalf("expected :0, got %q", res)
	}

	// 4. BITCOUNT bm -> returns :2
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("BITCOUNT"), []byte("bm")})
	if string(res) != ":2\r\n" {
		t.Fatalf("expected :2, got %q", res)
	}

	// 5. BITPOS bm 1 -> returns :0
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("BITPOS"), []byte("bm"), []byte("1")})
	if string(res) != ":0\r\n" {
		t.Fatalf("expected :0, got %q", res)
	}

	// 6. BITOP NOT bm_not bm -> returns :1
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("BITOP"), []byte("NOT"), []byte("bm_not"), []byte("bm")})
	if string(res) != ":1\r\n" {
		t.Fatalf("expected :1 length from BITOP, got %q", res)
	}

	// bm_not should have 6 bits set (0x7E = 0111 1110)
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("BITCOUNT"), []byte("bm_not")})
	if string(res) != ":6\r\n" {
		t.Fatalf("expected :6 for bm_not, got %q", res)
	}

	// 7. GET bm -> verify raw string interoperability
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("GET"), []byte("bm")})
	if !strings.Contains(string(res), "$1\r\n") {
		t.Fatalf("expected raw 1-byte bulk string, got %q", res)
	}
}
