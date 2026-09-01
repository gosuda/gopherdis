package quicklist

import (
	"fmt"
	"testing"
)

func TestQuicklistPushPop(t *testing.T) {
	ql := NewQuicklist()

	ql.RPush([]byte("a"))
	ql.RPush([]byte("b"))
	ql.LPush([]byte("c")) // c, a, b

	if ql.Len() != 3 {
		t.Fatalf("expected len 3, got %d", ql.Len())
	}

	val, ok := ql.LPop()
	if !ok || string(val) != "c" {
		t.Fatalf("expected 'c', got '%s'", val)
	}

	val, ok = ql.RPop()
	if !ok || string(val) != "b" {
		t.Fatalf("expected 'b', got '%s'", val)
	}

	val, ok = ql.LPop()
	if !ok || string(val) != "a" {
		t.Fatalf("expected 'a', got '%s'", val)
	}

	if ql.Len() != 0 {
		t.Fatalf("expected empty list, got len %d", ql.Len())
	}
}

func TestQuicklistLargeRange(t *testing.T) {
	ql := NewQuicklist()
	n := 300 // Exceeds single node capacity of 128

	for i := 0; i < n; i++ {
		ql.RPush([]byte(fmt.Sprintf("item_%d", i)))
	}

	if ql.Len() != n {
		t.Fatalf("expected %d, got %d", n, ql.Len())
	}

	// Full range 0 -1
	all := ql.LRange(0, -1)
	if len(all) != n {
		t.Fatalf("expected %d items, got %d", n, len(all))
	}
	if string(all[0]) != "item_0" || string(all[n-1]) != fmt.Sprintf("item_%d", n-1) {
		t.Fatalf("unexpected bounds: %s, %s", all[0], all[n-1])
	}

	// Partial range crossing node boundaries
	sub := ql.LRange(100, 150)
	if len(sub) != 51 {
		t.Fatalf("expected 51 items, got %d", len(sub))
	}
	if string(sub[0]) != "item_100" || string(sub[50]) != "item_150" {
		t.Fatalf("unexpected sub range: %s, %s", sub[0], sub[50])
	}

	// LIndex
	idxVal, ok := ql.LIndex(150)
	if !ok || string(idxVal) != "item_150" {
		t.Fatalf("expected 'item_150', got '%s'", idxVal)
	}
}
