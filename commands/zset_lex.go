package commands

import (
	"strconv"
	"strings"

	"github.com/gosuda/gopherdis/datastruct/skiplist"
)

func init() {
	reg := func(name string, h CommandHandler, arity int, flags CommandFlags) {
		DefaultTable.Register(&Command{Name: name, Handler: h, Arity: arity, Flags: flags})
	}
	reg("zrangebylex", zrangebylexCommand, -4, FlagReadOnly)
	reg("zrevrangebylex", zrevrangebylexCommand, -4, FlagReadOnly)
	reg("zlexcount", zlexcountCommand, 4, FlagReadOnly|FlagFast)
	reg("zremrangebylex", zremrangebylexCommand, 4, FlagWrite)
	reg("zrangestore", zrangestoreCommand, -5, FlagWrite)
}

// lexBound is one end of a lexicographic range: "-" and "+" are the minimum
// and maximum, "[x" includes x and "(x" excludes it.
type lexBound struct {
	value     string
	exclusive bool
	min, max  bool
}

func parseLexBound(s string) (lexBound, bool) {
	switch {
	case s == "-":
		return lexBound{min: true}, true
	case s == "+":
		return lexBound{max: true}, true
	case strings.HasPrefix(s, "["):
		return lexBound{value: s[1:]}, true
	case strings.HasPrefix(s, "("):
		return lexBound{value: s[1:], exclusive: true}, true
	default:
		return lexBound{}, false
	}
}

func (b lexBound) aboveMin(member string) bool {
	if b.min {
		return true
	}
	if b.max {
		return false
	}
	if b.exclusive {
		return member > b.value
	}
	return member >= b.value
}

func (b lexBound) belowMax(member string) bool {
	if b.max {
		return true
	}
	if b.min {
		return false
	}
	if b.exclusive {
		return member < b.value
	}
	return member <= b.value
}

// lexRange collects the members between two lexicographic bounds. It assumes
// the set is score-ordered, which for a lex range only makes sense when every
// member shares a score, exactly as Redis documents.
func lexRange(z *zsetView, min, max lexBound) []skiplist.ZSetElement {
	var out []skiplist.ZSetElement
	for _, el := range z.elements() {
		if min.aboveMin(el.Member) && max.belowMax(el.Member) {
			out = append(out, el)
		}
	}
	return out
}

func lexGeneric(ctx *Context, argv [][]byte, reverse bool) []byte {
	minArg, maxArg := string(argv[2]), string(argv[3])
	if reverse {
		minArg, maxArg = maxArg, minArg
	}
	min, ok1 := parseLexBound(minArg)
	max, ok2 := parseLexBound(maxArg)
	if !ok1 || !ok2 {
		return Error("min or max not valid string range item")
	}

	offset, count := 0, -1
	for i := 4; i < len(argv); i++ {
		if strings.ToUpper(string(argv[i])) != "LIMIT" || i+2 >= len(argv) {
			return Error("syntax error")
		}
		o, err1 := strconv.Atoi(string(argv[i+1]))
		c, err2 := strconv.Atoi(string(argv[i+2]))
		if err1 != nil || err2 != nil {
			return Error("value is not an integer or out of range")
		}
		offset, count = o, c
		i += 2
	}

	z, errReply := getZSetForRead(ctx, string(argv[1]))
	if errReply != nil {
		return errReply
	}
	items := lexRange(z, min, max)
	if reverse {
		for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
			items[i], items[j] = items[j], items[i]
		}
	}
	if offset < 0 {
		items = nil
	} else if offset > 0 {
		if offset >= len(items) {
			items = nil
		} else {
			items = items[offset:]
		}
	}
	if count >= 0 && count < len(items) {
		items = items[:count]
	}
	return zsetReply(items, false)
}

func zrangebylexCommand(ctx *Context, argv [][]byte) []byte {
	return lexGeneric(ctx, argv, false)
}

func zrevrangebylexCommand(ctx *Context, argv [][]byte) []byte {
	return lexGeneric(ctx, argv, true)
}

func zlexcountCommand(ctx *Context, argv [][]byte) []byte {
	min, ok1 := parseLexBound(string(argv[2]))
	max, ok2 := parseLexBound(string(argv[3]))
	if !ok1 || !ok2 {
		return Error("min or max not valid string range item")
	}
	z, errReply := getZSetForRead(ctx, string(argv[1]))
	if errReply != nil {
		return errReply
	}
	return Integer(int64(len(lexRange(z, min, max))))
}

func zremrangebylexCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	min, ok1 := parseLexBound(string(argv[2]))
	max, ok2 := parseLexBound(string(argv[3]))
	if !ok1 || !ok2 {
		return Error("min or max not valid string range item")
	}

	ctx.DB.LockKey(key)
	defer ctx.DB.UnlockKey(key)

	z, errReply := getZSetForRead(ctx, key)
	if errReply != nil {
		return errReply
	}
	items := lexRange(z, min, max)
	for _, el := range items {
		z.Remove(el.Member)
	}
	if z.Len() == 0 {
		ctx.DB.Del(key)
	}
	return Integer(int64(len(items)))
}

// zrangestoreCommand is ZRANGE with a destination key.
func zrangestoreCommand(ctx *Context, argv [][]byte) []byte {
	dst := string(argv[1])
	items, errReply := zrangeSelect(ctx, argv[2:])
	if errReply != nil {
		return errReply
	}

	ctx.DB.LockKey(dst)
	defer ctx.DB.UnlockKey(dst)

	if len(items) == 0 {
		ctx.DB.Del(dst)
		return Integer(0)
	}
	ctx.DB.Del(dst)
	z, _, errReply := getOrCreateZSet(ctx, dst)
	if errReply != nil {
		return errReply
	}
	for _, el := range items {
		z.Add(el.Member, el.Score)
	}
	return Integer(int64(len(items)))
}

// zrangeSelect implements the element selection shared by ZRANGE and
// ZRANGESTORE: args is "key start stop [BYSCORE|BYLEX] [REV] [LIMIT o c]".
//
// Redis 6.2 folded ZRANGEBYSCORE and ZRANGEBYLEX into ZRANGE behind these
// options, and LIMIT is only legal with BYSCORE or BYLEX.
func zrangeSelect(ctx *Context, args [][]byte) ([]skiplist.ZSetElement, []byte) {
	if len(args) < 3 {
		return nil, Error("syntax error")
	}
	key := string(args[0])
	startArg, stopArg := string(args[1]), string(args[2])

	byScore, byLex, reverse := false, false, false
	offset, count := 0, -1
	hasLimit := false

	for i := 3; i < len(args); i++ {
		switch strings.ToUpper(string(args[i])) {
		case "BYSCORE":
			byScore = true
		case "BYLEX":
			byLex = true
		case "REV":
			reverse = true
		case "WITHSCORES":
			// Handled by the caller, which decides whether to emit scores.
		case "LIMIT":
			if i+2 >= len(args) {
				return nil, Error("syntax error")
			}
			o, err1 := strconv.Atoi(string(args[i+1]))
			c, err2 := strconv.Atoi(string(args[i+2]))
			if err1 != nil || err2 != nil {
				return nil, Error("value is not an integer or out of range")
			}
			offset, count, hasLimit = o, c, true
			i += 2
		default:
			return nil, Error("syntax error")
		}
	}
	if byScore && byLex {
		return nil, Error("syntax error")
	}
	if hasLimit && !byScore && !byLex {
		return nil, Error("syntax error, LIMIT is only supported in combination with either BYSCORE or BYLEX")
	}

	z, errReply := getZSetForRead(ctx, key)
	if errReply != nil {
		return nil, errReply
	}

	var items []skiplist.ZSetElement
	switch {
	case byLex:
		lo, hi := startArg, stopArg
		if reverse {
			lo, hi = hi, lo
		}
		min, ok1 := parseLexBound(lo)
		max, ok2 := parseLexBound(hi)
		if !ok1 || !ok2 {
			return nil, Error("min or max not valid string range item")
		}
		items = lexRange(z, min, max)
		if reverse {
			reverseElements(items)
		}

	case byScore:
		lo, hi := startArg, stopArg
		if reverse {
			lo, hi = hi, lo
		}
		min, minEx, ok1 := scoreBound(lo)
		max, maxEx, ok2 := scoreBound(hi)
		if !ok1 || !ok2 {
			return nil, Error("min or max is not a float")
		}
		for _, el := range z.elements() {
			if inScoreRange(el.Score, min, max, minEx, maxEx) {
				items = append(items, el)
			}
		}
		if reverse {
			reverseElements(items)
		}

	default:
		start, err1 := strconv.ParseInt(startArg, 10, 64)
		stop, err2 := strconv.ParseInt(stopArg, 10, 64)
		if err1 != nil || err2 != nil {
			return nil, Error("value is not an integer or out of range")
		}
		items = z.Range(start, stop, reverse)
	}

	if offset < 0 {
		items = nil
	} else if offset > 0 {
		if offset >= len(items) {
			items = nil
		} else {
			items = items[offset:]
		}
	}
	if count >= 0 && count < len(items) {
		items = items[:count]
	}
	return items, nil
}

func reverseElements(items []skiplist.ZSetElement) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}
