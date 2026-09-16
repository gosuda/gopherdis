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

// TestRandCountOverflow covers a crash: negating a count of math.MinInt64
// overflows, and the allocation that followed took the process down.
func TestRandCountOverflow(t *testing.T) {
	ctx := newCtx()
	run(t, ctx, "HSET", "h", "f", "v")
	run(t, ctx, "SADD", "s", "m")
	run(t, ctx, "ZADD", "z", "1", "m")

	const minInt64 = "-9223372036854775808"
	for _, c := range [][]string{
		{"HRANDFIELD", "h", minInt64},
		{"SRANDMEMBER", "s", minInt64},
		{"ZRANDMEMBER", "z", minInt64},
	} {
		if got := run(t, ctx, c...); !strings.Contains(got, "value is out of range") {
			t.Errorf("%s with the smallest int64 = %q, want an out of range error", c[0], got)
		}
	}
	// A merely large count is refused rather than attempted.
	if got := run(t, ctx, "HRANDFIELD", "h", "-999999999"); !strings.Contains(got, "value is out of range") {
		t.Errorf("HRANDFIELD with a huge count = %q", got)
	}
	// Ordinary negative counts still work.
	if got := run(t, ctx, "HRANDFIELD", "h", "-3"); !strings.HasPrefix(got, "*3\r\n") {
		t.Errorf("HRANDFIELD -3 = %q, want 3 entries", got)
	}
}

func TestHashEncodingTransitions(t *testing.T) {
	ctx := newCtx()

	// A new hash starts compact.
	run(t, ctx, "HSET", "h", "f", "v")
	if got := run(t, ctx, "OBJECT", "ENCODING", "h"); !strings.Contains(got, "listpack") {
		t.Fatalf("a small hash should be listpack encoded, got %q", got)
	}

	// Crossing the entry threshold promotes it, and the data survives.
	for i := 0; i < encodingEntriesProbe; i++ {
		run(t, ctx, "HSET", "h", "f"+itoa(i), "v"+itoa(i))
	}
	if got := run(t, ctx, "OBJECT", "ENCODING", "h"); !strings.Contains(got, "hashtable") {
		t.Fatalf("a large hash should be hashtable encoded, got %q", got)
	}
	if got := run(t, ctx, "HGET", "h", "f5"); !strings.Contains(got, "v5") {
		t.Errorf("promotion lost data: HGET f5 = %q", got)
	}
	if got := run(t, ctx, "HLEN", "h"); !strings.Contains(got, itoa(encodingEntriesProbe+1)) {
		t.Errorf("promotion changed the field count: %q", got)
	}

	// A single oversized value also forces promotion.
	run(t, ctx, "HSET", "big", "f", strings.Repeat("x", 100))
	if got := run(t, ctx, "OBJECT", "ENCODING", "big"); !strings.Contains(got, "hashtable") {
		t.Errorf("an oversized value should force hashtable, got %q", got)
	}

	// Deleting back down does not demote, matching Redis.
	for i := 0; i < encodingEntriesProbe; i++ {
		run(t, ctx, "HDEL", "h", "f"+itoa(i))
	}
	if got := run(t, ctx, "OBJECT", "ENCODING", "h"); !strings.Contains(got, "hashtable") {
		t.Errorf("encoding should not convert back, got %q", got)
	}
}

const encodingEntriesProbe = 200

