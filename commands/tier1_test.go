package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gosuda/gopherdis/db"
)

func newCtx() *Context { return &Context{DB: db.NewShardedDB()} }

func run(t *testing.T, ctx *Context, args ...string) string {
	t.Helper()
	argv := make([][]byte, len(args))
	for i, a := range args {
		argv[i] = []byte(a)
	}
	return string(DefaultTable.Execute(ctx, argv))
}

func TestKeyspaceCommands(t *testing.T) {
	ctx := newCtx()
	run(t, ctx, "MSET", "a", "1", "ab", "2", "b", "3")

	if got := run(t, ctx, "KEYS", "a*"); !strings.Contains(got, "*2\r\n") {
		t.Errorf("KEYS a* = %q, want 2 keys", got)
	}
	// A glob star must cross a slash, which path.Match would not do.
	run(t, ctx, "SET", "x/y/z", "v")
	if got := run(t, ctx, "KEYS", "x*z"); !strings.Contains(got, "x/y/z") {
		t.Errorf("KEYS x*z = %q, want the slashed key", got)
	}

	if got := run(t, ctx, "TOUCH", "a", "b", "missing"); got != ":2\r\n" {
		t.Errorf("TOUCH = %q, want :2", got)
	}
	if got := run(t, ctx, "UNLINK", "a", "missing"); got != ":1\r\n" {
		t.Errorf("UNLINK = %q, want :1", got)
	}

	// RENAME carries the TTL across.
	run(t, ctx, "SET", "src", "v", "EX", "100")
	if got := run(t, ctx, "RENAME", "src", "dst"); got != "+OK\r\n" {
		t.Errorf("RENAME = %q", got)
	}
	if got := run(t, ctx, "TTL", "dst"); got == ":-1\r\n" || got == ":-2\r\n" {
		t.Errorf("RENAME lost the TTL: TTL = %q", got)
	}
	if got := run(t, ctx, "RENAME", "nope", "x"); !strings.HasPrefix(got, "-ERR no such key") {
		t.Errorf("RENAME of a missing key = %q", got)
	}

	if got := run(t, ctx, "PERSIST", "dst"); got != ":1\r\n" {
		t.Errorf("PERSIST = %q, want :1", got)
	}
	if got := run(t, ctx, "TTL", "dst"); got != ":-1\r\n" {
		t.Errorf("after PERSIST, TTL = %q, want :-1", got)
	}

	if got := run(t, ctx, "SELECT", "0"); got != "+OK\r\n" {
		t.Errorf("SELECT 0 = %q", got)
	}
	if got := run(t, ctx, "SELECT", "9"); !strings.Contains(got, "out of range") {
		t.Errorf("SELECT 9 should be out of range, got %q", got)
	}
}

