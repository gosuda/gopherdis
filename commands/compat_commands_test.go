package commands

import (
	"bytes"
	"testing"

	"github.com/gosuda/gopherdis/db"
)

func exec(t *testing.T, ctx *Context, args ...string) []byte {
	t.Helper()
	argv := make([][]byte, len(args))
	for i, a := range args {
		argv[i] = []byte(a)
	}
	return DefaultTable.Execute(ctx, argv)
}

// TestObjectEncodingMatchesRedis covers the encodings the official
// unit/type/incr suite asserts on.
func TestObjectEncodingMatchesRedis(t *testing.T) {
	ctx := &Context{DB: db.NewShardedDB()}

	for _, tc := range []struct {
		value string
		want  string
	}{
		{"1", "int"},
		{"-9223372036854775808", "int"},
		{"012", "embstr"},                 // leading zero does not round-trip as an int
		{"+1", "embstr"},                  // nor does an explicit plus
		{" 1", "embstr"},                  // nor does surrounding space
		{"9223372036854775808", "embstr"}, // one past int64 max
		{"hello", "embstr"},
		{"0123456789012345678901234567890123456789012345", "raw"}, // 46 chars
	} {
		exec(t, ctx, "SET", "k", tc.value)
		got := exec(t, ctx, "OBJECT", "ENCODING", "k")
		if !bytes.Contains(got, []byte(tc.want)) {
			t.Errorf("SET %q then OBJECT ENCODING: want %s, got %q", tc.value, tc.want, got)
		}
	}

	// INCR must move a value to int encoding, and the value must survive.
	exec(t, ctx, "SET", "n", "1")
	exec(t, ctx, "INCR", "n")
	if got := exec(t, ctx, "OBJECT", "ENCODING", "n"); !bytes.Contains(got, []byte("int")) {
		t.Errorf("after INCR want int encoding, got %q", got)
	}
	if got := exec(t, ctx, "GET", "n"); !bytes.Contains(got, []byte("2")) {
		t.Errorf("after INCR want 2, got %q", got)
	}

	if got := exec(t, ctx, "OBJECT", "REFCOUNT", "n"); !bytes.Equal(got, []byte(":1\r\n")) {
		t.Errorf("OBJECT REFCOUNT: want :1, got %q", got)
	}
	if got := exec(t, ctx, "OBJECT", "ENCODING", "missing"); !bytes.HasPrefix(got, []byte("-ERR no such key")) {
		t.Errorf("OBJECT on a missing key: want no such key, got %q", got)
	}
}

func TestIncrbyfloat(t *testing.T) {
	ctx := &Context{DB: db.NewShardedDB()}

	if got := exec(t, ctx, "INCRBYFLOAT", "f", "10.5"); !bytes.Contains(got, []byte("10.5")) {
		t.Fatalf("want 10.5, got %q", got)
	}
	if got := exec(t, ctx, "INCRBYFLOAT", "f", "0.1"); !bytes.Contains(got, []byte("10.6")) {
		t.Fatalf("want 10.6, got %q", got)
	}
	// Redis renders the result without an exponent and without trailing zeros.
	exec(t, ctx, "SET", "e", "3.0")
	if got := exec(t, ctx, "INCRBYFLOAT", "e", "1.000000000000000005"); !bytes.Contains(got, []byte("4")) {
		t.Fatalf("want 4, got %q", got)
	}
	// An infinite increment is parsed, then rejected on the result.
	exec(t, ctx, "SET", "i", "10.5")
	if got := exec(t, ctx, "INCRBYFLOAT", "i", "+inf"); !bytes.Contains(got, []byte("would produce")) {
		t.Fatalf("want a 'would produce NaN or Infinity' error, got %q", got)
	}
	exec(t, ctx, "SET", "s", " 11")
	if got := exec(t, ctx, "INCRBYFLOAT", "s", "1"); !bytes.Contains(got, []byte("not a valid float")) {
		t.Fatalf("a value with spaces must be rejected, got %q", got)
	}
	exec(t, ctx, "RPUSH", "l", "x")
	if got := exec(t, ctx, "INCRBYFLOAT", "l", "1"); !bytes.HasPrefix(got, []byte("-WRONGTYPE")) {
		t.Fatalf("want WRONGTYPE, got %q", got)
	}
}

func TestSetrangeGetrange(t *testing.T) {
	ctx := &Context{DB: db.NewShardedDB()}

	// An empty value against a missing key must not create the key.
	if got := exec(t, ctx, "SETRANGE", "nope", "0", ""); !bytes.Equal(got, []byte(":0\r\n")) {
		t.Fatalf("want :0, got %q", got)
	}
	if n := ctx.DB.Len(); n != 0 {
		t.Fatalf("SETRANGE with an empty value created a key: DBSIZE=%d", n)
	}

	// Writing past the end zero-pads.
	if got := exec(t, ctx, "SETRANGE", "k", "5", "hello"); !bytes.Equal(got, []byte(":10\r\n")) {
		t.Fatalf("want :10, got %q", got)
	}
	if got := exec(t, ctx, "GET", "k"); !bytes.Contains(got, []byte("\x00\x00\x00\x00\x00hello")) {
		t.Fatalf("want zero padding, got %q", got)
	}
	if got := exec(t, ctx, "SETRANGE", "k", "-1", "x"); !bytes.Contains(got, []byte("offset is out of range")) {
		t.Fatalf("want an offset error, got %q", got)
	}

	exec(t, ctx, "SET", "g", "This is a string")
	for _, tc := range []struct{ start, end, want string }{
		{"0", "3", "This"},
		{"-3", "-1", "ing"},
		{"0", "-1", "This is a string"},
		{"10", "100", "string"},
	} {
		got := exec(t, ctx, "GETRANGE", "g", tc.start, tc.end)
		if !bytes.Contains(got, []byte(tc.want)) {
			t.Errorf("GETRANGE g %s %s: want %q, got %q", tc.start, tc.end, tc.want, got)
		}
	}
	if got := exec(t, ctx, "GETRANGE", "missing", "0", "-1"); !bytes.Equal(got, []byte("$0\r\n\r\n")) {
		t.Errorf("GETRANGE on a missing key: want an empty bulk, got %q", got)
	}
}

func TestFunctionStub(t *testing.T) {
	ctx := &Context{DB: db.NewShardedDB()}

	// The official suite calls FUNCTION FLUSH during bootstrap.
	if got := exec(t, ctx, "FUNCTION", "FLUSH"); !bytes.Equal(got, []byte("+OK\r\n")) {
		t.Fatalf("FUNCTION FLUSH: want +OK, got %q", got)
	}
	if got := exec(t, ctx, "FUNCTION", "LIST"); !bytes.Equal(got, []byte("*0\r\n")) {
		t.Fatalf("FUNCTION LIST: want an empty array, got %q", got)
	}
	if got := exec(t, ctx, "FUNCTION", "LOAD", "x"); !bytes.HasPrefix(got, []byte("-ERR ")) {
		t.Fatalf("FUNCTION LOAD must report that it is unsupported, got %q", got)
	}
}