func TestSetEncodingTransitions(t *testing.T) {
	ctx := newCtx()

	// All-integer members use intset.
	run(t, ctx, "SADD", "ints", "1", "2", "3")
	if got := run(t, ctx, "OBJECT", "ENCODING", "ints"); !strings.Contains(got, "intset") {
		t.Fatalf("an all-integer set should be intset, got %q", got)
	}
	// Members come back as integers, not as whatever was typed.
	if got := run(t, ctx, "SISMEMBER", "ints", "2"); got != ":1\r\n" {
		t.Errorf("SISMEMBER on an intset = %q", got)
	}

	// A non-integer member converts to listpack and keeps everything.
	run(t, ctx, "SADD", "ints", "abc")
	if got := run(t, ctx, "OBJECT", "ENCODING", "ints"); !strings.Contains(got, "listpack") {
		t.Fatalf("a mixed set should be listpack, got %q", got)
	}
	if got := run(t, ctx, "SCARD", "ints"); got != ":4\r\n" {
		t.Errorf("conversion lost members: SCARD = %q", got)
	}
	for _, m := range []string{"1", "2", "3", "abc"} {
		if got := run(t, ctx, "SISMEMBER", "ints", m); got != ":1\r\n" {
			t.Errorf("member %q lost across conversion: %q", m, got)
		}
	}

	// An oversized member goes straight to a hash table.
	run(t, ctx, "SADD", "big", strings.Repeat("x", 100))
	if got := run(t, ctx, "OBJECT", "ENCODING", "big"); !strings.Contains(got, "hashtable") {
		t.Errorf("an oversized member should force hashtable, got %q", got)
	}

	// Crossing the entry threshold converts too.
	run(t, ctx, "CONFIG", "SET", "set-max-listpack-entries", "3")
	run(t, ctx, "SADD", "t", "a", "b")
	if got := run(t, ctx, "OBJECT", "ENCODING", "t"); !strings.Contains(got, "listpack") {
		t.Errorf("below the threshold should stay listpack, got %q", got)
	}
	run(t, ctx, "SADD", "t", "c", "d")
	if got := run(t, ctx, "OBJECT", "ENCODING", "t"); !strings.Contains(got, "hashtable") {
		t.Errorf("above the threshold should be hashtable, got %q", got)
	}
	if got := run(t, ctx, "SCARD", "t"); got != ":4\r\n" {
		t.Errorf("SCARD after conversion = %q", got)
	}
	run(t, ctx, "CONFIG", "SET", "set-max-listpack-entries", "128")
}

// TestSetAlgebraAcrossEncodings checks the set operations still work when the
// operands are stored differently from each other.
func TestSetAlgebraAcrossEncodings(t *testing.T) {
	ctx := newCtx()
	run(t, ctx, "SADD", "a", "1", "2", "3")            // intset
	run(t, ctx, "SADD", "b", "2", "3", "x")            // listpack
	run(t, ctx, "SADD", "c", strings.Repeat("y", 100)) // hashtable
	run(t, ctx, "SADD", "c", "3")

	if got := run(t, ctx, "SINTER", "a", "b"); !strings.HasPrefix(got, "*2\r\n") {
		t.Errorf("SINTER across intset and listpack = %q, want 2", got)
	}
	if got := run(t, ctx, "SINTER", "a", "c"); !strings.HasPrefix(got, "*1\r\n") {
		t.Errorf("SINTER across intset and hashtable = %q, want 1", got)
	}
	if got := run(t, ctx, "SUNION", "a", "b"); !strings.HasPrefix(got, "*4\r\n") {
		t.Errorf("SUNION = %q, want 4", got)
	}
	if got := run(t, ctx, "SDIFF", "a", "b"); !strings.HasPrefix(got, "*1\r\n") {
		t.Errorf("SDIFF = %q, want 1", got)
	}
	if got := run(t, ctx, "SMOVE", "a", "b", "1"); got != ":1\r\n" {
		t.Errorf("SMOVE across encodings = %q", got)
	}
	if got := run(t, ctx, "SISMEMBER", "b", "1"); got != ":1\r\n" {
		t.Errorf("SMOVE did not land: %q", got)
	}
}

