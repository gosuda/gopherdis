package commands

import (
	"github.com/gosuda/gopherdis/datastruct/intset"
	"github.com/gosuda/gopherdis/datastruct/listpack"
	"github.com/gosuda/gopherdis/datastruct/set"
	"github.com/gosuda/gopherdis/encoding"
	"github.com/gosuda/gopherdis/object"
)

// setView is a set stored as an intset, a listpack or a hash set.
//
// Redis picks the encoding from the contents: all-integer members below
// set-max-intset-entries use intset; otherwise small sets use listpack; past
// the listpack thresholds it becomes a hash table. Conversions only go
// forward, never back.
type setView struct {
	ctx *Context
	key string
	obj *object.Robj
	is  *intset.IntSet
	lp  *listpack.Listpack
	s   *set.Set
}

func (v *setView) Card() int {
	switch {
	case v.s != nil:
		return v.s.Card()
	case v.lp != nil:
		return v.lp.Len()
	default:
		return v.is.Len()
	}
}

func (v *setView) Contains(m string) bool {
	switch {
	case v.s != nil:
		return v.s.Contains(m)
	case v.lp != nil:
		return v.lp.Find([]byte(m), 0, 1) >= 0
	default:
		n, ok := intset.Parse(m)
		return ok && v.is.Contains(n)
	}
}

func (v *setView) Members() []string {
	switch {
	case v.s != nil:
		return v.s.Members()
	case v.lp != nil:
		out := make([]string, 0, v.lp.Len())
		v.lp.ForEach(func(_ int, b []byte) bool {
			out = append(out, string(b))
			return true
		})
		return out
	default:
		return v.is.Members()
	}
}

// Add inserts members, returning how many were new.
func (v *setView) Add(members ...string) int {
	added := 0
	for _, m := range members {
		if v.addOne(m) {
			added++
		}
	}
	return added
}

func (v *setView) addOne(m string) bool {
	if v.Contains(m) {
		return false
	}

	// intset holds integers only, and only up to its entry limit.
	if v.is != nil {
		n, isInt := intset.Parse(m)
		if isInt && v.is.Len() < encoding.SetMaxIntsetEntries() {
			return v.is.Add(n)
		}
		// A non-integer member, or one past the limit, leaves intset behind.
		if len(m) <= encoding.SetMaxListpackValue() &&
			v.is.Len() < encoding.SetMaxListpackEntries() {
			v.toListpack()
		} else {
			v.toHashSet()
		}
	}

	if v.lp != nil {
		if len(m) > encoding.SetMaxListpackValue() ||
			v.lp.Len() >= encoding.SetMaxListpackEntries() {
			v.toHashSet()
		} else {
			v.lp.Append([]byte(m))
			return true
		}
	}

	return v.s.Add(m) > 0
}

func (v *setView) Remove(members ...string) int {
	removed := 0
	for _, m := range members {
		switch {
		case v.s != nil:
			removed += v.s.Remove(m)
		case v.lp != nil:
			if i := v.lp.Find([]byte(m), 0, 1); i >= 0 {
				v.lp.DeleteRange(i, 1)
				removed++
			}
		default:
			if n, ok := intset.Parse(m); ok && v.is.Remove(n) {
				removed++
			}
		}
	}
	return removed
}

// Pop removes and returns up to count members.
func (v *setView) Pop(count int) []string {
	members := v.Members()
	if count > len(members) {
		count = len(members)
	}
	picked := members[:count]
	v.Remove(picked...)
	return picked
}

func (v *setView) toListpack() {
	lp := listpack.New()
	for _, m := range v.Members() {
		lp.Append([]byte(m))
	}
	v.is, v.lp, v.s = nil, lp, nil
	v.publish(lp, object.OBJ_ENCODING_LISTPACK)
}

func (v *setView) toHashSet() {
	s := set.New()
	s.Add(v.Members()...)
	v.is, v.lp, v.s = nil, nil, s
	v.publish(s, object.OBJ_ENCODING_HT)
}

func (v *setView) publish(ptr any, enc object.ObjectEncoding) {
	v.obj.Encoding = enc
	if v.ctx == nil || v.ctx.DB == nil {
		v.obj.Ptr = ptr
		return
	}
	_ = v.ctx.DB.SetKeepTTL(v.key, &object.Robj{
		Type:     object.OBJ_SET,
		Encoding: enc,
		Ptr:      ptr,
	})
}

func newSetView(ctx *Context, key string, obj *object.Robj) (*setView, []byte) {
	switch p := obj.Ptr.(type) {
	case *intset.IntSet:
		return &setView{ctx: ctx, key: key, obj: obj, is: p}, nil
	case *listpack.Listpack:
		return &setView{ctx: ctx, key: key, obj: obj, lp: p}, nil
	case *set.Set:
		return &setView{ctx: ctx, key: key, obj: obj, s: p}, nil
	case map[string]struct{}:
		// Legacy representation from an older snapshot.
		s := set.New()
		for m := range p {
			s.Add(m)
		}
		v := &setView{ctx: ctx, key: key, obj: obj, s: s}
		v.publish(s, object.OBJ_ENCODING_HT)
		return v, nil
	default:
		return nil, Error("internal set type error")
	}
}
