package reply

import (
	"fmt"

	"github.com/gosuda/gopherdis/parser"
)


// Reply type constants corresponding to RedisModule reply types.
const (
	REPLY_UNKNOWN         = -1
	REPLY_STRING          = 0
	REPLY_ERROR           = 1
	REPLY_INTEGER         = 2
	REPLY_ARRAY           = 3
	REPLY_NULL            = 4
	REPLY_MAP             = 5
	REPLY_SET             = 6
	REPLY_BOOL            = 7
	REPLY_DOUBLE          = 8
	REPLY_BIG_NUMBER      = 9
	REPLY_VERBATIM_STRING = 10
	REPLY_ATTRIBUTE       = 11
	REPLY_PROMISE         = 12
)

// Reply flag constants indicating parsing state and protocol version.
const (
	REPLY_FLAG_ROOT   = 1 << 0 // Root reply object
	REPLY_FLAG_PARSED = 1 << 1 // Successfully parsed
	REPLY_FLAG_RESP3  = 1 << 2 // Contains RESP3 types
)

// VerbatimString holds format (e.g. "txt", "markdown") and string payload.
type VerbatimString struct {
	Str    []byte
	Format []byte
}

// CallReply represents a parsed RESP reply node (tree structure for nested replies).
type CallReply struct {
	PrivateData any
	Proto       []byte         // Raw RESP protocol slice corresponding to this reply
	Type        int            // REPLY_* type
	Flags       int            // REPLY_FLAG_* bitmask
	Len         int64          // String length or element count for collection types
	Str         []byte         // String / error / big number payload
	VerbatimStr VerbatimString // Verbatim string format and payload
	IntVal      int64          // Integer or boolean value (1 / 0)
	DoubleVal   float64        // Floating point value
	Array       []*CallReply   // Sub-elements for array, set, and map (key-value pairs)
	Attribute   *CallReply     // Associated attribute reply if present
}

// CallReplyCallbacks binds ReplyParserCallbacks to CallReply population functions.
var CallReplyCallbacks = parser.ReplyParserCallbacks{
	NullArrayCallback:      callReplyNullArray,
	NullBulkStringCallback: callReplyNullBulkString,
	BulkStringCallback:     callReplyBulkString,
	ErrorCallback:          callReplyError,
	SimpleStrCallback:      callReplySimpleStr,
	LongCallback:           callReplyLong,
	ArrayCallback:          callReplyArray,
	SetCallback:            callReplySet,
	MapCallback:            callReplyMap,
	BoolCallback:           callReplyBool,
	DoubleCallback:         callReplyDouble,
	BigNumberCallback:      callReplyBigNumber,
	VerbatimStringCallback: callReplyVerbatimString,
	AttributeCallback:      callReplyAttribute,
	NullCallback:           callReplyNull,
}

// setSharedData sets common reply fields: type, raw protocol slice, and flags.
func setSharedData(rep *CallReply, replyType int, proto []byte, extraFlags int) {
	rep.Type = replyType
	rep.Proto = proto
	rep.Flags |= extraFlags
}

// callReplyNull handles RESP3 null replies ('_\r\n').
func callReplyNull(ctx any, proto []byte) {
	rep := ctx.(*CallReply)
	setSharedData(rep, REPLY_NULL, proto, REPLY_FLAG_RESP3)
}

// callReplyNullBulkString handles null bulk string replies ('$-1\r\n').
func callReplyNullBulkString(ctx any, proto []byte) {
	rep := ctx.(*CallReply)
	setSharedData(rep, REPLY_NULL, proto, 0)
}

// callReplyNullArray handles null array replies ('*-1\r\n').
func callReplyNullArray(ctx any, proto []byte) {
	rep := ctx.(*CallReply)
	setSharedData(rep, REPLY_NULL, proto, 0)
}

// callReplyBulkString handles bulk string replies ('$<len>\r\n<data>\r\n').
func callReplyBulkString(ctx any, str []byte, proto []byte) {
	rep := ctx.(*CallReply)
	setSharedData(rep, REPLY_STRING, proto, 0)
	rep.Len = int64(len(str))
	rep.Str = str
}

// callReplyError handles error replies ('-<err>\r\n').
func callReplyError(ctx any, str []byte, proto []byte) {
	rep := ctx.(*CallReply)
	setSharedData(rep, REPLY_ERROR, proto, 0)
	rep.Len = int64(len(str))
	rep.Str = str
}

