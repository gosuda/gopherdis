package commands

import (
	"strings"
	"testing"

	"github.com/gosuda/gopherdis/db"
)

// newProtoCtx builds a connection context pinned to a protocol version.
func newProtoCtx(proto int) *Context {
	return &Context{DB: db.NewShardedDB(), Proto: proto}
}

// TestHelloRecordsProtocol covers the bug where HELLO 3 succeeded but left the
// connection encoding every later reply as RESP2.
func TestHelloRecordsProtocol(t *testing.T) {
	ctx := newProtoCtx(2)

	if got := run(t, ctx, "HELLO", "3"); !strings.HasPrefix(got, "%") {
		t.Fatalf("HELLO 3 = %q, want a map reply", got)
	}
	if ctx.Proto != 3 {
		t.Fatalf("HELLO 3 left Proto at %d", ctx.Proto)
	}
	// A later reply must now use the RESP3 shape.
	run(t, ctx, "HSET", "h", "f", "v")
	if got := run(t, ctx, "HGETALL", "h"); !strings.HasPrefix(got, "%1\r\n") {
		t.Errorf("after HELLO 3, HGETALL = %q, want a map", got)
	}

	// Going back to 2 restores the RESP2 shapes.
	run(t, ctx, "HELLO", "2")
	if ctx.Proto != 2 {
		t.Fatalf("HELLO 2 left Proto at %d", ctx.Proto)
	}
	if got := run(t, ctx, "HGETALL", "h"); !strings.HasPrefix(got, "*2\r\n") {
		t.Errorf("after HELLO 2, HGETALL = %q, want an array", got)
	}
	if got := run(t, ctx, "HELLO", "4"); !strings.HasPrefix(got, "-NOPROTO") {
		t.Errorf("HELLO 4 = %q, want NOPROTO", got)
	}
}

// TestResp3ReplyShapes pins the wire encoding of every reply type that differs
// between the two protocols. A RESP3 client decodes by type, so emitting the
// RESP2 shape is a protocol error rather than a cosmetic difference.
func TestResp3ReplyShapes(t *testing.T) {
	cases := []struct {
		name  string
		setup [][]string
		cmd   []string
		resp2 string
		resp3 string
	}{
		{
			name:  "HGETALL is a map",
			setup: [][]string{{"HSET", "h", "a", "1"}},
			cmd:   []string{"HGETALL", "h"},
			resp2: "*2\r\n", resp3: "%1\r\n",
		},
		{
			name:  "SMEMBERS is a set",
			setup: [][]string{{"SADD", "s", "x"}},
			cmd:   []string{"SMEMBERS", "s"},
			resp2: "*1\r\n", resp3: "~1\r\n",
		},
		{
			name:  "SINTER is a set",
			setup: [][]string{{"SADD", "s", "x"}, {"SADD", "t", "x"}},
			cmd:   []string{"SINTER", "s", "t"},
			resp2: "*1\r\n", resp3: "~1\r\n",
		},
		{
			name:  "ZSCORE is a double",
			setup: [][]string{{"ZADD", "z", "1.5", "m"}},
			cmd:   []string{"ZSCORE", "z", "m"},
			resp2: "$3\r\n1.5\r\n", resp3: ",1.5\r\n",
		},
		{
			name:  "a missing score is null",
			setup: [][]string{{"ZADD", "z", "1", "m"}},
			cmd:   []string{"ZSCORE", "z", "nope"},
			resp2: "$-1\r\n", resp3: "_\r\n",
		},
		{
			name:  "INCRBYFLOAT is a double",
			setup: nil,
			cmd:   []string{"INCRBYFLOAT", "f", "1.5"},
			resp2: "$3\r\n1.5\r\n", resp3: ",1.5\r\n",
		},
		{
			name:  "WITHSCORES nests pairs",
			setup: [][]string{{"ZADD", "z", "1.5", "m"}},
			cmd:   []string{"ZRANGE", "z", "0", "-1", "WITHSCORES"},
			resp2: "*2\r\n$1\r\nm\r\n$3\r\n1.5\r\n",
			resp3: "*1\r\n*2\r\n$1\r\nm\r\n,1.5\r\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for _, proto := range []int{2, 3} {
				ctx := newProtoCtx(proto)
				for _, s := range c.setup {
					run(t, ctx, s...)
				}
				got := run(t, ctx, c.cmd...)
				want := c.resp2
				if proto == 3 {
					want = c.resp3
				}
				if !strings.HasPrefix(got, want) {
					t.Errorf("RESP%d: got %q, want prefix %q", proto, got, want)
				}
			}
		})
	}
}

// TestResp3DoubleSpecialValues covers the encodings RESP3 defines for values a
// bulk string would have rendered as text.
func TestResp3DoubleSpecialValues(t *testing.T) {
	ctx := newProtoCtx(3)
	run(t, ctx, "ZADD", "z", "+inf", "pos", "-inf", "neg")

	if got := run(t, ctx, "ZSCORE", "z", "pos"); got != ",inf\r\n" {
		t.Errorf("positive infinity = %q, want ,inf", got)
	}
	if got := run(t, ctx, "ZSCORE", "z", "neg"); got != ",-inf\r\n" {
		t.Errorf("negative infinity = %q, want ,-inf", got)
	}
}

// TestHmsetRepliesOK covers the alias bug: HMSET was wired straight to the HSET
// handler, so it replied with a field count where a client expects OK.
func TestHmsetRepliesOK(t *testing.T) {
	ctx := newProtoCtx(2)

	if got := run(t, ctx, "HMSET", "h", "a", "1", "b", "2"); got != "+OK\r\n" {
		t.Errorf("HMSET = %q, want +OK", got)
	}
	if got := run(t, ctx, "HSET", "h2", "a", "1"); got != ":1\r\n" {
		t.Errorf("HSET should still reply with a count, got %q", got)
	}
	// The error has to name the command the client actually sent.
	if got := run(t, ctx, "HMSET", "h", "a", "1", "b"); !strings.Contains(got, "'hmset'") {
		t.Errorf("HMSET arity error = %q, want it to name hmset", got)
	}
	if got := run(t, ctx, "HSET", "h", "a", "1", "b"); !strings.Contains(got, "'hset'") {
		t.Errorf("HSET arity error = %q, want it to name hset", got)
	}
}

func TestHrandfieldWithValuesNesting(t *testing.T) {
	for _, proto := range []int{2, 3} {
		ctx := newProtoCtx(proto)
		run(t, ctx, "HSET", "h", "f", "v")

		got := run(t, ctx, "HRANDFIELD", "h", "1", "WITHVALUES")
		want := "*2\r\n"
		if proto == 3 {
			want = "*1\r\n*2\r\n"
		}
		if !strings.HasPrefix(got, want) {
			t.Errorf("RESP%d HRANDFIELD WITHVALUES = %q, want prefix %q", proto, got, want)
		}
	}
}
