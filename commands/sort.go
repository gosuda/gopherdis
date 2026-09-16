package commands

import (
	"bytes"
	"sort"
	"strconv"
	"strings"

	"github.com/gosuda/gopherdis/datastruct/quicklist"
	"github.com/gosuda/gopherdis/object"
)

func init() {
	DefaultTable.Register(&Command{Name: "sort", Handler: sortCommand, Arity: -2, Flags: FlagWrite})
	DefaultTable.Register(&Command{Name: "sort_ro", Handler: sortCommand, Arity: -2, Flags: FlagReadOnly})
}

// lookupPattern resolves a BY/GET pattern for one element. "#" means the
// element itself; otherwise the first '*' is substituted and the result is read
// as a key, or as a hash field when the pattern contains "->".
func lookupPattern(ctx *Context, pattern, element string) ([]byte, bool) {
	if pattern == "#" {
		return []byte(element), true
	}
	star := strings.Index(pattern, "*")
	if star < 0 {
		return nil, false // no substitution means every element weighs the same
	}
	resolved := strings.Replace(pattern, "*", element, 1)

	if arrow := strings.Index(resolved, "->"); arrow >= 0 {
		key, field := resolved[:arrow], resolved[arrow+2:]
		d, errReply := getHash(ctx, key)
		if errReply != nil || d == nil {
			return nil, false
		}
		v, ok := d.Get(field)
		return v, ok
	}

	obj, ok := ctx.DB.Get(resolved)
	if !ok || obj == nil || obj.Type != object.OBJ_STRING {
		return nil, false
	}
	return obj.Bytes(), true
}

func sortCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])

	var (
		desc     bool
		alpha    bool
		byPat    string
		getPats  []string
		storeKey string
		limited  bool
		offset   int
		count    int
	)

	for i := 2; i < len(argv); i++ {
		switch strings.ToUpper(string(argv[i])) {
		case "ASC":
			desc = false
		case "DESC":
			desc = true
		case "ALPHA":
			alpha = true
		case "BY":
			if i+1 >= len(argv) {
				return Error("syntax error")
			}
			byPat = string(argv[i+1])
			i++
		case "GET":
			if i+1 >= len(argv) {
				return Error("syntax error")
			}
			getPats = append(getPats, string(argv[i+1]))
			i++
		case "STORE":
			if i+1 >= len(argv) {
				return Error("syntax error")
			}
			storeKey = string(argv[i+1])
			i++
		case "LIMIT":
			if i+2 >= len(argv) {
				return Error("syntax error")
			}
			o, err1 := strconv.Atoi(string(argv[i+1]))
			c, err2 := strconv.Atoi(string(argv[i+2]))
			if err1 != nil || err2 != nil {
				return Error("value is not an integer or out of range")
			}
			offset, count, limited = o, c, true
			i += 2
		default:
			return Error("syntax error")
		}
	}

	elements, errReply := sortSourceElements(ctx, key)
	if errReply != nil {
		return errReply
	}

	// "BY <constant>" has no '*', so Redis skips sorting entirely.
	noSort := byPat != "" && !strings.Contains(byPat, "*")

	if !noSort {
		weight := func(e string) ([]byte, bool) {
			if byPat == "" {
				return []byte(e), true
			}
			return lookupPattern(ctx, byPat, e)
		}

		if alpha {
			sort.SliceStable(elements, func(i, j int) bool {
				a, _ := weight(elements[i])
				b, _ := weight(elements[j])
				if desc {
					return bytes.Compare(a, b) > 0
				}
				return bytes.Compare(a, b) < 0
			})
		} else {
			nums := make([]float64, len(elements))
			for i, e := range elements {
				w, ok := weight(e)
				if !ok {
					nums[i] = 0
					continue
				}
				f, err := strconv.ParseFloat(strings.TrimSpace(string(w)), 64)
				if err != nil {
					return Error("One or more scores can't be converted into double")
				}
				nums[i] = f
			}
			idx := make([]int, len(elements))
			for i := range idx {
				idx[i] = i
			}
			sort.SliceStable(idx, func(a, b int) bool {
				if nums[idx[a]] == nums[idx[b]] {
					return elements[idx[a]] < elements[idx[b]]
				}
				if desc {
					return nums[idx[a]] > nums[idx[b]]
				}
				return nums[idx[a]] < nums[idx[b]]
			})
			sorted := make([]string, len(elements))
			for i, id := range idx {
				sorted[i] = elements[id]
			}
			elements = sorted
		}
	}

	if limited {
		if offset < 0 {
			offset = 0
		}
		if offset >= len(elements) {
			elements = nil
		} else {
			elements = elements[offset:]
			if count >= 0 && count < len(elements) {
				elements = elements[:count]
			}
		}
	}

	// Build the output, expanding GET patterns when present.
	var out [][]byte
	if len(getPats) == 0 {
		for _, e := range elements {
			out = append(out, []byte(e))
		}
	} else {
		for _, e := range elements {
			for _, p := range getPats {
				v, ok := lookupPattern(ctx, p, e)
				if !ok {
					out = append(out, nil)
					continue
				}
				out = append(out, v)
			}
		}
	}

	if storeKey != "" {
		ctx.DB.LockKey(storeKey)
		defer ctx.DB.UnlockKey(storeKey)

		if len(out) == 0 {
			ctx.DB.Del(storeKey)
			return Integer(0)
		}
		ql := quicklist.NewQuicklist()
		for _, v := range out {
			ql.RPush(v)
		}
		_ = ctx.DB.Set(storeKey, &object.Robj{
			Type:     object.OBJ_LIST,
			Encoding: object.OBJ_ENCODING_QUICKLIST,
			Ptr:      ql,
		})
		return Integer(int64(len(out)))
	}

	elems := make([][]byte, 0, len(out))
	for _, v := range out {
		if v == nil {
			elems = append(elems, NullBulkString())
		} else {
			elems = append(elems, BulkString(v))
		}
	}
	return Array(elems)
}

// sortSourceElements reads the sortable elements of a list, set or sorted set.
//
// It dispatches on the object type rather than the payload type: lists, sets
// and sorted sets can all be listpack backed, so the payload alone no longer
// identifies the container.
func sortSourceElements(ctx *Context, key string) ([]string, []byte) {
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return nil, nil
	}
	switch obj.Type {
	case object.OBJ_LIST:
		l, errReply := newListView(ctx, key, obj)
		if errReply != nil {
			return nil, errReply
		}
		items := l.All()
		out := make([]string, 0, len(items))
		for _, it := range items {
			out = append(out, string(it))
		}
		return out, nil

	case object.OBJ_SET:
		v, errReply := newSetView(ctx, key, obj)
		if errReply != nil {
			return nil, errReply
		}
		return v.Members(), nil

	case object.OBJ_ZSET:
		z, errReply := newZsetView(ctx, key, obj)
		if errReply != nil {
			return nil, errReply
		}
		els := z.elements()
		out := make([]string, 0, len(els))
		for _, e := range els {
			out = append(out, e.Member)
		}
		return out, nil

	default:
		return nil, Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
}
