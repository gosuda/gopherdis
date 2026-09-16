package commands

import (
	"strconv"
	"strings"
	"time"

	"github.com/gosuda/gopherdis/object"
)

func init() {
	reg := func(name string, h CommandHandler, arity int, flags CommandFlags) {
		DefaultTable.Register(&Command{Name: name, Handler: h, Arity: arity, Flags: flags})
	}
	reg("keys", keysCommand, 2, FlagReadOnly)
	reg("scan", scanCommand, -2, FlagReadOnly)
	reg("randomkey", randomkeyCommand, 1, FlagReadOnly|FlagFast)
	reg("touch", touchCommand, -2, FlagReadOnly|FlagFast)
	reg("unlink", unlinkCommand, -2, FlagWrite|FlagFast)
	reg("rename", renameCommand, 3, FlagWrite)
	reg("renamenx", renamenxCommand, 3, FlagWrite|FlagFast)
	reg("persist", persistCommand, 2, FlagWrite|FlagFast)
	reg("pttl", pttlCommand, 2, FlagReadOnly|FlagFast)
	reg("copy", copyCommand, -3, FlagWrite)
	reg("select", selectCommand, 2, FlagFast)
	reg("swapdb", swapdbCommand, 3, FlagWrite|FlagFast)
	reg("quit", quitCommand, -1, FlagFast)
	reg("expiretime", expiretimeCommand, 2, FlagReadOnly|FlagFast)
	reg("pexpiretime", pexpiretimeCommand, 2, FlagReadOnly|FlagFast)
}

func keysCommand(ctx *Context, argv [][]byte) []byte {
	keys := ctx.DB.Keys(string(argv[1]))
	elements := make([][]byte, 0, len(keys))
	for _, k := range keys {
		elements = append(elements, BulkString([]byte(k)))
	}
	return Array(elements)
}

func scanCommand(ctx *Context, argv [][]byte) []byte {
	cursor, err := strconv.ParseUint(string(argv[1]), 10, 64)
	if err != nil {
		return Error("invalid cursor")
	}

	pattern := ""
	count := 10
	for i := 2; i < len(argv); i++ {
		switch strings.ToUpper(string(argv[i])) {
		case "MATCH":
			if i+1 >= len(argv) {
				return Error("syntax error")
			}
			pattern = string(argv[i+1])
			i++
		case "COUNT":
			if i+1 >= len(argv) {
				return Error("syntax error")
			}
			n, err := strconv.Atoi(string(argv[i+1]))
			if err != nil || n < 1 {
				return Error("syntax error")
			}
			count = n
			i++
		case "TYPE":
			if i+1 >= len(argv) {
				return Error("syntax error")
			}
			// Accepted and applied below via the type filter.
			i++
		default:
			return Error("syntax error")
		}
	}

	next, keys := ctx.DB.Scan(cursor, pattern, count)

	// Apply an optional TYPE filter after the fact.
	if typ := scanTypeFilter(argv); typ != "" {
		filtered := keys[:0]
		for _, k := range keys {
			if obj, ok := ctx.DB.Get(k); ok && obj.TypeName() == typ {
				filtered = append(filtered, k)
			}
		}
		keys = filtered
	}

	elements := make([][]byte, 0, len(keys))
	for _, k := range keys {
		elements = append(elements, BulkString([]byte(k)))
	}
	return Array([][]byte{
		BulkString([]byte(strconv.FormatUint(next, 10))),
		Array(elements),
	})
}

func scanTypeFilter(argv [][]byte) string {
	for i := 2; i+1 < len(argv); i++ {
		if strings.ToUpper(string(argv[i])) == "TYPE" {
			return strings.ToLower(string(argv[i+1]))
		}
	}
	return ""
}

func randomkeyCommand(ctx *Context, argv [][]byte) []byte {
	k := ctx.DB.RandomKey()
	if k == "" {
		return NullBulkString()
	}
	return BulkString([]byte(k))
}

// touchCommand reports how many of the given keys exist. Reading them already
// refreshes their LRU clock inside DB.Get, which is the point of TOUCH.
func touchCommand(ctx *Context, argv [][]byte) []byte {
	var n int64
	for i := 1; i < len(argv); i++ {
		if _, ok := ctx.DB.Get(string(argv[i])); ok {
			n++
		}
	}
	return Integer(n)
}

// unlinkCommand is DEL. Gopherdis frees values through the Go GC, so there is
// no separate reclamation thread for this to hand work to.
func unlinkCommand(ctx *Context, argv [][]byte) []byte {
	var n int64
	for i := 1; i < len(argv); i++ {
		if ctx.DB.Del(string(argv[i])) {
			n++
		}
	}
	return Integer(n)
}

