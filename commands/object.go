package commands

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

func init() {
	DefaultTable.Register(&Command{
		Name:    "object",
		Handler: objectCommand,
		Arity:   -2,
		Flags:   FlagReadOnly,
	})
}

// objectCommand implements the OBJECT introspection container.
//
// REFCOUNT is always 1: gopherdis does not share objects between keys the way
// Redis shares small integers, so every key owns its value outright.
func objectCommand(ctx *Context, argv [][]byte) []byte {
	sub := strings.ToUpper(string(argv[1]))

	if sub == "HELP" {
		return Array([][]byte{
			BulkString([]byte("OBJECT <subcommand> [<arg> ...]. Subcommands are:")),
			BulkString([]byte("ENCODING <key>")),
			BulkString([]byte("FREQ <key>")),
			BulkString([]byte("IDLETIME <key>")),
			BulkString([]byte("REFCOUNT <key>")),
		})
	}

	if len(argv) != 3 {
		return Error(fmt.Sprintf("Unknown subcommand or wrong number of arguments for '%s'. Try OBJECT HELP.", string(argv[1])))
	}

	key := string(argv[2])
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return Error("no such key")
	}

	switch sub {
	case "REFCOUNT":
		return Integer(1)
	case "ENCODING":
		return BulkString([]byte(obj.EncodingName()))
	case "IDLETIME":
		lru := int64(atomic.LoadUint32(&obj.Lru))
		idle := time.Now().Unix() - lru
		if idle < 0 {
			idle = 0
		}
		return Integer(idle)
	case "FREQ":
		// gopherdis has no LFU eviction policy, so access frequency is never
		// tracked and this is always the correct answer.
		return Error("An LFU maxmemory policy is not selected, access frequency not tracked. Please note that when switching between maxmemory policies at runtime LFU and LRU data will take some time to adjust.")
	default:
		return Error(fmt.Sprintf("Unknown subcommand or wrong number of arguments for '%s'. Try OBJECT HELP.", string(argv[1])))
	}
}
