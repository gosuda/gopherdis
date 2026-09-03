package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gosuda/gopherdis/db"
)

type mockAOF struct {
	fed [][]byte
}

func (m *mockAOF) Feed(argv [][]byte) error {
	for _, a := range argv {
		m.fed = append(m.fed, bytes.Clone(a))
	}
	return nil
}

func TestCommandTableArity(t *testing.T) {
	database := db.NewShardedDB()
	ctx := &Context{DB: database}

	// Unknown command
	res := DefaultTable.Execute(ctx, [][]byte{[]byte("FOOBAR")})
	if !bytes.Contains(res, []byte("-ERR unknown command")) {
		t.Fatalf("expected unknown command error, got %s", res)
	}

	// Arity mismatch for GET (expects exactly 2)
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("GET")})
	if !bytes.Contains(res, []byte("-ERR wrong number of arguments")) {
		t.Fatalf("expected wrong number of arguments error, got %s", res)
	}

	// Arity mismatch for SET (expects at least 3)
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("SET"), []byte("foo")})
	if !bytes.Contains(res, []byte("-ERR wrong number of arguments")) {
		t.Fatalf("expected wrong number of arguments error, got %s", res)
	}
}

func TestStringCommands(t *testing.T) {
	database := db.NewShardedDB()
	ctx := &Context{DB: database}

	// SET foo bar
	res := DefaultTable.Execute(ctx, [][]byte{[]byte("SET"), []byte("foo"), []byte("bar")})
	if string(res) != "+OK\r\n" {
		t.Fatalf("expected +OK, got %q", res)
	}

	// GET foo
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("GET"), []byte("foo")})
	if string(res) != "$3\r\nbar\r\n" {
		t.Fatalf("expected bar, got %q", res)
	}

	// INCR num -> 1
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("INCR"), []byte("num")})
	if string(res) != ":1\r\n" {
		t.Fatalf("expected :1, got %q", res)
	}

	// INCRBY num 5 -> 6
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("INCRBY"), []byte("num"), []byte("5")})
	if string(res) != ":6\r\n" {
		t.Fatalf("expected :6, got %q", res)
	}

	// DECR num -> 5
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("DECR"), []byte("num")})
	if string(res) != ":5\r\n" {
		t.Fatalf("expected :5, got %q", res)
	}

	// DECRBY num 2 -> 3
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("DECRBY"), []byte("num"), []byte("2")})
	if string(res) != ":3\r\n" {
		t.Fatalf("expected :3, got %q", res)
	}

	// APPEND foo _baz -> len 7
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("APPEND"), []byte("foo"), []byte("_baz")})
	if string(res) != ":7\r\n" {
		t.Fatalf("expected :7, got %q", res)
	}

	// STRLEN foo -> 7
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("STRLEN"), []byte("foo")})
	if string(res) != ":7\r\n" {
		t.Fatalf("expected :7, got %q", res)
	}

	// MSET a 1 b 2
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("MSET"), []byte("a"), []byte("1"), []byte("b"), []byte("2")})
	if string(res) != "+OK\r\n" {
		t.Fatalf("expected +OK, got %q", res)
	}

	// MGET foo a b missing
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("MGET"), []byte("foo"), []byte("a"), []byte("b"), []byte("missing")})
	expected := "*4\r\n$7\r\nbar_baz\r\n$1\r\n1\r\n$1\r\n2\r\n$-1\r\n"
	if string(res) != expected {
		t.Fatalf("expected %q, got %q", expected, res)
	}
}

func TestListCommands(t *testing.T) {
	database := db.NewShardedDB()
	ctx := &Context{DB: database}

	// RPUSH mylist a b c
	res := DefaultTable.Execute(ctx, [][]byte{[]byte("RPUSH"), []byte("mylist"), []byte("a"), []byte("b"), []byte("c")})
	if string(res) != ":3\r\n" {
		t.Fatalf("expected :3, got %q", res)
	}

	// LPUSH mylist z -> 4
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("LPUSH"), []byte("mylist"), []byte("z")})
	if string(res) != ":4\r\n" {
		t.Fatalf("expected :4, got %q", res)
	}

	// LLEN mylist -> 4
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("LLEN"), []byte("mylist")})
	if string(res) != ":4\r\n" {
		t.Fatalf("expected :4, got %q", res)
	}

	// LINDEX mylist 0 -> z
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("LINDEX"), []byte("mylist"), []byte("0")})
	if string(res) != "$1\r\nz\r\n" {
		t.Fatalf("expected z, got %q", res)
	}

	// LSET mylist 0 replaced
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("LSET"), []byte("mylist"), []byte("0"), []byte("replaced")})
	if string(res) != "+OK\r\n" {
		t.Fatalf("expected +OK, got %q", res)
	}

	// LRANGE mylist 0 -1
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("LRANGE"), []byte("mylist"), []byte("0"), []byte("-1")})
	expected := "*4\r\n$8\r\nreplaced\r\n$1\r\na\r\n$1\r\nb\r\n$1\r\nc\r\n"
	if string(res) != expected {
		t.Fatalf("expected %q, got %q", expected, res)
	}

	// LPOP mylist -> replaced
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("LPOP"), []byte("mylist")})
	if string(res) != "$8\r\nreplaced\r\n" {
		t.Fatalf("expected replaced, got %q", res)
	}

	// RPOP mylist -> c
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("RPOP"), []byte("mylist")})
	if string(res) != "$1\r\nc\r\n" {
		t.Fatalf("expected c, got %q", res)
	}
}

