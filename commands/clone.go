package commands

import (
	"bytes"

	"github.com/gosuda/gopherdis/datastruct/dict"
	"github.com/gosuda/gopherdis/datastruct/quicklist"
	"github.com/gosuda/gopherdis/datastruct/set"
	"github.com/gosuda/gopherdis/datastruct/skiplist"
	"github.com/gosuda/gopherdis/object"
)

// cloneContainer deep copies a container value so that COPY produces an
// independent key rather than a second name for the same payload.
//
// Streams are not cloned: their entries live in a shared arena, so handing the
// same arena to two keys would let a trim on one corrupt reads on the other.
func cloneContainer(obj *object.Robj) (*object.Robj, bool) {
	switch v := obj.Ptr.(type) {
	case *dict.Dict:
		d := dict.New()
		v.ForEach(func(f string, val []byte) {
			d.Set(f, bytes.Clone(val))
		})
		return &object.Robj{Type: obj.Type, Encoding: obj.Encoding, Ptr: d}, true

	case map[string][]byte:
		d := dict.New()
		for f, val := range v {
			d.Set(f, bytes.Clone(val))
		}
		return &object.Robj{Type: object.OBJ_HASH, Encoding: object.OBJ_ENCODING_HT, Ptr: d}, true

	case *quicklist.Quicklist:
		ql := quicklist.NewQuicklist()
		for _, item := range v.LRange(0, -1) {
			ql.RPush(bytes.Clone(item))
		}
		return &object.Robj{Type: obj.Type, Encoding: obj.Encoding, Ptr: ql}, true

	case *set.Set:
		s := set.New()
		s.Add(v.Members()...)
		return &object.Robj{Type: obj.Type, Encoding: obj.Encoding, Ptr: s}, true

	case map[string]struct{}:
		s := set.New()
		for m := range v {
			s.Add(m)
		}
		return &object.Robj{Type: object.OBJ_SET, Encoding: object.OBJ_ENCODING_HT, Ptr: s}, true

	case *skiplist.ZSet:
		zs := skiplist.NewZSet()
		for _, el := range v.Range(0, -1, false) {
			zs.Add(el.Member, el.Score)
		}
		return &object.Robj{Type: obj.Type, Encoding: obj.Encoding, Ptr: zs}, true

	default:
		return nil, false
	}
}
