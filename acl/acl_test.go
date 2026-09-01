package acl

import (
	"testing"
)

func TestACL_UserRules(t *testing.T) {
	mgr := NewManager()

	// Default user
	def := mgr.GetUser("default")
	if !def.CanExecute("set", CatWrite) || !def.CanAccessKey("anykey") {
		t.Fatalf("default user should have all permissions")
	}

	// Create user alice
	alice := mgr.GetOrCreateUser("alice")
	_ = alice.ApplyRule("on")
	_ = alice.ApplyRule(">secret123")
	_ = alice.ApplyRule("~cached:*")
	_ = alice.ApplyRule("+get")
	_ = alice.ApplyRule("+mget")
	_ = alice.ApplyRule("-set")

	if !alice.CheckPassword("secret123") {
		t.Fatalf("alice password check failed")
	}
	if alice.CheckPassword("wrong") {
		t.Fatalf("alice wrong password should fail")
	}

	// Permissions
	if !alice.CanExecute("get", CatRead) {
		t.Fatalf("alice should be allowed to get")
	}
	if alice.CanExecute("set", CatWrite) {
		t.Fatalf("alice should not be allowed to set")
	}

	// Key patterns
	if !alice.CanAccessKey("cached:users") {
		t.Fatalf("alice should be able to access cached:users")
	}
	if alice.CanAccessKey("db:users") {
		t.Fatalf("alice should not be able to access db:users")
	}
}
