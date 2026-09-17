package commands

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/gosuda/gopherdis/datastruct/skiplist"
	"github.com/gosuda/gopherdis/object"
)

func init() {
	reg := func(name string, h CommandHandler, arity int, flags CommandFlags) {
		DefaultTable.Register(&Command{Name: name, Handler: h, Arity: arity, Flags: flags})
	}
	reg("zunionstore", zunionstoreCommand, -4, FlagWrite)
	reg("zinterstore", zinterstoreCommand, -4, FlagWrite)
	reg("zdiffstore", zdiffstoreCommand, -4, FlagWrite)
	reg("zunion", zunionCommand, -3, FlagReadOnly)
	reg("zinter", zinterCommand, -3, FlagReadOnly)
	reg("zdiff", zdiffCommand, -3, FlagReadOnly)
	reg("zintercard", zintercardCommand, -3, FlagReadOnly)
}

// zsetOperand is one input to a sorted set operation. A plain set counts as a
// sorted set whose members all score 1, which is what Redis does.
func zsetOperand(ctx *Context, key string) (map[string]float64, []string, []byte) {
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		return map[string]float64{}, nil, nil
	}

	scores := make(map[string]float64)
	var order []string

	switch obj.Type {
	case object.OBJ_ZSET:
		z, errReply := newZsetView(ctx, key, obj)
		if errReply != nil {
			return nil, nil, errReply
		}
		for _, el := range z.elements() {
			scores[el.Member] = el.Score
			order = append(order, el.Member)
		}
	case object.OBJ_SET:
		v, errReply := newSetView(ctx, key, obj)
		if errReply != nil {
			return nil, nil, errReply
		}
		for _, m := range v.Members() {
			scores[m] = 1
			order = append(order, m)
		}
	default:
		return nil, nil, Error("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	return scores, order, nil
}

// aggregate combines two scores under SUM, MIN or MAX.
func aggregate(mode string, a, b float64) float64 {
	switch mode {
	case "MIN":
		return math.Min(a, b)
	case "MAX":
		return math.Max(a, b)
	default:
		sum := a + b
		// Redis treats inf + -inf as 0 here rather than producing NaN.
		if math.IsNaN(sum) {
			return 0
		}
		return sum
	}
}

type zsetOpArgs struct {
	keys      []string
	weights   []float64
	aggregate string
	limit     int
	withScore bool
}

// parseZsetOp reads "numkeys key... [WEIGHTS ...] [AGGREGATE ...] [LIMIT n]
// [WITHSCORES]".
func parseZsetOp(argv [][]byte, allowLimit bool) (*zsetOpArgs, []byte) {
	numKeys, err := strconv.Atoi(string(argv[0]))
	if err != nil {
		return nil, Error("value is not an integer or out of range")
	}
	if numKeys <= 0 {
		return nil, Error("at least 1 input key is needed for the operation")
	}
	if len(argv) < 1+numKeys {
		return nil, Error("syntax error")
	}

	out := &zsetOpArgs{aggregate: "SUM", limit: 0}
	for i := 1; i <= numKeys; i++ {
		out.keys = append(out.keys, string(argv[i]))
	}
	out.weights = make([]float64, numKeys)
	for i := range out.weights {
		out.weights[i] = 1
	}

	for i := 1 + numKeys; i < len(argv); i++ {
		switch strings.ToUpper(string(argv[i])) {
		case "WEIGHTS":
			if i+numKeys >= len(argv) {
				return nil, Error("syntax error")
			}
			for j := 0; j < numKeys; j++ {
				w, err := strconv.ParseFloat(string(argv[i+1+j]), 64)
				if err != nil {
					return nil, Error("weight value is not a float")
				}
				out.weights[j] = w
			}
			i += numKeys
		case "AGGREGATE":
			if i+1 >= len(argv) {
				return nil, Error("syntax error")
			}
			mode := strings.ToUpper(string(argv[i+1]))
			// COUNT was added in Redis 8.10: it scores each member by how many
			// of the inputs contain it, ignoring weights.
			if mode != "SUM" && mode != "MIN" && mode != "MAX" && mode != "COUNT" {
				return nil, Error("syntax error")
			}
			out.aggregate = mode
			i++
		case "WITHSCORES":
			out.withScore = true
		case "LIMIT":
			if !allowLimit || i+1 >= len(argv) {
				return nil, Error("syntax error")
			}
			n, err := strconv.Atoi(string(argv[i+1]))
			if err != nil || n < 0 {
				return nil, Error("LIMIT can't be negative")
			}
			out.limit = n
			i++
		default:
			return nil, Error("syntax error")
		}
	}
	return out, nil
}

// zsetCombine computes the union, intersection or difference of the operands.
func zsetCombine(ctx *Context, op string, args *zsetOpArgs) ([]skiplist.ZSetElement, []byte) {
	type operand struct {
		scores map[string]float64
		order  []string
	}
	operands := make([]operand, 0, len(args.keys))
	for _, k := range args.keys {
		scores, order, errReply := zsetOperand(ctx, k)
		if errReply != nil {
			return nil, errReply
		}
		operands = append(operands, operand{scores, order})
	}

	result := make(map[string]float64)
	var order []string

	// AGGREGATE COUNT scores by occurrence, so it is computed over the raw
	// membership rather than through the weight and aggregate path.
	if args.aggregate == "COUNT" {
		counts := make(map[string]float64)
		for _, o := range operands {
			for _, m := range o.order {
				if _, seen := counts[m]; !seen && op == "union" {
					order = append(order, m)
				}
				counts[m]++
			}
		}
		if op == "union" {
			for _, m := range order {
				result[m] = counts[m]
			}
		} else {
			for _, m := range operands[0].order {
				if counts[m] == float64(len(operands)) {
					if op == "inter" {
						result[m] = counts[m]
						order = append(order, m)
					}
				} else if op == "diff" && counts[m] == 1 {
					result[m] = operands[0].scores[m]
					order = append(order, m)
				}
			}
		}
		out := make([]skiplist.ZSetElement, 0, len(order))
		for _, m := range order {
			out = append(out, skiplist.ZSetElement{Member: m, Score: result[m]})
		}
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].Score != out[j].Score {
				return out[i].Score < out[j].Score
			}
			return out[i].Member < out[j].Member
		})
		return out, nil
	}

	switch op {
	case "union":
		for i, o := range operands {
			for _, m := range o.order {
				w := o.scores[m] * args.weights[i]
				if math.IsNaN(w) {
					w = 0
				}
				if cur, seen := result[m]; seen {
					result[m] = aggregate(args.aggregate, cur, w)
				} else {
					result[m] = w
					order = append(order, m)
				}
			}
		}

	case "inter":
		if len(operands) == 0 {
			break
		}
		for _, m := range operands[0].order {
			score := operands[0].scores[m] * args.weights[0]
			if math.IsNaN(score) {
				score = 0
			}
			inAll := true
			for i := 1; i < len(operands); i++ {
				other, ok := operands[i].scores[m]
				if !ok {
					inAll = false
					break
				}
				w := other * args.weights[i]
				if math.IsNaN(w) {
					w = 0
				}
				score = aggregate(args.aggregate, score, w)
			}
			if inAll {
				result[m] = score
				order = append(order, m)
				if args.limit > 0 && len(order) >= args.limit {
					break
				}
			}
		}

	default: // diff
		if len(operands) == 0 {
			break
		}
		for _, m := range operands[0].order {
			found := false
			for i := 1; i < len(operands); i++ {
				if _, ok := operands[i].scores[m]; ok {
					found = true
					break
				}
			}
			if !found {
				// DIFF ignores weights and aggregation, as Redis does.
				result[m] = operands[0].scores[m]
				order = append(order, m)
			}
		}
	}

	out := make([]skiplist.ZSetElement, 0, len(order))
	for _, m := range order {
		out = append(out, skiplist.ZSetElement{Member: m, Score: result[m]})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score < out[j].Score
		}
		return out[i].Member < out[j].Member
	})
	return out, nil
}

