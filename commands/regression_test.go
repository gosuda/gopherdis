package commands

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gosuda/gopherdis/db"
	"github.com/gosuda/gopherdis/object"
	"github.com/gosuda/gopherdis/scripting"
)

// TestConcurrentIncrIsAtomic exercises the lost-update window in the
// Get -> compute -> Set sequence shared by INCR, HINCRBY and friends.
func TestConcurrentIncrIsAtomic(t *testing.T) {
	database := db.NewShardedDB()

	const goroutines, perGoroutine = 16, 500
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := &Context{DB: database}
			for i := 0; i < perGoroutine; i++ {
				DefaultTable.Execute(ctx, [][]byte{[]byte("INCR"), []byte("counter")})
			}
		}()
	}
	wg.Wait()

	want := fmt.Sprintf(":%d\r\n", goroutines*perGoroutine)
	got := DefaultTable.Execute(&Context{DB: database}, [][]byte{[]byte("GET"), []byte("counter")})
	if string(got) != fmt.Sprintf("$%d\r\n%d\r\n", len(fmt.Sprint(goroutines*perGoroutine)), goroutines*perGoroutine) {
		t.Fatalf("lost updates: want total %s, got %q", strings.TrimSpace(want), got)
	}
}

func TestConcurrentHincrbyIsAtomic(t *testing.T) {
	database := db.NewShardedDB()

	const goroutines, perGoroutine = 16, 300
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := &Context{DB: database}
			for i := 0; i < perGoroutine; i++ {
				DefaultTable.Execute(ctx, [][]byte{[]byte("HINCRBY"), []byte("h"), []byte("f"), []byte("1")})
			}
		}()
	}
	wg.Wait()

	got := DefaultTable.Execute(&Context{DB: database}, [][]byte{[]byte("HGET"), []byte("h"), []byte("f")})
	want := fmt.Sprint(goroutines * perGoroutine)
	if !bytes.Contains(got, []byte(want)) {
		t.Fatalf("lost updates: want %s, got %q", want, got)
	}
}

// TestWatchDetectsInPlaceMutation covers WATCH against container commands, which
// mutate the payload without ever calling DB.Set or DB.Del.
func TestWatchDetectsInPlaceMutation(t *testing.T) {
	// Each case pre-creates the key. Mutating a missing key goes through
	// getOrCreate* -> DB.Set, which bumps the version for an unrelated reason; the
	// bug is the mutation of an existing container.
	for _, tc := range []struct {
		name    string
		prepare [][]byte
		mutate  [][]byte
	}{
		{"hset",
			[][]byte{[]byte("HSET"), []byte("k"), []byte("f0"), []byte("v0")},
			[][]byte{[]byte("HSET"), []byte("k"), []byte("f"), []byte("v")}},
		{"lpush",
			[][]byte{[]byte("LPUSH"), []byte("k"), []byte("v0")},
			[][]byte{[]byte("LPUSH"), []byte("k"), []byte("v")}},
		{"sadd",
			[][]byte{[]byte("SADD"), []byte("k"), []byte("m0")},
			[][]byte{[]byte("SADD"), []byte("k"), []byte("m")}},
		{"zadd",
			[][]byte{[]byte("ZADD"), []byte("k"), []byte("1"), []byte("m0")},
			[][]byte{[]byte("ZADD"), []byte("k"), []byte("2"), []byte("m")}},
		{"xadd",
			[][]byte{[]byte("XADD"), []byte("k"), []byte("1-1"), []byte("f"), []byte("v")},
			[][]byte{[]byte("XADD"), []byte("k"), []byte("2-1"), []byte("f"), []byte("v")}},
		{"hincrby",
			[][]byte{[]byte("HSET"), []byte("k"), []byte("n"), []byte("1")},
			[][]byte{[]byte("HINCRBY"), []byte("k"), []byte("n"), []byte("1")}},
		{"setbit",
			[][]byte{[]byte("SET"), []byte("k"), []byte("ab")},
			[][]byte{[]byte("SETBIT"), []byte("k"), []byte("7"), []byte("1")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			database := db.NewShardedDB()
			watcher := &Context{DB: database, Tx: NewTxState()}
			other := &Context{DB: database}

			if tc.prepare != nil {
				DefaultTable.Execute(other, tc.prepare)
			}

			DefaultTable.Execute(watcher, [][]byte{[]byte("WATCH"), []byte("k")})
			DefaultTable.Execute(other, tc.mutate)
			DefaultTable.Execute(watcher, [][]byte{[]byte("MULTI")})
			DefaultTable.Execute(watcher, [][]byte{[]byte("GET"), []byte("k")})

			res := DefaultTable.Execute(watcher, [][]byte{[]byte("EXEC")})
			if !bytes.Equal(res, []byte("*-1\r\n")) {
				t.Fatalf("EXEC should abort after %s touched the watched key, got %q", tc.name, res)
			}
		})
	}
}

