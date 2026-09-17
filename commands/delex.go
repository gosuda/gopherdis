package commands

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/gosuda/gopherdis/object"
	"github.com/zeebo/xxh3"
)

func init() {
	DefaultTable.Register(&Command{
		Name:    "delex",
		Handler: delexCommand,
		Arity:   -2,
		Flags:   FlagWrite | FlagFast,
	})
}

// delexCommand deletes a key only when its current value satisfies a condition.
//
// With no condition it is DEL for one key. IFEQ and IFNE compare the value
// itself, IFDEQ and IFDNE compare its digest, which lets a caller guard a
// delete without sending the whole value back. The comparison and the delete
// happen under the key lock so nothing can change in between, which is the
// entire point of the command.
func delexCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])

	var mode string
	var operand []byte
	if len(argv) >= 3 {
		mode = strings.ToUpper(string(argv[2]))
		switch mode {
		case "IFEQ", "IFNE", "IFDEQ", "IFDNE":
			if len(argv) != 4 {
				return Error("wrong number of arguments for 'delex' command")
			}
			operand = argv[3]
		default:
			return Error("Invalid condition for 'delex' command")
		}
	} else if len(argv) != 2 {
		return Error("wrong number of arguments for 'delex' command")
	}

	ctx.DB.LockKey(key)
	defer ctx.DB.UnlockKey(key)

	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		ctx.Propagate(nil)
		return Integer(0)
	}

	// Unconditional DELEX works on any type; the comparisons need a string.
	if mode != "" {
		if obj.Type != object.OBJ_STRING {
			return Error("DELEX conditions can only be used with string values")
		}
		value := obj.Bytes()

		var match bool
		switch mode {
		case "IFEQ":
			match = bytes.Equal(value, operand)
		case "IFNE":
			match = !bytes.Equal(value, operand)
		case "IFDEQ", "IFDNE":
			// Checked here rather than during parsing: a missing key short
			// circuits above, so a malformed digest is only an error when it
			// would actually be compared.
			if !isHexDigest(operand) {
				return Error("digest must be exactly 16 hexadecimal characters")
			}
			digest := fmt.Sprintf("%016x", xxh3.Hash(value))
			equal := strings.EqualFold(digest, string(operand))
			match = equal == (mode == "IFDEQ")
		}
		if !match {
			// Nothing happened, so nothing reaches the replica. Propagating
			// DELEX itself would re-evaluate the condition there, where the
			// value may differ.
			ctx.Propagate(nil)
			return Integer(0)
		}
	}

	if ctx.DB.Del(key) {
		// Propagate the deletion rather than the condition: the replica's copy
		// must go regardless of what it would have compared against. DELEX
		// always sends DEL, it does not follow lazyfree-lazy-server-del.
		ctx.Propagate([][]byte{[]byte("DEL"), argv[1]})
		return Integer(1)
	}
	ctx.Propagate(nil)
	return Integer(0)
}