func TestZsetEncodingTransitions(t *testing.T) {
	ctx := newCtx()
	run(t, ctx, "ZADD", "z", "3", "c", "1", "a", "2", "b")

	if got := run(t, ctx, "OBJECT", "ENCODING", "z"); !strings.Contains(got, "listpack") {
		t.Fatalf("a small sorted set should be listpack, got %q", got)
	}
	// The listpack is kept in score order, so rank queries work without a
	// skiplist behind them.
	if got := run(t, ctx, "ZRANGE", "z", "0", "-1"); !strings.Contains(got, "a\r\n$1\r\nb\r\n$1\r\nc") {
		t.Errorf("listpack ordering wrong: %q", got)
	}
	if got := run(t, ctx, "ZRANK", "z", "b"); got != ":1\r\n" {
		t.Errorf("ZRANK on a listpack = %q, want :1", got)
	}
	// Re-scoring has to move the member, not just edit it in place.
	run(t, ctx, "ZADD", "z", "99", "a")
	if got := run(t, ctx, "ZRANK", "z", "a"); got != ":2\r\n" {
		t.Errorf("after re-scoring, ZRANK a = %q, want :2", got)
	}

	// Crossing the threshold promotes to skiplist with ranks intact.
	for i := 0; i < 200; i++ {
		run(t, ctx, "ZADD", "big", itoa(i), "m"+itoa(i))
	}
	if got := run(t, ctx, "OBJECT", "ENCODING", "big"); !strings.Contains(got, "skiplist") {
		t.Fatalf("a large sorted set should be skiplist, got %q", got)
	}
	if got := run(t, ctx, "ZCARD", "big"); got != ":200\r\n" {
		t.Errorf("promotion lost members: %q", got)
	}
	if got := run(t, ctx, "ZRANK", "big", "m5"); got != ":5\r\n" {
		t.Errorf("ZRANK after promotion = %q, want :5", got)
	}

	run(t, ctx, "ZADD", "long", "1", strings.Repeat("x", 100))
	if got := run(t, ctx, "OBJECT", "ENCODING", "long"); !strings.Contains(got, "skiplist") {
		t.Errorf("an oversized member should force skiplist, got %q", got)
	}
}

func TestZaddOptionsAndNaN(t *testing.T) {
	ctx := newCtx()

	// NaN is rejected rather than stored.
	if got := run(t, ctx, "ZADD", "z", "nan", "m"); !strings.Contains(got, "not a valid float") {
		t.Errorf("ZADD nan = %q, want a float error", got)
	}
	if got := run(t, ctx, "EXISTS", "z"); got != ":0\r\n" {
		t.Errorf("a rejected ZADD must not create the key")
	}
	run(t, ctx, "ZADD", "z", "1", "m")
	if got := run(t, ctx, "ZINCRBY", "z", "nan", "m"); !strings.Contains(got, "not a valid float") {
		t.Errorf("ZINCRBY nan = %q", got)
	}

	// NX only adds, XX only updates.
	if got := run(t, ctx, "ZADD", "z", "NX", "5", "m"); got != ":0\r\n" {
		t.Errorf("ZADD NX on an existing member = %q", got)
	}
	if got := run(t, ctx, "ZSCORE", "z", "m"); !strings.Contains(got, "1") {
		t.Errorf("ZADD NX changed the score: %q", got)
	}
	if got := run(t, ctx, "ZADD", "z", "XX", "5", "absent"); got != ":0\r\n" {
		t.Errorf("ZADD XX on a missing member = %q", got)
	}
	if got := run(t, ctx, "ZADD", "z", "CH", "7", "m"); got != ":1\r\n" {
		t.Errorf("ZADD CH should count the update, got %q", got)
	}

	// GT and LT only move the score in one direction.
	run(t, ctx, "ZADD", "z", "GT", "3", "m")
	if got := run(t, ctx, "ZSCORE", "z", "m"); !strings.Contains(got, "7") {
		t.Errorf("GT lowered the score: %q", got)
	}
	run(t, ctx, "ZADD", "z", "GT", "9", "m")
	if got := run(t, ctx, "ZSCORE", "z", "m"); !strings.Contains(got, "9") {
		t.Errorf("GT did not raise the score: %q", got)
	}

	if got := run(t, ctx, "ZADD", "z", "INCR", "1", "m"); !strings.Contains(got, "10") {
		t.Errorf("ZADD INCR = %q, want 10", got)
	}
	if got := run(t, ctx, "ZADD", "z", "NX", "XX", "1", "m"); !strings.Contains(got, "not compatible") {
		t.Errorf("NX with XX should be rejected, got %q", got)
	}
}

