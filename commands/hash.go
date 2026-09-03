package commands

import (
	"bytes"
	"strconv"

	"github.com/gosuda/gopherdis/datastruct/dict"
	"github.com/gosuda/gopherdis/object"
)

func init() {
	DefaultTable.Register(&Command{
		Name:    "hset",
		Handler: hsetCommand,
		Arity:   -4,
		Flags:   FlagFast | FlagWrite,
	})
	DefaultTable.Register(&Command{
		Name:    "hget",
		Handler: hgetCommand,
		Arity:   3,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "hdel",
		Handler: hdelCommand,
		Arity:   -3,
		Flags:   FlagFast | FlagWrite,
	})
	DefaultTable.Register(&Command{
		Name:    "hexists",
		Handler: hexistsCommand,
		Arity:   3,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "hgetall",
		Handler: hgetallCommand,
		Arity:   2,
		Flags:   FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "hlen",
		Handler: hlenCommand,
		Arity:   2,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "hkeys",
		Handler: hkeysCommand,
		Arity:   2,
		Flags:   FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "hvals",
		Handler: hvalsCommand,
		Arity:   2,
		Flags:   FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "hmget",
		Handler: hmgetCommand,
		Arity:   -3,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "hmset",
		Handler: hsetCommand,
		Arity:   -4,
		Flags:   FlagFast | FlagWrite,
	})
	DefaultTable.Register(&Command{
		Name:    "hincrby",
		Handler: hincrbyCommand,
		Arity:   4,
		Flags:   FlagFast | FlagWrite,
	})
}

func getOrCreateHash(ctx *Context, key string) (*dict.Dict, bool, []byte) {
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		d := dict.New()
		ctx.DB.Set(key, &object.Robj{
			Type:     object.OBJ_HASH,
			Encoding: object.OBJ_ENCODING_LISTPACK,
			Ptr:      d,
		})
		return d, true, nil
	}
	if obj.Type != object.OBJ_HASH {
		return nil, false, Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	if d, ok := obj.Ptr.(*dict.Dict); ok {
		return d, false, nil
	}
	if h, ok := obj.Ptr.(map[string][]byte); ok {
		d := dict.New()
		for k, v := range h {
			d.Set(k, v)
		}
		obj.Ptr = d
		return d, false, nil
	}
	return nil, false, Error("ERR internal hash type error")
}

func getHash(ctx *Context, key string) (*dict.Dict, []byte) {
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return nil, nil
	}
	if obj.Type != object.OBJ_HASH {
		return nil, Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	if d, ok := obj.Ptr.(*dict.Dict); ok {
		return d, nil
	}
	if h, ok := obj.Ptr.(map[string][]byte); ok {
		d := dict.New()
		for k, v := range h {
			d.Set(k, v)
		}
		obj.Ptr = d
		return d, nil
	}
	return nil, Error("ERR internal hash type error")
}

func hsetCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	pairs := argv[2:]
	if len(pairs)%2 != 0 {
		return Error("wrong number of arguments for 'hset' command")
	}

	d, _, errReply := getOrCreateHash(ctx, key)
	if errReply != nil {
		return errReply
	}

	createdCount := int64(0)
	for i := 0; i < len(pairs); i += 2 {
		f := string(pairs[i])
		v := bytes.Clone(pairs[i+1])
		if d.Set(f, v) {
			createdCount++
		}
	}
	return Integer(createdCount)
}

func hgetCommand(ctx *Context, argv [][]byte) []byte {
	d, errReply := getHash(ctx, string(argv[1]))
	if errReply != nil {
		return errReply
	}
	if d == nil {
		return NullBulkString()
	}

	val, exists := d.Get(string(argv[2]))
	if !exists {
		return NullBulkString()
	}
	return BulkString(val)
}

func hdelCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	d, errReply := getHash(ctx, key)
	if errReply != nil {
		return errReply
	}
	if d == nil {
		return Integer(0)
	}

	deleted := int64(0)
	for i := 2; i < len(argv); i++ {
		if d.Del(string(argv[i])) {
			deleted++
		}
	}
	if d.Len() == 0 {
		ctx.DB.Del(key)
	}
	return Integer(deleted)
}

func hexistsCommand(ctx *Context, argv [][]byte) []byte {
	d, errReply := getHash(ctx, string(argv[1]))
	if errReply != nil {
		return errReply
	}
	if d == nil {
		return Integer(0)
	}

	if _, exists := d.Get(string(argv[2])); exists {
		return Integer(1)
	}
	return Integer(0)
}

