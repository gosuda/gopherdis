package commands

import (
	"fmt"
	"strconv"
	"time"
)

func init() {
	DefaultTable.Register(&Command{
		Name:    "ping",
		Handler: pingCommand,
		Arity:   -1,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "echo",
		Handler: echoCommand,
		Arity:   2,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "exists",
		Handler: existsCommand,
		Arity:   -2,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "del",
		Handler: delCommand,
		Arity:   -2,
		Flags:   FlagWrite,
	})
	DefaultTable.Register(&Command{
		Name:    "ttl",
		Handler: ttlCommand,
		Arity:   2,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "type",
		Handler: typeCommand,
		Arity:   2,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "expire",
		Handler: expireCommand,
		Arity:   3,
		Flags:   FlagWrite | FlagFast,
	})
	DefaultTable.Register(&Command{
		Name:    "pexpire",
		Handler: pexpireCommand,
		Arity:   3,
		Flags:   FlagWrite | FlagFast,
	})
	DefaultTable.Register(&Command{
		Name:    "expireat",
		Handler: expireatCommand,
		Arity:   3,
		Flags:   FlagWrite | FlagFast,
	})
	DefaultTable.Register(&Command{
		Name:    "pexpireat",
		Handler: pexpireatCommand,
		Arity:   3,
		Flags:   FlagWrite | FlagFast,
	})
	DefaultTable.Register(&Command{
		Name:    "save",
		Handler: saveCommand,
		Arity:   1,
		Flags:   FlagAdmin,
	})
	DefaultTable.Register(&Command{
		Name:    "bgsave",
		Handler: bgsaveCommand,
		Arity:   1,
		Flags:   FlagAdmin,
	})
	DefaultTable.Register(&Command{
		Name:    "info",
		Handler: infoCommand,
		Arity:   -1,
		Flags:   FlagReadOnly | FlagAdmin,
	})
}

func saveCommand(ctx *Context, argv [][]byte) []byte {
	if ctx.RDB == nil {
		return Error("RDB persistence is disabled")
	}
	if err := ctx.RDB.Save(ctx.DB); err != nil {
		return Error(fmt.Sprintf("save error: %v", err))
	}
	return OK()
}

func bgsaveCommand(ctx *Context, argv [][]byte) []byte {
	if ctx.RDB == nil {
		return Error("RDB persistence is disabled")
	}
	if err := ctx.RDB.BGSave(ctx.DB, nil); err != nil {
		return Error(fmt.Sprintf("bgsave error: %v", err))
	}
	return SimpleString("Background saving started")
}

func expireCommand(ctx *Context, argv [][]byte) []byte {
	secs, err := strconv.ParseInt(string(argv[2]), 10, 64)
	if err != nil {
		return Error("value is not an integer or out of range")
	}
	if ctx.DB.SetExpire(string(argv[1]), time.Duration(secs)*time.Second) {
		return Integer(1)
	}
	return Integer(0)
}

func pexpireCommand(ctx *Context, argv [][]byte) []byte {
	ms, err := strconv.ParseInt(string(argv[2]), 10, 64)
	if err != nil {
		return Error("value is not an integer or out of range")
	}
	if ctx.DB.SetExpire(string(argv[1]), time.Duration(ms)*time.Millisecond) {
		return Integer(1)
	}
	return Integer(0)
}

func expireatCommand(ctx *Context, argv [][]byte) []byte {
	unixSecs, err := strconv.ParseInt(string(argv[2]), 10, 64)
	if err != nil {
		return Error("value is not an integer or out of range")
	}
	if ctx.DB.SetExpireAt(string(argv[1]), unixSecs*1000) {
		return Integer(1)
	}
	return Integer(0)
}

func pexpireatCommand(ctx *Context, argv [][]byte) []byte {
	unixMs, err := strconv.ParseInt(string(argv[2]), 10, 64)
	if err != nil {
		return Error("value is not an integer or out of range")
	}
	if ctx.DB.SetExpireAt(string(argv[1]), unixMs) {
		return Integer(1)
	}
	return Integer(0)
}

func pingCommand(ctx *Context, argv [][]byte) []byte {
	if len(argv) == 1 {
		return PONG()
	}
	return BulkString(argv[1])
}

func echoCommand(ctx *Context, argv [][]byte) []byte {
	return BulkString(argv[1])
}

func existsCommand(ctx *Context, argv [][]byte) []byte {
	count := int64(0)
	for i := 1; i < len(argv); i++ {
		if ctx.DB.Exists(string(argv[i])) {
			count++
		}
	}
	return Integer(count)
}

func delCommand(ctx *Context, argv [][]byte) []byte {
	count := int64(0)
	for i := 1; i < len(argv); i++ {
		if ctx.DB.Del(string(argv[i])) {
			count++
		}
	}
	return Integer(count)
}

func ttlCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	dur, code := ctx.DB.TTL(key)
	if code == -2 {
		return Integer(-2)
	}
	if code == -1 {
		return Integer(-1)
	}
	return Integer(int64(dur.Seconds()))
}

func typeCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return SimpleString("none")
	}
	return SimpleString(obj.TypeName())
}