func TestListEncodingTransitions(t *testing.T) {
	ctx := newCtx()
	run(t, ctx, "RPUSH", "l", "a", "b", "c")

	if got := run(t, ctx, "OBJECT", "ENCODING", "l"); !strings.Contains(got, "listpack") {
		t.Fatalf("a small list should be listpack, got %q", got)
	}
	if got := run(t, ctx, "LRANGE", "l", "0", "-1"); !strings.Contains(got, "a\r\n$1\r\nb\r\n$1\r\nc") {
		t.Errorf("listpack list order wrong: %q", got)
	}
	if got := run(t, ctx, "LINDEX", "l", "-1"); !strings.Contains(got, "c") {
		t.Errorf("negative LINDEX on a listpack = %q", got)
	}
	// Pops work from both ends while compact.
	if got := run(t, ctx, "LPOP", "l"); !strings.Contains(got, "a") {
		t.Errorf("LPOP = %q", got)
	}
	if got := run(t, ctx, "RPOP", "l"); !strings.Contains(got, "c") {
		t.Errorf("RPOP = %q", got)
	}

	// An oversized element converts to quicklist and keeps the contents.
	run(t, ctx, "RPUSH", "big", "small")
	run(t, ctx, "RPUSH", "big", strings.Repeat("x", 100))
	if got := run(t, ctx, "OBJECT", "ENCODING", "big"); !strings.Contains(got, "quicklist") {
		t.Fatalf("an oversized element should force quicklist, got %q", got)
	}
	if got := run(t, ctx, "LLEN", "big"); got != ":2\r\n" {
		t.Errorf("conversion lost elements: %q", got)
	}
	if got := run(t, ctx, "LINDEX", "big", "0"); !strings.Contains(got, "small") {
		t.Errorf("conversion reordered elements: %q", got)
	}

	// Crossing the entry threshold also converts, with indices intact.
	for i := 0; i < 200; i++ {
		run(t, ctx, "RPUSH", "many", "v"+itoa(i))
	}
	if got := run(t, ctx, "OBJECT", "ENCODING", "many"); !strings.Contains(got, "quicklist") {
		t.Fatalf("a long list should be quicklist, got %q", got)
	}
	if got := run(t, ctx, "LINDEX", "many", "5"); !strings.Contains(got, "v5") {
		t.Errorf("LINDEX after promotion = %q", got)
	}
}

// TestListReshapeAcrossEncodings covers the commands that rebuild the list.
func TestListReshapeAcrossEncodings(t *testing.T) {
	ctx := newCtx()
	run(t, ctx, "RPUSH", "l", "a", "b", "c", "b")

	if got := run(t, ctx, "LINSERT", "l", "BEFORE", "b", "X"); got != ":5\r\n" {
		t.Fatalf("LINSERT on a listpack = %q", got)
	}
	if got := run(t, ctx, "LREM", "l", "0", "b"); got != ":2\r\n" {
		t.Fatalf("LREM = %q", got)
	}
	run(t, ctx, "LTRIM", "l", "0", "0")
	if got := run(t, ctx, "LLEN", "l"); got != ":1\r\n" {
		t.Errorf("LTRIM = %q", got)
	}

	// The same commands on a promoted list.
	run(t, ctx, "RPUSH", "q", strings.Repeat("x", 100), "a", "b")
	if got := run(t, ctx, "OBJECT", "ENCODING", "q"); !strings.Contains(got, "quicklist") {
		t.Fatalf("setup: want quicklist, got %q", got)
	}
	if got := run(t, ctx, "LINSERT", "q", "AFTER", "a", "mid"); got != ":4\r\n" {
		t.Errorf("LINSERT on a quicklist = %q", got)
	}
	if got := run(t, ctx, "LINDEX", "q", "2"); !strings.Contains(got, "mid") {
		t.Errorf("LINSERT landed wrong: %q", got)
	}
	// Rebuilding keeps the oversized element, so it must stay a quicklist.
	if got := run(t, ctx, "OBJECT", "ENCODING", "q"); !strings.Contains(got, "quicklist") {
		t.Errorf("rebuild demoted a list holding an oversized element: %q", got)
	}
}

