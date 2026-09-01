package acl

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"sync"
)

var (
	ErrUserDisabled = errors.New("WRONGPASS User is disabled")
	ErrAuthFailed   = errors.New("WRONGPASS invalid username-password pair or user is disabled")
)

// ACL Category Flags mapped to Command Flags
const (
	CatRead   uint64 = 1 << 0
	CatWrite  uint64 = 1 << 1
	CatAdmin  uint64 = 1 << 2
	CatFast   uint64 = 1 << 3
	CatPubSub uint64 = 1 << 4
)

// User represents an ACL user account with fine-grained access rules.
type User struct {
	mu                  sync.RWMutex
	Name                string
	Enabled             bool
	NoPass              bool
	Passwords           map[string]struct{} // SHA256 hex hashes
	AllCommands         bool
	AllowedCommands     map[string]struct{}
	DisallowedCommands  map[string]struct{}
	AllowedCategories   uint64
	DisallowedCategories uint64
	AllKeys             bool
	KeyPatterns         []string
	AllChannels         bool
	ChannelPatterns     []string
}

// NewUser creates a new user instance. If isDefault is true, grants full permissions.
func NewUser(name string, isDefault bool) *User {
	u := &User{
		Name:               name,
		Passwords:          make(map[string]struct{}),
		AllowedCommands:    make(map[string]struct{}),
		DisallowedCommands: make(map[string]struct{}),
	}

	if isDefault {
		u.Enabled = true
		u.NoPass = true
		u.AllCommands = true
		u.AllKeys = true
		u.AllChannels = true
	} else {
		u.Enabled = false
		u.NoPass = false
	}

	return u
}

// HashPassword returns the SHA256 hex string of a password.
func HashPassword(pass string) string {
	sum := sha256.Sum256([]byte(pass))
	return hex.EncodeToString(sum[:])
}

// CheckPassword verifies if password matches any of the registered SHA256 hashes.
func (u *User) CheckPassword(pass string) bool {
	u.mu.RLock()
	defer u.mu.RUnlock()

	if !u.Enabled {
		return false
	}
	if u.NoPass {
		return true
	}

	hash := HashPassword(pass)
	_, ok := u.Passwords[hash]
	return ok
}

// ApplyRule modifies user permissions according to standard Redis ACL syntax.
func (u *User) ApplyRule(rule string) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	rule = strings.TrimSpace(rule)
	if rule == "on" {
		u.Enabled = true
	} else if rule == "off" {
		u.Enabled = false
	} else if rule == "nopass" {
		u.NoPass = true
	} else if rule == "resetpass" {
		u.NoPass = false
		u.Passwords = make(map[string]struct{})
	} else if strings.HasPrefix(rule, ">") {
		pass := rule[1:]
		u.Passwords[HashPassword(pass)] = struct{}{}
		u.NoPass = false
	} else if strings.HasPrefix(rule, "#") {
		hash := rule[1:]
		u.Passwords[strings.ToLower(hash)] = struct{}{}
		u.NoPass = false
	} else if strings.HasPrefix(rule, "<") {
		pass := rule[1:]
		delete(u.Passwords, HashPassword(pass))
	} else if rule == "+@all" || rule == "allcommands" {
		u.AllCommands = true
		u.DisallowedCommands = make(map[string]struct{})
		u.DisallowedCategories = 0
	} else if rule == "-@all" || rule == "nocommands" {
		u.AllCommands = false
		u.AllowedCommands = make(map[string]struct{})
		u.AllowedCategories = 0
	} else if strings.HasPrefix(rule, "+@") {
		cat := strings.ToLower(rule[2:])
		u.AllowedCategories |= parseCategory(cat)
	} else if strings.HasPrefix(rule, "-@") {
		cat := strings.ToLower(rule[2:])
		u.DisallowedCategories |= parseCategory(cat)
	} else if strings.HasPrefix(rule, "+") {
		cmd := strings.ToLower(rule[1:])
		u.AllowedCommands[cmd] = struct{}{}
		delete(u.DisallowedCommands, cmd)
	} else if strings.HasPrefix(rule, "-") {
		cmd := strings.ToLower(rule[1:])
		u.DisallowedCommands[cmd] = struct{}{}
		delete(u.AllowedCommands, cmd)
	} else if rule == "~*" || rule == "allkeys" {
		u.AllKeys = true
	} else if rule == "resetkeys" {
		u.AllKeys = false
		u.KeyPatterns = nil
	} else if strings.HasPrefix(rule, "~") {
		pat := rule[1:]
		u.KeyPatterns = append(u.KeyPatterns, pat)
	} else if rule == "&*" || rule == "allchannels" {
		u.AllChannels = true
	} else if rule == "resetchannels" {
		u.AllChannels = false
		u.ChannelPatterns = nil
	} else if strings.HasPrefix(rule, "&") {
		pat := rule[1:]
		u.ChannelPatterns = append(u.ChannelPatterns, pat)
	} else if rule == "reset" {
		u.Enabled = false
		u.NoPass = false
		u.AllCommands = false
		u.AllowedCommands = make(map[string]struct{})
		u.DisallowedCommands = make(map[string]struct{})
		u.AllowedCategories = 0
		u.DisallowedCategories = 0
		u.AllKeys = false
		u.KeyPatterns = nil
		u.AllChannels = false
		u.ChannelPatterns = nil
		u.Passwords = make(map[string]struct{})
	}

	return nil
}

func parseCategory(cat string) uint64 {
	switch cat {
	case "read":
		return CatRead
	case "write":
		return CatWrite
	case "admin":
		return CatAdmin
	case "fast":
		return CatFast
	case "pubsub":
		return CatPubSub
	default:
		return 0
	}
}

// CanExecute checks if user has permission to execute the command.
func (u *User) CanExecute(cmdName string, cmdFlags uint64) bool {
	u.mu.RLock()
	defer u.mu.RUnlock()

	if !u.Enabled {
		return false
	}

	// 1. Explicit deny
	if _, denied := u.DisallowedCommands[cmdName]; denied {
		return false
	}
	if u.DisallowedCategories&cmdFlags != 0 {
		return false
	}

	// 2. Explicit allow
	if _, allowed := u.AllowedCommands[cmdName]; allowed {
		return true
	}
	if u.AllowedCategories&cmdFlags != 0 {
		return true
	}

	return u.AllCommands
}

// CanAccessKey checks if user has permission to access the target key.
func (u *User) CanAccessKey(key string) bool {
	u.mu.RLock()
	defer u.mu.RUnlock()

	if !u.Enabled {
		return false
	}
	if u.AllKeys {
		return true
	}

	for _, pat := range u.KeyPatterns {
		if matched, _ := filepath.Match(pat, key); matched {
			return true
		}
	}
	return false
}

// CanAccessChannel checks if user has permission to access the target Pub/Sub channel.
func (u *User) CanAccessChannel(channel string) bool {
	u.mu.RLock()
	defer u.mu.RUnlock()

	if !u.Enabled {
		return false
	}
	if u.AllChannels {
		return true
	}

	for _, pat := range u.ChannelPatterns {
		if matched, _ := filepath.Match(pat, channel); matched {
			return true
		}
	}
	return false
}
