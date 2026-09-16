package commands

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/gosuda/gopherdis/object"
)

// maxStringLength mirrors Redis' proto-max-bulk-len ceiling for a string value.
const maxStringLength = 512 * 1024 * 1024

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
	DefaultTable.Register(&Command{
		Name:    "incrbyfloat",
		Handler: incrbyfloatCommand,
		Arity:   3,
		Flags:   FlagFast | FlagWrite,
	})
	DefaultTable.Register(&Command{
		Name:    "setrange",
		Handler: setrangeCommand,
		Arity:   4,
		Flags:   FlagWrite,
	})
	DefaultTable.Register(&Command{
		Name:    "getrange",
		Handler: getrangeCommand,
		Arity:   4,
		Flags:   FlagReadOnly,
	})
}

func setrangeCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	offset, err := strconv.Atoi(string(argv[2]))
	if err != nil {
		return Error("value is not an integer or out of range")
	}
	if offset < 0 {
		return Error("offset is out of range")
	}
	value := argv[3]
	if offset+len(value) > maxStringLength {
		return Error("string exceeds maximum allowed size (proto-max-bulk-len)")
	}

	ctx.DB.LockKey(key)
	defer ctx.DB.UnlockKey(key)

	obj, ok := ctx.DB.Get(key)
	var cur []byte
	if ok && obj != nil {
		if obj.Type != object.OBJ_STRING {
			return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
		}
		cur = obj.Bytes()
	} else if len(value) == 0 {
		// Setting an empty range on a missing key must not create it.
		return Integer(0)
	}

	if len(value) == 0 {
		return Integer(int64(len(cur)))
	}

	newLen := len(cur)
	if offset+len(value) > newLen {
		newLen = offset + len(value)
	}
	buf := make([]byte, newLen) // zero-filled, which is the padding Redis uses
	copy(buf, cur)
	copy(buf[offset:], value)

	ctx.DB.Set(key, object.CreateObject(object.OBJ_STRING, buf))
	return Integer(int64(len(buf)))
}

func getrangeCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	start, err1 := strconv.Atoi(string(argv[2]))
	end, err2 := strconv.Atoi(string(argv[3]))
	if err1 != nil || err2 != nil {
		return Error("value is not an integer or out of range")
	}

	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return BulkString(nil)
	}
	if obj.Type != object.OBJ_STRING {
		return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}

	b := obj.Bytes()
	n := len(b)
	if n == 0 {
		return BulkString(nil)
	}
	if start < 0 {
		start += n
	}
	if end < 0 {
		end += n
	}
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 0
	}
	if end >= n {
		end = n - 1
	}
	if start > end || start >= n {
		return BulkString(nil)
	}
	return BulkString(b[start : end+1])
}

// formatFloat renders a float the way Redis does: plain decimal notation with
// no exponent and no trailing zeros.
func formatFloat(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimSuffix(s, ".")
	}
	return s
}

func incrbyfloatCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	delta, err := strconv.ParseFloat(string(argv[2]), 64)
	if err != nil || math.IsNaN(delta) {
		return Error("value is not a valid float")
	}

	ctx.DB.LockKey(key)
	defer ctx.DB.UnlockKey(key)

	var current float64
	obj, ok := ctx.DB.Get(key)
	if ok && obj != nil {
		if obj.Type != object.OBJ_STRING {
			return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
		}
		// Reject the forms ParseFloat accepts but Redis does not.
		raw := strings.TrimSpace(string(obj.Bytes()))
		if raw != string(obj.Bytes()) {
			return Error("value is not a valid float")
		}
		current, err = strconv.ParseFloat(raw, 64)
		if err != nil {
			return Error("value is not a valid float")
		}
	}

	newVal := current + delta
	if math.IsNaN(newVal) || math.IsInf(newVal, 0) {
		return Error("increment would produce NaN or Infinity")
	}

	formatted := formatFloat(newVal)
	ctx.DB.Set(key, object.CreateStringObject(formatted))
	return BulkString([]byte(formatted))
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
	val := object.TryEncodeString(argv[2])

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
		val := object.TryEncodeString(pairs[i+1])
		ctx.DB.Set(key, val)
	}
	return OK()
}

func incrGeneric(ctx *Context, key string, delta int64) []byte {
	// Get -> compute -> Set is a read-modify-write: without the key lock two
	// concurrent INCRs can both read n and both write n+1.
	ctx.DB.LockKey(key)
	defer ctx.DB.UnlockKey(key)

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

	if (delta > 0 && current > math.MaxInt64-delta) || (delta < 0 && current < math.MinInt64-delta) {
		return Error("increment or decrement would overflow")
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
	if delta == math.MinInt64 {
		// -math.MinInt64 wraps back to itself, turning DECRBY into INCRBY.
		return Error("decrement would overflow")
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

	ctx.DB.LockKey(key)
	defer ctx.DB.UnlockKey(key)

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
