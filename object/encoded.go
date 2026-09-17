package object

import (
	"github.com/gosuda/gopherdis/datastruct/dict"
	"github.com/gosuda/gopherdis/datastruct/intset"
	"github.com/gosuda/gopherdis/datastruct/listpack"
	"github.com/gosuda/gopherdis/datastruct/quicklist"
	"github.com/gosuda/gopherdis/datastruct/set"
	"github.com/gosuda/gopherdis/datastruct/skiplist"
	"github.com/gosuda/gopherdis/encoding"
)

// The constructors here pick the physical encoding from the contents, the same
// way the command handlers do when a collection is built one element at a time.
//
// Anything that materialises a whole collection at once, such as loading a
// snapshot, has to go through them. Building a hash table directly would make a
// value come back from RDB or DEBUG RELOAD reporting a different OBJECT
// ENCODING than it had when it was written.

// NewSetFrom builds a set with the encoding its members call for.
func NewSetFrom(members []string) *Robj {
	allInts := true
	maxLen := 0
	for _, m := range members {
		if _, ok := intset.Parse(m); !ok {
			allInts = false
		}
		if len(m) > maxLen {
			maxLen = len(m)
		}
	}

	if allInts && len(members) <= encoding.SetMaxIntsetEntries() {
		is := intset.New()
		for _, m := range members {
			n, _ := intset.Parse(m)
			is.Add(n)
		}
		return &Robj{Type: OBJ_SET, Encoding: OBJ_ENCODING_INTSET, Ptr: is}
	}
	if len(members) <= encoding.SetMaxListpackEntries() && maxLen <= encoding.SetMaxListpackValue() {
		lp := listpack.New()
		for _, m := range members {
			lp.Append([]byte(m))
		}
		return &Robj{Type: OBJ_SET, Encoding: OBJ_ENCODING_LISTPACK, Ptr: lp}
	}
	s := set.New()
	s.Add(members...)
	return &Robj{Type: OBJ_SET, Encoding: OBJ_ENCODING_HT, Ptr: s}
}

// NewHashFrom builds a hash from alternating field and value entries.
func NewHashFrom(entries [][]byte) *Robj {
	maxLen := 0
	for _, e := range entries {
		if len(e) > maxLen {
			maxLen = len(e)
		}
	}
	if len(entries)/2 <= encoding.HashMaxListpackEntries() && maxLen <= encoding.HashMaxListpackValue() {
		lp := listpack.New()
		for _, e := range entries {
			lp.Append(e)
		}
		return &Robj{Type: OBJ_HASH, Encoding: OBJ_ENCODING_LISTPACK, Ptr: lp}
	}
	d := dict.New()
	for i := 0; i+1 < len(entries); i += 2 {
		d.Set(string(entries[i]), entries[i+1])
	}
	return &Robj{Type: OBJ_HASH, Encoding: OBJ_ENCODING_HT, Ptr: d}
}

// ZSetEntry is one member and score, for NewZSetFrom.
type ZSetEntry struct {
	Member string
	Score  float64
}

// NewZSetFrom builds a sorted set. Entries must already be in score order,
// which is how both the RDB layout and the listpack encoding store them.
func NewZSetFrom(entries []ZSetEntry, formatScore func(float64) string) *Robj {
	maxLen := 0
	for _, e := range entries {
		if len(e.Member) > maxLen {
			maxLen = len(e.Member)
		}
	}
	if len(entries) <= encoding.ZsetMaxListpackEntries() && maxLen <= encoding.ZsetMaxListpackValue() {
		lp := listpack.New()
		for _, e := range entries {
			lp.Append([]byte(e.Member))
			lp.Append([]byte(formatScore(e.Score)))
		}
		return &Robj{Type: OBJ_ZSET, Encoding: OBJ_ENCODING_LISTPACK, Ptr: lp}
	}
	zs := skiplist.NewZSet()
	for _, e := range entries {
		zs.Add(e.Member, e.Score)
	}
	return &Robj{Type: OBJ_ZSET, Encoding: OBJ_ENCODING_SKIPLIST, Ptr: zs}
}

// NewListFrom builds a list with the encoding its elements call for.
func NewListFrom(items [][]byte) *Robj {
	total, maxLen := 0, 0
	for _, it := range items {
		total += len(it) + 2
		if len(it) > maxLen {
			maxLen = len(it)
		}
	}

	compact := maxLen <= 64
	if limit := encoding.ListMaxListpackSize(); limit >= 0 {
		compact = compact && len(items) <= limit
	} else {
		budget := 8 * 1024
		switch limit {
		case -1:
			budget = 4 * 1024
		case -3:
			budget = 16 * 1024
		case -4:
			budget = 32 * 1024
		case -5:
			budget = 64 * 1024
		}
		compact = compact && total <= budget
	}

	if compact {
		lp := listpack.New()
		for _, it := range items {
			lp.Append(it)
		}
		return &Robj{Type: OBJ_LIST, Encoding: OBJ_ENCODING_LISTPACK, Ptr: lp}
	}
	ql := quicklist.NewQuicklist()
	for _, it := range items {
		ql.RPush(it)
	}
	return &Robj{Type: OBJ_LIST, Encoding: OBJ_ENCODING_QUICKLIST, Ptr: ql}
}