func TestZrankWithScore(t *testing.T) {
	ctx := newCtx()
	run(t, ctx, "ZADD", "z", "1", "a", "2", "b")

	if got := run(t, ctx, "ZRANK", "z", "b"); got != ":1\r\n" {
		t.Errorf("ZRANK = %q", got)
	}
	if got := run(t, ctx, "ZRANK", "z", "b", "WITHSCORE"); !strings.HasPrefix(got, "*2\r\n:1\r\n") {
		t.Errorf("ZRANK WITHSCORE = %q, want a rank and score pair", got)
	}
	// A missing member replies with a nil array in the WITHSCORE form, because
	// the present form is a two element array.
	if got := run(t, ctx, "ZRANK", "z", "nope", "WITHSCORE"); got != "*-1\r\n" {
		t.Errorf("ZRANK WITHSCORE on a missing member = %q, want a nil array", got)
	}
	if got := run(t, ctx, "ZRANK", "z", "nope"); got != "$-1\r\n" {
		t.Errorf("ZRANK on a missing member = %q, want a nil bulk", got)
	}
	if got := run(t, ctx, "ZRANK", "z", "b", "NONSENSE"); !strings.Contains(got, "syntax error") {
		t.Errorf("ZRANK with a bad option = %q", got)
	}
	if got := run(t, ctx, "ZREVRANK", "z", "b", "WITHSCORE"); !strings.HasPrefix(got, "*2\r\n:0\r\n") {
		t.Errorf("ZREVRANK WITHSCORE = %q", got)
	}
}

// TestDebugDigestValue checks the digest ignores encoding and, for unordered
// containers, insertion order.
func TestDebugDigestValue(t *testing.T) {
	ctx := newCtx()

	run(t, ctx, "SADD", "s1", "a", "b", "c")
	run(t, ctx, "SADD", "s2", "c", "b", "a")
	d1 := run(t, ctx, "DEBUG", "DIGEST-VALUE", "s1")
	d2 := run(t, ctx, "DEBUG", "DIGEST-VALUE", "s2")
	if d1 != d2 {
		t.Errorf("set digest depends on insertion order:\n %q\n %q", d1, d2)
	}

	// Same members, different encodings, same digest.
	run(t, ctx, "SADD", "ints1", "1", "2", "3")
	run(t, ctx, "SADD", "ints2", "1", "2", "3", strings.Repeat("x", 100))
	run(t, ctx, "SREM", "ints2", strings.Repeat("x", 100))
	if enc := run(t, ctx, "OBJECT", "ENCODING", "ints2"); !strings.Contains(enc, "hashtable") {
		t.Fatalf("setup: ints2 should have been promoted, got %q", enc)
	}
	if a, b := run(t, ctx, "DEBUG", "DIGEST-VALUE", "ints1"), run(t, ctx, "DEBUG", "DIGEST-VALUE", "ints2"); a != b {
		t.Errorf("digest differs across encodings:\n %q\n %q", a, b)
	}

	// A different value must produce a different digest.
	run(t, ctx, "SADD", "s3", "a", "b", "d")
	if run(t, ctx, "DEBUG", "DIGEST-VALUE", "s3") == d1 {
		t.Error("different sets produced the same digest")
	}

	// Lists are ordered, so order matters there.
	run(t, ctx, "RPUSH", "l1", "a", "b")
	run(t, ctx, "RPUSH", "l2", "b", "a")
	if run(t, ctx, "DEBUG", "DIGEST-VALUE", "l1") == run(t, ctx, "DEBUG", "DIGEST-VALUE", "l2") {
		t.Error("list digest ignored order")
	}
}

func TestDumpRestoreRoundTrip(t *testing.T) {
	ctx := newCtx()
	run(t, ctx, "RPUSH", "src", "a", "b", "c")

	dumped := run(t, ctx, "DUMP", "src")
	if !strings.HasPrefix(dumped, "$") || strings.HasPrefix(dumped, "$-1") {
		t.Fatalf("DUMP = %q", dumped)
	}
	// Pull the payload out of the bulk reply.
	idx := strings.Index(dumped, "\r\n")
	payload := dumped[idx+2 : len(dumped)-2]

	argv := [][]byte{[]byte("RESTORE"), []byte("dst"), []byte("0"), []byte(payload)}
	if got := string(DefaultTable.Execute(ctx, argv)); got != "+OK\r\n" {
		t.Fatalf("RESTORE = %q", got)
	}
	if got := run(t, ctx, "LRANGE", "dst", "0", "-1"); !strings.Contains(got, "a\r\n$1\r\nb\r\n$1\r\nc") {
		t.Errorf("restored list = %q", got)
	}

	// Restoring onto an existing key needs REPLACE.
	if got := string(DefaultTable.Execute(ctx, argv)); !strings.HasPrefix(got, "-BUSYKEY") {
		t.Errorf("RESTORE onto an existing key = %q, want BUSYKEY", got)
	}
	withReplace := append(append([][]byte{}, argv...), []byte("REPLACE"))
	if got := string(DefaultTable.Execute(ctx, withReplace)); got != "+OK\r\n" {
		t.Errorf("RESTORE REPLACE = %q", got)
	}

	// A corrupted payload is rejected rather than half applied.
	bad := []byte(payload)
	bad[len(bad)/2] ^= 0xff
	corrupt := [][]byte{[]byte("RESTORE"), []byte("other"), []byte("0"), bad}
	if got := string(DefaultTable.Execute(ctx, corrupt)); !strings.Contains(got, "checksum") {
		t.Errorf("corrupted RESTORE = %q, want a checksum error", got)
	}
	if got := run(t, ctx, "EXISTS", "other"); got != ":0\r\n" {
		t.Error("a rejected RESTORE must not create the key")
	}
	if got := run(t, ctx, "DUMP", "missing"); got != "$-1\r\n" {
		t.Errorf("DUMP of a missing key = %q", got)
	}
}

