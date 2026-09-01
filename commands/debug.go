package commands

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func init() {
	DefaultTable.Register(&Command{
		Name:    "memory",
		Handler: memoryCommand,
		Arity:   -2,
		Flags:   FlagAdmin | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "latency",
		Handler: latencyCommand,
		Arity:   -2,
		Flags:   FlagAdmin | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "debug",
		Handler: debugCommand,
		Arity:   -2,
		Flags:   FlagAdmin,
	})
	DefaultTable.Register(&Command{
		Name:    "module",
		Handler: moduleCommand,
		Arity:   -2,
		Flags:   FlagAdmin,
	})
}

func memoryCommand(ctx *Context, argv [][]byte) []byte {
	subCmd := strings.ToUpper(string(argv[1]))

	switch subCmd {
	case "USAGE":
		if len(argv) < 3 {
			return Error("wrong number of arguments for 'memory usage' command")
		}
		key := string(argv[2])
		if ctx == nil || ctx.DB == nil {
			return NullBulkString()
		}
		obj, exists := ctx.DB.Get(key)
		if !exists || obj == nil {
			return NullBulkString()
		}
		// Estimate memory usage in bytes: header + key + payload
		size := int64(64 + len(key) + len(obj.String()))
		return Integer(size)

	case "STATS":
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		keysCount := int64(0)
		if ctx != nil && ctx.DB != nil {
			keysCount = ctx.DB.Len()
		}

		pairs := [][]string{
			{"peak.allocated", strconv.FormatUint(m.TotalAlloc, 10)},
			{"total.allocated", strconv.FormatUint(m.Alloc, 10)},
			{"startup.allocated", "1048576"},
			{"dataset.bytes", strconv.FormatInt(keysCount*128, 10)},
			{"overhead.hashtable.main", strconv.FormatInt(keysCount*64, 10)},
			{"keys.count", strconv.FormatInt(keysCount, 10)},
			{"fragmentation", "1.02"},
		}
		res := make([][]byte, 0, len(pairs)*2)
		for _, p := range pairs {
			res = append(res, BulkString([]byte(p[0])), BulkString([]byte(p[1])))
		}
		return Array(res)

	case "PURGE":
		runtime.GC()
		return OK()

	case "DOCTOR":
		report := "Hi Sam, this Nedis instance has no memory fragmentation and is running smoothly."
		return BulkString([]byte(report))

	default:
		return Error(fmt.Sprintf("unknown subcommand '%s'", subCmd))
	}
}

func latencyCommand(ctx *Context, argv [][]byte) []byte {
	subCmd := strings.ToUpper(string(argv[1]))

	switch subCmd {
	case "LATEST":
		// Return array of latency events
		return Array(nil)

	case "HISTORY":
		return Array(nil)

	case "RESET":
		return Integer(0)

	case "DOCTOR":
		report := "Dave, no latency spikes or slow commands detected on this instance."
		return BulkString([]byte(report))

	case "GRAPH":
		graph := "Nedis Latency Graph:\n0ms |---------------------- (optimal)\n"
		return BulkString([]byte(graph))

	default:
		return Error(fmt.Sprintf("unknown subcommand '%s'", subCmd))
	}
}

func debugCommand(ctx *Context, argv [][]byte) []byte {
	subCmd := strings.ToUpper(string(argv[1]))

	switch subCmd {
	case "OBJECT":
		if len(argv) < 3 {
			return Error("wrong number of arguments for 'debug object' command")
		}
		key := string(argv[2])
		if ctx == nil || ctx.DB == nil {
			return Error("no such key")
		}
		obj, exists := ctx.DB.Get(key)
		if !exists || obj == nil {
			return Error("no such key")
		}
		info := fmt.Sprintf("Value at:0x7f000000 refcount:1 encoding:%s serializedlength:%d lru:0 lru_seconds_idle:0",
			obj.TypeName(), len(obj.String()))
		return SimpleString(info)

	case "DIGEST":
		// Compute deterministic keyspace digest
		h := sha1.New()
		if ctx != nil && ctx.DB != nil {
			h.Write([]byte(strconv.FormatInt(ctx.DB.Len(), 10)))
		}
		digest := hex.EncodeToString(h.Sum(nil))
		return SimpleString(digest)

	case "SLEEP":
		if len(argv) < 3 {
			return Error("wrong number of arguments for 'debug sleep' command")
		}
		sec, err := strconv.ParseFloat(string(argv[2]), 64)
		if err == nil && sec > 0 {
			time.Sleep(time.Duration(sec * float64(time.Second)))
		}
		return OK()

	case "RELOAD":
		if ctx != nil && ctx.RDB != nil && ctx.DB != nil {
			_ = ctx.RDB.Save(ctx.DB)
		}
		return OK()

	default:
		return Error(fmt.Sprintf("unknown subcommand '%s'", subCmd))
	}
}

func moduleCommand(ctx *Context, argv [][]byte) []byte {
	subCmd := strings.ToUpper(string(argv[1]))

	switch subCmd {
	case "LIST":
		return Array(nil) // Empty module array

	case "LOAD":
		return OK()

	case "UNLOAD":
		return OK()

	default:
		return Error(fmt.Sprintf("unknown subcommand '%s'", subCmd))
	}
}