func zsetStoreGeneric(ctx *Context, argv [][]byte, op string) []byte {
	dst := string(argv[1])
	args, errReply := parseZsetOp(argv[2:], false)
	if errReply != nil {
		return errReply
	}
	items, errReply := zsetCombine(ctx, op, args)
	if errReply != nil {
		return errReply
	}

	ctx.DB.LockKey(dst)
	defer ctx.DB.UnlockKey(dst)

	ctx.DB.Del(dst)
	if len(items) == 0 {
		return Integer(0)
	}
	z, _, errReply := getOrCreateZSet(ctx, dst)
	if errReply != nil {
		return errReply
	}
	for _, el := range items {
		z.Add(el.Member, el.Score)
	}
	return Integer(int64(len(items)))
}

func zunionstoreCommand(ctx *Context, argv [][]byte) []byte {
	return zsetStoreGeneric(ctx, argv, "union")
}
func zinterstoreCommand(ctx *Context, argv [][]byte) []byte {
	return zsetStoreGeneric(ctx, argv, "inter")
}
func zdiffstoreCommand(ctx *Context, argv [][]byte) []byte {
	return zsetStoreGeneric(ctx, argv, "diff")
}

func zsetReadGeneric(ctx *Context, argv [][]byte, op string) []byte {
	args, errReply := parseZsetOp(argv[1:], false)
	if errReply != nil {
		return errReply
	}
	items, errReply := zsetCombine(ctx, op, args)
	if errReply != nil {
		return errReply
	}
	return zsetReply(ctx, items, args.withScore)
}

func zunionCommand(ctx *Context, argv [][]byte) []byte { return zsetReadGeneric(ctx, argv, "union") }
func zinterCommand(ctx *Context, argv [][]byte) []byte { return zsetReadGeneric(ctx, argv, "inter") }
func zdiffCommand(ctx *Context, argv [][]byte) []byte  { return zsetReadGeneric(ctx, argv, "diff") }

func zintercardCommand(ctx *Context, argv [][]byte) []byte {
	args, errReply := parseZsetOp(argv[1:], true)
	if errReply != nil {
		return errReply
	}
	items, errReply := zsetCombine(ctx, "inter", args)
	if errReply != nil {
		return errReply
	}
	return Integer(int64(len(items)))
}
