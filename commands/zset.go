package commands

import (
	"bytes"
	"math"
	"strconv"
	"strings"

	"github.com/gosuda/gopherdis/datastruct/listpack"
	"github.com/gosuda/gopherdis/object"
)

func init() {
	DefaultTable.Register(&Command{
		Name:    "zadd",
		Handler: zaddCommand,
		Arity:   -4,
		Flags:   FlagFast | FlagWrite,
	})
	DefaultTable.Register(&Command{
		Name:    "zscore",
		Handler: zscoreCommand,
		Arity:   3,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "zrank",
		Handler: zrankCommand,
		Arity:   -3,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "zrevrank",
		Handler: zrevrankCommand,
		Arity:   -3,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "zrange",
		Handler: zrangeCommand,
		Arity:   -4,
		Flags:   FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "zrevrange",
		Handler: zrevrangeCommand,
		Arity:   -4,
		Flags:   FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "zcard",
		Handler: zcardCommand,
		Arity:   2,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "zcount",
		Handler: zcountCommand,
		Arity:   4,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "zrem",
		Handler: zremCommand,
		Arity:   -3,
		Flags:   FlagFast | FlagWrite,
	})
	DefaultTable.Register(&Command{
		Name:    "zincrby",
		Handler: zincrbyCommand,
		Arity:   4,
		Flags:   FlagFast | FlagWrite,
	})
}

// getZSetForRead resolves key to its sorted set without ever storing anything.
// A missing key yields a detached empty view so that read-only callers can
// share the normal code path; read commands must never materialise a key.
func getZSetForRead(ctx *Context, key string) (*zsetView, []byte) {
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return emptyZsetView(), nil
	}
	if obj.Type != object.OBJ_ZSET {
		return nil, Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	return newZsetView(ctx, key, obj)
}

func getOrCreateZSet(ctx *Context, key string) (*zsetView, bool, []byte) {
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		// New sorted sets start compact, as Redis does.
		lp := listpack.New()
		newObj := &object.Robj{
			Type:     object.OBJ_ZSET,
			Encoding: object.OBJ_ENCODING_LISTPACK,
			Ptr:      lp,
		}
		ctx.DB.Set(key, newObj)
		return &zsetView{ctx: ctx, key: key, obj: newObj, lp: lp}, true, nil
	}
	if obj.Type != object.OBJ_ZSET {
		return nil, false, Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	z, errReply := newZsetView(ctx, key, obj)
	if errReply != nil {
		return nil, false, errReply
	}
	return z, false, nil
}

func zaddCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])

	var nx, xx, gt, lt, ch, incr bool
	idx := 2
	for ; idx < len(argv); idx++ {
		switch strings.ToUpper(string(argv[idx])) {
		case "NX":
			nx = true
		case "XX":
			xx = true
		case "GT":
			gt = true
		case "LT":
			lt = true
		case "CH":
			ch = true
		case "INCR":
			incr = true
		default:
			goto parsed
		}
	}
parsed:

	if nx && xx {
		return Error("XX and NX options at the same time are not compatible")
	}
	if nx && (gt || lt) {
		return Error("GT, LT, and/or NX options at the same time are not compatible")
	}
	if gt && lt {
		return Error("GT, LT, and/or NX options at the same time are not compatible")
	}

	pairs := argv[idx:]
	if len(pairs) == 0 || len(pairs)%2 != 0 {
		return Error("syntax error")
	}
	if incr && len(pairs) != 2 {
		return Error("INCR option supports a single increment-element pair")
	}

	// Parse every score before touching the set: ZADD is all or nothing on a
	// malformed argument.
	scores := make([]float64, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		score, err := strconv.ParseFloat(string(pairs[i]), 64)
		if err != nil || math.IsNaN(score) {
			return Error("value is not a valid float")
		}
		scores[i/2] = score
	}

	ctx.DB.LockKey(key)
	defer ctx.DB.UnlockKey(key)

	zs, created, errReply := getOrCreateZSet(ctx, key)
	if errReply != nil {
		return errReply
	}

	var addedCount, changedCount int64
	for i := 0; i < len(pairs); i += 2 {
		score := scores[i/2]
		member := string(pairs[i+1])

		cur, exists := zs.Score(member)
		if (nx && exists) || (xx && !exists) {
			continue
		}
		if incr {
			if exists {
				score += cur
				if math.IsNaN(score) {
					return Error("resulting score is not a number (NaN)")
				}
			}
		}
		if exists && ((gt && score <= cur) || (lt && score >= cur)) {
			continue
		}

		added, updated := zs.Add(member, score)
		if added {
			addedCount++
		}
		if added || updated {
			changedCount++
		}
		if incr {
			return BulkString([]byte(formatFloat(score)))
		}
	}

	// INCR with a skipped element replies with a nil, not a count.
	if incr {
		if created && zs.Len() == 0 {
			ctx.DB.Del(key)
		}
		return NullBulkString()
	}
	if created && zs.Len() == 0 {
		// XX against a key that did not exist must not leave one behind.
		ctx.DB.Del(key)
	}
	if ch {
		return Integer(changedCount)
	}
	return Integer(addedCount)
}

func zscoreCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	member := string(argv[2])

	zs, errReply := getZSetForRead(ctx, key)
	if errReply != nil {
		return errReply
	}

	score, ok := zs.Score(member)
	if !ok {
		return NullBulkString()
	}
	return BulkString([]byte(strconv.FormatFloat(score, 'f', -1, 64)))
}

