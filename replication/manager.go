package replication

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gosuda/nedis/db"
	"github.com/gosuda/nedis/rdb"
)

type Role int

const (
	RoleMaster Role = iota
	RoleReplica
)

// ReplicaSession tracks an individual connected replica listening to master's stream.
type ReplicaSession struct {
	ID    uint64
	MsgCh chan []byte
}

// Manager manages master and replica synchronization lifecycle.
type Manager struct {
	mu               sync.RWMutex
	role             Role
	masterHost       string
	masterPort       int
	masterReplID     string
	masterReplOffset int64
	backlog          *Backlog
	replicas         map[uint64]*ReplicaSession
	nextReplicaID    uint64
	db               *db.ShardedDB
	cancelSync       context.CancelFunc
}

// NewManager creates a new ReplicationManager for the server.
func NewManager(database *db.ShardedDB) *Manager {
	replID := generateRandomHex(20) // 40 hex chars
	return &Manager{
		role:             RoleMaster,
		masterReplID:     replID,
		masterReplOffset: 0,
		backlog:          NewBacklog(1024 * 1024),
		replicas:         make(map[uint64]*ReplicaSession),
		db:               database,
	}
}

func generateRandomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Role returns the current replication role.
func (m *Manager) Role() Role {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.role
}

// MasterInfo returns master host, port, and master repl ID.
func (m *Manager) MasterInfo() (Role, string, int, string, int64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.role, m.masterHost, m.masterPort, m.masterReplID, m.masterReplOffset
}

// FeedCommand serializes a write command and feeds it to backlog and connected replicas.
func (m *Manager) FeedCommand(argv [][]byte) {
	if len(argv) == 0 {
		return
	}

	// Format as RESP Array
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("*%d\r\n", len(argv)))
	for _, arg := range argv {
		buf.WriteString(fmt.Sprintf("$%d\r\n%s\r\n", len(arg), arg))
	}
	data := buf.Bytes()

	m.mu.Lock()
	m.backlog.Feed(data)
	m.masterReplOffset += int64(len(data))

	// Fan out to active replicas
	for _, r := range m.replicas {
		select {
		case r.MsgCh <- data:
		default:
		}
	}
	m.mu.Unlock()
}

// RegisterReplica registers a replica connection to receive live streamed writes.
func (m *Manager) RegisterReplica() *ReplicaSession {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := atomic.AddUint64(&m.nextReplicaID, 1)
	session := &ReplicaSession{
		ID:    id,
		MsgCh: make(chan []byte, 1024),
	}
	m.replicas[id] = session
	return session
}

// UnregisterReplica removes a disconnected replica.
func (m *Manager) UnregisterReplica(session *ReplicaSession) {
	if session == nil {
		return
	}
	m.mu.Lock()
	delete(m.replicas, session.ID)
	m.mu.Unlock()
}

// HandlePSync handles PSYNC negotiation from a replica.
// Returns (initial_reply, initial_payload_bytes, error)
func (m *Manager) HandlePSync(session *ReplicaSession, replID string, offset int64) ([]byte, []byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if partial resync is possible
	if replID != "" && replID == m.masterReplID && m.backlog.CanPartialSync(offset) {
		diff := m.backlog.ReadFromOffset(offset)
		resp := fmt.Sprintf("+CONTINUE %s\r\n", m.masterReplID)
		return []byte(resp), diff, nil
	}

	// Full Resynchronization
	var rdbBuf bytes.Buffer
	enc := rdb.NewEncoder(&rdbBuf)
	_ = enc.WriteHeader()
	_ = enc.WriteStandardAuxFields()
	_ = enc.WriteSelectDB(0)

	_ = m.db.ForEachShardSnapshot(func(entries []db.DBEntry) error {
		for _, entry := range entries {
			_ = enc.WriteEntry(entry)
		}
		return nil
	})
	_ = enc.WriteFooter()

	rdbBytes := rdbBuf.Bytes()
	header := fmt.Sprintf("+FULLRESYNC %s %d\r\n", m.masterReplID, m.masterReplOffset)

	var payload bytes.Buffer
	payload.WriteString(fmt.Sprintf("$%d\r\n", len(rdbBytes)))
	payload.Write(rdbBytes)

	return []byte(header), payload.Bytes(), nil
}

// SetMasterInfo updates master replication coordinates when becoming a replica.
func (m *Manager) SetMasterInfo(replID string, offset int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if replID != "" {
		m.masterReplID = replID
	}
	m.masterReplOffset = offset
}
