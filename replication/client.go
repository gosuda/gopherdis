package replication

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/gosuda/beaver/pure"
	"github.com/gosuda/gopherdis/db"
	"github.com/gosuda/gopherdis/parser"
	"github.com/gosuda/gopherdis/rdb"
)

// ServerTarget abstracts the server capabilities needed by the replication client.
type ServerTarget interface {
	GetDB() *db.ShardedDB
	ExecuteReplicaCommand(argv [][]byte)
}

// StartReplicationClient launches the replication background handshake and stream consumer.
func (m *Manager) StartReplicationClient(ctx context.Context, masterHost string, masterPort int, myPort int, target ServerTarget) {
	m.mu.Lock()
	if m.cancelSync != nil {
		m.cancelSync()
	}
	syncCtx, cancel := context.WithCancel(ctx)
	m.cancelSync = cancel
	m.role = RoleReplica
	m.masterHost = masterHost
	m.masterPort = masterPort
	m.mu.Unlock()

	go m.replicationLoop(syncCtx, masterHost, masterPort, myPort, target)
}

// StopReplication stops syncing and promotes server back to master.
func (m *Manager) StopReplication() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancelSync != nil {
		m.cancelSync()
		m.cancelSync = nil
	}
	m.role = RoleMaster
	m.masterHost = ""
	m.masterPort = 0
}

func (m *Manager) replicationLoop(ctx context.Context, host string, port int, myPort int, target ServerTarget) {
	addr := fmt.Sprintf("%s:%d", host, port)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := m.syncWithMaster(ctx, addr, myPort, target)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(1 * time.Second):
				// Reconnect retry
			}
		}
	}
}

func (m *Manager) syncWithMaster(ctx context.Context, addr string, myPort int, target ServerTarget) error {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	reader := bufio.NewReaderSize(conn, 64*1024)

	// 1. PING
	if _, err := conn.Write([]byte("*1\r\n$4\r\nPING\r\n")); err != nil {
		return err
	}
	line, err := reader.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "+PONG") {
		return fmt.Errorf("master ping failed: %q", line)
	}

	// 2. REPLCONF listening-port
	replConfPort := fmt.Sprintf("*3\r\n$8\r\nREPLCONF\r\n$14\r\nlistening-port\r\n$%d\r\n%d\r\n", len(strconv.Itoa(myPort)), myPort)
	if _, err := conn.Write([]byte(replConfPort)); err != nil {
		return err
	}
	line, err = reader.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "+OK") {
		return fmt.Errorf("replconf listening-port failed: %q", line)
	}

	// 3. REPLCONF capa psync2
	if _, err := conn.Write([]byte("*3\r\n$8\r\nREPLCONF\r\n$4\r\ncapa\r\n$6\r\npsync2\r\n")); err != nil {
		return err
	}
	line, err = reader.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "+OK") {
		return fmt.Errorf("replconf capa failed: %q", line)
	}

	// 4. PSYNC
	m.mu.RLock()
	reqReplID := m.masterReplID
	reqOffset := m.masterReplOffset
	if reqReplID == "" {
		reqReplID = "?"
		reqOffset = -1
	}
	m.mu.RUnlock()

	psyncCmd := fmt.Sprintf("*3\r\n$5\r\nPSYNC\r\n$%d\r\n%s\r\n$%d\r\n%d\r\n", len(reqReplID), reqReplID, len(strconv.FormatInt(reqOffset, 10)), reqOffset)
	if _, err := conn.Write([]byte(psyncCmd)); err != nil {
		return err
	}

	line, err = reader.ReadString('\n')
	if err != nil {
		return err
	}

	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "+FULLRESYNC") {
		parts := strings.Split(trimmed, " ")
		if len(parts) >= 3 {
			newReplID := parts[1]
			newOffset, _ := strconv.ParseInt(parts[2], 10, 64)
			m.SetMasterInfo(newReplID, newOffset)
		}

		// Read RDB Payload ($<len>\r\n<bytes>)
		lenLine, err := reader.ReadString('\n')
		if err != nil || !strings.HasPrefix(lenLine, "$") {
			return fmt.Errorf("expected RDB bulk len, got %q", lenLine)
		}
		rdbLen, err := strconv.ParseInt(strings.TrimSpace(lenLine[1:]), 10, 64)
		if err != nil {
			return err
		}

		rdbBytes := make([]byte, rdbLen)
		if _, err := io.ReadFull(reader, rdbBytes); err != nil {
			return err
		}

		// Clear local DB and load RDB snapshot
		localDB := target.GetDB()
		localDB.FlushAll()

		dec := rdb.NewDecoder(bytes.NewReader(rdbBytes))
		if err := dec.Load(localDB); err != nil {
			return fmt.Errorf("rdb load failed during replication: %w", err)
		}
	} else if strings.HasPrefix(trimmed, "+CONTINUE") {
		// Partial resynchronization accepted
	} else {
		return fmt.Errorf("unexpected PSYNC reply: %q", trimmed)
	}

	// 5. Stream Live Commands from Master
	arena := pure.NewPool(4096).Get()
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		arena.Reset()
		argv, err := parser.ParseRequest(reader, arena)
		if err != nil {
			return err
		}

		if len(argv) == 0 {
			continue
		}

		// Execute master command directly on local DB without re-replicating
		target.ExecuteReplicaCommand(argv)
	}
}
