package commands

import (
	"strconv"

	"github.com/gosuda/nedis/datastruct/quicklist"
	"github.com/gosuda/nedis/object"
)

func init() {
	DefaultTable.Register(&Command{
		Name:    "lpush",
		Handler: lpushCommand,
		Arity:   -3,
		Flags:   FlagFast | FlagWrite,
	})
	DefaultTable.Register(&Command{
		Name:    "rpush",
		Handler: rpushCommand,
		Arity:   -3,
		Flags:   FlagFast | FlagWrite,
	})
	DefaultTable.Register(&Command{
		Name:    "lpop",
		Handler: lpopCommand,
		Arity:   2,
		Flags:   FlagFast | FlagWrite,
	})
	DefaultTable.Register(&Command{
		Name:    "rpop",
		Handler: rpopCommand,
		Arity:   2,
		Flags:   FlagFast | FlagWrite,
	})
	DefaultTable.Register(&Command{
		Name:    "lrange",
		Handler: lrangeCommand,
		Arity:   4,
		Flags:   FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "llen",
		Handler: llenCommand,
		Arity:   2,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "lindex",
		Handler: lindexCommand,
		Arity:   3,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "lset",
		Handler: lsetCommand,
		Arity:   4,
		Flags:   FlagWrite,
	})
}

func getOrCreateList(ctx *Context, key string) (*quicklist.Quicklist, bool, []byte) {
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		ql := quicklist.NewQuicklist()
		ctx.DB.Set(key, &object.Robj{
			Type:     object.OBJ_LIST,
			Encoding: object.OBJ_ENCODING_QUICKLIST,
			Ptr:      ql,
		})
		return ql, true, nil
	}
	if obj.Type != object.OBJ_LIST {
		return nil, false, Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	ql, ok := obj.Ptr.(*quicklist.Quicklist)
	if !ok {
		return nil, false, Error("ERR internal list type error")
	}
	return ql, false, nil
}

func lpushCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	ql, _, errReply := getOrCreateList(ctx, key)
	if errReply != nil {
		return errReply
	}

	for i := 2; i < len(argv); i++ {
		ql.LPush(argv[i])
	}
	return Integer(int64(ql.Len()))
}

func rpushCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	ql, _, errReply := getOrCreateList(ctx, key)
	if errReply != nil {
		return errReply
	}

	for i := 2; i < len(argv); i++ {
		ql.RPush(argv[i])
	}
	return Integer(int64(ql.Len()))
}

func lpopCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return NullBulkString()
	}
	if obj.Type != object.OBJ_LIST {
		return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	ql, ok := obj.Ptr.(*quicklist.Quicklist)
	if !ok {
		return Error("ERR internal list type error")
	}

	val, found := ql.LPop()
	if !found {
		return NullBulkString()
	}
	if ql.Len() == 0 {
		ctx.DB.Del(key)
	}
	return BulkString(val)
}

func rpopCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return NullBulkString()
	}
	if obj.Type != object.OBJ_LIST {
		return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	ql, ok := obj.Ptr.(*quicklist.Quicklist)
	if !ok {
		return Error("ERR internal list type error")
	}

	val, found := ql.RPop()
	if !found {
		return NullBulkString()
	}
	if ql.Len() == 0 {
		ctx.DB.Del(key)
	}
	return BulkString(val)
}

func lrangeCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	start, err1 := strconv.Atoi(string(argv[2]))
	stop, err2 := strconv.Atoi(string(argv[3]))
	if err1 != nil || err2 != nil {
		return Error("value is not an integer or out of range")
	}

	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return Array(nil)
	}
	if obj.Type != object.OBJ_LIST {
		return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	ql, ok := obj.Ptr.(*quicklist.Quicklist)
	if !ok {
		return Error("ERR internal list type error")
	}

	items := ql.LRange(start, stop)
	elements := make([][]byte, len(items))
	for i, item := range items {
		elements[i] = BulkString(item)
	}
	return Array(elements)
}

func llenCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return Integer(0)
	}
	if obj.Type != object.OBJ_LIST {
		return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	ql, ok := obj.Ptr.(*quicklist.Quicklist)
	if !ok {
		return Error("ERR internal list type error")
	}
	return Integer(int64(ql.Len()))
}

func lindexCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	idx, err := strconv.Atoi(string(argv[2]))
	if err != nil {
		return Error("value is not an integer or out of range")
	}

	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return NullBulkString()
	}
	if obj.Type != object.OBJ_LIST {
		return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	ql, ok := obj.Ptr.(*quicklist.Quicklist)
	if !ok {
		return Error("ERR internal list type error")
	}

	val, found := ql.LIndex(idx)
	if !found {
		return NullBulkString()
	}
	return BulkString(val)
}

func lsetCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	idx, err := strconv.Atoi(string(argv[2]))
	if err != nil {
		return Error("value is not an integer or out of range")
	}

	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return Error("no such key")
	}
	if obj.Type != object.OBJ_LIST {
		return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	ql, ok := obj.Ptr.(*quicklist.Quicklist)
	if !ok {
		return Error("ERR internal list type error")
	}

	if !ql.LSet(idx, argv[3]) {
		return Error("index out of range")
	}
	return OK()
}
