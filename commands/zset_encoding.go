package commands

import (
	"sort"
	"strconv"

	"github.com/gosuda/gopherdis/datastruct/listpack"
	"github.com/gosuda/gopherdis/datastruct/skiplist"
	"github.com/gosuda/gopherdis/encoding"
	"github.com/gosuda/gopherdis/object"
)

// zsetView is a sorted set stored either as a listpack or as a skiplist.
//
// A small sorted set lives in a listpack holding member and score alternately,
// kept in score order so range queries are a slice of the buffer. Past
// zset-max-listpack-entries, or with a member longer than
// zset-max-listpack-value, it is promoted to the skiplist, which is what makes
// ZRANK and score ranges logarithmic instead of linear.
type zsetView struct {
	ctx *Context
	key string
	obj *object.Robj
	lp  *listpack.Listpack
	zs  *skiplist.ZSet
}

func (z *zsetView) Len() int64 {
	if z.zs != nil {
		return z.zs.Len()
	}
	return int64(z.lp.Len() / 2)
}

func (z *zsetView) Score(member string) (float64, bool) {
	if z.zs != nil {
		return z.zs.Score(member)
	}
	i := z.lp.Find([]byte(member), 0, 2)
	if i < 0 {
		return 0, false
	}
	f, err := strconv.ParseFloat(string(z.lp.Get(i+1)), 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// elements returns every member in score order.
func (z *zsetView) elements() []skiplist.ZSetElement {
	if z.zs != nil {
		return z.zs.Range(0, -1, false)
	}
	out := make([]skiplist.ZSetElement, 0, z.lp.Len()/2)
	var member string
	z.lp.ForEach(func(i int, v []byte) bool {
		if i%2 == 0 {
			member = string(v)
		} else {
			f, _ := strconv.ParseFloat(string(v), 64)
			out = append(out, skiplist.ZSetElement{Member: member, Score: f})
		}
		return true
	})
	return out
}

// Range returns elements by rank, honouring negative indices.
func (z *zsetView) Range(start, stop int64, reverse bool) []skiplist.ZSetElement {
	if z.zs != nil {
		return z.zs.Range(start, stop, reverse)
	}
	all := z.elements()
	if reverse {
		for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
			all[i], all[j] = all[j], all[i]
		}
	}
	n := int64(len(all))
	if n == 0 {
		return nil
	}
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
		return nil
	}
	return all[start : stop+1]
}

func (z *zsetView) Rank(member string, reverse bool) (int64, bool) {
	if z.zs != nil {
		return z.zs.Rank(member, reverse)
	}
	all := z.elements()
	for i, el := range all {
		if el.Member == member {
			if reverse {
				return int64(len(all) - 1 - i), true
			}
			return int64(i), true
		}
	}
	return -1, false
}

// Add inserts or updates a member.
func (z *zsetView) Add(member string, score float64) (added bool, updated bool) {
	if z.zs == nil && z.shouldPromote(member) {
		z.promote()
	}
	if z.zs != nil {
		return z.zs.Add(member, score)
	}

	scoreStr := formatFloat(score)
	if i := z.lp.Find([]byte(member), 0, 2); i >= 0 {
		old := string(z.lp.Get(i + 1))
		if old == scoreStr {
			return false, false
		}
		// The listpack stays score ordered, so a changed score is removed and
		// reinserted at its new position rather than edited in place.
		z.lp.DeleteRange(i, 2)
		z.insertOrdered(member, scoreStr, score)
		return false, true
	}
	z.insertOrdered(member, scoreStr, score)
	return true, false
}

// insertOrdered places member at the position that keeps the listpack sorted by
// score, breaking ties lexicographically as Redis does.
func (z *zsetView) insertOrdered(member, scoreStr string, score float64) {
	all := z.elements()
	idx := sort.Search(len(all), func(i int) bool {
		if all[i].Score != score {
			return all[i].Score > score
		}
		return all[i].Member > member
	})
	z.lp.InsertAt(idx*2, []byte(member))
	z.lp.InsertAt(idx*2+1, []byte(scoreStr))
}

func (z *zsetView) Remove(member string) bool {
	if z.zs != nil {
		return z.zs.Remove(member)
	}
	i := z.lp.Find([]byte(member), 0, 2)
	if i < 0 {
		return false
	}
	z.lp.DeleteRange(i, 2)
	return true
}

func (z *zsetView) shouldPromote(member string) bool {
	if len(member) > encoding.ZsetMaxListpackValue() {
		return true
	}
	if z.lp.Find([]byte(member), 0, 2) >= 0 {
		return false
	}
	return z.lp.Len()/2 >= encoding.ZsetMaxListpackEntries()
}

func (z *zsetView) promote() {
	zs := skiplist.NewZSet()
	for _, el := range z.elements() {
		zs.Add(el.Member, el.Score)
	}
	z.zs = zs
	z.lp = nil
	z.publish(zs, object.OBJ_ENCODING_SKIPLIST)
}

func (z *zsetView) publish(ptr any, enc object.ObjectEncoding) {
	z.obj.Encoding = enc
	if z.ctx == nil || z.ctx.DB == nil {
		z.obj.Ptr = ptr
		return
	}
	_ = z.ctx.DB.SetKeepTTL(z.key, &object.Robj{
		Type:     object.OBJ_ZSET,
		Encoding: enc,
		Ptr:      ptr,
	})
}

func newZsetView(ctx *Context, key string, obj *object.Robj) (*zsetView, []byte) {
	switch p := obj.Ptr.(type) {
	case *listpack.Listpack:
		return &zsetView{ctx: ctx, key: key, obj: obj, lp: p}, nil
	case *skiplist.ZSet:
		return &zsetView{ctx: ctx, key: key, obj: obj, zs: p}, nil
	default:
		return nil, Error("internal zset type error")
	}
}

// emptyZsetView is a detached sorted set used by read-only commands so that a
// missing key does not have to be materialised.
func emptyZsetView() *zsetView {
	return &zsetView{obj: &object.Robj{Type: object.OBJ_ZSET}, lp: listpack.New()}
}
