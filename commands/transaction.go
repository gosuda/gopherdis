package commands

import (
	"sort"
	"strings"

	"github.com/gosuda/nedis/db"
)

// TxState tracks client-level transaction state for MULTI, EXEC, WATCH, DISCARD.
type TxState struct {
	InMulti    bool
	QueuedCmds [][][]byte
	Watched    map[string]uint64 // Key -> Version counter at WATCH time
	DirtyCAS   bool
}

// NewTxState initializes a new transaction state for a client connection.
func NewTxState() *TxState {
	return &TxState{
		Watched: make(map[string]uint64),
	}
}

// Reset clears transaction and watch states.
func (tx *TxState) Reset() {
	tx.InMulti = false
	tx.QueuedCmds = nil
	tx.Watched = make(map[string]uint64)
	tx.DirtyCAS = false
}

func init() {
	DefaultTable.Register(&Command{
		Name:    "multi",
		Handler: multiCommand,
		Arity:   1,
		Flags:   FlagFast | FlagAdmin,
	})
	DefaultTable.Register(&Command{
		Name:    "discard",
		Handler: discardCommand,
		Arity:   1,
		Flags:   FlagFast | FlagAdmin,
	})
	DefaultTable.Register(&Command{
		Name:    "watch",
		Handler: watchCommand,
		Arity:   -2,
		Flags:   FlagFast | FlagAdmin,
	})
	DefaultTable.Register(&Command{
		Name:    "unwatch",
		Handler: unwatchCommand,
		Arity:   1,
		Flags:   FlagFast | FlagAdmin,
	})
	DefaultTable.Register(&Command{
		Name:    "exec",
		Handler: execCommand,
		Arity:   1,
		Flags:   FlagAdmin,
	})
}

func multiCommand(ctx *Context, argv [][]byte) []byte {
	if ctx.Tx == nil {
		ctx.Tx = NewTxState()
	}
	if ctx.Tx.InMulti {
		return Error("MULTI calls can not be nested")
	}
	ctx.Tx.InMulti = true
	ctx.Tx.QueuedCmds = nil
	return OK()
}

func discardCommand(ctx *Context, argv [][]byte) []byte {
	if ctx.Tx == nil || !ctx.Tx.InMulti {
		return Error("DISCARD without MULTI")
	}
	ctx.Tx.Reset()
	return OK()
}

func watchCommand(ctx *Context, argv [][]byte) []byte {
	if ctx.Tx == nil {
		ctx.Tx = NewTxState()
	}
	if ctx.Tx.InMulti {
		return Error("WATCH inside MULTI is not allowed")
	}
	for i := 1; i < len(argv); i++ {
		key := string(argv[i])
		ctx.Tx.Watched[key] = ctx.DB.GetVersion(key)
	}
	return OK()
}

func unwatchCommand(ctx *Context, argv [][]byte) []byte {
	if ctx.Tx != nil {
		ctx.Tx.Watched = make(map[string]uint64)
		ctx.Tx.DirtyCAS = false
	}
	return OK()
}

func execCommand(ctx *Context, argv [][]byte) []byte {
	if ctx.Tx == nil || !ctx.Tx.InMulti {
		return Error("EXEC without MULTI")
	}

	// 1. Acquire transaction lock
	ctx.DB.BeginTx()
	defer ctx.DB.EndTx()

	// 2. CAS Verification for WATCHed keys
	if ctx.Tx.DirtyCAS {
		ctx.Tx.Reset()
		return NullArray()
	}

	for key, originalVer := range ctx.Tx.Watched {
		if ctx.DB.GetVersion(key) != originalVer {
			ctx.Tx.Reset()
			return NullArray()
		}
	}

	queued := ctx.Tx.QueuedCmds
	if len(queued) == 0 {
		ctx.Tx.Reset()
		return Array(nil)
	}

	// Temporarily exit InMulti mode so queued commands are executed, not re-queued
	ctx.Tx.InMulti = false

	// 3. Sequentially execute queued commands
	replies := make([][]byte, len(queued))
	for i, cmdArgv := range queued {
		replies[i] = DefaultTable.Execute(ctx, cmdArgv)
	}

	ctx.Tx.Reset()
	return Array(replies)
}

func NullArray() []byte {
	return []byte("*-1\r\n")
}

func extractSortedShards(database *db.ShardedDB, cmds [][][]byte) ([]int, bool) {
	shardSet := make(map[int]struct{})

	for _, argv := range cmds {
		if len(argv) == 0 {
			continue
		}
		cmdName := strings.ToLower(string(argv[0]))
		switch cmdName {
		case "flushall", "flushdb", "keys":
			return nil, true
		case "mset":
			// MSET k1 v1 k2 v2
			for i := 1; i < len(argv); i += 2 {
				shardSet[database.GetShardIdx(string(argv[i]))] = struct{}{}
			}
		case "mget":
			// MGET k1 k2 k3
			for i := 1; i < len(argv); i++ {
				shardSet[database.GetShardIdx(string(argv[i]))] = struct{}{}
			}
		default:
			if len(argv) > 1 {
				shardSet[database.GetShardIdx(string(argv[1]))] = struct{}{}
			}
		}
	}

	shards := make([]int, 0, len(shardSet))
	for s := range shardSet {
		shards = append(shards, s)
	}
	sort.Ints(shards)
	return shards, false
}
