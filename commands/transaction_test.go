package commands

import (
	"strings"
	"sync"
	"testing"

	"github.com/gosuda/nedis/db"
)

func TestTransaction_BasicMultiExec(t *testing.T) {
	database := db.NewShardedDB()
	ctx := &Context{DB: database, Tx: NewTxState()}

	// MULTI
	res := DefaultTable.Execute(ctx, [][]byte{[]byte("MULTI")})
	if string(res) != "+OK\r\n" {
		t.Fatalf("expected +OK, got %q", res)
	}

	// Queue commands
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("SET"), []byte("k1"), []byte("v1")})
	if string(res) != "+QUEUED\r\n" {
		t.Fatalf("expected +QUEUED, got %q", res)
	}

	res = DefaultTable.Execute(ctx, [][]byte{[]byte("INCR"), []byte("counter")})
	if string(res) != "+QUEUED\r\n" {
		t.Fatalf("expected +QUEUED, got %q", res)
	}

	res = DefaultTable.Execute(ctx, [][]byte{[]byte("GET"), []byte("k1")})
	if string(res) != "+QUEUED\r\n" {
		t.Fatalf("expected +QUEUED, got %q", res)
	}

	// EXEC
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("EXEC")})
	expected := "*3\r\n+OK\r\n:1\r\n$2\r\nv1\r\n"
	if string(res) != expected {
		t.Fatalf("expected %q, got %q", expected, res)
	}

	// Verify state after EXEC
	val, ok := database.Get("k1")
	if !ok || val.String() != "v1" {
		t.Fatalf("expected k1=v1 in database")
	}
}

func TestTransaction_Discard(t *testing.T) {
	database := db.NewShardedDB()
	ctx := &Context{DB: database, Tx: NewTxState()}

	_ = DefaultTable.Execute(ctx, [][]byte{[]byte("MULTI")})
	_ = DefaultTable.Execute(ctx, [][]byte{[]byte("SET"), []byte("k1"), []byte("v1")})

	// DISCARD
	res := DefaultTable.Execute(ctx, [][]byte{[]byte("DISCARD")})
	if string(res) != "+OK\r\n" {
		t.Fatalf("expected +OK, got %q", res)
	}

	// Key should not exist
	if database.Exists("k1") {
		t.Fatalf("k1 should not exist after DISCARD")
	}

	// DISCARD without MULTI should error
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("DISCARD")})
	if !strings.Contains(string(res), "-ERR DISCARD without MULTI") {
		t.Fatalf("expected error, got %q", res)
	}
}

func TestTransaction_WatchSuccess(t *testing.T) {
	database := db.NewShardedDB()
	ctx := &Context{DB: database, Tx: NewTxState()}

	_ = database.Set("watched_k", nil)

	// WATCH
	res := DefaultTable.Execute(ctx, [][]byte{[]byte("WATCH"), []byte("watched_k")})
	if string(res) != "+OK\r\n" {
		t.Fatalf("expected +OK, got %q", res)
	}

	_ = DefaultTable.Execute(ctx, [][]byte{[]byte("MULTI")})
	_ = DefaultTable.Execute(ctx, [][]byte{[]byte("SET"), []byte("watched_k"), []byte("val_new")})

	// EXEC (no concurrent modification)
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("EXEC")})
	if !strings.Contains(string(res), "+OK") {
		t.Fatalf("expected success, got %q", res)
	}

	val, ok := database.Get("watched_k")
	if !ok || val.String() != "val_new" {
		t.Fatalf("expected val_new, got %v", val)
	}
}

func TestTransaction_WatchAbortedByOtherClient(t *testing.T) {
	database := db.NewShardedDB()
	clientA := &Context{DB: database, Tx: NewTxState()}
	clientB := &Context{DB: database, Tx: NewTxState()}

	// Client A watches 'key_x'
	_ = DefaultTable.Execute(clientA, [][]byte{[]byte("WATCH"), []byte("key_x")})

	// Client A starts MULTI
	_ = DefaultTable.Execute(clientA, [][]byte{[]byte("MULTI")})
	_ = DefaultTable.Execute(clientA, [][]byte{[]byte("SET"), []byte("key_x"), []byte("client_a_val")})

	// Client B modifies 'key_x' in between
	_ = DefaultTable.Execute(clientB, [][]byte{[]byte("SET"), []byte("key_x"), []byte("client_b_val")})

	// Client A calls EXEC -> should abort and return null array (*-1\r\n)
	res := DefaultTable.Execute(clientA, [][]byte{[]byte("EXEC")})
	if string(res) != "*-1\r\n" {
		t.Fatalf("expected *-1\\r\\n on WATCH collision, got %q", res)
	}

	// Final value should remain client B's value
	val, ok := database.Get("key_x")
	if !ok || val.String() != "client_b_val" {
		t.Fatalf("expected client_b_val, got %v", val)
	}
}

func TestTransaction_ConcurrentMultiShardDeadlockFree(t *testing.T) {
	database := db.NewShardedDB()
	const numGoroutines = 20
	const numTxsPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(clientID int) {
			defer wg.Done()
			ctx := &Context{DB: database, Tx: NewTxState()}

			for j := 0; j < numTxsPerGoroutine; j++ {
				_ = DefaultTable.Execute(ctx, [][]byte{[]byte("MULTI")})
				_ = DefaultTable.Execute(ctx, [][]byte{[]byte("SET"), []byte("shared_k1"), []byte("v1")})
				_ = DefaultTable.Execute(ctx, [][]byte{[]byte("SET"), []byte("shared_k2"), []byte("v2")})
				_ = DefaultTable.Execute(ctx, [][]byte{[]byte("SET"), []byte("shared_k3"), []byte("v3")})
				_ = DefaultTable.Execute(ctx, [][]byte{[]byte("EXEC")})
			}
		}(i)
	}

	wg.Wait()

	// Verify DB is clean and intact
	if !database.Exists("shared_k1") || !database.Exists("shared_k2") || !database.Exists("shared_k3") {
		t.Fatalf("expected keys to exist after concurrent transactions")
	}
}
