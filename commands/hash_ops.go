package commands

import (
	"bytes"
	"math/rand"
	"strconv"
	"strings"
)

func init() {
	reg := func(name string, h CommandHandler, arity int, flags CommandFlags) {
		DefaultTable.Register(&Command{Name: name, Handler: h, Arity: arity, Flags: flags})
	}
	reg("hsetnx", hsetnxCommand, 4, FlagWrite|FlagFast)
	reg("hstrlen", hstrlenCommand, 3, FlagReadOnly|FlagFast)
	reg("hrandfield", hrandfieldCommand, -2, FlagReadOnly)
}

func hsetnxCommand(ctx *Context, argv [][]byte) []byte {
	key, field := string(argv[1]), string(argv[2])

	ctx.DB.LockKey(key)
	defer ctx.DB.UnlockKey(key)

	d, _, errReply := getOrCreateHash(ctx, key)
	if errReply != nil {
		return errReply
	}
	if _, exists := d.Get(field); exists {
		return Integer(0)
	}
	d.Set(field, bytes.Clone(argv[3]))
	return Integer(1)
}

func hstrlenCommand(ctx *Context, argv [][]byte) []byte {
	d, errReply := getHash(ctx, string(argv[1]))
	if errReply != nil {
		return errReply
	}
	if d == nil {
		return Integer(0)
	}
	v, ok := d.Get(string(argv[2]))
	if !ok {
		return Integer(0)
	}
	return Integer(int64(len(v)))
}

func hrandfieldCommand(ctx *Context, argv [][]byte) []byte {
	d, errReply := getHash(ctx, string(argv[1]))
	if errReply != nil {
		return errReply
	}

	if len(argv) == 2 {
		if d == nil || d.Len() == 0 {
			return NullBulkString()
		}
		keys := d.Keys()
		return BulkString([]byte(keys[rand.Intn(len(keys))]))
	}

	raw, err := strconv.ParseInt(string(argv[2]), 10, 64)
	if err != nil {
		return Error("value is not an integer or out of range")
	}
	count, withRepeats, errReply := randCount(raw)
	if errReply != nil {
		return errReply
	}
	withValues := len(argv) > 3 && strings.ToUpper(string(argv[3])) == "WITHVALUES"
	if d == nil || d.Len() == 0 {
		return Array(nil)
	}
	keys := d.Keys()

	emit := func(fields []string) []byte {
		if !withValues {
			elems := make([][]byte, 0, len(fields))
			for _, f := range fields {
				elems = append(elems, BulkString([]byte(f)))
			}
			return Array(elems)
		}
		// WITHVALUES nests each field and value in RESP3, where RESP2 flattens.
		pairs := make([][2][]byte, 0, len(fields))
		for _, f := range fields {
			v, _ := d.Get(f)
			pairs = append(pairs, [2][]byte{BulkString([]byte(f)), BulkString(v)})
		}
		return PairsReply(ctx, pairs)
	}

	// A negative count may repeat fields and returns exactly |count|.
	if withRepeats {
		out := make([]string, 0, count)
		for i := 0; i < count; i++ {
			out = append(out, keys[rand.Intn(len(keys))])
		}
		return emit(out)
	}
	if count >= len(keys) {
		return emit(keys)
	}
	rand.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
	return emit(keys[:count])
}
