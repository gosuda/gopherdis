package replication

import (
	"bytes"
	"testing"
)

func TestReplication_BacklogRingBuffer(t *testing.T) {
	bl := NewBacklog(100) // 100 bytes capacity

	// 1. Initial write
	bl.Feed([]byte("hello"))
	first, last := bl.Offsets()
	if first != 0 || last != 5 {
		t.Fatalf("expected offsets 0, 5, got %d, %d", first, last)
	}

	if !bl.CanPartialSync(0) {
		t.Fatalf("expected offset 0 to be valid")
	}

	data := bl.ReadFromOffset(0)
	if string(data) != "hello" {
		t.Fatalf("expected 'hello', got %q", string(data))
	}

	// 2. Overflow ring buffer (write 150 bytes)
	chunk := bytes.Repeat([]byte("A"), 150)
	bl.Feed(chunk)

	first, last = bl.Offsets()
	if last != 155 || first != 55 {
		t.Fatalf("expected offsets 55, 155 after wrap, got %d, %d", first, last)
	}

	if bl.CanPartialSync(10) {
		t.Fatalf("offset 10 should have been evicted")
	}

	if !bl.CanPartialSync(60) {
		t.Fatalf("offset 60 should be in buffer")
	}

	readData := bl.ReadFromOffset(100)
	if len(readData) != 55 {
		t.Fatalf("expected 55 bytes, got %d", len(readData))
	}
}