func TestHashCommands(t *testing.T) {
	database := db.NewShardedDB()
	ctx := &Context{DB: database}

	// HSET user:1 name alice age 30
	res := DefaultTable.Execute(ctx, [][]byte{[]byte("HSET"), []byte("user:1"), []byte("name"), []byte("alice"), []byte("age"), []byte("30")})
	if string(res) != ":2\r\n" {
		t.Fatalf("expected :2, got %q", res)
	}

	// HGET user:1 name
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("HGET"), []byte("user:1"), []byte("name")})
	if string(res) != "$5\r\nalice\r\n" {
		t.Fatalf("expected alice, got %q", res)
	}

	// HINCRBY user:1 age 5 -> 35
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("HINCRBY"), []byte("user:1"), []byte("age"), []byte("5")})
	if string(res) != ":35\r\n" {
		t.Fatalf("expected :35, got %q", res)
	}

	// HEXISTS user:1 age
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("HEXISTS"), []byte("user:1"), []byte("age")})
	if string(res) != ":1\r\n" {
		t.Fatalf("expected :1, got %q", res)
	}

	// HLEN user:1 -> 2
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("HLEN"), []byte("user:1")})
	if string(res) != ":2\r\n" {
		t.Fatalf("expected :2, got %q", res)
	}

	// HDEL user:1 age
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("HDEL"), []byte("user:1"), []byte("age")})
	if string(res) != ":1\r\n" {
		t.Fatalf("expected :1, got %q", res)
	}
}

func TestSetCommands(t *testing.T) {
	database := db.NewShardedDB()
	ctx := &Context{DB: database}

	// SADD myset a b c
	res := DefaultTable.Execute(ctx, [][]byte{[]byte("SADD"), []byte("myset"), []byte("a"), []byte("b"), []byte("c")})
	if string(res) != ":3\r\n" {
		t.Fatalf("expected :3, got %q", res)
	}

	// SISMEMBER myset a -> 1
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("SISMEMBER"), []byte("myset"), []byte("a")})
	if string(res) != ":1\r\n" {
		t.Fatalf("expected :1, got %q", res)
	}

	// SISMEMBER myset z -> 0
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("SISMEMBER"), []byte("myset"), []byte("z")})
	if string(res) != ":0\r\n" {
		t.Fatalf("expected :0, got %q", res)
	}

	// SCARD myset -> 3
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("SCARD"), []byte("myset")})
	if string(res) != ":3\r\n" {
		t.Fatalf("expected :3, got %q", res)
	}

	// SPOP myset -> 1 element
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("SPOP"), []byte("myset")})
	if len(res) == 0 || res[0] != '$' {
		t.Fatalf("expected bulk string for SPOP, got %q", res)
	}

	// SREM myset b -> 1
	_ = DefaultTable.Execute(ctx, [][]byte{[]byte("SREM"), []byte("myset"), []byte("b")})
}

func TestZSetCommands(t *testing.T) {
	database := db.NewShardedDB()
	ctx := &Context{DB: database}

	// ZADD myzset 100 alice 90 bob 95 charlie
	res := DefaultTable.Execute(ctx, [][]byte{[]byte("ZADD"), []byte("myzset"), []byte("100"), []byte("alice"), []byte("90"), []byte("bob"), []byte("95"), []byte("charlie")})
	if string(res) != ":3\r\n" {
		t.Fatalf("expected :3, got %q", res)
	}

	// ZSCORE myzset alice -> 100
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("ZSCORE"), []byte("myzset"), []byte("alice")})
	if string(res) != "$3\r\n100\r\n" {
		t.Fatalf("expected 100, got %q", res)
	}

	// ZINCRBY myzset 10 bob -> 100
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("ZINCRBY"), []byte("myzset"), []byte("10"), []byte("bob")})
	if !strings.Contains(string(res), "100") {
		t.Fatalf("expected score 100, got %q", res)
	}

	// ZCARD myzset -> 3
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("ZCARD"), []byte("myzset")})
	if string(res) != ":3\r\n" {
		t.Fatalf("expected :3, got %q", res)
	}

	// ZREM myzset alice -> 1
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("ZREM"), []byte("myzset"), []byte("alice")})
	if string(res) != ":1\r\n" {
		t.Fatalf("expected :1, got %q", res)
	}
}

func TestGenericAndAOFFeeding(t *testing.T) {
	database := db.NewShardedDB()
	aofMock := &mockAOF{}
	ctx := &Context{DB: database, AOF: aofMock}

	// SET k1 v1
	DefaultTable.Execute(ctx, [][]byte{[]byte("SET"), []byte("k1"), []byte("v1")})

	// EXPIRE k1 10 -> should feed PEXPIREAT to aofMock
	res := DefaultTable.Execute(ctx, [][]byte{[]byte("EXPIRE"), []byte("k1"), []byte("10")})
	if string(res) != ":1\r\n" {
		t.Fatalf("expected :1, got %q", res)
	}

	var foundPexpireat bool
	for _, b := range aofMock.fed {
		if string(b) == "PEXPIREAT" {
			foundPexpireat = true
			break
		}
	}
	if !foundPexpireat {
		t.Fatalf("expected PEXPIREAT fed to AOF, fed entries: %v", aofMock.fed)
	}

	// TTL k1
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("TTL"), []byte("k1")})
	if !strings.HasPrefix(string(res), ":") {
		t.Fatalf("expected integer reply for TTL, got %q", res)
	}

	// FLUSHDB
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("FLUSHDB")})
	if string(res) != "+OK\r\n" {
		t.Fatalf("expected +OK, got %q", res)
	}
}