func TestZsetLexAndUnifiedRange(t *testing.T) {
	ctx := newCtx()
	run(t, ctx, "ZADD", "z", "0", "a", "0", "b", "0", "c", "0", "d")

	if got := run(t, ctx, "ZRANGEBYLEX", "z", "[a", "[b"); !strings.HasPrefix(got, "*2\r\n") {
		t.Errorf("ZRANGEBYLEX inclusive = %q, want 2", got)
	}
	if got := run(t, ctx, "ZRANGEBYLEX", "z", "(a", "[c"); !strings.HasPrefix(got, "*2\r\n") {
		t.Errorf("ZRANGEBYLEX exclusive min = %q, want 2", got)
	}
	if got := run(t, ctx, "ZRANGEBYLEX", "z", "-", "+"); !strings.HasPrefix(got, "*4\r\n") {
		t.Errorf("ZRANGEBYLEX full range = %q, want 4", got)
	}
	if got := run(t, ctx, "ZLEXCOUNT", "z", "-", "+"); got != ":4\r\n" {
		t.Errorf("ZLEXCOUNT = %q", got)
	}
	if got := run(t, ctx, "ZRANGEBYLEX", "z", "a", "b"); !strings.Contains(got, "not valid string range") {
		t.Errorf("a bound without a bracket should be rejected, got %q", got)
	}
	if got := run(t, ctx, "ZREMRANGEBYLEX", "z", "[a", "[b"); got != ":2\r\n" {
		t.Errorf("ZREMRANGEBYLEX = %q", got)
	}

	// The unified ZRANGE options from Redis 6.2.
	run(t, ctx, "ZADD", "s", "1", "a", "2", "b", "3", "c")
	if got := run(t, ctx, "ZRANGE", "s", "(1", "3", "BYSCORE"); !strings.HasPrefix(got, "*2\r\n") {
		t.Errorf("ZRANGE BYSCORE = %q, want 2", got)
	}
	if got := run(t, ctx, "ZRANGE", "s", "0", "-1", "REV"); !strings.Contains(got, "c\r\n$1\r\nb\r\n$1\r\na") {
		t.Errorf("ZRANGE REV = %q", got)
	}
	if got := run(t, ctx, "ZRANGE", "s", "-inf", "+inf", "BYSCORE", "LIMIT", "1", "1"); !strings.HasPrefix(got, "*1\r\n") {
		t.Errorf("ZRANGE BYSCORE LIMIT = %q", got)
	}
	// LIMIT is only legal with BYSCORE or BYLEX.
	if got := run(t, ctx, "ZRANGE", "s", "0", "-1", "LIMIT", "0", "1"); !strings.Contains(got, "LIMIT") {
		t.Errorf("ZRANGE LIMIT without BYSCORE should be rejected, got %q", got)
	}
	if got := run(t, ctx, "ZRANGESTORE", "out", "s", "0", "1"); got != ":2\r\n" {
		t.Errorf("ZRANGESTORE = %q", got)
	}
	if got := run(t, ctx, "ZCARD", "out"); got != ":2\r\n" {
		t.Errorf("ZRANGESTORE destination = %q", got)
	}
}