func zrankCommand(ctx *Context, argv [][]byte) []byte {
	return zrankGeneric(ctx, argv, false)
}

func zrevrankCommand(ctx *Context, argv [][]byte) []byte {
	return zrankGeneric(ctx, argv, true)
}

func zrankGeneric(ctx *Context, argv [][]byte, reverse bool) []byte {
	key := string(argv[1])

	withScore := false
	if len(argv) == 4 {
		if !strings.EqualFold(string(argv[3]), "WITHSCORE") {
			return Error("syntax error")
		}
		withScore = true
	} else if len(argv) > 4 {
		return Error("syntax error")
	}

	zs, errReply := getZSetForRead(ctx, key)
	if errReply != nil {
		return errReply
	}

	member := string(argv[2])
	rank, found := zs.Rank(member, reverse)
	if !found {
		// WITHSCORE replies with a nil array rather than a nil bulk string,
		// because the non-nil form is a two element array.
		if withScore {
			return NullArray()
		}
		return NullBulkString()
	}
	if withScore {
		score, _ := zs.Score(member)
		return Array([][]byte{
			Integer(rank),
			BulkString([]byte(formatFloat(score))),
		})
	}
	return Integer(rank)
}

func zrangeCommand(ctx *Context, argv [][]byte) []byte {
	return zrangeGeneric(ctx, argv, false)
}

func zrevrangeCommand(ctx *Context, argv [][]byte) []byte {
	return zrangeGeneric(ctx, argv, true)
}

func zrangeGeneric(ctx *Context, argv [][]byte, defaultRev bool) []byte {
	key := string(argv[1])
	start, err1 := strconv.ParseInt(string(argv[2]), 10, 64)
	stop, err2 := strconv.ParseInt(string(argv[3]), 10, 64)
	if err1 != nil || err2 != nil {
		return Error("value is not an integer or out of range")
	}

	withScores := false
	reverse := defaultRev

	for i := 4; i < len(argv); i++ {
		opt := strings.ToUpper(string(argv[i]))
		if opt == "WITHSCORES" {
			withScores = true
		} else if opt == "REV" {
			reverse = true
		}
	}

	zs, errReply := getZSetForRead(ctx, key)
	if errReply != nil {
		return errReply
	}

	items := zs.Range(start, stop, reverse)
	if len(items) == 0 {
		return []byte("*0\r\n")
	}

	totalCount := len(items)
	if withScores {
		totalCount *= 2
	}

	var buf bytes.Buffer
	buf.Grow(totalCount * 32)
	buf.WriteByte('*')
	buf.Write(strconv.AppendInt(nil, int64(totalCount), 10))
	buf.WriteString("\r\n")

	for _, item := range items {
		buf.WriteByte('$')
		buf.Write(strconv.AppendInt(nil, int64(len(item.Member)), 10))
		buf.WriteString("\r\n")
		buf.WriteString(item.Member)
		buf.WriteString("\r\n")

		if withScores {
			scoreBytes := strconv.AppendFloat(nil, item.Score, 'f', -1, 64)
			buf.WriteByte('$')
			buf.Write(strconv.AppendInt(nil, int64(len(scoreBytes)), 10))
			buf.WriteString("\r\n")
			buf.Write(scoreBytes)
			buf.WriteString("\r\n")
		}
	}
	return buf.Bytes()
}

func zcardCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	zs, errReply := getZSetForRead(ctx, key)
	if errReply != nil {
		return errReply
	}
	return Integer(zs.Len())
}

func zremCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])

	ctx.DB.LockKey(key)
	defer ctx.DB.UnlockKey(key)

	zs, errReply := getZSetForRead(ctx, key)
	if errReply != nil {
		return errReply
	}

	deleted := int64(0)
	for i := 2; i < len(argv); i++ {
		member := string(argv[i])
		if zs.Remove(member) {
			deleted++
		}
	}
	if zs.Len() == 0 {
		ctx.DB.Del(key)
	}
	return Integer(deleted)
}

func zincrbyCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	delta, err := strconv.ParseFloat(string(argv[2]), 64)
	if err != nil || math.IsNaN(delta) {
		return Error("value is not a valid float")
	}
	member := string(argv[3])

	ctx.DB.LockKey(key)
	defer ctx.DB.UnlockKey(key)

	zs, _, errReply := getOrCreateZSet(ctx, key)
	if errReply != nil {
		return errReply
	}

	// Score -> Add is a read-modify-write and needs the key lock, same as INCR.
	currentScore, _ := zs.Score(member)
	newScore := currentScore + delta
	if math.IsNaN(newScore) {
		return Error("resulting score is not a number (NaN)")
	}
	zs.Add(member, newScore)

	return BulkString([]byte(formatFloat(newScore)))
}

func zcountCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	minScore, err := strconv.ParseFloat(string(argv[2]), 64)
	if err != nil {
		return Error("min or max is not a float")
	}
	maxScore, err := strconv.ParseFloat(string(argv[3]), 64)
	if err != nil {
		return Error("min or max is not a float")
	}

	zs, errReply := getZSetForRead(ctx, key)
	if errReply != nil {
		return errReply
	}

	items := zs.Range(0, -1, false)
	count := int64(0)
	for _, item := range items {
		if item.Score >= minScore && item.Score <= maxScore {
			count++
		}
	}
	return Integer(count)
}
