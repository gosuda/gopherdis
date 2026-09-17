package commands

import (
	"fmt"

	"github.com/gosuda/gopherdis/object"
	"github.com/zeebo/xxh3"
)

func init() {
	DefaultTable.Register(&Command{
		Name:    "digest",
		Handler: digestCommand,
		Arity:   2,
		Flags:   FlagReadOnly | FlagFast,
	})
}

// digestCommand returns the XXH3 64-bit hash of a string value as 16 lowercase
// hex digits.
//
// The algorithm is fixed, not an implementation detail: a client can compute
// XXH3 itself and compare, so any other hash would hand back a plausible digest
// that disagrees with Redis. Integer-encoded values are hashed as their decimal
// text, which is what Robj.Bytes already renders.
func digestCommand(ctx *Context, argv [][]byte) []byte {
	obj, ok := ctx.DB.Get(string(argv[1]))
	if !ok || obj == nil {
		return Null(ctx)
	}
	if obj.Type != object.OBJ_STRING {
		return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	return BulkString([]byte(fmt.Sprintf("%016x", xxh3.Hash(obj.Bytes()))))
}