func TestCopyStream(t *testing.T) {
	ctx := newCtx()
	run(t, ctx, "XADD", "st", "1-1", "f", "v")
	run(t, ctx, "XADD", "st", "2-1", "g", "w")

	if got := run(t, ctx, "COPY", "st", "st2"); got != ":1\r\n" {
		t.Fatalf("COPY of a stream = %q", got)
	}
	if got := run(t, ctx, "XLEN", "st2"); got != ":2\r\n" {
		t.Errorf("copied stream length = %q", got)
	}
	// The copy must be independent: trimming one cannot affect the other.
	run(t, ctx, "XTRIM", "st", "MAXLEN", "1")
	if got := run(t, ctx, "XLEN", "st"); got != ":1\r\n" {
		t.Errorf("source after trim = %q", got)
	}
	if got := run(t, ctx, "XLEN", "st2"); got != ":2\r\n" {
		t.Errorf("copy was affected by trimming the source: %q", got)
	}
	if got := run(t, ctx, "XRANGE", "st2", "-", "+"); !strings.Contains(got, "1-1") {
		t.Errorf("copied stream lost entries: %q", got)
	}
}

func TestZsetSetOperations(t *testing.T) {
	ctx := newCtx()
	run(t, ctx, "ZADD", "a", "1", "a", "1", "b", "1", "c")
	run(t, ctx, "ZADD", "b", "1", "b", "1", "c", "1", "d")

	if got := run(t, ctx, "ZUNIONSTORE", "u", "2", "a", "b"); got != ":4\r\n" {
		t.Errorf("ZUNIONSTORE = %q, want :4", got)
	}
	if got := run(t, ctx, "ZSCORE", "u", "b"); !strings.Contains(got, "2") {
		t.Errorf("SUM aggregation = %q, want 2", got)
	}
	if got := run(t, ctx, "ZINTERSTORE", "i", "2", "a", "b"); got != ":2\r\n" {
		t.Errorf("ZINTERSTORE = %q, want :2", got)
	}
	if got := run(t, ctx, "ZDIFFSTORE", "d", "2", "a", "b"); got != ":1\r\n" {
		t.Errorf("ZDIFFSTORE = %q, want :1", got)
	}

	// WEIGHTS and AGGREGATE.
	run(t, ctx, "ZUNIONSTORE", "w", "2", "a", "b", "WEIGHTS", "2", "3")
	if got := run(t, ctx, "ZSCORE", "w", "b"); !strings.Contains(got, "5") {
		t.Errorf("WEIGHTS = %q, want 2+3", got)
	}
	run(t, ctx, "ZUNIONSTORE", "m", "2", "a", "b", "AGGREGATE", "MAX")
	if got := run(t, ctx, "ZSCORE", "m", "b"); !strings.Contains(got, "1") {
		t.Errorf("AGGREGATE MAX = %q", got)
	}
	// AGGREGATE COUNT scores by how many inputs hold the member.
	if got := run(t, ctx, "ZUNION", "2", "a", "b", "AGGREGATE", "COUNT", "WITHSCORES"); !strings.Contains(got, "b\r\n$1\r\n2") {
		t.Errorf("AGGREGATE COUNT = %q, want b scored 2", got)
	}
	if got := run(t, ctx, "ZUNIONSTORE", "x", "2", "a", "b", "AGGREGATE", "NONSENSE"); !strings.Contains(got, "syntax error") {
		t.Errorf("an unknown aggregate should be rejected, got %q", got)
	}

	// A plain set counts as a sorted set whose members all score 1.
	run(t, ctx, "SADD", "plain", "b", "z")
	if got := run(t, ctx, "ZUNIONSTORE", "mix", "2", "a", "plain"); got != ":4\r\n" {
		t.Errorf("union with a plain set = %q, want :4", got)
	}
	if got := run(t, ctx, "ZSCORE", "mix", "b"); !strings.Contains(got, "2") {
		t.Errorf("set member should contribute score 1: %q", got)
	}

	if got := run(t, ctx, "ZINTERCARD", "2", "a", "b"); got != ":2\r\n" {
		t.Errorf("ZINTERCARD = %q", got)
	}
	if got := run(t, ctx, "ZINTERCARD", "2", "a", "b", "LIMIT", "1"); got != ":1\r\n" {
		t.Errorf("ZINTERCARD LIMIT = %q", got)
	}
	if got := run(t, ctx, "ZUNIONSTORE", "z", "0", "a"); !strings.Contains(got, "at least 1 input key") {
		t.Errorf("numkeys 0 should be rejected, got %q", got)
	}
}

