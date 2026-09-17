package commands

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/gosuda/gopherdis/object"
)

func init() {
	DefaultTable.Register(&Command{
		Name:    "msetex",
		Handler: msetexCommand,
		Arity:   -4,
		Flags:   FlagWrite,
	})
}

// msetexCommand sets several keys with one shared expiry.
//
// Layout is "MSETEX numkeys key value [key value ...] [NX|XX]
// [EX s|PX ms|EXAT s|PXAT ms|KEEPTTL]". The condition applies to the batch as a
// whole: with NX, a single existing key makes the whole command a no-op, which
// is what keeps it usable as a multi-key guard.
func msetexCommand(ctx *Context, argv [][]byte) []byte {
	numKeys64, err := strconv.ParseInt(string(argv[1]), 10, 64)
	// A numkeys that cannot be a key count at all is rejected as such; one that
	// is merely larger than the arguments provided is a pair-count mismatch.
	// The boundary is the 32 bit range, where doubling it would overflow.
	if err != nil || numKeys64 <= 0 || numKeys64 > math.MaxInt32 {
		return Error("invalid numkeys value")
	}
	numKeys := int(numKeys64)
	// numkeys itself is usable, but it has to agree with how many pairs follow.
	if len(argv) < 2+numKeys*2 {
		return Error("wrong number of key-value pairs")
	}

	pairs := argv[2 : 2+numKeys*2]
	rest := argv[2+numKeys*2:]

	var (
		nx, xx, keepTTL bool
		ttl             time.Duration
		hasTTL          bool
		expAt           int64
	)
	for i := 0; i < len(rest); i++ {
		opt := strings.ToUpper(string(rest[i]))
		switch opt {
		case "NX":
			nx = true
		case "XX":
			xx = true
		case "KEEPTTL":
			if hasTTL || expAt > 0 {
				return Error("syntax error")
			}
			keepTTL = true
		case "EX", "PX", "EXAT", "PXAT":
			if hasTTL || expAt > 0 || keepTTL {
				// The expiration options are mutually exclusive.
				return Error("syntax error")
			}
			if i+1 >= len(rest) {
				return Error("syntax error")
			}
			n, err := strconv.ParseInt(string(rest[i+1]), 10, 64)
			if err != nil {
				return Error("value is not an integer or out of range")
			}
			switch opt {
			case "EX":
				if n <= 0 || !validExpireSeconds(n) {
					return Error("invalid expire time in 'msetex' command")
				}
				ttl, hasTTL = time.Duration(n)*time.Second, true
			case "PX":
				if n <= 0 || !validExpireMillis(n) {
					return Error("invalid expire time in 'msetex' command")
				}
				ttl, hasTTL = time.Duration(n)*time.Millisecond, true
			case "EXAT":
				expAt = n * 1000
			case "PXAT":
				expAt = n
			}
			i++
		default:
			return Error("syntax error")
		}
	}
	if nx && xx {
		return Error("syntax error")
	}

	// The condition covers the whole batch, so it is evaluated before anything
	// is written.
	for i := 0; i < len(pairs); i += 2 {
		exists := ctx.DB.Exists(string(pairs[i]))
		if (nx && exists) || (xx && !exists) {
			return Integer(0)
		}
	}

	for i := 0; i < len(pairs); i += 2 {
		key := string(pairs[i])
		val := object.TryEncodeString(pairs[i+1])
		switch {
		case hasTTL:
			ctx.DB.SetWithExpire(key, val, ttl)
		case expAt > 0:
			ctx.DB.Set(key, val)
			ctx.DB.SetExpireAt(key, expAt)
		case keepTTL:
			ctx.DB.SetKeepTTL(key, val)
		default:
			ctx.DB.Set(key, val)
		}
	}
	return Integer(1)
}
