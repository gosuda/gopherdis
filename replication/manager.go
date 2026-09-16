package replication

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gosuda/gopherdis/db"
	"github.com/gosuda/gopherdis/rdb"
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

	// Done is closed when the master gives up on this replica, either because it
	// fell too far behind or because it was unregistered. The connection goroutine
	// selects on it so a dead replica cannot leak a goroutine forever.
	Done chan struct{}

	closeOnce sync.Once
}

// close releases the session. Safe to call repeatedly and from several goroutines.
func (r *ReplicaSession) close() {
	r.closeOnce.Do(func() { close(r.Done) })
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

	// Fan out to active replicas. A replica whose queue is full has fallen behind:
	// dropping the write and carrying on leaves it silently diverged from the
	// master forever, so disconnect it instead and let it come back for a full
	// resync.
	var stalled []*ReplicaSession
	for _, r := range m.replicas {
		select {
		case r.MsgCh <- data:
		default:
			stalled = append(stalled, r)
		}
	}
	for _, r := range stalled {
		delete(m.replicas, r.ID)
	}
	m.mu.Unlock()

	for _, r := range stalled {
		r.close()
	}
}

// selectPreamble is the SELECT every replication stream opens with. A replica
// applies the stream against whatever database it last selected, so the stream
// has to state which one it targets before the first write.
var selectPreamble = []byte("*2\r\n$6\r\nSELECT\r\n$1\r\n0\r\n")

// newSessionLocked allocates a replica session. Caller must hold m.mu.
func (m *Manager) newSessionLocked() *ReplicaSession {
	id := atomic.AddUint64(&m.nextReplicaID, 1)
	session := &ReplicaSession{
		ID:    id,
		MsgCh: make(chan []byte, 1024),
		Done:  make(chan struct{}),
	}
	// Seeded rather than sent on the first write, so a replica that connects and
	// receives nothing still knows which database the stream belongs to.
	session.MsgCh <- selectPreamble
	m.replicas[id] = session
	return session
}

// RegisterReplica registers a replica connection to receive live streamed writes.
func (m *Manager) RegisterReplica() *ReplicaSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.newSessionLocked()
}

// UnregisterReplica removes a disconnected replica.
func (m *Manager) UnregisterReplica(session *ReplicaSession) {
	if session == nil {
		return
	}
	m.mu.Lock()
	delete(m.replicas, session.ID)
	m.mu.Unlock()
	session.close()
}

// HandlePSync handles PSYNC negotiation from a replica and registers it.
//
// Registration happens inside the same critical section that produces the RDB
// snapshot, because FeedCommand takes the same lock: registering first left a
// window in which a write was captured by the snapshot *and* queued for the new
// replica, so the replica applied it twice.
// Returns (session, initial_reply, initial_payload_bytes, error)
func (m *Manager) HandlePSync(replID string, offset int64) (*ReplicaSession, []byte, []byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session := m.newSessionLocked()

	// Check if partial resync is possible
	if replID != "" && replID == m.masterReplID && m.backlog.CanPartialSync(offset) {
		diff := m.backlog.ReadFromOffset(offset)
		resp := fmt.Sprintf("+CONTINUE %s\r\n", m.masterReplID)
		return session, []byte(resp), diff, nil
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

	return session, []byte(header), payload.Bytes(), nil
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
