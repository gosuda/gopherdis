package commands

import (
	"strconv"

	"github.com/gosuda/gopherdis/datastruct/listpack"
	"github.com/gosuda/gopherdis/object"
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

func getOrCreateList(ctx *Context, key string) (*listView, bool, []byte) {
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		// New lists start compact, as Redis does.
		lp := listpack.New()
		newObj := &object.Robj{
			Type:     object.OBJ_LIST,
			Encoding: object.OBJ_ENCODING_LISTPACK,
			Ptr:      lp,
		}
		ctx.DB.Set(key, newObj)
		return &listView{ctx: ctx, key: key, obj: newObj, lp: lp}, true, nil
	}
	if obj.Type != object.OBJ_LIST {
		return nil, false, Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	l, errReply := newListView(ctx, key, obj)
	if errReply != nil {
		return nil, false, errReply
	}
	return l, false, nil
}

func lpushCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])

	ctx.DB.LockKey(key)
	defer ctx.DB.UnlockKey(key)

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

	ctx.DB.LockKey(key)
	defer ctx.DB.UnlockKey(key)

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

	ctx.DB.LockKey(key)
	defer ctx.DB.UnlockKey(key)

	ql, errReply := getListView(ctx, key)
	if errReply != nil {
		return errReply
	}
	if ql == nil {
		return NullBulkString()
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

	ctx.DB.LockKey(key)
	defer ctx.DB.UnlockKey(key)

	ql, errReply := getListView(ctx, key)
	if errReply != nil {
		return errReply
	}
	if ql == nil {
		return NullBulkString()
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

	ql, errReply := getListView(ctx, key)
	if errReply != nil {
		return errReply
	}
	if ql == nil {
		return Array(nil)
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
	ql, errReply := getListView(ctx, key)
	if errReply != nil {
		return errReply
	}
	if ql == nil {
		return Integer(0)
	}
	return Integer(int64(ql.Len()))
}

func lindexCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	idx, err := strconv.Atoi(string(argv[2]))
	if err != nil {
		return Error("value is not an integer or out of range")
	}

	ql, errReply := getListView(ctx, key)
	if errReply != nil {
		return errReply
	}
	if ql == nil {
		return NullBulkString()
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

	ql, errReply := getListView(ctx, key)
	if errReply != nil {
		return errReply
	}
	if ql == nil {
		return Error("no such key")
	}

	if !ql.LSet(idx, argv[3]) {
		return Error("index out of range")
	}
	return OK()
}

// getListView resolves a list without creating it.
func getListView(ctx *Context, key string) (*listView, []byte) {
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return nil, nil
	}
	if obj.Type != object.OBJ_LIST {
		return nil, Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	return newListView(ctx, key, obj)
}
