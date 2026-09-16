package commands

import (
	"bytes"

	"github.com/gosuda/gopherdis/datastruct/dict"
	"github.com/gosuda/gopherdis/datastruct/listpack"
	"github.com/gosuda/gopherdis/encoding"
	"github.com/gosuda/gopherdis/object"
)

// hashView is a hash that may be stored either as a listpack or as a dict.
//
// Small hashes live in a listpack, which is one contiguous buffer holding
// field and value alternately, and are promoted to a dict once they exceed
// hash-max-listpack-entries or hold a value longer than
// hash-max-listpack-value. Promotion is one way, matching Redis: an encoding
// never converts back, because a hash that was once large is likely to be
// large again.
type hashView struct {
	ctx *Context
	key string
	obj *object.Robj
	lp  *listpack.Listpack
	d   *dict.Dict
}

func (h *hashView) Len() int {
	if h.d != nil {
		return h.d.Len()
	}
	return h.lp.Len() / 2
}

func (h *hashView) Get(field string) ([]byte, bool) {
	if h.d != nil {
		return h.d.Get(field)
	}
	i := h.lp.Find([]byte(field), 0, 2)
	if i < 0 {
		return nil, false
	}
	return h.lp.Get(i + 1), true
}

func (h *hashView) ForEach(fn func(field string, val []byte)) {
	if h.d != nil {
		h.d.ForEach(fn)
		return
	}
	var field string
	h.lp.ForEach(func(i int, v []byte) bool {
		if i%2 == 0 {
			field = string(v)
		} else {
			fn(field, v)
		}
		return true
	})
}

func (h *hashView) Keys() []string {
	if h.d != nil {
		return h.d.Keys()
	}
	out := make([]string, 0, h.lp.Len()/2)
	h.lp.ForEach(func(i int, v []byte) bool {
		if i%2 == 0 {
			out = append(out, string(v))
		}
		return true
	})
	return out
}

// Set stores field, reporting whether it is new.
func (h *hashView) Set(field string, val []byte) bool {
	if h.d == nil && h.shouldPromote(field, val) {
		h.promote()
	}
	if h.d != nil {
		return h.d.Set(field, val)
	}
	if i := h.lp.Find([]byte(field), 0, 2); i >= 0 {
		h.lp.ReplaceAt(i+1, val)
		return false
	}
	h.lp.Append([]byte(field))
	h.lp.Append(val)
	return true
}

func (h *hashView) Del(field string) bool {
	if h.d != nil {
		return h.d.Del(field)
	}
	i := h.lp.Find([]byte(field), 0, 2)
	if i < 0 {
		return false
	}
	h.lp.DeleteRange(i, 2)
	return true
}

// shouldPromote reports whether adding this field would push the listpack past
// either configured threshold.
func (h *hashView) shouldPromote(field string, val []byte) bool {
	if len(val) > encoding.HashMaxListpackValue() || len(field) > encoding.HashMaxListpackValue() {
		return true
	}
	if h.lp.Find([]byte(field), 0, 2) >= 0 {
		return false // replacing an existing field does not grow the entry count
	}
	return h.lp.Len()/2 >= encoding.HashMaxListpackEntries()
}

// promote converts the listpack to a dict and republishes the object under the
// shard lock, so a concurrent reader cannot observe a half-written Ptr.
func (h *hashView) promote() {
	d := dict.New()
	var field string
	h.lp.ForEach(func(i int, v []byte) bool {
		if i%2 == 0 {
			field = string(v)
		} else {
			d.Set(field, bytes.Clone(v))
		}
		return true
	})
	h.d = d
	h.lp = nil
	h.obj.Encoding = object.OBJ_ENCODING_HT
	h.publish(d, object.OBJ_ENCODING_HT)
}

func (h *hashView) publish(ptr any, enc object.ObjectEncoding) {
	if h.ctx == nil || h.ctx.DB == nil {
		h.obj.Ptr = ptr
		h.obj.Encoding = enc
		return
	}
	_ = h.ctx.DB.SetKeepTTL(h.key, &object.Robj{
		Type:     object.OBJ_HASH,
		Encoding: enc,
		Ptr:      ptr,
	})
}

// newHashView wraps whatever representation the object currently holds.
func newHashView(ctx *Context, key string, obj *object.Robj) (*hashView, []byte) {
	switch v := obj.Ptr.(type) {
	case *listpack.Listpack:
		return &hashView{ctx: ctx, key: key, obj: obj, lp: v}, nil
	case *dict.Dict:
		return &hashView{ctx: ctx, key: key, obj: obj, d: v}, nil
	case map[string][]byte:
		// Legacy representation from an older snapshot; fold it into a dict.
		d := dict.New()
		for f, val := range v {
			d.Set(f, val)
		}
		h := &hashView{ctx: ctx, key: key, obj: obj, d: d}
		h.publish(d, object.OBJ_ENCODING_HT)
		return h, nil
	default:
		return nil, Error("internal hash type error")
	}
}
