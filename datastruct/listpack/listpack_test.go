package listpack

import (
	"bytes"
	"fmt"
	"testing"
)

func collect(lp *Listpack) []string {
	var out []string
	lp.ForEach(func(_ int, v []byte) bool {
		out = append(out, string(v))
		return true
	})
	return out
}

func TestAppendAndGet(t *testing.T) {
	lp := New()
	want := []string{"a", "", "hello world", string(make([]byte, 300))}
	for _, w := range want {
		lp.Append([]byte(w))
	}
	if lp.Len() != len(want) {
		t.Fatalf("Len = %d, want %d", lp.Len(), len(want))
	}
	for i, w := range want {
		if got := lp.Get(i); !bytes.Equal(got, []byte(w)) {
			t.Errorf("Get(%d) length %d, want %d", i, len(got), len(w))
		}
	}
	if lp.Get(-1) != nil || lp.Get(len(want)) != nil {
		t.Error("out of range Get should return nil")
	}
}

func TestReplaceAt(t *testing.T) {
	lp := New()
	for _, s := range []string{"a", "b", "c"} {
		lp.Append([]byte(s))
	}
	// Replace with a longer value: the following elements must stay intact.
	if !lp.ReplaceAt(1, []byte("much-longer-value")) {
		t.Fatal("ReplaceAt returned false")
	}
	if got := collect(lp); fmt.Sprint(got) != "[a much-longer-value c]" {
		t.Fatalf("after ReplaceAt: %v", got)
	}
	// And with a shorter one.
	if !lp.ReplaceAt(1, []byte("x")) {
		t.Fatal("ReplaceAt returned false")
	}
	if got := collect(lp); fmt.Sprint(got) != "[a x c]" {
		t.Fatalf("after shrinking ReplaceAt: %v", got)
	}
	if lp.Len() != 3 {
		t.Errorf("ReplaceAt changed Len to %d", lp.Len())
	}
}

func TestDeleteRange(t *testing.T) {
	lp := New()
	for _, s := range []string{"a", "b", "c", "d", "e"} {
		lp.Append([]byte(s))
	}
	if !lp.DeleteRange(1, 2) {
		t.Fatal("DeleteRange returned false")
	}
	if got := collect(lp); fmt.Sprint(got) != "[a d e]" {
		t.Fatalf("after DeleteRange: %v", got)
	}
	if lp.Len() != 3 {
		t.Errorf("Len = %d, want 3", lp.Len())
	}
	if lp.DeleteRange(2, 5) {
		t.Error("DeleteRange past the end should fail")
	}
	// Deleting the tail and then the head leaves it empty and reusable.
	lp.DeleteRange(0, 3)
	if lp.Len() != 0 || len(collect(lp)) != 0 {
		t.Errorf("listpack not empty after deleting everything: %v", collect(lp))
	}
	lp.Append([]byte("fresh"))
	if got := collect(lp); fmt.Sprint(got) != "[fresh]" {
		t.Errorf("reuse after full delete: %v", got)
	}
}

func TestInsertAt(t *testing.T) {
	lp := New()
	for _, s := range []string{"a", "c"} {
		lp.Append([]byte(s))
	}
	lp.InsertAt(1, []byte("b"))
	if got := collect(lp); fmt.Sprint(got) != "[a b c]" {
		t.Fatalf("InsertAt middle: %v", got)
	}
	lp.InsertAt(0, []byte("z"))
	if got := collect(lp); fmt.Sprint(got) != "[z a b c]" {
		t.Fatalf("InsertAt head: %v", got)
	}
	lp.InsertAt(lp.Len(), []byte("end"))
	if got := collect(lp); fmt.Sprint(got) != "[z a b c end]" {
		t.Fatalf("InsertAt tail: %v", got)
	}
}

// TestFindStride covers searching only the key positions of a key/value pack,
// which is how the hash and sorted set encodings use it.
func TestFindStride(t *testing.T) {
	lp := New()
	for _, s := range []string{"f1", "v1", "f2", "v2"} {
		lp.Append([]byte(s))
	}
	if got := lp.Find([]byte("f2"), 0, 2); got != 2 {
		t.Errorf("Find(f2) = %d, want 2", got)
	}
	// "v1" sits at an odd index, so a key-only search must not find it.
	if got := lp.Find([]byte("v1"), 0, 2); got != -1 {
		t.Errorf("Find(v1) on key positions = %d, want -1", got)
	}
	if got := lp.Find([]byte("v1"), 1, 2); got != 1 {
		t.Errorf("Find(v1) on value positions = %d, want 1", got)
	}
	if got := lp.Find([]byte("nope"), 0, 2); got != -1 {
		t.Errorf("Find(missing) = %d, want -1", got)
	}
}

// TestCompactness is the reason this encoding exists: one buffer, not one
// allocation per entry.
func TestCompactness(t *testing.T) {
	lp := New()
	for i := 0; i < 64; i++ {
		lp.Append([]byte(fmt.Sprintf("field%d", i)))
		lp.Append([]byte(fmt.Sprintf("value%d", i)))
	}
	// 128 elements averaging ~7 bytes plus a 1 byte header each.
	if lp.Bytes() > 1200 {
		t.Errorf("listpack buffer is %d bytes, larger than expected", lp.Bytes())
	}
	if lp.Len() != 128 {
		t.Errorf("Len = %d, want 128", lp.Len())
	}
}
