package commands

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gosuda/gopherdis/acl"
	"github.com/gosuda/gopherdis/cluster"
	"github.com/gosuda/gopherdis/db"
	"github.com/gosuda/gopherdis/pubsub"
	"github.com/gosuda/gopherdis/scripting"
)

// CommandFlags specifies behavior attributes of commands.
type CommandFlags int

const (
	FlagWrite    CommandFlags = 1 << iota // Modifies dataset
	FlagReadOnly                          // Read-only operation
	FlagAdmin                             // Administrative command
	FlagFast                              // O(1) or fast latency command
)

// AOFFeeder abstracts the append-only file feeding mechanism.
type AOFFeeder interface {
	Feed(argv [][]byte) error
}

// RDBManager abstracts the RDB snapshot manager.
type RDBManager interface {
	Save(targetDB *db.ShardedDB) error
	BGSave(targetDB *db.ShardedDB, onComplete func(err error)) error
}

// PubSubHub abstracts the pubsub broker.
type PubSubHub interface {
	Subscribe(sub *pubsub.Subscriber, channel string) int
	Unsubscribe(sub *pubsub.Subscriber, channel string) int
	PSubscribe(sub *pubsub.Subscriber, pattern string) int
	PUnsubscribe(sub *pubsub.Subscriber, pattern string) int
	UnsubscribeAll(sub *pubsub.Subscriber)
	Publish(channel string, message []byte) int
	PubSubChannels(pattern string) []string
	PubSubNumSub(channels []string) map[string]int
	PubSubNumPat() int
}

// ReplicationFeeder abstracts master-replica write feeding and replica management.
type ReplicationFeeder interface {
	FeedCommand(argv [][]byte)
	ReplicaOf(host string, port int)
	Role() string
}

// Context encapsulates execution context passed to command handlers.
type Context struct {
	DB          *db.ShardedDB
	AOF         AOFFeeder
	Tx          *TxState
	RDB         RDBManager
	PubSub      PubSubHub
	Sub         *pubsub.Subscriber
	Replication ReplicationFeeder
	ACL         *acl.Manager
	User        *acl.User
	Scripting   *scripting.Engine
	Cluster     *cluster.ClusterManager
	InTxExecution bool
}

// CommandHandler is the signature for command execution functions.
type CommandHandler func(ctx *Context, argv [][]byte) []byte

// Command represents a Redis command specification.
type Command struct {
	Name    string         // Command name in lowercase
	Handler CommandHandler // Execution function
	Arity   int            // > 0 exact count, < 0 at least -Arity count (e.g. -2 = >= 2)
	Flags   CommandFlags   // Attribute flags
}

// Table maps command names to their command definitions.
type Table struct {
	cmds map[string]*Command
}

// NewTable creates an empty command table.
func NewTable() *Table {
	return &Table{
		cmds: make(map[string]*Command),
	}
}

// Register adds a command definition to the table.
func (t *Table) Register(cmd *Command) {
	name := strings.ToLower(cmd.Name)
	cmd.Name = name
	t.cmds[name] = cmd
}

// Lookup finds a command by name (case-insensitive).
func (t *Table) Lookup(name string) *Command {
	return t.cmds[strings.ToLower(name)]
}

// Count returns the number of registered commands.
func (t *Table) Count() int {
	return len(t.cmds)
}

// AllNames returns a slice of all registered command names.
func (t *Table) AllNames() []string {
	names := make([]string, 0, len(t.cmds))
	for name := range t.cmds {
		names = append(names, name)
	}
	return names
}