func TestZsetRangeEdgeCases(t *testing.T) {
	ctx := newCtx()
	run(t, ctx, "ZADD", "z", "1", "a", "2", "b", "3", "c")

	// A negative LIMIT offset returns nothing rather than the whole range.
	if got := run(t, ctx, "ZRANGEBYSCORE", "z", "0", "10", "LIMIT", "-1", "2"); got != "*0\r\n" {
		t.Errorf("negative LIMIT offset = %q, want empty", got)
	}
	// NaN is not an orderable bound.
	if got := run(t, ctx, "ZRANGEBYSCORE", "z", "1", "NaN"); !strings.Contains(got, "not a float") {
		t.Errorf("NaN bound = %q, want a float error", got)
	}
	if got := run(t, ctx, "ZCOUNT", "z", "(1", "3"); got != ":2\r\n" {
		t.Errorf("ZCOUNT with an exclusive bound = %q, want :2", got)
	}
	if got := run(t, ctx, "ZCOUNT", "z", "-inf", "+inf"); got != ":3\r\n" {
		t.Errorf("ZCOUNT with infinities = %q", got)
	}
}

func TestStreamIntrospection(t *testing.T) {
	ctx := newCtx()
	run(t, ctx, "XADD", "st", "1-1", "f", "v")
	run(t, ctx, "XADD", "st", "2-1", "g", "w")
	run(t, ctx, "XGROUP", "CREATE", "st", "grp", "0")

	info := run(t, ctx, "XINFO", "STREAM", "st")
	for _, want := range []string{"length", "last-generated-id", "2-1", "first-entry"} {
		if !strings.Contains(info, want) {
			t.Errorf("XINFO STREAM missing %q in %q", want, info)
		}
	}
	groups := run(t, ctx, "XINFO", "GROUPS", "st")
	if !strings.Contains(groups, "grp") {
		t.Errorf("XINFO GROUPS = %q", groups)
	}
	if got := run(t, ctx, "XINFO", "CONSUMERS", "st", "nosuchgroup"); !strings.HasPrefix(got, "-NOGROUP") {
		t.Errorf("XINFO CONSUMERS on a missing group = %q", got)
	}
	if got := run(t, ctx, "XINFO", "STREAM", "missing"); !strings.Contains(got, "no such key") {
		t.Errorf("XINFO on a missing key = %q", got)
	}

	// XGROUP SETID repositions the group cursor.
	if got := run(t, ctx, "XGROUP", "SETID", "st", "grp", "$"); got != "+OK\r\n" {
		t.Errorf("XGROUP SETID = %q", got)
	}
	if got := run(t, ctx, "XGROUP", "SETID", "st", "nosuchgroup", "0"); !strings.HasPrefix(got, "-NOGROUP") {
		t.Errorf("XGROUP SETID on a missing group = %q", got)
	}
}

// TestStreamDigestIgnoresIdentity guards the digest against falling back to
// formatting the struct, which would hash internal state instead of entries.
func TestStreamDigestIgnoresIdentity(t *testing.T) {
	ctx := newCtx()
	run(t, ctx, "XADD", "a", "1-1", "f", "v")
	run(t, ctx, "XADD", "b", "1-1", "f", "v")

	if x, y := run(t, ctx, "DEBUG", "DIGEST-VALUE", "a"), run(t, ctx, "DEBUG", "DIGEST-VALUE", "b"); x != y {
		t.Errorf("identical streams digested differently:\n %q\n %q", x, y)
	}
	run(t, ctx, "XADD", "b", "2-1", "g", "w")
	if x, y := run(t, ctx, "DEBUG", "DIGEST-VALUE", "a"), run(t, ctx, "DEBUG", "DIGEST-VALUE", "b"); x == y {
		t.Error("streams with different entries digested the same")
	}
}