// callReplySimpleStr handles simple string replies ('+<str>\r\n').
func callReplySimpleStr(ctx any, str []byte, proto []byte) {
	rep := ctx.(*CallReply)
	setSharedData(rep, REPLY_STRING, proto, 0)
	rep.Len = int64(len(str))
	rep.Str = str
}

// callReplyLong handles integer replies (':<val>\r\n').
func callReplyLong(ctx any, val int64, proto []byte) {
	rep := ctx.(*CallReply)
	setSharedData(rep, REPLY_INTEGER, proto, 0)
	rep.IntVal = val
}

// callReplyDouble handles RESP3 double replies (',<val>\r\n').
func callReplyDouble(ctx any, val float64, proto []byte) {
	rep := ctx.(*CallReply)
	setSharedData(rep, REPLY_DOUBLE, proto, REPLY_FLAG_RESP3)
	rep.DoubleVal = val
}

// callReplyVerbatimString handles RESP3 verbatim string replies ('=<len>\r\n<format>:<str>\r\n').
func callReplyVerbatimString(ctx any, format []byte, str []byte, proto []byte) {
	rep := ctx.(*CallReply)
	setSharedData(rep, REPLY_VERBATIM_STRING, proto, REPLY_FLAG_RESP3)
	rep.Len = int64(len(str))
	rep.VerbatimStr = VerbatimString{
		Str:    str,
		Format: format,
	}
}

// callReplyBigNumber handles RESP3 big number replies ('(<val>\r\n').
func callReplyBigNumber(ctx any, str []byte, proto []byte) {
	rep := ctx.(*CallReply)
	setSharedData(rep, REPLY_BIG_NUMBER, proto, REPLY_FLAG_RESP3)
	rep.Len = int64(len(str))
	rep.Str = str
}

// callReplyBool handles RESP3 boolean replies ('#t\r\n' / '#f\r\n').
func callReplyBool(ctx any, val bool, proto []byte) {
	rep := ctx.(*CallReply)
	setSharedData(rep, REPLY_BOOL, proto, REPLY_FLAG_RESP3)
	if val {
		rep.IntVal = 1
	} else {
		rep.IntVal = 0
	}
}

// parseCollection recursively parses collection elements (array, set, map).
func parseCollection(p *parser.ReplyParser, rep *CallReply, length int64, proto []byte, elementsPerEntry int) {
	rep.Len = length
	totalElements := int(length) * elementsPerEntry
	rep.Array = make([]*CallReply, totalElements)
	for i := 0; i < totalElements; i++ {
		elem := &CallReply{
			PrivateData: rep.PrivateData,
		}
		p.ParseReply(elem)
		elem.Flags |= REPLY_FLAG_PARSED
		if (elem.Flags & REPLY_FLAG_RESP3) != 0 {
			rep.Flags |= REPLY_FLAG_RESP3
		}
		rep.Array[i] = elem
	}
	protoLen := len(proto) - len(p.CurrLocation)
	rep.Proto = proto[:protoLen]
}

// callReplyArray handles array replies ('*<len>\r\n').
func callReplyArray(p *parser.ReplyParser, ctx any, length int64, proto []byte) {
	rep := ctx.(*CallReply)
	setSharedData(rep, REPLY_ARRAY, proto, 0)
	parseCollection(p, rep, length, proto, 1)
}

// callReplySet handles RESP3 set replies ('~<len>\r\n').
func callReplySet(p *parser.ReplyParser, ctx any, length int64, proto []byte) {
	rep := ctx.(*CallReply)
	setSharedData(rep, REPLY_SET, proto, REPLY_FLAG_RESP3)
	parseCollection(p, rep, length, proto, 1)
}

// callReplyMap handles RESP3 map replies ('%<len>\r\n').
func callReplyMap(p *parser.ReplyParser, ctx any, length int64, proto []byte) {
	rep := ctx.(*CallReply)
	setSharedData(rep, REPLY_MAP, proto, REPLY_FLAG_RESP3)
	parseCollection(p, rep, length, proto, 2)
}

// callReplyAttribute handles RESP3 attribute replies ('|<len>\r\n').
func callReplyAttribute(p *parser.ReplyParser, ctx any, length int64, proto []byte) {
	rep := ctx.(*CallReply)
	rep.Attribute = &CallReply{
		Len:         length,
		Type:        REPLY_ATTRIBUTE,
		PrivateData: rep.PrivateData,
		Flags:       REPLY_FLAG_PARSED | REPLY_FLAG_RESP3,
	}
	parseCollection(p, rep.Attribute, length, proto, 2)

	// Continue parsing the main reply following the attribute
	p.ParseReply(rep)
	protoLen := len(proto) - len(p.CurrLocation)
	rep.Proto = proto[:protoLen]
	rep.Flags |= REPLY_FLAG_RESP3
}

