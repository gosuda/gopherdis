package commands

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gosuda/gopherdis/datastruct/bitmap"
	"github.com/gosuda/gopherdis/object"
)

func init() {
	DefaultTable.Register(&Command{
		Name:    "setbit",
		Handler: setbitCommand,
		Arity:   4,
		Flags:   FlagWrite | FlagFast,
	})
	DefaultTable.Register(&Command{
		Name:    "getbit",
		Handler: getbitCommand,
		Arity:   3,
		Flags:   FlagReadOnly | FlagFast,
	})
	DefaultTable.Register(&Command{
		Name:    "bitcount",
		Handler: bitcountCommand,
		Arity:   -2,
		Flags:   FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "bitpos",
		Handler: bitposCommand,
		Arity:   -3,
		Flags:   FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "bitop",
		Handler: bitopCommand,
		Arity:   -4,
		Flags:   FlagWrite,
	})
}

func setbitCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	offset, err := strconv.ParseInt(string(argv[2]), 10, 64)
	if err != nil || offset < 0 || offset > (1<<32)-1 {
		return Error("bit offset is not an integer or out of range")
	}

	val, err := strconv.Atoi(string(argv[3]))
	if err != nil || (val != 0 && val != 1) {
		return Error("bit is not the integer 0 or 1")
	}

	byteIdx := int(offset / 8)
	bitIdx := 7 - (offset % 8)

	obj, exists := ctx.DB.Get(key)
	if exists && obj != nil {
		if obj.Type != object.OBJ_STRING {
			return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
		}
		if b, ok := obj.Ptr.([]byte); ok && byteIdx < len(b) {
			oldBit := int((b[byteIdx] >> bitIdx) & 1)
			if val == 1 {
				b[byteIdx] |= (1 << bitIdx)
			} else {
				b[byteIdx] &= ^(1 << bitIdx)
			}
			return Integer(int64(oldBit))
		}
	}

	var rawBytes []byte
	if exists && obj != nil {
		if b, ok := obj.Ptr.([]byte); ok {
			rawBytes = b
		} else {
			rawBytes = obj.Bytes()
		}
	}

	newBytes, oldBit := bitmap.SetBit(rawBytes, offset, val)
	if exists && obj != nil {
		obj.Ptr = newBytes
	} else {
		_ = ctx.DB.Set(key, object.CreateRawStringObject(newBytes))
	}

	return Integer(int64(oldBit))
}

func getbitCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	offset, err := strconv.ParseInt(string(argv[2]), 10, 64)
	if err != nil || offset < 0 {
		return Error("bit offset is not an integer or out of range")
	}

	obj, exists := ctx.DB.Get(key)
	if !exists || obj == nil {
		return Integer(0)
	}
	if obj.Type != object.OBJ_STRING {
		return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}

	byteIdx := int(offset / 8)
	if b, ok := obj.Ptr.([]byte); ok {
		if byteIdx >= len(b) {
			return Integer(0)
		}
		bitIdx := 7 - (offset % 8)
		return Integer(int64((b[byteIdx] >> bitIdx) & 1))
	}

	bit := bitmap.GetBit(obj.Bytes(), offset)
	return Integer(int64(bit))
}

func bitcountCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	obj, exists := ctx.DB.Get(key)
	if !exists {
		return Integer(0)
	}
	if obj.Type != object.OBJ_STRING {
		return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}

	var raw []byte
	if b, ok := obj.Ptr.([]byte); ok {
		raw = b
	} else {
		raw = obj.Bytes()
	}
	start := 0
	end := -1

	if len(argv) >= 3 {
		s, err := strconv.Atoi(string(argv[2]))
		if err != nil {
			return Error("value is not an integer or out of range")
		}
		start = s
	}
	if len(argv) >= 4 {
		e, err := strconv.Atoi(string(argv[3]))
		if err != nil {
			return Error("value is not an integer or out of range")
		}
		end = e
	}

	count := bitmap.BitCount(raw, start, end)
	return Integer(count)
}

func bitposCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	bitVal, err := strconv.Atoi(string(argv[2]))
	if err != nil || (bitVal != 0 && bitVal != 1) {
		return Error("bit is not the integer 0 or 1")
	}

	obj, exists := ctx.DB.Get(key)
	var raw []byte
	if exists {
		if obj.Type != object.OBJ_STRING {
			return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
		}
		raw = obj.Bytes()
	}

	start := 0
	end := -1
	hasEnd := false

	if len(argv) >= 4 {
		s, err := strconv.Atoi(string(argv[3]))
		if err != nil {
			return Error("value is not an integer or out of range")
		}
		start = s
	}
	if len(argv) >= 5 {
		e, err := strconv.Atoi(string(argv[4]))
		if err != nil {
			return Error("value is not an integer or out of range")
		}
		end = e
		hasEnd = true
	}

	pos := bitmap.BitPos(raw, bitVal, start, end, hasEnd)
	return Integer(pos)
}

func bitopCommand(ctx *Context, argv [][]byte) []byte {
	op := strings.ToUpper(string(argv[1]))
	destKey := string(argv[2])

	if op != "AND" && op != "OR" && op != "XOR" && op != "NOT" {
		return Error(fmt.Sprintf("unknown bitop operation '%s'", op))
	}

	if op == "NOT" && len(argv) != 4 {
		return Error("BITOP NOT takes exactly one source key")
	}

	srcs := make([][]byte, 0, len(argv)-3)
	for i := 3; i < len(argv); i++ {
		srcKey := string(argv[i])
		obj, exists := ctx.DB.Get(srcKey)
		if exists {
			if obj.Type != object.OBJ_STRING {
				return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
			}
			srcs = append(srcs, obj.Bytes())
		} else {
			srcs = append(srcs, []byte{})
		}
	}

	dst := bitmap.BitOp(op, srcs)
	if len(dst) > 0 {
		_ = ctx.DB.Set(destKey, object.CreateRawStringObject(dst))
	} else {
		_ = ctx.DB.Del(destKey)
	}

	return Integer(int64(len(dst)))
}
