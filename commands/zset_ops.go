package commands

import (
	"math"
	"math/rand"
	"strconv"
	"strings"

	"github.com/gosuda/gopherdis/datastruct/skiplist"
)

func init() {
	reg := func(name string, h CommandHandler, arity int, flags CommandFlags) {
		DefaultTable.Register(&Command{Name: name, Handler: h, Arity: arity, Flags: flags})
	}
	reg("zrangebyscore", zrangebyscoreCommand, -4, FlagReadOnly)
	reg("zrevrangebyscore", zrevrangebyscoreCommand, -4, FlagReadOnly)
	reg("zpopmin", zpopminCommand, -2, FlagWrite|FlagFast)
	reg("zpopmax", zpopmaxCommand, -2, FlagWrite|FlagFast)
	reg("zremrangebyrank", zremrangebyrankCommand, 4, FlagWrite)
	reg("zremrangebyscore", zremrangebyscoreCommand, 4, FlagWrite)
	reg("zmscore", zmscoreCommand, -3, FlagReadOnly|FlagFast)
	reg("zrandmember", zrandmemberCommand, -2, FlagReadOnly)
}

// scoreBound parses a ZRANGEBYSCORE bound: a number, "+inf"/"-inf", or a value
// prefixed with '(' for an exclusive bound.
func scoreBound(s string) (val float64, exclusive bool, ok bool) {
	if strings.HasPrefix(s, "(") {
		exclusive = true
		s = s[1:]
	}
	switch strings.ToLower(s) {
	case "+inf", "inf":
		return math.Inf(1), exclusive, true
	case "-inf":
		return math.Inf(-1), exclusive, true
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false, false
	}
	return v, exclusive, true
}

func inScoreRange(score, min, max float64, minEx, maxEx bool) bool {
	if minEx && score <= min {
		return false
	}
	if !minEx && score < min {
		return false
	}
	if maxEx && score >= max {
		return false
	}
	if !maxEx && score > max {
		return false
	}
	return true
}

func zrangeByScoreGeneric(ctx *Context, argv [][]byte, reverse bool) []byte {
	key := string(argv[1])
	minArg, maxArg := string(argv[2]), string(argv[3])
	if reverse {
		minArg, maxArg = maxArg, minArg
	}
	min, minEx, ok1 := scoreBound(minArg)
	max, maxEx, ok2 := scoreBound(maxArg)
	if !ok1 || !ok2 {
		return Error("min or max is not a float")
	}

	withScores := false
	offset, count := 0, -1
	for i := 4; i < len(argv); i++ {
		switch strings.ToUpper(string(argv[i])) {
		case "WITHSCORES":
			withScores = true
		case "LIMIT":
			if i+2 >= len(argv) {
				return Error("syntax error")
			}
			o, err1 := strconv.Atoi(string(argv[i+1]))
			c, err2 := strconv.Atoi(string(argv[i+2]))
			if err1 != nil || err2 != nil {
				return Error("value is not an integer or out of range")
			}
			offset, count = o, c
			i += 2
		default:
			return Error("syntax error")
		}
	}

	zs, errReply := getZSetForRead(ctx, key)
	if errReply != nil {
		return errReply
	}

	all := zs.Range(0, -1, reverse)
	var picked []skiplist.ZSetElement
	for _, el := range all {
		if inScoreRange(el.Score, min, max, minEx, maxEx) {
			picked = append(picked, el)
		}
	}
	if offset > 0 {
		if offset >= len(picked) {
			picked = nil
		} else {
			picked = picked[offset:]
		}
	}
	if count >= 0 && count < len(picked) {
		picked = picked[:count]
	}
	return zsetReply(picked, withScores)
}

func zsetReply(items []skiplist.ZSetElement, withScores bool) []byte {
	elems := make([][]byte, 0, len(items)*2)
	for _, el := range items {
		elems = append(elems, BulkString([]byte(el.Member)))
		if withScores {
			elems = append(elems, BulkString([]byte(formatFloat(el.Score))))
		}
	}
	return Array(elems)
}

func zrangebyscoreCommand(ctx *Context, argv [][]byte) []byte {
	return zrangeByScoreGeneric(ctx, argv, false)
}

func zrevrangebyscoreCommand(ctx *Context, argv [][]byte) []byte {
	return zrangeByScoreGeneric(ctx, argv, true)
}