// CallReplyCreate creates and parses a CallReply hierarchy from raw RESP bytes.
func CallReplyCreate(raw []byte, privateData any) (*CallReply, error) {
	rep := &CallReply{
		PrivateData: privateData,
		Flags:       REPLY_FLAG_ROOT,
	}
	p := parser.ReplyParser{
		CurrLocation: raw,
		Callbacks:    CallReplyCallbacks,
	}
	if p.ParseReply(rep) != parser.C_OK {
		return nil, fmt.Errorf("failed to parse reply")
	}
	rep.Flags |= REPLY_FLAG_PARSED
	return rep, nil
}

// CallReplyType returns the type (REPLY_*) of the reply.
func CallReplyType(rep *CallReply) int {
	if rep == nil {
		return REPLY_UNKNOWN
	}
	return rep.Type
}

// CallReplyGetString returns the byte slice payload for string or error replies.
func CallReplyGetString(rep *CallReply) []byte {
	if rep == nil {
		return nil
	}
	return rep.Str
}

// CallReplyGetLongLong returns the integer value for integer or boolean replies.
func CallReplyGetLongLong(rep *CallReply) int64 {
	if rep == nil {
		return 0
	}
	return rep.IntVal
}

// CallReplyGetDouble returns the float64 value for double replies.
func CallReplyGetDouble(rep *CallReply) float64 {
	if rep == nil {
		return 0
	}
	return rep.DoubleVal
}

// CallReplyGetBool returns the boolean value for bool replies.
func CallReplyGetBool(rep *CallReply) bool {
	if rep == nil {
		return false
	}
	return rep.IntVal != 0
}

// CallReplyGetLen returns the length or element count of the reply.
func CallReplyGetLen(rep *CallReply) int64 {
	if rep == nil {
		return 0
	}
	return rep.Len
}

// CallReplyGetArrayElement returns the sub-reply element at index idx from an array or set.
func CallReplyGetArrayElement(rep *CallReply, idx int) *CallReply {
	if rep == nil || idx < 0 || idx >= len(rep.Array) {
		return nil
	}
	return rep.Array[idx]
}

// CallReplyGetSetElement returns the sub-reply element at index idx from a set.
func CallReplyGetSetElement(rep *CallReply, idx int) *CallReply {
	return CallReplyGetArrayElement(rep, idx)
}

// CallReplyGetMapElement returns the key and value sub-replies at index idx from a map.
func CallReplyGetMapElement(rep *CallReply, idx int) (key *CallReply, val *CallReply) {
	if rep == nil || idx < 0 || (idx*2+1) >= len(rep.Array) {
		return nil, nil
	}
	return rep.Array[idx*2], rep.Array[idx*2+1]
}

// CallReplyGetAttribute returns the associated attribute sub-reply if present.
func CallReplyGetAttribute(rep *CallReply) *CallReply {
	if rep == nil {
		return nil
	}
	return rep.Attribute
}

// CallReplyGetAttributeElement returns key and value at index idx from the reply's attribute map.
func CallReplyGetAttributeElement(rep *CallReply, idx int) (key *CallReply, val *CallReply) {
	if rep == nil || rep.Attribute == nil {
		return nil, nil
	}
	return CallReplyGetMapElement(rep.Attribute, idx)
}

// CallReplyGetBigNumber returns the byte slice of a big number reply.
func CallReplyGetBigNumber(rep *CallReply) []byte {
	if rep == nil {
		return nil
	}
	return rep.Str
}

// CallReplyGetVerbatim returns format and string byte slices of a verbatim string reply.
func CallReplyGetVerbatim(rep *CallReply) (format []byte, str []byte) {
	if rep == nil {
		return nil, nil
	}
	return rep.VerbatimStr.Format, rep.VerbatimStr.Str
}

// CallReplyGetProto returns the underlying raw RESP byte slice for this reply.
func CallReplyGetProto(rep *CallReply) []byte {
	if rep == nil {
		return nil
	}
	return rep.Proto
}

// CallReplyIsResp3 reports whether this reply contains RESP3 specific types.
func CallReplyIsResp3(rep *CallReply) bool {
	if rep == nil {
		return false
	}
	return (rep.Flags & REPLY_FLAG_RESP3) != 0
}

