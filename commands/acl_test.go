package commands

import (
	"strings"
	"testing"

	"github.com/gosuda/nedis/acl"
	"github.com/gosuda/nedis/db"
)

func TestACLCommands(t *testing.T) {
	database := db.NewShardedDB()
	aclMgr := acl.NewManager()
	ctx := &Context{
		DB:   database,
		ACL:  aclMgr,
		User: aclMgr.GetUser("default"),
	}

	// 1. ACL WHOAMI -> default
	res := DefaultTable.Execute(ctx, [][]byte{[]byte("ACL"), []byte("WHOAMI")})
	if string(res) != "$7\r\ndefault\r\n" {
		t.Fatalf("expected default from WHOAMI, got %q", res)
	}

	// 2. ACL SETUSER alice on >secret123 +@read +set -del +acl
	res = DefaultTable.Execute(ctx, [][]byte{
		[]byte("ACL"), []byte("SETUSER"), []byte("alice"),
		[]byte("on"), []byte(">secret123"), []byte("+@read"), []byte("+set"), []byte("-del"), []byte("+acl"),
	})
	if string(res) != "+OK\r\n" {
		t.Fatalf("expected +OK on ACL SETUSER, got %q", res)
	}

	// 3. AUTH alice wrongpass -> WRONGPASS error
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("AUTH"), []byte("alice"), []byte("wrongpass")})
	if !strings.HasPrefix(string(res), "-WRONGPASS") {
		t.Fatalf("expected WRONGPASS error, got %q", res)
	}

	// 4. AUTH alice secret123 -> +OK
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("AUTH"), []byte("alice"), []byte("secret123")})
	if string(res) != "+OK\r\n" {
		t.Fatalf("expected +OK on correct AUTH, got %q", res)
	}

	// 5. ACL WHOAMI -> alice
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("ACL"), []byte("WHOAMI")})
	if string(res) != "$5\r\nalice\r\n" {
		t.Fatalf("expected alice from WHOAMI, got %q", res)
	}

	// 6. SET k v -> allowed (+set)
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("SET"), []byte("k"), []byte("v")})
	if string(res) != "+OK\r\n" {
		t.Fatalf("expected +OK on SET for alice, got %q", res)
	}

	// 7. GET k -> allowed (+@read)
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("GET"), []byte("k")})
	if string(res) != "$1\r\nv\r\n" {
		t.Fatalf("expected $1\\r\\nv\\r\\n on GET, got %q", res)
	}

	// 8. DEL k -> denied (-del) -> -NOPERM
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("DEL"), []byte("k")})
	if !strings.HasPrefix(string(res), "-NOPERM") {
		t.Fatalf("expected -NOPERM on DEL for alice, got %q", res)
	}
}
