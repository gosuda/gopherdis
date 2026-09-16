package commands

import (
	"bytes"

	"github.com/gosuda/gopherdis/datastruct/listpack"
	"github.com/gosuda/gopherdis/datastruct/quicklist"
	"github.com/gosuda/gopherdis/encoding"
	"github.com/gosuda/gopherdis/object"
)

// listView is a list stored either as a listpack or as a quicklist.
//
// Short lists live in a single listpack buffer; past list-max-listpack-size
// entries, or with an element longer than 64 bytes, they become a quicklist.
// Conversions only move forward.
type listView struct {
	ctx *Context
	key string
	obj *object.Robj
	lp  *listpack.Listpack
	ql  *quicklist.Quicklist
}

// listMaxListpackValue is the element length above which a list leaves the
// compact encoding, matching Redis' packed entry limit.
const listMaxListpackValue = 64

func (l *listView) Len() int {
	if l.ql != nil {
		return l.ql.Len()
	}
	return l.lp.Len()
}

func (l *listView) All() [][]byte {
	if l.ql != nil {
		return l.ql.LRange(0, -1)
	}
	return l.lp.All()
}

func (l *listView) LRange(start, stop int) [][]byte {
	if l.ql != nil {
		return l.ql.LRange(start, stop)
	}
	all := l.lp.All()
	n := len(all)
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

func (l *listView) LIndex(i int) ([]byte, bool) {
	if l.ql != nil {
		return l.ql.LIndex(i)
	}
	n := l.lp.Len()
	if i < 0 {
		i += n
	}
	if i < 0 || i >= n {
		return nil, false
	}
	return l.lp.Get(i), true
}

func (l *listView) LSet(i int, val []byte) bool {
	if l.ql == nil && len(val) > listMaxListpackValue {
		l.promote()
	}
	if l.ql != nil {
		return l.ql.LSet(i, val)
	}
	n := l.lp.Len()
	if i < 0 {
		i += n
	}
	if i < 0 || i >= n {
		return false
	}
	return l.lp.ReplaceAt(i, val)
}

func (l *listView) LPush(val []byte) {
	if l.growWouldPromote(val) {
		l.promote()
	}
	if l.ql != nil {
		l.ql.LPush(val)
		return
	}
	l.lp.InsertAt(0, bytes.Clone(val))
}

func (l *listView) RPush(val []byte) {
	if l.growWouldPromote(val) {
		l.promote()
	}
	if l.ql != nil {
		l.ql.RPush(val)
		return
	}
	l.lp.Append(bytes.Clone(val))
}

func (l *listView) LPop() ([]byte, bool) {
	if l.ql != nil {
		return l.ql.LPop()
	}
	if l.lp.Len() == 0 {
		return nil, false
	}
	v := bytes.Clone(l.lp.Get(0))
	l.lp.DeleteRange(0, 1)
	return v, true
}

func (l *listView) RPop() ([]byte, bool) {
	if l.ql != nil {
		return l.ql.RPop()
	}
	n := l.lp.Len()
	if n == 0 {
		return nil, false
	}
	v := bytes.Clone(l.lp.Get(n - 1))
	l.lp.DeleteRange(n-1, 1)
	return v, true
}

func (l *listView) growWouldPromote(val []byte) bool {
	if l.ql != nil {
		return false
	}
	return len(val) > listMaxListpackValue || l.lp.Len() >= encoding.ListMaxListpackSize()
}

func (l *listView) promote() {
	ql := quicklist.NewQuicklist()
	for _, item := range l.lp.All() {
		ql.RPush(item)
	}
	l.ql = ql
	l.lp = nil
	l.publish(ql, object.OBJ_ENCODING_QUICKLIST)
}

func (l *listView) publish(ptr any, enc object.ObjectEncoding) {
	l.obj.Encoding = enc
	if l.ctx == nil || l.ctx.DB == nil {
		l.obj.Ptr = ptr
		return
	}
	_ = l.ctx.DB.SetKeepTTL(l.key, &object.Robj{
		Type:     object.OBJ_LIST,
		Encoding: enc,
		Ptr:      ptr,
	})
}

// replaceAll rewrites the list's contents, picking the encoding that fits.
func (l *listView) replaceAll(items [][]byte) {
	oversized := false
	for _, it := range items {
		if len(it) > listMaxListpackValue {
			oversized = true
			break
		}
	}
	if l.ql != nil || oversized || len(items) > encoding.ListMaxListpackSize() {
		ql := quicklist.NewQuicklist()
		for _, it := range items {
			ql.RPush(it)
		}
		l.ql, l.lp = ql, nil
		l.publish(ql, object.OBJ_ENCODING_QUICKLIST)
		return
	}
	lp := listpack.New()
	for _, it := range items {
		lp.Append(it)
	}
	l.lp, l.ql = lp, nil
	l.publish(lp, object.OBJ_ENCODING_LISTPACK)
}

func newListView(ctx *Context, key string, obj *object.Robj) (*listView, []byte) {
	switch p := obj.Ptr.(type) {
	case *listpack.Listpack:
		return &listView{ctx: ctx, key: key, obj: obj, lp: p}, nil
	case *quicklist.Quicklist:
		return &listView{ctx: ctx, key: key, obj: obj, ql: p}, nil
	default:
		return nil, Error("internal list type error")
	}
}
