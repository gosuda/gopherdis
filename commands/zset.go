package commands

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/gosuda/nedis/datastruct/skiplist"
	"github.com/gosuda/nedis/object"
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
		Arity:   3,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "zrevrank",
		Handler: zrevrankCommand,
		Arity:   3,
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

func getOrCreateZSet(ctx *Context, key string) (*skiplist.ZSet, bool, []byte) {
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		zs := skiplist.NewZSet()
		ctx.DB.Set(key, &object.Robj{
			Type:     object.OBJ_ZSET,
			Encoding: object.OBJ_ENCODING_SKIPLIST,
			Ptr:      zs,
		})
		return zs, true, nil
	}
	if obj.Type != object.OBJ_ZSET {
		return nil, false, Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	zs, ok := obj.Ptr.(*skiplist.ZSet)
	if !ok {
		return nil, false, Error("ERR internal zset type error")
	}
	return zs, false, nil
}

func zaddCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	pairs := argv[2:]
	if len(pairs)%2 != 0 {
		return Error("syntax error")
	}

	zs, _, errReply := getOrCreateZSet(ctx, key)
	if errReply != nil {
		return errReply
	}

	addedCount := int64(0)
	for i := 0; i < len(pairs); i += 2 {
		score, err := strconv.ParseFloat(string(pairs[i]), 64)
		if err != nil {
			return Error("value is not a valid float")
		}
		member := string(pairs[i+1])
		added, _ := zs.Add(member, score)
		if added {
			addedCount++
		}
	}
	return Integer(addedCount)
}

func zscoreCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	member := string(argv[2])

	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return NullBulkString()
	}
	if obj.Type != object.OBJ_ZSET {
		return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	zs, ok := obj.Ptr.(*skiplist.ZSet)
	if !ok {
		return Error("ERR internal zset type error")
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
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return NullBulkString()
	}
	if obj.Type != object.OBJ_ZSET {
		return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	zs, ok := obj.Ptr.(*skiplist.ZSet)
	if !ok {
		return Error("ERR internal zset type error")
	}

	rank, found := zs.Rank(string(argv[2]), reverse)
	if !found {
		return NullBulkString()
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

	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return []byte("*0\r\n")
	}
	if obj.Type != object.OBJ_ZSET {
		return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	zs, ok := obj.Ptr.(*skiplist.ZSet)
	if !ok {
		return Error("ERR internal zset type error")
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
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return Integer(0)
	}
	if obj.Type != object.OBJ_ZSET {
		return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	zs, ok := obj.Ptr.(*skiplist.ZSet)
	if !ok {
		return Error("ERR internal zset type error")
	}
	return Integer(zs.Len())
}

func zremCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return Integer(0)
	}
	if obj.Type != object.OBJ_ZSET {
		return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	zs, ok := obj.Ptr.(*skiplist.ZSet)
	if !ok {
		return Error("ERR internal zset type error")
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
	if err != nil {
		return Error("value is not a valid float")
	}
	member := string(argv[3])

	zs, _, errReply := getOrCreateZSet(ctx, key)
	if errReply != nil {
		return errReply
	}

	currentScore, _ := zs.Score(member)
	newScore := currentScore + delta
	zs.Add(member, newScore)

	return BulkString([]byte(fmt.Sprintf("%g", newScore)))
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

	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return Integer(0)
	}
	if obj.Type != object.OBJ_ZSET {
		return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	zs, ok := obj.Ptr.(*skiplist.ZSet)
	if !ok {
		return Error("ERR internal zset type error")
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
