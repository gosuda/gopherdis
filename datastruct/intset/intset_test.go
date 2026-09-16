package intset

import (
	"fmt"
	"testing"
)

func TestAddRemoveContains(t *testing.T) {
	is := New()
	for _, v := range []int64{5, 1, 3, 1} {
		is.Add(v)
	}
	if is.Len() != 3 {
		t.Fatalf("Len = %d, want 3 (duplicate ignored)", is.Len())
	}
	// Members come back in sorted numeric order, which is what intset gives.
	if got := fmt.Sprint(is.Members()); got != "[1 3 5]" {
		t.Fatalf("Members = %v, want sorted", got)
	}
	if !is.Contains(3) || is.Contains(4) {
		t.Error("Contains is wrong")
	}
	if !is.Remove(3) || is.Remove(3) {
		t.Error("Remove should report whether the value was present")
	}
	if got := fmt.Sprint(is.Members()); got != "[1 5]" {
		t.Errorf("after Remove: %v", got)
	}
}

func TestParseRejectsNonCanonical(t *testing.T) {
	for _, s := range []string{"1", "-1", "0", "9223372036854775807"} {
		if _, ok := Parse(s); !ok {
			t.Errorf("Parse(%q) should succeed", s)
		}
	}
	// These parse as numbers but do not round-trip, so a set holding them
	// cannot use the intset encoding.
	for _, s := range []string{"012", "+1", " 1", "1 ", "", "abc", "1.0", "9223372036854775808"} {
		if _, ok := Parse(s); ok {
			t.Errorf("Parse(%q) should fail", s)
		}
	}
}

func TestNegativeAndOrder(t *testing.T) {
	is := New()
	for _, v := range []int64{3, -5, 0, -1} {
		is.Add(v)
	}
	if got := fmt.Sprint(is.Values()); got != "[-5 -1 0 3]" {
		t.Errorf("Values = %v, want ascending", got)
	}
}
