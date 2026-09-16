package commands

import (
	"crypto/sha1"
	"encoding/hex"
	"sort"

	"github.com/gosuda/gopherdis/object"
)

// digestValue computes a stable digest of a key's logical contents.
//
// The suite uses DEBUG DIGEST-VALUE to compare two values without caring how
// they are stored, so the digest has to ignore encoding and, for unordered
// containers, ordering. Set and hash digests are built by sorting the element
// digests before mixing, which is the same property Redis gets by xoring them.
func digestValue(ctx *Context, key string) string {
	obj, ok := ctx.DB.Get(key)
	if !ok || obj == nil {
		// Redis reports an all-zero digest for a missing key.
		return "0000000000000000000000000000000000000000"
	}

	var parts []string
	unordered := false

	switch obj.Type {
	case object.OBJ_STRING:
		parts = []string{sha1hex(obj.Bytes())}

	case object.OBJ_LIST:
		l, errReply := newListView(ctx, key, obj)
		if errReply != nil {
			return ""
		}
		for _, item := range l.All() {
			parts = append(parts, sha1hex(item))
		}

	case object.OBJ_SET:
		v, errReply := newSetView(ctx, key, obj)
		if errReply != nil {
			return ""
		}
		for _, m := range v.Members() {
			parts = append(parts, sha1hex([]byte(m)))
		}
		unordered = true

	case object.OBJ_HASH:
		h, errReply := newHashView(ctx, key, obj)
		if errReply != nil {
			return ""
		}
		h.ForEach(func(f string, val []byte) {
			parts = append(parts, sha1hex([]byte(f+"="+string(val))))
		})
		unordered = true

	case object.OBJ_ZSET:
		z, errReply := newZsetView(ctx, key, obj)
		if errReply != nil {
			return ""
		}
		for _, el := range z.elements() {
			parts = append(parts, sha1hex([]byte(el.Member+":"+formatFloat(el.Score))))
		}

	default:
		parts = []string{sha1hex(obj.Bytes())}
	}

	if unordered {
		sort.Strings(parts)
	}

	h := sha1.New()
	for _, p := range parts {
		h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func sha1hex(b []byte) string {
	sum := sha1.Sum(b)
	return hex.EncodeToString(sum[:])
}
