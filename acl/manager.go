package acl

import (
	"sync"
)

// Manager coordinates user accounts and authentications.
type Manager struct {
	mu    sync.RWMutex
	users map[string]*User
}

// NewManager initializes an ACL manager with a default unrestricted user.
func NewManager() *Manager {
	m := &Manager{
		users: make(map[string]*User),
	}
	m.users["default"] = NewUser("default", true)
	return m
}

// GetUser returns the user object by name or nil if absent.
func (m *Manager) GetUser(name string) *User {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.users[name]
}

// GetOrCreateUser returns an existing user or creates a new disabled user.
func (m *Manager) GetOrCreateUser(name string) *User {
	m.mu.Lock()
	defer m.mu.Unlock()

	u, exists := m.users[name]
	if !exists {
		u = NewUser(name, false)
		m.users[name] = u
	}
	return u
}

// DelUser removes users by name. The default user cannot be deleted.
func (m *Manager) DelUser(name string) bool {
	if name == "default" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.users[name]; exists {
		delete(m.users, name)
		return true
	}
	return false
}

// ListUsers returns a list of all user names.
func (m *Manager) ListUsers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	res := make([]string, 0, len(m.users))
	for name := range m.users {
		res = append(res, name)
	}
	return res
}

// Auth authenticates credentials and returns the User if successful.
func (m *Manager) Auth(username, password string) (*User, error) {
	if username == "" {
		username = "default"
	}

	u := m.GetUser(username)
	if u == nil {
		return nil, ErrAuthFailed
	}

	if !u.CheckPassword(password) {
		return nil, ErrAuthFailed
	}

	return u, nil
}