func zpopGeneric(ctx *Context, argv [][]byte, reverse bool) []byte {
	key := string(argv[1])
	count := 1
	explicit := false
	if len(argv) >= 3 {
		n, err := strconv.Atoi(string(argv[2]))
		if err != nil || n < 0 {
			return Error("value is out of range, must be positive")
		}
		count, explicit = n, true
	}

	ctx.DB.LockKey(key)
	defer ctx.DB.UnlockKey(key)

	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return Array(nil)
	}
	zs, errReply := getZSetForRead(ctx, key)
	if errReply != nil {
		return errReply
	}

	items := zs.Range(0, int64(count)-1, reverse)
	for _, el := range items {
		zs.Remove(el.Member)
	}
	if zs.Len() == 0 {
		ctx.DB.Del(key)
	}
	_ = explicit
	return zsetReply(items, true)
}

func zpopminCommand(ctx *Context, argv [][]byte) []byte { return zpopGeneric(ctx, argv, false) }
func zpopmaxCommand(ctx *Context, argv [][]byte) []byte { return zpopGeneric(ctx, argv, true) }

func zremrangebyrankCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	start, err1 := strconv.Atoi(string(argv[2]))
	stop, err2 := strconv.Atoi(string(argv[3]))
	if err1 != nil || err2 != nil {
		return Error("value is not an integer or out of range")
	}

	ctx.DB.LockKey(key)
	defer ctx.DB.UnlockKey(key)

	zs, errReply := getZSetForRead(ctx, key)
	if errReply != nil {
		return errReply
	}
	items := zs.Range(int64(start), int64(stop), false)
	for _, el := range items {
		zs.Remove(el.Member)
	}
	if zs.Len() == 0 {
		ctx.DB.Del(key)
	}
	return Integer(int64(len(items)))
}

func zremrangebyscoreCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	min, minEx, ok1 := scoreBound(string(argv[2]))
	max, maxEx, ok2 := scoreBound(string(argv[3]))
	if !ok1 || !ok2 {
		return Error("min or max is not a float")
	}

	ctx.DB.LockKey(key)
	defer ctx.DB.UnlockKey(key)

	zs, errReply := getZSetForRead(ctx, key)
	if errReply != nil {
		return errReply
	}
	removed := 0
	for _, el := range zs.Range(0, -1, false) {
		if inScoreRange(el.Score, min, max, minEx, maxEx) {
			zs.Remove(el.Member)
			removed++
		}
	}
	if zs.Len() == 0 {
		ctx.DB.Del(key)
	}
	return Integer(int64(removed))
}

func zmscoreCommand(ctx *Context, argv [][]byte) []byte {
	zs, errReply := getZSetForRead(ctx, string(argv[1]))
	if errReply != nil {
		return errReply
	}
	elems := make([][]byte, 0, len(argv)-2)
	for i := 2; i < len(argv); i++ {
		if score, ok := zs.Score(string(argv[i])); ok {
			elems = append(elems, BulkString([]byte(formatFloat(score))))
		} else {
			elems = append(elems, NullBulkString())
		}
	}
	return Array(elems)
}

func zrandmemberCommand(ctx *Context, argv [][]byte) []byte {
	zs, errReply := getZSetForRead(ctx, string(argv[1]))
	if errReply != nil {
		return errReply
	}
	all := zs.Range(0, -1, false)

	if len(argv) == 2 {
		if len(all) == 0 {
			return NullBulkString()
		}
		return BulkString([]byte(all[rand.Intn(len(all))].Member))
	}

	raw, err := strconv.ParseInt(string(argv[2]), 10, 64)
	if err != nil {
		return Error("value is not an integer or out of range")
	}
	count, withRepeats, errReply := randCount(raw)
	if errReply != nil {
		return errReply
	}
	withScores := len(argv) > 3 && strings.ToUpper(string(argv[3])) == "WITHSCORES"
	if len(all) == 0 {
		return Array(nil)
	}

	// A negative count may repeat members and returns exactly |count|.
	if withRepeats {
		out := make([]skiplist.ZSetElement, 0, count)
		for i := 0; i < count; i++ {
			out = append(out, all[rand.Intn(len(all))])
		}
		return zsetReply(out, withScores)
	}
	if count >= len(all) {
		return zsetReply(all, withScores)
	}
	rand.Shuffle(len(all), func(i, j int) { all[i], all[j] = all[j], all[i] })
	return zsetReply(all[:count], withScores)
}