func TestScanCoversEveryKey(t *testing.T) {
	ctx := newCtx()
	const n = 500
	want := map[string]bool{}
	for i := 0; i < n; i++ {
		k := "k:" + itoa(i)
		run(t, ctx, "SET", k, "v")
		want[k] = true
	}

	// The coverage guarantee is a property of the cursor, so assert it against
	// the database directly rather than by re-parsing RESP.
	seen := map[string]bool{}
	var cursor uint64
	for iter := 0; ; iter++ {
		next, keys := ctx.DB.Scan(cursor, "", 10)
		for _, k := range keys {
			seen[k] = true
		}
		cursor = next
		if cursor == 0 {
			break
		}
		if iter > 1000 {
			t.Fatal("SCAN cursor did not terminate")
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("SCAN returned %d distinct keys, want %d", len(seen), len(want))
	}
	for k := range want {
		if !seen[k] {
			t.Fatalf("SCAN missed key %q", k)
		}
	}

	_, filtered := ctx.DB.Scan(0, "k:1", 1000)
	if len(filtered) != 1 || filtered[0] != "k:1" {
		t.Errorf("SCAN MATCH k:1 returned %v", filtered)
	}
	if got := run(t, ctx, "SCAN", "0", "COUNT", "5"); !strings.HasPrefix(got, "*2\r\n") {
		t.Errorf("SCAN reply shape = %q", got)
	}
	if got := run(t, ctx, "SCAN", "notanumber"); !strings.Contains(got, "invalid cursor") {
		t.Errorf("SCAN with a bad cursor = %q", got)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

func TestSetOptionsAndStringCommands(t *testing.T) {
	ctx := newCtx()

	if got := run(t, ctx, "SET", "k", "v1", "NX"); got != "+OK\r\n" {
		t.Errorf("SET NX on a new key = %q", got)
	}
	if got := run(t, ctx, "SET", "k", "v2", "NX"); got != "$-1\r\n" {
		t.Errorf("SET NX on an existing key = %q, want nil", got)
	}
	if got := run(t, ctx, "GET", "k"); !strings.Contains(got, "v1") {
		t.Errorf("SET NX overwrote the value: %q", got)
	}
	if got := run(t, ctx, "SET", "k", "v3", "XX", "GET"); !strings.Contains(got, "v1") {
		t.Errorf("SET XX GET = %q, want the old value", got)
	}
	if got := run(t, ctx, "SET", "missing", "v", "XX"); got != "$-1\r\n" {
		t.Errorf("SET XX on a missing key = %q", got)
	}

	// KEEPTTL must not clear an existing expiry.
	run(t, ctx, "SET", "t", "v", "EX", "100")
	run(t, ctx, "SET", "t", "v2", "KEEPTTL")
	if got := run(t, ctx, "TTL", "t"); got == ":-1\r\n" {
		t.Errorf("KEEPTTL dropped the TTL")
	}
	run(t, ctx, "SET", "t", "v3")
	if got := run(t, ctx, "TTL", "t"); got != ":-1\r\n" {
		t.Errorf("plain SET should clear the TTL, got %q", got)
	}

	if got := run(t, ctx, "SETNX", "fresh", "v"); got != ":1\r\n" {
		t.Errorf("SETNX = %q", got)
	}
	if got := run(t, ctx, "SETNX", "fresh", "v2"); got != ":0\r\n" {
		t.Errorf("SETNX on existing = %q", got)
	}
	if got := run(t, ctx, "GETDEL", "fresh"); !strings.Contains(got, "v") {
		t.Errorf("GETDEL = %q", got)
	}
	if got := run(t, ctx, "EXISTS", "fresh"); got != ":0\r\n" {
		t.Errorf("GETDEL did not delete the key")
	}
	if got := run(t, ctx, "GETSET", "gs", "new"); got != "$-1\r\n" {
		t.Errorf("GETSET on a missing key = %q", got)
	}
}

func TestListOps(t *testing.T) {
	ctx := newCtx()
	run(t, ctx, "RPUSH", "l", "a", "b", "c", "b")

	if got := run(t, ctx, "LINSERT", "l", "BEFORE", "b", "X"); got != ":5\r\n" {
		t.Errorf("LINSERT = %q, want :5", got)
	}
	if got := run(t, ctx, "LRANGE", "l", "0", "-1"); !strings.Contains(got, "X") {
		t.Errorf("LINSERT did not insert: %q", got)
	}
	if got := run(t, ctx, "LINSERT", "l", "BEFORE", "zzz", "X"); got != ":-1\r\n" {
		t.Errorf("LINSERT with a missing pivot = %q, want :-1", got)
	}
	if got := run(t, ctx, "LREM", "l", "0", "b"); got != ":2\r\n" {
		t.Errorf("LREM = %q, want :2", got)
	}
	if got := run(t, ctx, "LTRIM", "l", "0", "0"); got != "+OK\r\n" {
		t.Errorf("LTRIM = %q", got)
	}
	if got := run(t, ctx, "LLEN", "l"); got != ":1\r\n" {
		t.Errorf("after LTRIM, LLEN = %q, want :1", got)
	}

	run(t, ctx, "RPUSH", "src", "1", "2", "3")
	if got := run(t, ctx, "RPOPLPUSH", "src", "dst"); !strings.Contains(got, "3") {
		t.Errorf("RPOPLPUSH = %q", got)
	}
	if got := run(t, ctx, "LRANGE", "dst", "0", "-1"); !strings.Contains(got, "3") {
		t.Errorf("RPOPLPUSH did not push: %q", got)
	}
	if got := run(t, ctx, "LPUSHX", "nosuchlist", "v"); got != ":0\r\n" {
		t.Errorf("LPUSHX must not create the key, got %q", got)
	}
	if got := run(t, ctx, "LPOS", "dst", "3"); got != ":0\r\n" {
		t.Errorf("LPOS = %q, want :0", got)
	}
}

func TestSetOps(t *testing.T) {
	ctx := newCtx()
	run(t, ctx, "SADD", "s1", "a", "b", "c")
	run(t, ctx, "SADD", "s2", "b", "c", "d")

	if got := run(t, ctx, "SMISMEMBER", "s1", "a", "z"); got != "*2\r\n:1\r\n:0\r\n" {
		t.Errorf("SMISMEMBER = %q", got)
	}
	if got := run(t, ctx, "SINTER", "s1", "s2"); !strings.HasPrefix(got, "*2\r\n") {
		t.Errorf("SINTER = %q, want 2 members", got)
	}
	if got := run(t, ctx, "SUNION", "s1", "s2"); !strings.HasPrefix(got, "*4\r\n") {
		t.Errorf("SUNION = %q, want 4 members", got)
	}
	if got := run(t, ctx, "SDIFF", "s1", "s2"); !strings.HasPrefix(got, "*1\r\n") {
		t.Errorf("SDIFF = %q, want 1 member", got)
	}
	if got := run(t, ctx, "SINTERSTORE", "dst", "s1", "s2"); got != ":2\r\n" {
		t.Errorf("SINTERSTORE = %q", got)
	}
	if got := run(t, ctx, "SCARD", "dst"); got != ":2\r\n" {
		t.Errorf("SINTERSTORE destination = %q", got)
	}
	if got := run(t, ctx, "SINTERCARD", "2", "s1", "s2"); got != ":2\r\n" {
		t.Errorf("SINTERCARD = %q", got)
	}
	if got := run(t, ctx, "SINTERCARD", "2", "s1", "s2", "LIMIT", "1"); got != ":1\r\n" {
		t.Errorf("SINTERCARD LIMIT = %q", got)
	}
	if got := run(t, ctx, "SMOVE", "s1", "s2", "a"); got != ":1\r\n" {
		t.Errorf("SMOVE = %q", got)
	}
	if got := run(t, ctx, "SISMEMBER", "s2", "a"); got != ":1\r\n" {
		t.Errorf("SMOVE did not land: %q", got)
	}
}

func TestCopyIsDeep(t *testing.T) {
	ctx := newCtx()
	run(t, ctx, "RPUSH", "l", "a")
	if got := run(t, ctx, "COPY", "l", "l2"); got != ":1\r\n" {
		t.Fatalf("COPY = %q", got)
	}
	// Mutating the copy must not be visible through the original.
	run(t, ctx, "RPUSH", "l2", "b")
	if got := run(t, ctx, "LLEN", "l"); got != ":1\r\n" {
		t.Errorf("COPY aliased the payload: source LLEN = %q", got)
	}
	if got := run(t, ctx, "COPY", "l", "l2"); got != ":0\r\n" {
		t.Errorf("COPY without REPLACE onto an existing key = %q", got)
	}
	if got := run(t, ctx, "COPY", "l", "l2", "REPLACE"); got != ":1\r\n" {
		t.Errorf("COPY REPLACE = %q", got)
	}
}

func TestDebugSetActiveExpire(t *testing.T) {
	ctx := newCtx()
	if got := run(t, ctx, "DEBUG", "SET-ACTIVE-EXPIRE", "0"); got != "+OK\r\n" {
		t.Errorf("DEBUG SET-ACTIVE-EXPIRE 0 = %q", got)
	}
	if got := run(t, ctx, "DEBUG", "STRINGMATCH-LEN", "a*c", "a/b/c"); got != ":1\r\n" {
		t.Errorf("DEBUG STRINGMATCH-LEN = %q, want :1", got)
	}
	if !bytes.Equal([]byte(run(t, ctx, "DEBUG", "JMAP")), []byte("+OK\r\n")) {
		t.Error("DEBUG JMAP should be accepted")
	}
}

func TestExecAbortOnQueueError(t *testing.T) {
	ctx := &Context{DB: db.NewShardedDB(), Tx: NewTxState()}

	run(t, ctx, "MULTI")
	run(t, ctx, "SET", "k", "v")
	// An unknown command cannot be queued and must poison the transaction.
	if got := run(t, ctx, "NOSUCHCOMMAND"); !strings.HasPrefix(got, "-ERR unknown command") {
		t.Fatalf("queueing an unknown command = %q", got)
	}
	got := run(t, ctx, "EXEC")
	if !strings.HasPrefix(got, "-EXECABORT ") {
		t.Fatalf("EXEC after a queue error = %q, want EXECABORT", got)
	}
	if run(t, ctx, "GET", "k") != "$-1\r\n" {
		t.Error("an aborted transaction must not have applied its commands")
	}
	// MULTI state has to be cleared, so a plain command works again.
	if got := run(t, ctx, "SET", "after", "v"); got != "+OK\r\n" {
		t.Errorf("client still in MULTI after abort: %q", got)
	}

	// A WATCH conflict is a different outcome: nil array, not EXECABORT.
	ctx2 := &Context{DB: ctx.DB, Tx: NewTxState()}
	other := &Context{DB: ctx.DB}
	run(t, ctx2, "SET", "w", "1")
	run(t, ctx2, "WATCH", "w")
	run(t, other, "SET", "w", "2")
	run(t, ctx2, "MULTI")
	run(t, ctx2, "GET", "w")
	if got := run(t, ctx2, "EXEC"); got != "*-1\r\n" {
		t.Errorf("EXEC after a WATCH conflict = %q, want a nil array", got)
	}
}

func TestExpireOverflowIsRejected(t *testing.T) {
	ctx := newCtx()
	run(t, ctx, "SET", "foo", "bar")

	// Seconds that overflow when converted to a millisecond deadline.
	if got := run(t, ctx, "EXPIRE", "foo", "9223370399119966"); !strings.Contains(got, "invalid expire time") {
		t.Errorf("EXPIRE with a huge value = %q, want an invalid expire error", got)
	}
	if got := run(t, ctx, "SET", "foo", "bar", "EX", "9999999999999999"); !strings.Contains(got, "invalid expire time") {
		t.Errorf("SET EX with a huge value = %q", got)
	}
	if got := run(t, ctx, "GETEX", "foo", "EX", "9999999999999999"); !strings.Contains(got, "invalid expire time") {
		t.Errorf("GETEX with a huge value = %q", got)
	}
	if got := run(t, ctx, "GETEX", "foo", "EX", "-1"); !strings.Contains(got, "invalid expire time") {
		t.Errorf("GETEX with a negative value = %q", got)
	}
	// A sane TTL still works.
	if got := run(t, ctx, "EXPIRE", "foo", "100"); got != ":1\r\n" {
		t.Errorf("ordinary EXPIRE = %q", got)
	}
}

func TestZsetRangeAndPop(t *testing.T) {
	ctx := newCtx()
	run(t, ctx, "ZADD", "z", "1", "a", "2", "b", "3", "c")

	if got := run(t, ctx, "ZRANGEBYSCORE", "z", "1", "2"); !strings.HasPrefix(got, "*2\r\n") {
		t.Errorf("ZRANGEBYSCORE 1 2 = %q, want 2 members", got)
	}
	if got := run(t, ctx, "ZRANGEBYSCORE", "z", "(1", "3"); !strings.HasPrefix(got, "*2\r\n") {
		t.Errorf("exclusive min = %q, want 2 members", got)
	}
	if got := run(t, ctx, "ZRANGEBYSCORE", "z", "-inf", "+inf"); !strings.HasPrefix(got, "*3\r\n") {
		t.Errorf("infinite bounds = %q, want 3 members", got)
	}
	if got := run(t, ctx, "ZRANGEBYSCORE", "z", "-inf", "+inf", "LIMIT", "1", "1"); !strings.HasPrefix(got, "*1\r\n") {
		t.Errorf("LIMIT = %q, want 1 member", got)
	}
	if got := run(t, ctx, "ZPOPMIN", "z"); !strings.Contains(got, "a") {
		t.Errorf("ZPOPMIN = %q, want member a", got)
	}
	if got := run(t, ctx, "ZPOPMAX", "z"); !strings.Contains(got, "c") {
		t.Errorf("ZPOPMAX = %q, want member c", got)
	}
	if got := run(t, ctx, "ZCARD", "z"); got != ":1\r\n" {
		t.Errorf("after pops, ZCARD = %q", got)
	}

	run(t, ctx, "ZADD", "z2", "1", "a", "2", "b", "3", "c")
	if got := run(t, ctx, "ZREMRANGEBYSCORE", "z2", "1", "2"); got != ":2\r\n" {
		t.Errorf("ZREMRANGEBYSCORE = %q", got)
	}
	if got := run(t, ctx, "ZMSCORE", "z2", "c", "gone"); !strings.Contains(got, "$-1") {
		t.Errorf("ZMSCORE should report a nil for a missing member: %q", got)
	}
}

func TestSortCommand(t *testing.T) {
	ctx := newCtx()
	run(t, ctx, "RPUSH", "nums", "3", "1", "2")

	if got := run(t, ctx, "SORT", "nums"); !strings.HasPrefix(got, "*3\r\n$1\r\n1\r\n$1\r\n2\r\n$1\r\n3") {
		t.Errorf("SORT = %q, want ascending", got)
	}
	if got := run(t, ctx, "SORT", "nums", "DESC"); !strings.Contains(got, "$1\r\n3\r\n$1\r\n2") {
		t.Errorf("SORT DESC = %q", got)
	}
	if got := run(t, ctx, "SORT", "nums", "LIMIT", "0", "2"); !strings.HasPrefix(got, "*2\r\n") {
		t.Errorf("SORT LIMIT = %q", got)
	}

	run(t, ctx, "RPUSH", "words", "banana", "apple")
	if got := run(t, ctx, "SORT", "words", "ALPHA"); !strings.Contains(got, "apple") {
		t.Errorf("SORT ALPHA = %q", got)
	}
	if got := run(t, ctx, "SORT", "words"); !strings.Contains(got, "can't be converted") {
		t.Errorf("numeric SORT of non-numbers should error, got %q", got)
	}

	// BY with an external weight, and STORE.
	run(t, ctx, "MSET", "w_1", "10", "w_2", "5", "w_3", "1")
	if got := run(t, ctx, "SORT", "nums", "BY", "w_*"); !strings.Contains(got, "$1\r\n3\r\n$1\r\n2\r\n$1\r\n1") {
		t.Errorf("SORT BY = %q, want weight order 3,2,1", got)
	}
	if got := run(t, ctx, "SORT", "nums", "STORE", "dst"); got != ":3\r\n" {
		t.Errorf("SORT STORE = %q", got)
	}
	if got := run(t, ctx, "LLEN", "dst"); got != ":3\r\n" {
		t.Errorf("SORT STORE destination = %q", got)
	}
}

func TestHashOps(t *testing.T) {
	ctx := newCtx()
	run(t, ctx, "HSET", "h", "f", "value")

	if got := run(t, ctx, "HSETNX", "h", "f", "other"); got != ":0\r\n" {
		t.Errorf("HSETNX on an existing field = %q", got)
	}
	if got := run(t, ctx, "HSETNX", "h", "new", "v"); got != ":1\r\n" {
		t.Errorf("HSETNX on a new field = %q", got)
	}
	if got := run(t, ctx, "HSTRLEN", "h", "f"); got != ":5\r\n" {
		t.Errorf("HSTRLEN = %q, want :5", got)
	}
	if got := run(t, ctx, "HSTRLEN", "h", "missing"); got != ":0\r\n" {
		t.Errorf("HSTRLEN on a missing field = %q", got)
	}
	if got := run(t, ctx, "HRANDFIELD", "h", "-5"); !strings.HasPrefix(got, "*5\r\n") {
		t.Errorf("HRANDFIELD with a negative count = %q, want 5 entries", got)
	}
}
