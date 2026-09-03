package commands

import (
	"strconv"
	"strings"
	"time"

	"github.com/gosuda/gopherdis/object"
)


func init() {
	DefaultTable.Register(&Command{
		Name:    "get",
		Handler: getCommand,
		Arity:   2,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "set",
		Handler: setCommand,
		Arity:   -3,
		Flags:   FlagWrite,
	})
	DefaultTable.Register(&Command{
		Name:    "mget",
		Handler: mgetCommand,
		Arity:   -2,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "mset",
		Handler: msetCommand,
		Arity:   -3,
		Flags:   FlagWrite,
	})
	DefaultTable.Register(&Command{
		Name:    "incr",
		Handler: incrCommand,
		Arity:   2,
		Flags:   FlagFast | FlagWrite,
	})
	DefaultTable.Register(&Command{
		Name:    "decr",
		Handler: decrCommand,
		Arity:   2,
		Flags:   FlagFast | FlagWrite,
	})
	DefaultTable.Register(&Command{
		Name:    "incrby",
		Handler: incrbyCommand,
		Arity:   3,
		Flags:   FlagFast | FlagWrite,
	})
	DefaultTable.Register(&Command{
		Name:    "decrby",
		Handler: decrbyCommand,
		Arity:   3,
		Flags:   FlagFast | FlagWrite,
	})
	DefaultTable.Register(&Command{
		Name:    "strlen",
		Handler: strlenCommand,
		Arity:   2,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "append",
		Handler: appendCommand,
		Arity:   3,
		Flags:   FlagWrite,
	})
}

func getCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return NullBulkString()
	}
	if obj.Type != object.OBJ_STRING {
		return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	return BulkString(obj.Bytes())
}

func setCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	val := object.CreateRawStringObject(argv[2])

	var ttl time.Duration
	var hasTTL bool

	for i := 3; i < len(argv); i++ {
		opt := strings.ToUpper(string(argv[i]))
		switch opt {
		case "EX":
			if i+1 >= len(argv) {
				return Error("syntax error")
			}
			sec, err := strconv.ParseInt(string(argv[i+1]), 10, 64)
			if err != nil || sec <= 0 {
				return Error("invalid expire time in 'set' command")
			}
			ttl = time.Duration(sec) * time.Second
			hasTTL = true
			i++
		case "PX":
			if i+1 >= len(argv) {
				return Error("syntax error")
			}
			ms, err := strconv.ParseInt(string(argv[i+1]), 10, 64)
			if err != nil || ms <= 0 {
				return Error("invalid expire time in 'set' command")
			}
			ttl = time.Duration(ms) * time.Millisecond
			hasTTL = true
			i++
		default:
			return Error("syntax error")
		}
	}

	if hasTTL {
		ctx.DB.SetWithExpire(key, val, ttl)
	} else {
		ctx.DB.Set(key, val)
	}
	return OK()
}

func mgetCommand(ctx *Context, argv [][]byte) []byte {
	elements := make([][]byte, 0, len(argv)-1)
	for i := 1; i < len(argv); i++ {
		key := string(argv[i])
		obj, ok := ctx.DB.Get(key)
		if !ok || obj == nil || obj.Type != object.OBJ_STRING {
			elements = append(elements, NullBulkString())
		} else {
			elements = append(elements, BulkString(obj.Bytes()))
		}
	}
	return Array(elements)
}

func msetCommand(ctx *Context, argv [][]byte) []byte {
	pairs := argv[1:]
	if len(pairs)%2 != 0 {
		return Error("wrong number of arguments for 'mset' command")
	}

	for i := 0; i < len(pairs); i += 2 {
		key := string(pairs[i])
		val := object.CreateRawStringObject(pairs[i+1])
		ctx.DB.Set(key, val)
	}
	return OK()
}

func incrGeneric(ctx *Context, key string, delta int64) []byte {
	obj, ok := ctx.DB.Get(key)
	var current int64
	if ok && obj != nil {
		if obj.Type != object.OBJ_STRING {
			return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
		}
		val, err := obj.Int64()
		if err != nil {
			return Error("value is not an integer or out of range")
		}
		current = val
	}

	newVal := current + delta
	ctx.DB.Set(key, object.CreateStringObjectFromLongLong(newVal))
	return Integer(newVal)
}

func incrCommand(ctx *Context, argv [][]byte) []byte {
	return incrGeneric(ctx, string(argv[1]), 1)
}

func decrCommand(ctx *Context, argv [][]byte) []byte {
	return incrGeneric(ctx, string(argv[1]), -1)
}

func incrbyCommand(ctx *Context, argv [][]byte) []byte {
	delta, err := strconv.ParseInt(string(argv[2]), 10, 64)
	if err != nil {
		return Error("value is not an integer or out of range")
	}
	return incrGeneric(ctx, string(argv[1]), delta)
}

func decrbyCommand(ctx *Context, argv [][]byte) []byte {
	delta, err := strconv.ParseInt(string(argv[2]), 10, 64)
	if err != nil {
		return Error("value is not an integer or out of range")
	}
	return incrGeneric(ctx, string(argv[1]), -delta)
}

func strlenCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return Integer(0)
	}
	if obj.Type != object.OBJ_STRING {
		return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	return Integer(int64(len(obj.Bytes())))
}

func appendCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	appendBytes := argv[2]

	obj, ok := ctx.DB.Get(key)
	var newBytes []byte
	if !ok || obj == nil {
		newBytes = appendBytes
	} else {
		if obj.Type != object.OBJ_STRING {
			return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
		}
		newBytes = append(obj.Bytes(), appendBytes...)
	}

	ctx.DB.Set(key, object.CreateRawStringObject(newBytes))
	return Integer(int64(len(newBytes)))
}
