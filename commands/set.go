package commands

import (
	"github.com/gosuda/nedis/object"
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

func getOrCreateSet(ctx *Context, key string) (map[string]struct{}, bool, []byte) {
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		s := make(map[string]struct{})
		ctx.DB.Set(key, &object.Robj{
			Type:     object.OBJ_SET,
			Encoding: object.OBJ_ENCODING_HT,
			Ptr:      s,
		})
		return s, true, nil
	}
	if obj.Type != object.OBJ_SET {
		return nil, false, Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	s, ok := obj.Ptr.(map[string]struct{})
	if !ok {
		return nil, false, Error("ERR internal set type error")
	}
	return s, false, nil
}

func saddCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	s, _, errReply := getOrCreateSet(ctx, key)
	if errReply != nil {
		return errReply
	}

	added := int64(0)
	for i := 2; i < len(argv); i++ {
		member := string(argv[i])
		if _, exists := s[member]; !exists {
			s[member] = struct{}{}
			added++
		}
	}
	return Integer(added)
}

func sremCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return Integer(0)
	}
	if obj.Type != object.OBJ_SET {
		return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	s, ok := obj.Ptr.(map[string]struct{})
	if !ok {
		return Error("ERR internal set type error")
	}

	removed := int64(0)
	for i := 2; i < len(argv); i++ {
		member := string(argv[i])
		if _, exists := s[member]; exists {
			delete(s, member)
			removed++
		}
	}
	if len(s) == 0 {
		ctx.DB.Del(key)
	}
	return Integer(removed)
}

func smembersCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return Array(nil)
	}
	if obj.Type != object.OBJ_SET {
		return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	s, ok := obj.Ptr.(map[string]struct{})
	if !ok {
		return Error("ERR internal set type error")
	}

	elements := make([][]byte, 0, len(s))
	for m := range s {
		elements = append(elements, BulkString([]byte(m)))
	}
	return Array(elements)
}

func sismemberCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	member := string(argv[2])

	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return Integer(0)
	}
	if obj.Type != object.OBJ_SET {
		return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	s, ok := obj.Ptr.(map[string]struct{})
	if !ok {
		return Error("ERR internal set type error")
	}

	if _, exists := s[member]; exists {
		return Integer(1)
	}
	return Integer(0)
}

func scardCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return Integer(0)
	}
	if obj.Type != object.OBJ_SET {
		return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	s, ok := obj.Ptr.(map[string]struct{})
	if !ok {
		return Error("ERR internal set type error")
	}
	return Integer(int64(len(s)))
}

func spopCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return NullBulkString()
	}
	if obj.Type != object.OBJ_SET {
		return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	s, ok := obj.Ptr.(map[string]struct{})
	if !ok {
		return Error("ERR internal set type error")
	}

	if len(s) == 0 {
		return NullBulkString()
	}

	var popped string
	for m := range s {
		popped = m
		break
	}
	delete(s, popped)
	if len(s) == 0 {
		ctx.DB.Del(key)
	}
	return BulkString([]byte(popped))
}
