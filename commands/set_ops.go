package commands

import (
	"math/rand"
	"strconv"
	"strings"

	"github.com/gosuda/gopherdis/object"
)

func init() {
	reg := func(name string, h CommandHandler, arity int, flags CommandFlags) {
		DefaultTable.Register(&Command{Name: name, Handler: h, Arity: arity, Flags: flags})
	}
	reg("smismember", smismemberCommand, -3, FlagReadOnly|FlagFast)
	reg("sinter", sinterCommand, -2, FlagReadOnly)
	reg("sunion", sunionCommand, -2, FlagReadOnly)
	reg("sdiff", sdiffCommand, -2, FlagReadOnly)
	reg("sinterstore", sinterstoreCommand, -3, FlagWrite)
	reg("sunionstore", sunionstoreCommand, -3, FlagWrite)
	reg("sdiffstore", sdiffstoreCommand, -3, FlagWrite)
	reg("sintercard", sintercardCommand, -3, FlagReadOnly)
	reg("sunioncard", sunioncardCommand, -3, FlagReadOnly)
	reg("sdiffcard", sdiffcardCommand, -3, FlagReadOnly)
	reg("smove", smoveCommand, 4, FlagWrite|FlagFast)
	reg("srandmember", srandmemberCommand, -2, FlagReadOnly)
}

func smismemberCommand(ctx *Context, argv [][]byte) []byte {
	s, errReply := getSetView(ctx, string(argv[1]))
	if errReply != nil {
		return errReply
	}
	elems := make([][]byte, 0, len(argv)-2)
	for i := 2; i < len(argv); i++ {
		if s != nil && s.Contains(string(argv[i])) {
			elems = append(elems, Integer(1))
		} else {
			elems = append(elems, Integer(0))
		}
	}
	return Array(elems)
}

// setOp computes the union, intersection or difference of the given keys.
func setOp(ctx *Context, keys [][]byte, op string, limit int) ([]string, []byte) {
	sets := make([]*setView, 0, len(keys))
	for _, k := range keys {
		s, errReply := getSetView(ctx, string(k))
		if errReply != nil {
			return nil, errReply
		}
		sets = append(sets, s)
	}

	switch op {
	case "inter":
		if len(sets) == 0 || sets[0] == nil {
			return nil, nil // an empty operand makes the intersection empty
		}
		var out []string
		for _, m := range sets[0].Members() {
			inAll := true
			for _, other := range sets[1:] {
				if other == nil || !other.Contains(m) {
					inAll = false
					break
				}
			}
			if inAll {
				out = append(out, m)
				if limit > 0 && len(out) >= limit {
					break
				}
			}
		}
		return out, nil

	case "union":
		seen := make(map[string]struct{})
		var out []string
		for _, s := range sets {
			if s == nil {
				continue
			}
			for _, m := range s.Members() {
				if _, dup := seen[m]; !dup {
					seen[m] = struct{}{}
					out = append(out, m)
				}
			}
		}
		return out, nil

	default: // diff
		if len(sets) == 0 || sets[0] == nil {
			return nil, nil
		}
		var out []string
		for _, m := range sets[0].Members() {
			inOther := false
			for _, other := range sets[1:] {
				if other != nil && other.Contains(m) {
					inOther = true
					break
				}
			}
			if !inOther {
				out = append(out, m)
			}
		}
		return out, nil
	}
}

// membersReply emits set members: a set type in RESP3, an array in RESP2.
func membersReply(ctx *Context, members []string) []byte {
	elems := make([][]byte, 0, len(members))
	for _, m := range members {
		elems = append(elems, BulkString([]byte(m)))
	}
	return SetReply(ctx, elems)
}

func sinterCommand(ctx *Context, argv [][]byte) []byte {
	m, err := setOp(ctx, argv[1:], "inter", 0)
	if err != nil {
		return err
	}
	return membersReply(ctx, m)
}

func sunionCommand(ctx *Context, argv [][]byte) []byte {
	m, err := setOp(ctx, argv[1:], "union", 0)
	if err != nil {
		return err
	}
	return membersReply(ctx, m)
}

func sdiffCommand(ctx *Context, argv [][]byte) []byte {
	m, err := setOp(ctx, argv[1:], "diff", 0)
	if err != nil {
		return err
	}
	return membersReply(ctx, m)
}

func storeOp(ctx *Context, argv [][]byte, op string) []byte {
	dst := string(argv[1])
	members, errReply := setOp(ctx, argv[2:], op, 0)
	if errReply != nil {
		return errReply
	}

	ctx.DB.LockKey(dst)
	defer ctx.DB.UnlockKey(dst)

	if len(members) == 0 {
		ctx.DB.Del(dst)
		return Integer(0)
	}
	// Build through the view so the destination lands on whichever encoding its
	// contents call for. Storing a hash set unconditionally made every STORE
	// result report hashtable, even for a handful of integers.
	ctx.DB.Del(dst)
	v, _, errReply := getOrCreateSet(ctx, dst)
	if errReply != nil {
		return errReply
	}
	v.Add(members...)
	return Integer(int64(len(members)))
}

func sinterstoreCommand(ctx *Context, argv [][]byte) []byte { return storeOp(ctx, argv, "inter") }
func sunionstoreCommand(ctx *Context, argv [][]byte) []byte { return storeOp(ctx, argv, "union") }
func sdiffstoreCommand(ctx *Context, argv [][]byte) []byte  { return storeOp(ctx, argv, "diff") }

