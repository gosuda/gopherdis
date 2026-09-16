package server

import (
	"path/filepath"
	"testing"

	"github.com/gosuda/gopherdis/aof"
	"github.com/gosuda/gopherdis/commands"
	"github.com/gosuda/gopherdis/object"
)

// TestEnableAOFRoundTrip covers the persistence path end to end: a server with
// AOF enabled must replay its file into a fresh instance with the values intact.
//
// The fixtures are the shapes that actually break naive serialization: a sorted
// set whose member is a JSON blob containing spaces and quotes, a hash with
// multi-byte field values, and a plain string.
func TestEnableAOFRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "appendonly.aof")

	member := `{"name":"평석의 달인","stage":5}`

	first := NewServer()
	if err := first.EnableAOF(path, aof.FsyncAlways); err != nil {
		t.Fatalf("EnableAOF: %v", err)
	}
	ctx := &commands.Context{DB: first.DB, AOF: first.AOF}

	run := func(args ...string) []byte {
		argv := make([][]byte, len(args))
		for i, a := range args {
			argv[i] = []byte(a)
		}
		return first.Commands.Execute(ctx, argv)
	}

	run("ZADD", "ranking:global", "57847", member)
	run("HSET", "account:평석의 달인", "passwordHash", "deadbeef", "name", "평석의 달인")
	run("SET", "plain", "hello")
	run("RPUSH", "mylist", "a", "b")
	run("SADD", "myset", "x", "y")

	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	second := NewServer()
	if err := second.EnableAOF(path, aof.FsyncAlways); err != nil {
		t.Fatalf("EnableAOF on reload: %v", err)
	}
	defer second.Close()

	if got := second.DB.Len(); got != 5 {
		t.Fatalf("reloaded key count = %d, want 5", got)
	}

	obj, ok := second.DB.Get("plain")
	if !ok || string(obj.Bytes()) != "hello" {
		t.Errorf("plain string did not survive: ok=%v obj=%v", ok, obj)
	}

	ctx2 := &commands.Context{DB: second.DB}
	reply := func(args ...string) string {
		argv := make([][]byte, len(args))
		for i, a := range args {
			argv[i] = []byte(a)
		}
		return string(second.Commands.Execute(ctx2, argv))
	}

	if got := reply("ZSCORE", "ranking:global", member); got != "$5\r\n57847\r\n" {
		t.Errorf("ZSCORE after reload = %q", got)
	}
	if got := reply("HGET", "account:평석의 달인", "passwordHash"); got != "$8\r\ndeadbeef\r\n" {
		t.Errorf("HGET after reload = %q", got)
	}
	if got := reply("LLEN", "mylist"); got != ":2\r\n" {
		t.Errorf("LLEN after reload = %q", got)
	}
	if got := reply("SCARD", "myset"); got != ":2\r\n" {
		t.Errorf("SCARD after reload = %q", got)
	}

	if o, ok := second.DB.Get("myset"); ok && o.Type != object.OBJ_SET {
		t.Errorf("myset reloaded with type %v", o.Type)
	}
}
