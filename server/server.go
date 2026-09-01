package server

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gosuda/beaver/pure"
	"github.com/gosuda/nedis/acl"
	"github.com/gosuda/nedis/cluster"
	"github.com/gosuda/nedis/commands"
	"github.com/gosuda/nedis/db"
	"github.com/gosuda/nedis/parser"
	"github.com/gosuda/nedis/pubsub"
	"github.com/gosuda/nedis/replication"
	"github.com/gosuda/nedis/scripting"
)


// Server is a Redis-compatible in-memory TCP server.
type Server struct {
	DB          *db.ShardedDB
	Commands    *commands.Table
	AOF         commands.AOFFeeder
	RDB         commands.RDBManager
	PubSub      *pubsub.ShardedHub
	Replication *replication.Manager
	ACL         *acl.Manager
	Scripting   *scripting.Engine
	Cluster     *cluster.ClusterManager
	listener    net.Listener
	arenaPool   *pure.Pool
	mu          sync.Mutex
	closed      bool
}

// NewServer initializes a new Server with a ShardedDB, default command table, and Beaver Arena pool.
func NewServer() *Server {
	database := db.NewShardedDB()
	return &Server{
		DB:          database,
		Commands:    commands.DefaultTable,
		PubSub:      pubsub.NewShardedHub(),
		Replication: replication.NewManager(database),
		ACL:         acl.NewManager(),
		Scripting:   scripting.NewEngine(),
		Cluster:     cluster.NewClusterManager("node_local", "127.0.0.1:6379"),
		arenaPool:   pure.NewPool(4096),
	}
}

func (s *Server) GetDB() *db.ShardedDB {
	return s.DB
}

func (s *Server) ExecuteReplicaCommand(argv [][]byte) {
	ctx := &commands.Context{
		DB: s.DB,
	}
	s.Commands.Execute(ctx, argv)
}

func (s *Server) FeedCommand(argv [][]byte) {
	if s.Replication != nil {
		s.Replication.FeedCommand(argv)
	}
}

func (s *Server) ReplicaOf(host string, port int) {
	if strings.ToLower(host) == "no" {
		s.Replication.StopReplication()
		return
	}
	s.Replication.StartReplicationClient(context.Background(), host, port, 6379, s)
}

func (s *Server) Role() string {
	if s.Replication != nil && s.Replication.Role() == replication.RoleReplica {
		return "slave"
	}
	return "master"
}

// SetAOF configures the AOF persistence engine for the server.
func (s *Server) SetAOF(aof commands.AOFFeeder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.AOF = aof
}

// SetRDB configures the RDB persistence engine for the server.
func (s *Server) SetRDB(rdb commands.RDBManager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.RDB = rdb
}

// Listen starts listening on the given TCP address and serves incoming connections.
func (s *Server) Listen(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()

	s.DB.StartCron(100 * time.Millisecond)

	for {
		conn, err := ln.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return nil
			}
			continue
		}
		go s.handleConnection(conn)
	}
}

// Addr returns the listener's network address if active.
func (s *Server) Addr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return s.listener.Addr()
	}
	return nil
}

// Close gracefully stops the listener and background tasks.
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.DB.StopCron()
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}


// handleConnection handles an individual client TCP connection lifecycle.
func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetNoDelay(true)
		_ = tcpConn.SetReadBuffer(65536)
		_ = tcpConn.SetWriteBuffer(65536)
	}

	reader := bufio.NewReaderSize(conn, 4096)
	writer := bufio.NewWriterSize(conn, 4096)

	arena := s.arenaPool.Get()
	defer s.arenaPool.Put(arena)

	sub := pubsub.NewSubscriber(s.PubSub.NextSubscriberID())
	defer func() {
		s.PubSub.UnsubscribeAll(sub)
		sub.Close()
	}()

	cmdCtx := &commands.Context{
		DB:          s.DB,
		AOF:         s.AOF,
		Tx:          commands.NewTxState(),
		RDB:         s.RDB,
		PubSub:      s.PubSub,
		Sub:         sub,
		Replication: s,
		ACL:         s.ACL,
		User:        s.ACL.GetUser("default"),
		Scripting:   s.Scripting,
		Cluster:     s.Cluster,
	}

	var writeMu sync.Mutex
	doneCh := make(chan struct{})
	defer close(doneCh)

	var pubsubOnce sync.Once
	ensurePubSubWriter := func() {
		go func() {
			for {
				select {
				case <-doneCh:
					return
				case msg, ok := <-sub.MsgCh:
					if !ok {
						return
					}
					writeMu.Lock()
					_, _ = writer.Write(msg)
					_ = writer.Flush()
					writeMu.Unlock()
				}
			}
		}()
	}

	for {
		arena.Reset()

		argv, err := parser.ParseRequest(reader, arena)
		if err != nil {
			if err == io.EOF || errorsIsClosed(err) {
				return
			}
			writeMu.Lock()
			writeError(writer, fmt.Sprintf("ERR %v", err))
			writer.Flush()
			writeMu.Unlock()
			return
		}

		if len(argv) == 0 {
			continue
		}

		// Handle PSYNC replication command from replica
		if strings.ToLower(string(argv[0])) == "psync" && s.Replication != nil {
			replID := ""
			offset := int64(-1)
			if len(argv) >= 2 {
				replID = string(argv[1])
			}
			if len(argv) >= 3 {
				offset, _ = strconv.ParseInt(string(argv[2]), 10, 64)
			}
			session := s.Replication.RegisterReplica()
			defer s.Replication.UnregisterReplica(session)

			header, payload, err := s.Replication.HandlePSync(session, replID, offset)
			if err != nil {
				writeMu.Lock()
				writeError(writer, err.Error())
				writer.Flush()
				writeMu.Unlock()
				return
			}

			writeMu.Lock()
			writer.Write(header)
			writer.Write(payload)
			writer.Flush()
			writeMu.Unlock()

			// Stream write commands continuously to this replica
			for msg := range session.MsgCh {
				writeMu.Lock()
				_, _ = writer.Write(msg)
				_ = writer.Flush()
				writeMu.Unlock()
			}
			return
		}

		cmdName := strings.ToLower(string(argv[0]))
		if cmdName == "subscribe" || cmdName == "psubscribe" || cmdName == "ssubscribe" {
			pubsubOnce.Do(ensurePubSubWriter)
		}

		// When subscribed, only PubSub control commands and ping/quit/reset are permitted
		if sub.HasSubscriptions() {
			switch cmdName {
			case "subscribe", "unsubscribe", "psubscribe", "punsubscribe", "ping", "quit", "reset":
				// Permitted
			default:
				writeMu.Lock()
				writeError(writer, "ERR only (P)SUBSCRIBE / (P)UNSUBSCRIBE / PING / QUIT are allowed in this context")
				writer.Flush()
				writeMu.Unlock()
				continue
			}
		}

		reply := s.Commands.Execute(cmdCtx, argv)

		writeMu.Lock()
		if _, err := writer.Write(reply); err != nil {
			writeMu.Unlock()
			return
		}
		if err := writer.Flush(); err != nil {
			writeMu.Unlock()
			return
		}
		writeMu.Unlock()
	}
}

func writeError(w *bufio.Writer, msg string) {
	w.WriteString("-" + msg + "\r\n")
}

func errorsIsClosed(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "use of closed network connection")
}

