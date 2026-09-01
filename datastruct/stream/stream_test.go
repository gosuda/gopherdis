package stream

import (
	"fmt"
	"testing"
	"time"
)

func TestStream_AddAndRange(t *testing.T) {
	s := NewStream()

	// Add 100 entries with common schema
	for i := 0; i < 100; i++ {
		id, err := ParseID("*", s.LastID())
		if err != nil {
			t.Fatalf("parse ID failed: %v", err)
		}
		_, err = s.Add(id, []string{"user_id", "action"}, [][]byte{[]byte(fmt.Sprintf("user_%d", i)), []byte("login")}, 0, false)
		if err != nil {
			t.Fatalf("Add failed: %v", err)
		}
	}

	if s.Len() != 100 {
		t.Fatalf("expected len 100, got %d", s.Len())
	}

	// XRANGE - +
	entries := s.Range(ZeroID, MaxID, 10, false)
	if len(entries) != 10 {
		t.Fatalf("expected 10 entries, got %d", len(entries))
	}
	if entries[0].Fields[0] != "user_id" || string(entries[0].Values[0]) != "user_0" {
		t.Fatalf("first entry mismatch: %v", entries[0])
	}

	// XREVRANGE + -
	revEntries := s.Range(MaxID, ZeroID, 5, true)
	if len(revEntries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(revEntries))
	}
	if string(revEntries[0].Values[0]) != "user_99" {
		t.Fatalf("rev entry mismatch: %s", string(revEntries[0].Values[0]))
	}
}

func TestStream_TrimAndChunkPruning(t *testing.T) {
	s := NewStream()

	for i := 0; i < 1200; i++ {
		id, _ := ParseID("*", s.LastID())
		_, _ = s.Add(id, []string{"k"}, [][]byte{[]byte("v")}, 0, false)
	}

	if s.Len() != 1200 {
		t.Fatalf("expected 1200, got %d", s.Len())
	}

	// Trim approx to 500
	deleted := s.Trim(500, true)
	if s.Len() > 600 {
		t.Fatalf("expected len ~500, got %d (deleted %d)", s.Len(), deleted)
	}
}

func TestStream_ConsumerGroupAndPEL(t *testing.T) {
	s := NewStream()

	// Add 5 messages
	var ids []StreamID
	for i := 0; i < 5; i++ {
		id, _ := ParseID("*", s.LastID())
		id, _ = s.Add(id, []string{"task"}, [][]byte{[]byte(fmt.Sprintf("task_%d", i))}, 0, false)
		ids = append(ids, id)
	}

	// Create Consumer Group
	err := s.CreateGroup("workers", ZeroID)
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}

	// Consumer A reads 2 messages with >
	readA, err := s.ReadGroup("workers", "consumerA", ZeroID, true, 2, false)
	if err != nil || len(readA) != 2 {
		t.Fatalf("expected 2 messages for consumerA, got %d (err %v)", len(readA), err)
	}

	// Consumer B reads 2 messages with >
	readB, err := s.ReadGroup("workers", "consumerB", ZeroID, true, 2, false)
	if err != nil || len(readB) != 2 {
		t.Fatalf("expected 2 messages for consumerB, got %d", len(readB))
	}

	// Check Pending count
	pending := s.Pending("workers", ZeroID, MaxID, 10, "")
	if len(pending) != 4 {
		t.Fatalf("expected 4 pending entries, got %d", len(pending))
	}

	// Consumer A ACKs its first message
	acked := s.Ack("workers", []StreamID{readA[0].ID})
	if acked != 1 {
		t.Fatalf("expected 1 acked, got %d", acked)
	}

	// Pending should now be 3
	pending = s.Pending("workers", ZeroID, MaxID, 10, "")
	if len(pending) != 3 {
		t.Fatalf("expected 3 pending after ACK, got %d", len(pending))
	}

	// Claim Consumer B's message by Consumer C
	claimed := s.Claim("workers", "consumerC", 0, []StreamID{readB[0].ID})
	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed message, got %d", len(claimed))
	}
}

func TestStream_BlockingNotification(t *testing.T) {
	s := NewStream()
	waiter := s.RegisterWaiter()
	defer s.UnregisterWaiter(waiter)

	doneCh := make(chan struct{})
	go func() {
		select {
		case <-waiter.WakeCh:
			close(doneCh)
		case <-time.After(1 * time.Second):
			t.Errorf("waiter timed out")
		}
	}()

	time.Sleep(10 * time.Millisecond)
	id, _ := ParseID("*", s.LastID())
	_, _ = s.Add(id, []string{"f"}, [][]byte{[]byte("v")}, 0, false)

	select {
	case <-doneCh:
		// Successfully awakened
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("blocking waiter was not awakened")
	}
}