func renameGeneric(ctx *Context, src, dst string, failIfExists bool) []byte {
	if src == dst {
		if !ctx.DB.Exists(src) {
			return Error("no such key")
		}
		if failIfExists {
			return Integer(0)
		}
		return OK()
	}

	// Lock the lower stripe first so two concurrent renames of the same pair
	// cannot deadlock against each other.
	first, second := src, dst
	if first > second {
		first, second = second, first
	}
	ctx.DB.LockKey(first)
	if second != first {
		ctx.DB.LockKey(second)
	}
	defer func() {
		if second != first {
			ctx.DB.UnlockKey(second)
		}
		ctx.DB.UnlockKey(first)
	}()

	obj, expireAt, ok := ctx.DB.GetWithTTL(src)
	if !ok {
		return Error("no such key")
	}
	if failIfExists && ctx.DB.Exists(dst) {
		return Integer(0)
	}

	if expireAt > 0 {
		_ = ctx.DB.Set(dst, obj)
		ctx.DB.SetExpireAt(dst, expireAt)
	} else {
		_ = ctx.DB.Set(dst, obj)
	}
	ctx.DB.Del(src)

	if failIfExists {
		return Integer(1)
	}
	return OK()
}

func renameCommand(ctx *Context, argv [][]byte) []byte {
	return renameGeneric(ctx, string(argv[1]), string(argv[2]), false)
}

func renamenxCommand(ctx *Context, argv [][]byte) []byte {
	return renameGeneric(ctx, string(argv[1]), string(argv[2]), true)
}

func persistCommand(ctx *Context, argv [][]byte) []byte {
	if ctx.DB.Persist(string(argv[1])) {
		return Integer(1)
	}
	return Integer(0)
}

func pttlCommand(ctx *Context, argv [][]byte) []byte {
	d, code := ctx.DB.TTL(string(argv[1]))
	if code != 0 {
		return Integer(int64(code))
	}
	return Integer(d.Milliseconds())
}

func copyCommand(ctx *Context, argv [][]byte) []byte {
	src := string(argv[1])
	dst := string(argv[2])
	replace := false
	for i := 3; i < len(argv); i++ {
		switch strings.ToUpper(string(argv[i])) {
		case "REPLACE":
			replace = true
		case "DB":
			if i+1 >= len(argv) {
				return Error("syntax error")
			}
			n, err := strconv.Atoi(string(argv[i+1]))
			if err != nil {
				return Error("value is not an integer or out of range")
			}
			if n != 0 {
				return Error("DB index is out of range")
			}
			i++
		default:
			return Error("syntax error")
		}
	}
	if src == dst {
		return Error("source and destination objects are the same")
	}

	first, second := src, dst
	if first > second {
		first, second = second, first
	}
	ctx.DB.LockKey(first)
	if second != first {
		ctx.DB.LockKey(second)
	}
	defer func() {
		if second != first {
			ctx.DB.UnlockKey(second)
		}
		ctx.DB.UnlockKey(first)
	}()

	obj, expireAt, ok := ctx.DB.GetWithTTL(src)
	if !ok {
		return Integer(0)
	}
	if !replace && ctx.DB.Exists(dst) {
		return Integer(0)
	}

	// Deep copy: sharing the payload would make the two keys alias each other.
	dup, errReply := deepCopy(obj)
	if errReply != nil {
		return errReply
	}
	_ = ctx.DB.Set(dst, dup)
	if expireAt > 0 {
		ctx.DB.SetExpireAt(dst, expireAt)
	}
	return Integer(1)
}

// selectCommand accepts only index 0: gopherdis exposes a single database.
func selectCommand(ctx *Context, argv [][]byte) []byte {
	n, err := strconv.Atoi(string(argv[1]))
	if err != nil {
		return Error("value is not an integer or out of range")
	}
	if n != 0 {
		return Error("DB index is out of range")
	}
	return OK()
}

func swapdbCommand(ctx *Context, argv [][]byte) []byte {
	a, err1 := strconv.Atoi(string(argv[1]))
	b, err2 := strconv.Atoi(string(argv[2]))
	if err1 != nil || err2 != nil {
		return Error("invalid first DB index")
	}
	if a != 0 || b != 0 {
		return Error("DB index is out of range")
	}
	return OK()
}

// quitCommand replies +OK; the connection loop closes the socket when it sees
// this command name.
func quitCommand(ctx *Context, argv [][]byte) []byte {
	return OK()
}

// deepCopy duplicates an object so COPY does not alias the source payload.
func deepCopy(obj *object.Robj) (*object.Robj, []byte) {
	if obj == nil {
		return nil, Error("no such key")
	}
	switch obj.Type {
	case object.OBJ_STRING:
		return object.TryEncodeString(obj.Bytes()), nil
	default:
		dup, ok := cloneContainer(obj)
		if !ok {
			return nil, Error("COPY is not supported for this value type")
		}
		return dup, nil
	}
}

// expiretimeGeneric backs EXPIRETIME and PEXPIRETIME: the absolute expiry of a
// key, -1 when it has none and -2 when the key does not exist.
func expiretimeGeneric(ctx *Context, key string, ms bool) []byte {
	d, code := ctx.DB.TTL(key)
	if code != 0 {
		return Integer(int64(code))
	}
	at := time.Now().Add(d).UnixMilli()
	if ms {
		return Integer(at)
	}
	return Integer(at / 1000)
}

func expiretimeCommand(ctx *Context, argv [][]byte) []byte {
	return expiretimeGeneric(ctx, string(argv[1]), false)
}

func pexpiretimeCommand(ctx *Context, argv [][]byte) []byte {
	return expiretimeGeneric(ctx, string(argv[1]), true)
}
