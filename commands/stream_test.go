package commands

import (
	"strings"
	"testing"
	"time"

	"github.com/gosuda/gopherdis/db"
)

func TestStreamCommands(t *testing.T) {
	database := db.NewShardedDB()
	ctx := &Context{DB: database}

	// 1. XADD mystream 1000-0 sensor_id 1 temp 25.5 -> returns $6\r\n1000-0\r\n
	res := DefaultTable.Execute(ctx, [][]byte{
		[]byte("XADD"), []byte("mystream"), []byte("1000-0"),
		[]byte("sensor_id"), []byte("1"), []byte("temp"), []byte("25.5"),
	})
	if string(res) != "$6\r\n1000-0\r\n" {
		t.Fatalf("expected $6\\r\\n1000-0\\r\\n, got %q", res)
	}

	// 2. XADD mystream 1000-1 sensor_id 2 temp 26.0
	res = DefaultTable.Execute(ctx, [][]byte{
		[]byte("XADD"), []byte("mystream"), []byte("1000-1"),
		[]byte("sensor_id"), []byte("2"), []byte("temp"), []byte("26.0"),
	})
	if string(res) != "$6\r\n1000-1\r\n" {
		t.Fatalf("expected $6\\r\\n1000-1\\r\\n, got %q", res)
	}

	// 3. XLEN mystream -> returns :2
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("XLEN"), []byte("mystream")})
	if string(res) != ":2\r\n" {
		t.Fatalf("expected :2, got %q", res)
	}

	// 4. XRANGE mystream - +
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("XRANGE"), []byte("mystream"), []byte("-"), []byte("+")})
	if !strings.HasPrefix(string(res), "*2\r\n") {
		t.Fatalf("expected *2 array from XRANGE, got %q", res)
	}

	// 5. XGROUP CREATE mystream mygroup $
	res = DefaultTable.Execute(ctx, [][]byte{
		[]byte("XGROUP"), []byte("CREATE"), []byte("mystream"), []byte("mygroup"), []byte("0"),
	})
	if string(res) != "+OK\r\n" {
		t.Fatalf("expected +OK on XGROUP CREATE, got %q", res)
	}

	// 6. XREADGROUP GROUP mygroup consumer1 COUNT 1 STREAMS mystream >
	res = DefaultTable.Execute(ctx, [][]byte{
		[]byte("XREADGROUP"), []byte("GROUP"), []byte("mygroup"), []byte("consumer1"),
		[]byte("COUNT"), []byte("1"), []byte("STREAMS"), []byte("mystream"), []byte(">"),
	})
	if !strings.Contains(string(res), "1000-0") {
		t.Fatalf("expected 1000-0 in XREADGROUP reply, got %q", res)
	}

	// 7. XACK mystream mygroup 1000-0
	res = DefaultTable.Execute(ctx, [][]byte{
		[]byte("XACK"), []byte("mystream"), []byte("mygroup"), []byte("1000-0"),
	})
	if string(res) != ":1\r\n" {
		t.Fatalf("expected :1 on XACK, got %q", res)
	}

	// 8. XDEL mystream 1000-1
	res = DefaultTable.Execute(ctx, [][]byte{
		[]byte("XDEL"), []byte("mystream"), []byte("1000-1"),
	})
	if string(res) != ":1\r\n" {
		t.Fatalf("expected :1 on XDEL, got %q", res)
	}

	// 9. XLEN mystream -> now :1
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("XLEN"), []byte("mystream")})
	if string(res) != ":1\r\n" {
		t.Fatalf("expected :1 after XDEL, got %q", res)
	}
}

func TestStream_XReadBlocking(t *testing.T) {
	database := db.NewShardedDB()
	ctxA := &Context{DB: database}
	ctxB := &Context{DB: database}

	doneCh := make(chan []byte)

	// Client A blocks on XREAD with BLOCK 1000ms
	go func() {
		res := DefaultTable.Execute(ctxA, [][]byte{
			[]byte("XREAD"), []byte("BLOCK"), []byte("1000"), []byte("STREAMS"), []byte("event_stream"), []byte("0"),
		})
		doneCh <- res
	}()

	time.Sleep(20 * time.Millisecond)

	// Client B adds entry
	DefaultTable.Execute(ctxB, [][]byte{
		[]byte("XADD"), []byte("event_stream"), []byte("2000-0"), []byte("msg"), []byte("hello"),
	})

	select {
	case res := <-doneCh:
		if !strings.Contains(string(res), "2000-0") {
			t.Fatalf("expected 2000-0 in XREAD block reply, got %q", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("XREAD block timed out")
	}
}