func TestFlushAllAbortsWatch(t *testing.T) {
	database := db.NewShardedDB()
	watcher := &Context{DB: database, Tx: NewTxState()}
	other := &Context{DB: database}

	DefaultTable.Execute(other, [][]byte{[]byte("SET"), []byte("k"), []byte("v")})
	DefaultTable.Execute(watcher, [][]byte{[]byte("WATCH"), []byte("k")})
	DefaultTable.Execute(other, [][]byte{[]byte("FLUSHALL")})
	DefaultTable.Execute(watcher, [][]byte{[]byte("MULTI")})
	DefaultTable.Execute(watcher, [][]byte{[]byte("GET"), []byte("k")})

	if res := DefaultTable.Execute(watcher, [][]byte{[]byte("EXEC")}); !bytes.Equal(res, []byte("*-1\r\n")) {
		t.Fatalf("EXEC should abort after FLUSHALL removed the watched key, got %q", res)
	}
}

// TestReadCommandsDoNotCreateKeys guards against phantom keys from read paths.
func TestReadCommandsDoNotCreateKeys(t *testing.T) {
	reads := [][][]byte{
		{[]byte("GEODIST"), []byte("nope"), []byte("a"), []byte("b")},
		{[]byte("GEOPOS"), []byte("nope"), []byte("a")},
		{[]byte("GEOHASH"), []byte("nope"), []byte("a")},
		{[]byte("GEORADIUS"), []byte("nope"), []byte("0"), []byte("0"), []byte("1"), []byte("km")},
		{[]byte("GEORADIUSBYMEMBER"), []byte("nope"), []byte("a"), []byte("1"), []byte("km")},
		// Only the BLOCK path used to create the stream.
		{[]byte("XREAD"), []byte("BLOCK"), []byte("50"), []byte("STREAMS"), []byte("nope"), []byte("$")},
	}
	for _, argv := range reads {
		t.Run(string(argv[0]), func(t *testing.T) {
			database := db.NewShardedDB()
			DefaultTable.Execute(&Context{DB: database}, argv)
			if n := database.Len(); n != 0 {
				t.Fatalf("%s on a missing key created it: DBSIZE=%d", argv[0], n)
			}
		})
	}
}

// TestPfaddPersistsOverInvalidBlob covers the case where the stored value is not
// a usable dense HLL: FromBytes hands back a detached copy, so the update has to
// be written back explicitly.
func TestPfaddPersistsOverInvalidBlob(t *testing.T) {
	database := db.NewShardedDB()
	ctx := &Context{DB: database}

	_ = database.Set("hll", object.CreateRawStringObject([]byte("too-short")))

	if res := DefaultTable.Execute(ctx, [][]byte{[]byte("PFADD"), []byte("hll"), []byte("x")}); !bytes.Equal(res, []byte(":1\r\n")) {
		t.Fatalf("expected PFADD to report a change, got %q", res)
	}
	res := DefaultTable.Execute(ctx, [][]byte{[]byte("PFCOUNT"), []byte("hll")})
	if bytes.Equal(res, []byte(":0\r\n")) {
		t.Fatalf("PFADD reported :1 but persisted nothing: PFCOUNT=%q", res)
	}
}

