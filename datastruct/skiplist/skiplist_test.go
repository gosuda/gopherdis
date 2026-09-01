package skiplist

import (
	"testing"
)

func TestZSetBasic(t *testing.T) {
	zs := NewZSet()

	added, updated := zs.Add("alice", 100)
	if !added || updated {
		t.Fatalf("expected added=true, updated=false")
	}

	added, updated = zs.Add("bob", 85)
	if !added || updated {
		t.Fatalf("expected added=true, updated=false")
	}

	added, updated = zs.Add("charlie", 95)
	if !added || updated {
		t.Fatalf("expected added=true, updated=false")
	}

	// Update score of bob
	added, updated = zs.Add("bob", 105)
	if added || !updated {
		t.Fatalf("expected added=false, updated=true")
	}

	// Score lookup
	score, ok := zs.Score("bob")
	if !ok || score != 105 {
		t.Fatalf("expected score 105, got %v", score)
	}

	// Rank (ascending: charlie(95) -> 0, alice(100) -> 1, bob(105) -> 2)
	rank, ok := zs.Rank("charlie", false)
	if !ok || rank != 0 {
		t.Fatalf("expected charlie rank 0, got %d", rank)
	}

	rank, ok = zs.Rank("bob", false)
	if !ok || rank != 2 {
		t.Fatalf("expected bob rank 2, got %d", rank)
	}

	// Rank (descending: bob(105) -> 0, alice(100) -> 1, charlie(95) -> 2)
	rank, ok = zs.Rank("bob", true)
	if !ok || rank != 0 {
		t.Fatalf("expected bob rev rank 0, got %d", rank)
	}

	// Range ascending
	res := zs.Range(0, -1, false)
	if len(res) != 3 {
		t.Fatalf("expected 3 items, got %d", len(res))
	}
	if res[0].Member != "charlie" || res[1].Member != "alice" || res[2].Member != "bob" {
		t.Fatalf("unexpected order: %+v", res)
	}

	// Range descending
	res = zs.Range(0, 1, true)
	if len(res) != 2 || res[0].Member != "bob" || res[1].Member != "alice" {
		t.Fatalf("unexpected rev range: %+v", res)
	}

	// Remove
	if !zs.Remove("alice") {
		t.Fatalf("expected remove to succeed")
	}
	if zs.Len() != 2 {
		t.Fatalf("expected len 2, got %d", zs.Len())
	}
}
