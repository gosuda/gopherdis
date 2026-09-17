package commands

import (
	"bytes"
	"math"
	"strconv"
)

// RESP3 replaced several RESP2 shapes with dedicated types. A flat array of
// alternating keys and values became a map, a bulk string holding a number
// became a double, and the two null forms collapsed into one. Clients that
// negotiated RESP3 with HELLO decode by type, so emitting the RESP2 shape to
// them is a protocol error rather than a cosmetic difference.
//
// Every helper here takes the connection context and falls back to the RESP2
// encoding when the client did not ask for RESP3.

// Null is the empty reply: "_" in RESP3, and the bulk string form in RESP2.
func Null(ctx *Context) []byte {
	if ctx.resp3() {
		return []byte("_\r\n")
	}
	return []byte("$-1\r\n")
}

// NullArrayReply is the empty reply where RESP2 used the array form.
func NullArrayReply(ctx *Context) []byte {
	if ctx.resp3() {
		return []byte("_\r\n")
	}
	return []byte("*-1\r\n")
}

// Bool is "#t"/"#f" in RESP3 and an integer in RESP2.
func Bool(ctx *Context, b bool) []byte {
	if ctx.resp3() {
		if b {
			return []byte("#t\r\n")
		}
		return []byte("#f\r\n")
	}
	if b {
		return []byte(":1\r\n")
	}
	return []byte(":0\r\n")
}

// Double is "," in RESP3 and a bulk string in RESP2.
func Double(ctx *Context, f float64) []byte {
	if !ctx.resp3() {
		return BulkString([]byte(formatFloat(f)))
	}
	switch {
	case math.IsInf(f, 1):
		return []byte(",inf\r\n")
	case math.IsInf(f, -1):
		return []byte(",-inf\r\n")
	case math.IsNaN(f):
		return []byte(",nan\r\n")
	}
	return []byte("," + formatFloat(f) + "\r\n")
}

// MapReply takes already-encoded elements in key, value order and emits a map
// in RESP3 or the flat array RESP2 expects.
func MapReply(ctx *Context, kv [][]byte) []byte {
	var buf bytes.Buffer
	if ctx.resp3() {
		buf.WriteByte('%')
		buf.WriteString(strconv.Itoa(len(kv) / 2))
	} else {
		buf.WriteByte('*')
		buf.WriteString(strconv.Itoa(len(kv)))
	}
	buf.WriteString("\r\n")
	for _, e := range kv {
		buf.Write(e)
	}
	return buf.Bytes()
}

// SetReply emits a set in RESP3 and an array in RESP2.
func SetReply(ctx *Context, elements [][]byte) []byte {
	var buf bytes.Buffer
	if ctx.resp3() {
		buf.WriteByte('~')
	} else {
		buf.WriteByte('*')
	}
	buf.WriteString(strconv.Itoa(len(elements)))
	buf.WriteString("\r\n")
	for _, e := range elements {
		buf.Write(e)
	}
	return buf.Bytes()
}

// PairsReply emits member/score style results. RESP2 flattens them into one
// array; RESP3 nests each pair, so a client can tell the members from the
// scores without knowing the command's arity.
func PairsReply(ctx *Context, pairs [][2][]byte) []byte {
	var buf bytes.Buffer
	if ctx.resp3() {
		buf.WriteByte('*')
		buf.WriteString(strconv.Itoa(len(pairs)))
		buf.WriteString("\r\n")
		for _, p := range pairs {
			buf.WriteString("*2\r\n")
			buf.Write(p[0])
			buf.Write(p[1])
		}
		return buf.Bytes()
	}
	buf.WriteByte('*')
	buf.WriteString(strconv.Itoa(len(pairs) * 2))
	buf.WriteString("\r\n")
	for _, p := range pairs {
		buf.Write(p[0])
		buf.Write(p[1])
	}
	return buf.Bytes()
}
