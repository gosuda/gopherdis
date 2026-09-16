package commands

import (
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gosuda/gopherdis/db"
)

// propagated runs a command and returns what the AOF and replication sinks were
// handed, which must be identical.
func propagated(t *testing.T, ctx *Context, aof *mockAOF, args ...string) string {
	t.Helper()
	aof.fed = nil
	run(t, ctx, args...)
	parts := make([]string, 0, len(aof.fed))
	for _, a := range aof.fed {
		parts = append(parts, string(a))
	}
	return strings.Join(parts, " ")
}

// TestTTLPropagationIsAbsolute covers the rewriting that keeps a replica or an
// AOF replay from computing a later deadline than the master applied.
func TestTTLPropagationIsAbsolute(t *testing.T) {
	aof := &mockAOF{}
	ctx := newCtx()
	ctx.AOF = aof

	now := time.Now().UnixMilli()

	cases := []struct {
		name    string
		args    []string
		wantCmd string
		absAt   int // index of the absolute-ms argument, -1 when not applicable
	}{
		{"SET EX", []string{"SET", "k", "v", "EX", "200"}, "SET", 4},
		{"SET PX", []string{"SET", "k", "v", "PX", "100000"}, "SET", 4},
		{"SET EXAT", []string{"SET", "k", "v", "EXAT", strconv.FormatInt(now/1000+300, 10)}, "SET", 4},
		{"SETEX", []string{"SETEX", "k", "100", "v"}, "SET", 4},
		{"PSETEX", []string{"PSETEX", "k", "100000", "v"}, "SET", 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := propagated(t, ctx, aof, c.args...)
			fields := strings.Fields(got)
			if len(fields) < 5 || !strings.EqualFold(fields[0], c.wantCmd) {
				t.Fatalf("propagated %q", got)
			}
			if !strings.EqualFold(fields[3], "PXAT") {
				t.Fatalf("expected a PXAT rewrite, propagated %q", got)
			}
			ms, err := strconv.ParseInt(fields[c.absAt], 10, 64)
			if err != nil || ms < now {
				t.Fatalf("absolute deadline %q is not in the future", got)
			}
		})
	}

	// Relative expiry commands become PEXPIREAT.
	run(t, ctx, "SET", "e", "v")
	got := propagated(t, ctx, aof, "EXPIRE", "e", "100")
	if !strings.HasPrefix(strings.ToUpper(got), "PEXPIREAT") {
		t.Errorf("EXPIRE propagated as %q, want PEXPIREAT", got)
	}

	// An expiry already in the past becomes a DEL, so the replica removes the
	// key instead of resurrecting it with a future deadline.
	run(t, ctx, "SET", "past", "v")
	got = propagated(t, ctx, aof, "EXPIREAT", "past", "1")
	if !strings.HasPrefix(strings.ToUpper(got), "DEL") {
		t.Errorf("a past EXPIREAT propagated as %q, want DEL", got)
	}
}

func TestGetexPropagation(t *testing.T) {
	aof := &mockAOF{}
	ctx := newCtx()
	ctx.AOF = aof
	run(t, ctx, "SET", "k", "v")

	// GETEX propagates the expiry change it made, not itself.
	got := propagated(t, ctx, aof, "GETEX", "k", "EX", "200")
	if !strings.HasPrefix(strings.ToUpper(got), "PEXPIREAT") {
		t.Errorf("GETEX EX propagated as %q, want PEXPIREAT", got)
	}
	got = propagated(t, ctx, aof, "GETEX", "k", "PERSIST")
	if !strings.HasPrefix(strings.ToUpper(got), "PERSIST") {
		t.Errorf("GETEX PERSIST propagated as %q", got)
	}
	// A plain GETEX changes nothing and must propagate nothing.
	if got := propagated(t, ctx, aof, "GETEX", "k"); got != "" {
		t.Errorf("a plain GETEX propagated %q, want nothing", got)
	}
}

// TestSetConditionalsAreNotPropagated checks that a decided NX/XX is not
// re-evaluated on the replica, where the key may be in a different state.
func TestSetConditionalsAreNotPropagated(t *testing.T) {
	aof := &mockAOF{}
	ctx := newCtx()
	ctx.AOF = aof

	got := propagated(t, ctx, aof, "SET", "k", "v", "NX")
	if strings.Contains(strings.ToUpper(got), "NX") {
		t.Errorf("NX leaked into the propagated form: %q", got)
	}
	if !strings.Contains(got, "k") || !strings.Contains(got, "v") {
		t.Errorf("propagated form lost the key or value: %q", got)
	}
}

// TestExpiredKeyPropagatesDel covers the replication hook. A replica does not
// expire keys on its own, so without an explicit DEL an expired key survives
// there after the master has dropped it.
func TestExpiredKeyPropagatesDel(t *testing.T) {
	database := db.NewShardedDB()

	var mu sync.Mutex
	var propagated []string
	database.SetExpiredCallback(func(key string) {
		mu.Lock()
		propagated = append(propagated, key)
		mu.Unlock()
	})

	ctx := &Context{DB: database}
	run(t, ctx, "SET", "gone", "v", "PX", "1")
	time.Sleep(20 * time.Millisecond)

	// The read triggers lazy expiry.
	if got := run(t, ctx, "GET", "gone"); got != "$-1\r\n" {
		t.Fatalf("expected the key to have expired, got %q", got)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(propagated) != 1 || propagated[0] != "gone" {
		t.Fatalf("lazy expiry propagated %v, want one DEL for \"gone\"", propagated)
	}
	if n := database.ExpiredKeys(); n != 1 {
		t.Errorf("expired_keys = %d, want 1", n)
	}
}

func TestExpireConditions(t *testing.T) {
	ctx := newCtx()
	run(t, ctx, "SET", "k", "v")

	// NX only sets when there is no TTL.
	if got := run(t, ctx, "EXPIRE", "k", "100", "NX"); got != ":1\r\n" {
		t.Errorf("EXPIRE NX on a key without a TTL = %q", got)
	}
	if got := run(t, ctx, "EXPIRE", "k", "200", "NX"); got != ":0\r\n" {
		t.Errorf("EXPIRE NX on a key with a TTL = %q", got)
	}
	// XX is the inverse.
	if got := run(t, ctx, "EXPIRE", "k", "300", "XX"); got != ":1\r\n" {
		t.Errorf("EXPIRE XX on a key with a TTL = %q", got)
	}
	// GT only raises.
	if got := run(t, ctx, "EXPIRE", "k", "100", "GT"); got != ":0\r\n" {
		t.Errorf("EXPIRE GT with a lower value = %q, want :0", got)
	}
	if got := run(t, ctx, "EXPIRE", "k", "500", "GT"); got != ":1\r\n" {
		t.Errorf("EXPIRE GT with a higher value = %q, want :1", got)
	}
	// A key with no TTL expires at infinity, so GT never overwrites it and LT
	// always does.
	run(t, ctx, "SET", "noTTL", "v")
	if got := run(t, ctx, "EXPIRE", "noTTL", "100", "GT"); got != ":0\r\n" {
		t.Errorf("EXPIRE GT on a key without a TTL = %q, want :0", got)
	}
	if got := run(t, ctx, "EXPIRE", "noTTL", "100", "LT"); got != ":1\r\n" {
		t.Errorf("EXPIRE LT on a key without a TTL = %q, want :1", got)
	}
	if got := run(t, ctx, "EXPIRE", "k", "100", "NX", "XX"); !strings.Contains(got, "not compatible") {
		t.Errorf("conflicting options = %q", got)
	}
}