func sintercardCommand(ctx *Context, argv [][]byte) []byte {
	numKeys, err := strconv.Atoi(string(argv[1]))
	if err != nil || numKeys <= 0 {
		return Error("numkeys should be greater than 0")
	}
	if len(argv) < 2+numKeys {
		return Error("Number of keys can't be greater than number of args")
	}
	limit := 0
	if rest := argv[2+numKeys:]; len(rest) > 0 {
		if len(rest) != 2 || strings.ToUpper(string(rest[0])) != "LIMIT" {
			return Error("syntax error")
		}
		n, err := strconv.Atoi(string(rest[1]))
		if err != nil || n < 0 {
			return Error("LIMIT can't be negative")
		}
		limit = n
	}
	members, errReply := setOp(ctx, argv[2:2+numKeys], "inter", limit)
	if errReply != nil {
		return errReply
	}
	return Integer(int64(len(members)))
}

func smoveCommand(ctx *Context, argv [][]byte) []byte {
	src, dst, member := string(argv[1]), string(argv[2]), string(argv[3])

	first, second := src, dst
	if first > second {
		first, second = second, first
	}
	ctx.DB.LockKey(first)
	if second != first {
		ctx.DB.LockKey(second)
	}
	defer func() {
		if second != first {
			ctx.DB.UnlockKey(second)
		}
		ctx.DB.UnlockKey(first)
	}()

	srcSet, errReply := getSetView(ctx, src)
	if errReply != nil {
		return errReply
	}
	if srcSet == nil || !srcSet.Contains(member) {
		return Integer(0)
	}
	if dstObj, ok := ctx.DB.Get(dst); ok && dstObj != nil && dstObj.Type != object.OBJ_SET {
		return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}

	srcSet.Remove(member)
	if srcSet.Card() == 0 {
		ctx.DB.Del(src)
	}
	dstSet, _, errReply := getOrCreateSet(ctx, dst)
	if errReply != nil {
		return errReply
	}
	dstSet.Add(member)
	return Integer(1)
}

func srandmemberCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	s, errReply := getSetView(ctx, key)
	if errReply != nil {
		return errReply
	}

	if len(argv) == 2 {
		if s == nil || s.Card() == 0 {
			return NullBulkString()
		}
		m := s.Members()
		return BulkString([]byte(m[rand.Intn(len(m))]))
	}

	raw, err := strconv.ParseInt(string(argv[2]), 10, 64)
	if err != nil {
		return Error("value is not an integer or out of range")
	}
	count, withRepeats, errReply := randCount(raw)
	if errReply != nil {
		return errReply
	}
	if s == nil || s.Card() == 0 {
		return Array(nil)
	}
	members := s.Members()

	// A negative count may repeat members and always returns exactly |count|.
	if withRepeats {
		out := make([][]byte, 0, count)
		for i := 0; i < count; i++ {
			out = append(out, BulkString([]byte(members[rand.Intn(len(members))])))
		}
		return Array(out)
	}
	// SRANDMEMBER is an array even in RESP3, since it may repeat members.
	if count >= len(members) {
		return arrayOfBulk(members)
	}
	rand.Shuffle(len(members), func(i, j int) { members[i], members[j] = members[j], members[i] })
	return arrayOfBulk(members[:count])
}

// arrayOfBulk emits members as a plain array, for the commands that stay arrays
// in RESP3 because they may contain duplicates.
func arrayOfBulk(members []string) []byte {
	elems := make([][]byte, 0, len(members))
	for _, m := range members {
		elems = append(elems, BulkString([]byte(m)))
	}
	return Array(elems)
}

// sunioncardCommand reports the cardinality of the union without building it.
//
// APPROX is accepted and answered exactly: the token lets Redis return an
// estimate, and an exact count is a valid answer to a request for an
// approximate one.
func sunioncardCommand(ctx *Context, argv [][]byte) []byte {
	return setCardGeneric(ctx, argv, "union")
}

func sdiffcardCommand(ctx *Context, argv [][]byte) []byte {
	return setCardGeneric(ctx, argv, "diff")
}

func setCardGeneric(ctx *Context, argv [][]byte, op string) []byte {
	numKeys, err := strconv.Atoi(string(argv[1]))
	if err != nil {
		return Error("numkeys should be greater than 0")
	}
	if numKeys <= 0 {
		return Error("numkeys should be greater than 0")
	}
	if len(argv) < 2+numKeys {
		return Error("Number of keys can't be greater than number of args")
	}

	limit := 0
	for i := 2 + numKeys; i < len(argv); i++ {
		switch strings.ToUpper(string(argv[i])) {
		case "APPROX":
			// Accepted; the answer below is exact.
		case "LIMIT":
			if i+1 >= len(argv) {
				return Error("syntax error")
			}
			n, err := strconv.Atoi(string(argv[i+1]))
			if err != nil || n < 0 {
				return Error("LIMIT can't be negative")
			}
			limit = n
			i++
		default:
			return Error("syntax error")
		}
	}

	members, errReply := setOp(ctx, argv[2:2+numKeys], op, 0)
	if errReply != nil {
		return errReply
	}
	n := len(members)
	if limit > 0 && n > limit {
		n = limit
	}
	return Integer(int64(n))
}
