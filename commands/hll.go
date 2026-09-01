package commands

import (
	"github.com/gosuda/nedis/datastruct/hll"
	"github.com/gosuda/nedis/object"
)

func init() {
	DefaultTable.Register(&Command{
		Name:    "pfadd",
		Handler: pfaddCommand,
		Arity:   -2,
		Flags:   FlagWrite | FlagFast,
	})
	DefaultTable.Register(&Command{
		Name:    "pfcount",
		Handler: pfcountCommand,
		Arity:   -2,
		Flags:   FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "pfmerge",
		Handler: pfmergeCommand,
		Arity:   -2,
		Flags:   FlagWrite,
	})
}

func pfaddCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	var h *hll.HLL

	obj, exists := ctx.DB.Get(key)
	if exists {
		if obj.Type != object.OBJ_STRING {
			return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
		}
		if b, ok := obj.Ptr.([]byte); ok && len(b) >= hll.HLL_DENSE_SIZE {
			h = hll.FromBytes(b)
		} else {
			h = hll.FromBytes(obj.Bytes())
		}
	} else {
		h = hll.NewHLL()
	}

	updated := false
	for i := 2; i < len(argv); i++ {
		if h.Add(argv[i]) {
			updated = true
		}
	}

	if !exists {
		_ = ctx.DB.Set(key, object.CreateRawStringObject(h.Bytes()))
	}

	if updated {
		return Integer(1)
	}
	return Integer(0)
}

func pfcountCommand(ctx *Context, argv [][]byte) []byte {
	if len(argv) == 2 {
		// Single key fast path
		key := string(argv[1])
		obj, exists := ctx.DB.Get(key)
		if !exists {
			return Integer(0)
		}
		if obj.Type != object.OBJ_STRING {
			return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
		}
		var h *hll.HLL
		if b, ok := obj.Ptr.([]byte); ok && len(b) >= hll.HLL_DENSE_SIZE {
			h = hll.FromBytes(b)
		} else {
			h = hll.FromBytes(obj.Bytes())
		}
		return Integer(int64(h.Count()))
	}

	// Multi-key union estimation
	merged := hll.NewHLL()
	for i := 1; i < len(argv); i++ {
		key := string(argv[i])
		obj, exists := ctx.DB.Get(key)
		if exists {
			if obj.Type != object.OBJ_STRING {
				return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
			}
			h := hll.FromBytes(obj.Bytes())
			merged.Merge(h)
		}
	}

	return Integer(int64(merged.Count()))
}

func pfmergeCommand(ctx *Context, argv [][]byte) []byte {
	destKey := string(argv[1])
	merged := hll.NewHLL()

	// Merge dest key if it already exists
	destObj, exists := ctx.DB.Get(destKey)
	if exists {
		if destObj.Type != object.OBJ_STRING {
			return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
		}
		merged.Merge(hll.FromBytes(destObj.Bytes()))
	}

	// Merge all source keys
	for i := 2; i < len(argv); i++ {
		srcKey := string(argv[i])
		obj, exists := ctx.DB.Get(srcKey)
		if exists {
			if obj.Type != object.OBJ_STRING {
				return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
			}
			merged.Merge(hll.FromBytes(obj.Bytes()))
		}
	}

	_ = ctx.DB.Set(destKey, object.CreateRawStringObject(merged.Bytes()))
	return OK()
}
