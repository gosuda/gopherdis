package commands

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/gosuda/gopherdis/object"
)

func init() {
	reg := func(name string, h CommandHandler, arity int, flags CommandFlags) {
		DefaultTable.Register(&Command{Name: name, Handler: h, Arity: arity, Flags: flags})
	}
	reg("linsert", linsertCommand, 5, FlagWrite)
	reg("ltrim", ltrimCommand, 4, FlagWrite)
	reg("lrem", lremCommand, 4, FlagWrite)
	reg("lpos", lposCommand, -3, FlagReadOnly)
	reg("lmove", lmoveCommand, 5, FlagWrite)
	reg("rpoplpush", rpoplpushCommand, 3, FlagWrite)
	reg("lpushx", lpushxCommand, -3, FlagWrite|FlagFast)
	reg("rpushx", rpushxCommand, -3, FlagWrite|FlagFast)
}

// storeList replaces a key's list with items, deleting the key when empty.
// The listpack has no splice primitive and neither does the quicklist, so the
// commands that reshape a list rebuild it; Redis' own LTRIM, LREM and LINSERT
// are linear as well. replaceAll picks the encoding that fits the result.
func storeList(ctx *Context, key string, items [][]byte) {
	if len(items) == 0 {
		ctx.DB.Del(key)
		return
	}
	l, _, errReply := getOrCreateList(ctx, key)
	if errReply != nil {
		return
	}
	l.replaceAll(items)
}

func linsertCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	where := strings.ToUpper(string(argv[2]))
	if where != "BEFORE" && where != "AFTER" {
		return Error("syntax error")
	}
	pivot, element := argv[3], argv[4]

	ctx.DB.LockKey(key)
	defer ctx.DB.UnlockKey(key)

	ql, errReply := getListView(ctx, key)
	if errReply != nil {
		return errReply
	}
	if ql == nil {
		return Integer(0)
	}

	items := ql.All()
	idx := -1
	for i, it := range items {
		if bytes.Equal(it, pivot) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return Integer(-1) // pivot not found
	}
	if where == "AFTER" {
		idx++
	}

	out := make([][]byte, 0, len(items)+1)
	out = append(out, items[:idx]...)
	out = append(out, bytes.Clone(element))
	out = append(out, items[idx:]...)
	storeList(ctx, key, out)
	return Integer(int64(len(out)))
}

func ltrimCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	start, err1 := strconv.Atoi(string(argv[2]))
	stop, err2 := strconv.Atoi(string(argv[3]))
	if err1 != nil || err2 != nil {
		return Error("value is not an integer or out of range")
	}

	ctx.DB.LockKey(key)
	defer ctx.DB.UnlockKey(key)

	ql, errReply := getListView(ctx, key)
	if errReply != nil {
		return errReply
	}
	if ql == nil {
		return OK()
	}

	n := ql.Len()
	if start < 0 {
		start += n
	}
	if stop < 0 {
		stop += n
	}
	if start < 0 {
		start = 0
	}
	if stop >= n {
		stop = n - 1
	}
	if start > stop || start >= n {
		ctx.DB.Del(key)
		return OK()
	}
	storeList(ctx, key, ql.LRange(start, stop))
	return OK()
}

func lremCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	count, err := strconv.Atoi(string(argv[2]))
	if err != nil {
		return Error("value is not an integer or out of range")
	}
	target := argv[3]

	ctx.DB.LockKey(key)
	defer ctx.DB.UnlockKey(key)

	ql, errReply := getListView(ctx, key)
	if errReply != nil {
		return errReply
	}
	if ql == nil {
		return Integer(0)
	}

	items := ql.All()
	// count > 0 removes from head, count < 0 from tail, 0 removes all.
	limit := count
	fromTail := false
	if count < 0 {
		limit, fromTail = -count, true
	}

	remove := make([]bool, len(items))
	removed := 0
	if fromTail {
		for i := len(items) - 1; i >= 0 && (limit == 0 || removed < limit); i-- {
			if bytes.Equal(items[i], target) {
				remove[i] = true
				removed++
			}
		}
	} else {
		for i := 0; i < len(items) && (limit == 0 || removed < limit); i++ {
			if bytes.Equal(items[i], target) {
				remove[i] = true
				removed++
			}
		}
	}
	if removed == 0 {
		return Integer(0)
	}

	out := make([][]byte, 0, len(items)-removed)
	for i, it := range items {
		if !remove[i] {
			out = append(out, it)
		}
	}
	storeList(ctx, key, out)
	return Integer(int64(removed))
}

func lposCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	target := argv[2]

	rank, count := 1, -1 // count < 0 means "not requested"
	maxlen := 0
	for i := 3; i < len(argv); i++ {
		if i+1 >= len(argv) {
			return Error("syntax error")
		}
		n, err := strconv.Atoi(string(argv[i+1]))
		if err != nil {
			return Error("value is not an integer or out of range")
		}
		switch strings.ToUpper(string(argv[i])) {
		case "RANK":
			if n == 0 {
				return Error("RANK can't be zero. Use 1 to start searching from the first matching element in the head of the list or -1 searching backward from the tail of the list")
			}
			rank = n
		case "COUNT":
			if n < 0 {
				return Error("COUNT can't be negative")
			}
			count = n
		case "MAXLEN":
			if n < 0 {
				return Error("MAXLEN can't be negative")
			}
			maxlen = n
		default:
			return Error("syntax error")
		}
		i++
	}

	ql, errReply := getListView(ctx, key)
	if errReply != nil {
		return errReply
	}
	if ql == nil {
		if count >= 0 {
			return Array(nil)
		}
		return NullBulkString()
	}

	items := ql.All()
	var found []int64
	skip := rank
	if rank < 0 {
		skip = -rank
	}

	scan := func(i int) bool {
		if !bytes.Equal(items[i], target) {
			return false
		}
		skip--
		if skip > 0 {
			return false
		}
		found = append(found, int64(i))
		return count >= 0 && count != 0 && len(found) >= count
	}

	if rank > 0 {
		for i := 0; i < len(items); i++ {
			if maxlen > 0 && i >= maxlen {
				break
			}
			if scan(i) {
				break
			}
		}
	} else {
		for i, seen := len(items)-1, 0; i >= 0; i, seen = i-1, seen+1 {
			if maxlen > 0 && seen >= maxlen {
				break
			}
			if scan(i) {
				break
			}
		}
	}

	if count >= 0 {
		elems := make([][]byte, 0, len(found))
		for _, f := range found {
			elems = append(elems, Integer(f))
		}
		return Array(elems)
	}
	if len(found) == 0 {
		return NullBulkString()
	}
	return Integer(found[0])
}

func lmoveGeneric(ctx *Context, src, dst, from, to string) []byte {
	if from != "LEFT" && from != "RIGHT" {
		return Error("syntax error")
	}
	if to != "LEFT" && to != "RIGHT" {
		return Error("syntax error")
	}

	// Order the stripe locks so two mirrored LMOVEs cannot deadlock.
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

	srcList, errReply := getListView(ctx, src)
	if errReply != nil {
		return errReply
	}
	if srcList == nil {
		return NullBulkString()
	}
	if dstObj, ok := ctx.DB.Get(dst); ok && dstObj != nil && dstObj.Type != object.OBJ_LIST {
		return Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}

	var val []byte
	var found bool
	if from == "LEFT" {
		val, found = srcList.LPop()
	} else {
		val, found = srcList.RPop()
	}
	if !found {
		return NullBulkString()
	}
	val = bytes.Clone(val)
	if srcList.Len() == 0 {
		ctx.DB.Del(src)
	}

	dstList, _, errReply := getOrCreateList(ctx, dst)
	if errReply != nil {
		return errReply
	}
	if to == "LEFT" {
		dstList.LPush(val)
	} else {
		dstList.RPush(val)
	}
	return BulkString(val)
}

func lmoveCommand(ctx *Context, argv [][]byte) []byte {
	return lmoveGeneric(ctx, string(argv[1]), string(argv[2]),
		strings.ToUpper(string(argv[3])), strings.ToUpper(string(argv[4])))
}

func rpoplpushCommand(ctx *Context, argv [][]byte) []byte {
	return lmoveGeneric(ctx, string(argv[1]), string(argv[2]), "RIGHT", "LEFT")
}

func pushxGeneric(ctx *Context, argv [][]byte, left bool) []byte {
	key := string(argv[1])

	ctx.DB.LockKey(key)
	defer ctx.DB.UnlockKey(key)

	ql, errReply := getListView(ctx, key)
	if errReply != nil {
		return errReply
	}
	if ql == nil {
		return Integer(0) // PUSHX never creates the key
	}
	for i := 2; i < len(argv); i++ {
		if left {
			ql.LPush(argv[i])
		} else {
			ql.RPush(argv[i])
		}
	}
	return Integer(int64(ql.Len()))
}

func lpushxCommand(ctx *Context, argv [][]byte) []byte { return pushxGeneric(ctx, argv, true) }
func rpushxCommand(ctx *Context, argv [][]byte) []byte { return pushxGeneric(ctx, argv, false) }