// Execute validates arguments, queues if in MULTI mode, executes the requested command, and automatically feeds AOF on writes.
func (t *Table) Execute(ctx *Context, argv [][]byte) []byte {
	if len(argv) == 0 {
		return Error("empty command")
	}

	name := string(argv[0])
	cmd := t.Lookup(name)
	if cmd == nil {
		if ctx != nil && ctx.Tx != nil && ctx.Tx.InMulti {
			ctx.Tx.DirtyCAS = true
		}
		return Error(fmt.Sprintf("unknown command '%s'", name))
	}

	argc := len(argv)
	if cmd.Arity > 0 && argc != cmd.Arity {
		if ctx != nil && ctx.Tx != nil && ctx.Tx.InMulti {
			ctx.Tx.DirtyCAS = true
		}
		return Error(fmt.Sprintf("wrong number of arguments for '%s' command", cmd.Name))
	}
	if cmd.Arity < 0 && argc < -cmd.Arity {
		if ctx != nil && ctx.Tx != nil && ctx.Tx.InMulti {
			ctx.Tx.DirtyCAS = true
		}
		return Error(fmt.Sprintf("wrong number of arguments for '%s' command", cmd.Name))
	}

	// ACL Permission Check
	if ctx != nil && ctx.User != nil {
		var cat uint64 = 0
		if cmd.Flags&FlagReadOnly != 0 {
			cat |= acl.CatRead
		}
		if cmd.Flags&FlagWrite != 0 {
			cat |= acl.CatWrite
		}
		if cmd.Flags&FlagAdmin != 0 {
			cat |= acl.CatAdmin
		}
		if cmd.Flags&FlagFast != 0 {
			cat |= acl.CatFast
		}
		cmdLower := strings.ToLower(cmd.Name)
		if cmdLower != "auth" && !ctx.User.CanExecute(cmdLower, cat) {
			return []byte(fmt.Sprintf("-NOPERM this user has no permissions to run the '%s' command\r\n", cmd.Name))
		}
	}

	// Cluster Slot Redirection (-MOVED) Check
	if ctx != nil && ctx.Cluster != nil && ctx.Cluster.ClusterEnabled && len(argv) >= 2 {
		cmdLower := strings.ToLower(cmd.Name)
		if cmdLower != "cluster" && cmdLower != "auth" && cmdLower != "ping" && cmdLower != "info" && cmdLower != "quit" {
			key := string(argv[1])
			if isLocal, slot, ownerAddr := ctx.Cluster.IsSlotLocal(key); !isLocal {
				return []byte(fmt.Sprintf("-MOVED %d %s\r\n", slot, ownerAddr))
			}
		}
	}

	// If transaction is active, queue non-transaction control commands
	if ctx != nil && ctx.Tx != nil && ctx.Tx.InMulti {
		lower := strings.ToLower(name)
		if lower != "exec" && lower != "discard" && lower != "multi" && lower != "watch" {
			clonedArgv := make([][]byte, len(argv))
			for i, arg := range argv {
				clonedArgv[i] = bytes.Clone(arg)
			}
			ctx.Tx.QueuedCmds = append(ctx.Tx.QueuedCmds, clonedArgv)
			return Queued()
		}
	}

	// Acquire operation lock on DB for transaction isolation (unless inside an active EXEC/EVAL transaction)
	if ctx != nil && ctx.DB != nil && !ctx.InTxExecution {
		cmdLower := strings.ToLower(cmd.Name)
		if cmdLower != "exec" && cmdLower != "eval" && cmdLower != "evalsha" {
			ctx.DB.BeginOp()
			defer ctx.DB.EndOp()
		}
	}

	reply := cmd.Handler(ctx, argv)

	// Intercept write commands for automatic AOF & Replication feeding
	if (cmd.Flags&FlagWrite) != 0 && ctx != nil {
		// Only feed if execution succeeded (not an error reply)
		if len(reply) > 0 && reply[0] != '-' {
			if ctx.AOF != nil {
				feedCommand(ctx.AOF, cmd.Name, argv, reply)
			}
			if ctx.Replication != nil {
				ctx.Replication.FeedCommand(argv)
			}
		}
	}

	return reply
}

func Queued() []byte {
	return []byte("+QUEUED\r\n")
}

// feedCommand handles fast-path zero-copy feeding and slow-path command normalization (e.g., EXPIRE -> PEXPIREAT).
func feedCommand(aof AOFFeeder, cmdName string, argv [][]byte, reply []byte) {
	switch cmdName {
	case "expire":
		if bytes.Equal(reply, []byte(":1\r\n")) && len(argv) >= 3 {
			if secs, err := strconv.ParseInt(string(argv[2]), 10, 64); err == nil {
				absMs := time.Now().UnixMilli() + secs*1000
				aof.Feed([][]byte{
					[]byte("PEXPIREAT"),
					argv[1],
					[]byte(strconv.FormatInt(absMs, 10)),
				})
				return
			}
		}
	case "pexpire":
		if bytes.Equal(reply, []byte(":1\r\n")) && len(argv) >= 3 {
			if ms, err := strconv.ParseInt(string(argv[2]), 10, 64); err == nil {
				absMs := time.Now().UnixMilli() + ms
				aof.Feed([][]byte{
					[]byte("PEXPIREAT"),
					argv[1],
					[]byte(strconv.FormatInt(absMs, 10)),
				})
				return
			}
		}
	case "expireat":
		if bytes.Equal(reply, []byte(":1\r\n")) && len(argv) >= 3 {
			if unixSec, err := strconv.ParseInt(string(argv[2]), 10, 64); err == nil {
				absMs := unixSec * 1000
				aof.Feed([][]byte{
					[]byte("PEXPIREAT"),
					argv[1],
					[]byte(strconv.FormatInt(absMs, 10)),
				})
				return
			}
		}
	default:
		// Fast-path: Zero-copy, zero-allocation pass through
		aof.Feed(argv)
	}
}

// Global default table containing all standard commands.
var DefaultTable = NewTable()

// Helper functions for formatting RESP responses

func OK() []byte {
	return []byte("+OK\r\n")
}

func PONG() []byte {
	return []byte("+PONG\r\n")
}

func SimpleString(s string) []byte {
	return []byte("+" + s + "\r\n")
}

func Error(msg string) []byte {
	return []byte("-ERR " + msg + "\r\n")
}

func Integer(n int64) []byte {
	return []byte(fmt.Sprintf(":%d\r\n", n))
}

func NullBulkString() []byte {
	return []byte("$-1\r\n")
}

func BulkString(data []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("$%d\r\n", len(data)))
	buf.Write(data)
	buf.WriteString("\r\n")
	return buf.Bytes()
}

func Array(elements [][]byte) []byte {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("*%d\r\n", len(elements)))
	for _, el := range elements {
		buf.Write(el)
	}
	return buf.Bytes()
}
