package commands

import (
	"fmt"
	"strconv"
	"strings"
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
		Arity:   -3,
		Flags:   FlagWrite | FlagFast,
	})
	DefaultTable.Register(&Command{
		Name:    "pexpire",
		Handler: pexpireCommand,
		Arity:   -3,
		Flags:   FlagWrite | FlagFast,
	})
	DefaultTable.Register(&Command{
		Name:    "expireat",
		Handler: expireatCommand,
		Arity:   -3,
		Flags:   FlagWrite | FlagFast,
	})
	DefaultTable.Register(&Command{
		Name:    "pexpireat",
		Handler: pexpireatCommand,
		Arity:   -3,
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

// maxExpireMillis bounds an absolute expiry so that converting seconds to
// milliseconds cannot overflow int64. Redis rejects the conversion rather than
// wrapping into a timestamp in the past.
const maxExpireMillis = int64(1) << 46

// validExpireSeconds reports whether a relative expiry in seconds can be
// converted to an absolute millisecond deadline without overflowing.
func validExpireSeconds(secs int64) bool {
	if secs > maxExpireMillis/1000 || secs < -maxExpireMillis/1000 {
		return false
	}
	return true
}

// validExpireMillis is validExpireSeconds for a relative expiry already in ms.
func validExpireMillis(ms int64) bool {
	return ms <= maxExpireMillis && ms >= -maxExpireMillis
}

// expireGeneric backs EXPIRE, PEXPIRE, EXPIREAT and PEXPIREAT, including the
// NX, XX, GT and LT conditions Redis 7 added.
//
// A key with no TTL counts as expiring at infinity, so GT never overwrites it
// and LT always does.
func expireGeneric(ctx *Context, argv [][]byte, kind string) []byte {
	n, err := strconv.ParseInt(string(argv[2]), 10, 64)
	if err != nil {
		return Error("value is not an integer or out of range")
	}

	var nx, xx, gt, lt bool
	for i := 3; i < len(argv); i++ {
		switch strings.ToUpper(string(argv[i])) {
		case "NX":
			nx = true
		case "XX":
			xx = true
		case "GT":
			gt = true
		case "LT":
			lt = true
		default:
			return Error("Unsupported option " + string(argv[i]))
		}
	}
	if (gt && lt) || (nx && (xx || gt || lt)) {
		return Error("NX and XX, GT or LT options at the same time are not compatible")
	}

	key := string(argv[1])
	now := time.Now().UnixMilli()

	var absMs int64
	switch kind {
	case "expire":
		if !validExpireSeconds(n) {
			return Error("invalid expire time in 'expire' command")
		}
		absMs = now + n*1000
	case "pexpire":
		if !validExpireMillis(n) {
			return Error("invalid expire time in 'pexpire' command")
		}
		absMs = now + n
	case "expireat":
		absMs = n * 1000
	default:
		absMs = n
	}

	ctx.DB.LockKey(key)
	defer ctx.DB.UnlockKey(key)

	if !ctx.DB.Exists(key) {
		return Integer(0)
	}

	// Current deadline, or 0 when the key has no TTL.
	var current int64
	if d, code := ctx.DB.TTL(key); code == 0 {
		current = now + d.Milliseconds()
	}
	hasTTL := current != 0

	switch {
	case nx && hasTTL:
		return Integer(0)
	case xx && !hasTTL:
		return Integer(0)
	case gt && (!hasTTL || absMs <= current):
		return Integer(0)
	case lt && hasTTL && absMs >= current:
		return Integer(0)
	}

	if ctx.DB.SetExpireAt(key, absMs) {
		return Integer(1)
	}
	return Integer(0)
}

func expireCommand(ctx *Context, argv [][]byte) []byte {
	return expireGeneric(ctx, argv, "expire")
}

func pexpireCommand(ctx *Context, argv [][]byte) []byte {
	return expireGeneric(ctx, argv, "pexpire")
}

func expireatCommand(ctx *Context, argv [][]byte) []byte {
	return expireGeneric(ctx, argv, "expireat")
}

func pexpireatCommand(ctx *Context, argv [][]byte) []byte {
	return expireGeneric(ctx, argv, "pexpireat")
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