// TestErrorCodesAreNotDoublePrefixed checks the RESP error prefix handling.
func TestErrorCodesAreNotDoublePrefixed(t *testing.T) {
	database := db.NewShardedDB()
	ctx := &Context{DB: database}

	DefaultTable.Execute(ctx, [][]byte{[]byte("LPUSH"), []byte("mylist"), []byte("v")})
	res := DefaultTable.Execute(ctx, [][]byte{[]byte("GET"), []byte("mylist")})
	if !bytes.HasPrefix(res, []byte("-WRONGTYPE ")) {
		t.Fatalf("want a -WRONGTYPE reply, got %q", res)
	}

	if res := Error("some plain message"); !bytes.Equal(res, []byte("-ERR some plain message\r\n")) {
		t.Fatalf("plain messages still need the ERR prefix, got %q", res)
	}
	if res := Error("ERR already prefixed"); !bytes.Equal(res, []byte("-ERR already prefixed\r\n")) {
		t.Fatalf("want no doubled prefix, got %q", res)
	}
	if res := Error("BITOP NOT takes exactly one source key"); !bytes.HasPrefix(res, []byte("-ERR BITOP")) {
		t.Fatalf("a command name is not an error code, got %q", res)
	}
}

func TestIncrOverflow(t *testing.T) {
	database := db.NewShardedDB()
	ctx := &Context{DB: database}

	DefaultTable.Execute(ctx, [][]byte{[]byte("SET"), []byte("n"), []byte("9223372036854775807")})
	res := DefaultTable.Execute(ctx, [][]byte{[]byte("INCR"), []byte("n")})
	if !bytes.HasPrefix(res, []byte("-ERR increment or decrement would overflow")) {
		t.Fatalf("want an overflow error, got %q", res)
	}

	DefaultTable.Execute(ctx, [][]byte{[]byte("SET"), []byte("m"), []byte("0")})
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("DECRBY"), []byte("m"), []byte("-9223372036854775808")})
	if !bytes.HasPrefix(res, []byte("-ERR ")) {
		t.Fatalf("want an overflow error for DECRBY math.MinInt64, got %q", res)
	}
}

// TestXaddMinIDTrimsByID covers MINID, which used to be parsed as a MAXLEN count.
func TestXaddMinIDTrimsByID(t *testing.T) {
	database := db.NewShardedDB()
	ctx := &Context{DB: database}

	for _, id := range []string{"100-1", "200-1", "300-1"} {
		DefaultTable.Execute(ctx, [][]byte{[]byte("XADD"), []byte("s"), []byte(id), []byte("f"), []byte("v")})
	}
	// MINID 300 keeps only entries with ID >= 300-0. Read as MAXLEN 300 it would
	// keep everything instead.
	DefaultTable.Execute(ctx, [][]byte{
		[]byte("XADD"), []byte("s"), []byte("MINID"), []byte("300"), []byte("400-1"), []byte("f"), []byte("v"),
	})

	res := DefaultTable.Execute(ctx, [][]byte{[]byte("XLEN"), []byte("s")})
	if !bytes.Equal(res, []byte(":2\r\n")) {
		t.Fatalf("MINID should have left 2 entries, got XLEN=%q", res)
	}
}

// TestEvalDoesNotFreezeServer covers the unbounded script execution that used to
// hold the exclusive transaction lock forever.
func TestEvalDoesNotFreezeServer(t *testing.T) {
	old := scripting.ScriptTimeout
	scripting.ScriptTimeout = 200 * time.Millisecond
	defer func() { scripting.ScriptTimeout = old }()

	database := db.NewShardedDB()
	ctx := &Context{DB: database, Scripting: scripting.NewEngine()}

	done := make(chan []byte, 1)
	go func() {
		done <- DefaultTable.Execute(ctx, [][]byte{
			[]byte("EVAL"), []byte("while true do end"), []byte("0"),
		})
	}()

	select {
	case res := <-done:
		if !bytes.HasPrefix(res, []byte("-BUSY ")) {
			t.Fatalf("want a BUSY reply from the aborted script, got %q", res)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("runaway script was never interrupted")
	}

	// The database must be usable again afterwards.
	if res := DefaultTable.Execute(&Context{DB: database}, [][]byte{[]byte("PING")}); !bytes.Equal(res, []byte("+PONG\r\n")) {
		t.Fatalf("server did not recover after the script aborted: %q", res)
	}
}