func hgetallCommand(ctx *Context, argv [][]byte) []byte {
	d, errReply := getHash(ctx, string(argv[1]))
	if errReply != nil {
		return errReply
	}
	if d == nil || d.Len() == 0 {
		return []byte("*0\r\n")
	}

	var buf bytes.Buffer
	buf.Grow(d.Len() * 64)
	buf.WriteByte('*')
	buf.Write(strconv.AppendInt(nil, int64(d.Len()*2), 10))
	buf.WriteString("\r\n")

	d.ForEach(func(f string, v []byte) {
		buf.WriteByte('$')
		buf.Write(strconv.AppendInt(nil, int64(len(f)), 10))
		buf.WriteString("\r\n")
		buf.WriteString(f)
		buf.WriteString("\r\n")

		buf.WriteByte('$')
		buf.Write(strconv.AppendInt(nil, int64(len(v)), 10))
		buf.WriteString("\r\n")
		buf.Write(v)
		buf.WriteString("\r\n")
	})
	return buf.Bytes()
}

func hlenCommand(ctx *Context, argv [][]byte) []byte {
	d, errReply := getHash(ctx, string(argv[1]))
	if errReply != nil {
		return errReply
	}
	if d == nil {
		return Integer(0)
	}
	return Integer(int64(d.Len()))
}

func hkeysCommand(ctx *Context, argv [][]byte) []byte {
	d, errReply := getHash(ctx, string(argv[1]))
	if errReply != nil {
		return errReply
	}
	if d == nil || d.Len() == 0 {
		return []byte("*0\r\n")
	}

	var buf bytes.Buffer
	buf.Grow(d.Len() * 32)
	buf.WriteByte('*')
	buf.Write(strconv.AppendInt(nil, int64(d.Len()), 10))
	buf.WriteString("\r\n")

	d.ForEach(func(f string, _ []byte) {
		buf.WriteByte('$')
		buf.Write(strconv.AppendInt(nil, int64(len(f)), 10))
		buf.WriteString("\r\n")
		buf.WriteString(f)
		buf.WriteString("\r\n")
	})
	return buf.Bytes()
}

func hvalsCommand(ctx *Context, argv [][]byte) []byte {
	d, errReply := getHash(ctx, string(argv[1]))
	if errReply != nil {
		return errReply
	}
	if d == nil || d.Len() == 0 {
		return []byte("*0\r\n")
	}

	var buf bytes.Buffer
	buf.Grow(d.Len() * 32)
	buf.WriteByte('*')
	buf.Write(strconv.AppendInt(nil, int64(d.Len()), 10))
	buf.WriteString("\r\n")

	d.ForEach(func(_ string, v []byte) {
		buf.WriteByte('$')
		buf.Write(strconv.AppendInt(nil, int64(len(v)), 10))
		buf.WriteString("\r\n")
		buf.Write(v)
		buf.WriteString("\r\n")
	})
	return buf.Bytes()
}

func hmgetCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	numFields := len(argv) - 2

	var buf bytes.Buffer
	buf.Grow(numFields * 32)
	buf.WriteByte('*')
	buf.Write(strconv.AppendInt(nil, int64(numFields), 10))
	buf.WriteString("\r\n")

	d, errReply := getHash(ctx, key)
	if errReply != nil || d == nil {
		for i := 0; i < numFields; i++ {
			buf.WriteString("$-1\r\n")
		}
		return buf.Bytes()
	}

	for i := 2; i < len(argv); i++ {
		if v, exists := d.Get(string(argv[i])); exists {
			buf.WriteByte('$')
			buf.Write(strconv.AppendInt(nil, int64(len(v)), 10))
			buf.WriteString("\r\n")
			buf.Write(v)
			buf.WriteString("\r\n")
		} else {
			buf.WriteString("$-1\r\n")
		}
	}
	return buf.Bytes()
}

func hincrbyCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	field := string(argv[2])
	incr, err := strconv.ParseInt(string(argv[3]), 10, 64)
	if err != nil {
		return Error("value is not an integer or out of range")
	}

	d, _, errReply := getOrCreateHash(ctx, key)
	if errReply != nil {
		return errReply
	}

	var curVal int64 = 0
	if existing, exists := d.Get(field); exists {
		parsed, err := strconv.ParseInt(string(existing), 10, 64)
		if err != nil {
			return Error("hash value is not an integer")
		}
		curVal = parsed
	}

	newVal := curVal + incr
	newValBytes := []byte(strconv.FormatInt(newVal, 10))
	d.Set(field, newValBytes)
	return Integer(newVal)
}
