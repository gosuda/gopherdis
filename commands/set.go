package commands

import (
	"github.com/gosuda/gopherdis/datastruct/intset"
	"github.com/gosuda/gopherdis/object"
)

func init() {
	DefaultTable.Register(&Command{
		Name:    "sadd",
		Handler: saddCommand,
		Arity:   -3,
		Flags:   FlagFast | FlagWrite,
	})
	DefaultTable.Register(&Command{
		Name:    "srem",
		Handler: sremCommand,
		Arity:   -3,
		Flags:   FlagFast | FlagWrite,
	})
	DefaultTable.Register(&Command{
		Name:    "smembers",
		Handler: smembersCommand,
		Arity:   2,
		Flags:   FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "sismember",
		Handler: sismemberCommand,
		Arity:   3,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "scard",
		Handler: scardCommand,
		Arity:   2,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "spop",
		Handler: spopCommand,
		Arity:   -2,
		Flags:   FlagFast | FlagWrite,
	})
}

func getOrCreateSet(ctx *Context, key string) (*setView, bool, []byte) {
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		// New sets start as an intset, which is what Redis does; the first
		// non-integer member converts it.
		is := intset.New()
		newObj := &object.Robj{
			Type:     object.OBJ_SET,
			Encoding: object.OBJ_ENCODING_INTSET,
			Ptr:      is,
		}
		ctx.DB.Set(key, newObj)
		return &setView{ctx: ctx, key: key, obj: newObj, is: is}, true, nil
	}
	if obj.Type != object.OBJ_SET {
		return nil, false, Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	v, errReply := newSetView(ctx, key, obj)
	if errReply != nil {
		return nil, false, errReply
	}
	return v, false, nil
}

func saddCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])

	ctx.DB.LockKey(key)
	defer ctx.DB.UnlockKey(key)

	s, _, errReply := getOrCreateSet(ctx, key)
	if errReply != nil {
		return errReply
	}

	members := make([]string, 0, len(argv)-2)
	for i := 2; i < len(argv); i++ {
		members = append(members, string(argv[i]))
	}
	added := s.Add(members...)
	return Integer(int64(added))
}

func sremCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])

	ctx.DB.LockKey(key)
	defer ctx.DB.UnlockKey(key)

	s, errReply := getSetView(ctx, key)
	if errReply != nil {
		return errReply
	}
	if s == nil {
		return Integer(0)
	}

	members := make([]string, 0, len(argv)-2)
	for i := 2; i < len(argv); i++ {
		members = append(members, string(argv[i]))
	}
	removed := s.Remove(members...)
	if s.Card() == 0 {
		ctx.DB.Del(key)
	}
	return Integer(int64(removed))
}

func smembersCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	s, errReply := getSetView(ctx, key)
	if errReply != nil {
		return errReply
	}
	if s == nil {
		return Array(nil)
	}

	mems := s.Members()
	elements := make([][]byte, 0, len(mems))
	for _, m := range mems {
		elements = append(elements, BulkString([]byte(m)))
	}
	return Array(elements)
}

func sismemberCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	member := string(argv[2])

	s, errReply := getSetView(ctx, key)
	if errReply != nil {
		return errReply
	}
	if s == nil {
		return Integer(0)
	}

	if s.Contains(member) {
		return Integer(1)
	}
	return Integer(0)
}

func scardCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	s, errReply := getSetView(ctx, key)
	if errReply != nil {
		return errReply
	}
	if s == nil {
		return Integer(0)
	}
	return Integer(int64(s.Card()))
}

func spopCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])

	ctx.DB.LockKey(key)
	defer ctx.DB.UnlockKey(key)

	s, errReply := getSetView(ctx, key)
	if errReply != nil {
		return errReply
	}
	if s == nil {
		return NullBulkString()
	}

	popped := s.Pop(1)
	if len(popped) == 0 {
		return NullBulkString()
	}
	if s.Card() == 0 {
		ctx.DB.Del(key)
	}
	return BulkString([]byte(popped[0]))
}

// getSetView resolves a set without creating it.
func getSetView(ctx *Context, key string) (*setView, []byte) {
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return nil, nil
	}
	if obj.Type != object.OBJ_SET {
		return nil, Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	return newSetView(ctx, key, obj)
}
