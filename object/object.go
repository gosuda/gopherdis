package object

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/gosuda/gopherdis/datastruct/quicklist"
	"github.com/gosuda/gopherdis/datastruct/skiplist"
)

// CreateObject creates a basic Robj with the given type and pointer payload.
func CreateObject(typ ObjectType, ptr any) *Robj {
	return &Robj{
		Type:     typ,
		Encoding: OBJ_ENCODING_RAW,
		Ptr:      ptr,
	}
}

// CreateRawStringObject creates a string object backed by a byte slice with RAW encoding.
func CreateRawStringObject(val []byte) *Robj {
	return &Robj{
		Type:     OBJ_STRING,
		Encoding: OBJ_ENCODING_RAW,
		Ptr:      bytes.Clone(val),
	}
}

// CreateStringObject creates a string object backed by an immutable Go string with EMBSTR encoding.
func CreateStringObject(val string) *Robj {
	return &Robj{
		Type:     OBJ_STRING,
		Encoding: OBJ_ENCODING_EMBSTR,
		Ptr:      val,
	}
}

// CreateStringObjectFromLongLong creates a string object encoded as an integer (OBJ_ENCODING_INT).
func CreateStringObjectFromLongLong(val int64) *Robj {
	return &Robj{
		Type:     OBJ_STRING,
		Encoding: OBJ_ENCODING_INT,
		Ptr:      val,
	}
}

// CreateListObject creates a new empty list object using Quicklist.
func CreateListObject() *Robj {
	return &Robj{
		Type:     OBJ_LIST,
		Encoding: OBJ_ENCODING_QUICKLIST,
		Ptr:      quicklist.NewQuicklist(),
	}
}

// CreateHashObject creates a new empty hash object.
func CreateHashObject() *Robj {
	return &Robj{
		Type:     OBJ_HASH,
		Encoding: OBJ_ENCODING_HT,
		Ptr:      make(map[string][]byte),
	}
}

// CreateSetObject creates a new empty set object.
func CreateSetObject() *Robj {
	return &Robj{
		Type:     OBJ_SET,
		Encoding: OBJ_ENCODING_HT,
		Ptr:      make(map[string]struct{}),
	}
}

// CreateZsetObject creates a new empty sorted set object using ZSet (Skiplist + Dict).
func CreateZsetObject() *Robj {
	return &Robj{
		Type:     OBJ_ZSET,
		Encoding: OBJ_ENCODING_SKIPLIST,
		Ptr:      skiplist.NewZSet(),
	}
}


// String returns a string representation of the object's payload.
func (o *Robj) String() string {
	if o == nil || o.Ptr == nil {
		return ""
	}
	switch v := o.Ptr.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// Bytes returns a byte slice representation of the object's payload.
func (o *Robj) Bytes() []byte {
	if o == nil || o.Ptr == nil {
		return nil
	}
	switch v := o.Ptr.(type) {
	case []byte:
		return v
	case string:
		return []byte(v)
	case int64:
		return []byte(strconv.FormatInt(v, 10))
	case int:
		return []byte(strconv.Itoa(v))
	case float64:
		return []byte(strconv.FormatFloat(v, 'f', -1, 64))
	default:
		return []byte(fmt.Sprintf("%v", v))
	}
}

// Int64 parses or extracts the integer value if the object is integer-encoded or represents a number.
func (o *Robj) Int64() (int64, error) {
	if o == nil || o.Ptr == nil {
		return 0, fmt.Errorf("nil object or pointer")
	}
	switch v := o.Ptr.(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case string:
		return strconv.ParseInt(v, 10, 64)
	case []byte:
		return strconv.ParseInt(string(v), 10, 64)
	default:
		return 0, fmt.Errorf("value is not an integer or integer string")
	}
}

// Float64 parses or extracts the floating point value of the object.
func (o *Robj) Float64() (float64, error) {
	if o == nil || o.Ptr == nil {
		return 0, fmt.Errorf("nil object or pointer")
	}
	switch v := o.Ptr.(type) {
	case float64:
		return v, nil
	case int64:
		return float64(v), nil
	case int:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(v, 64)
	case []byte:
		return strconv.ParseFloat(string(v), 64)
	default:
		return 0, fmt.Errorf("value is not a valid float")
	}
}

// TypeName returns the human-readable string name for the object type.
func (o *Robj) TypeName() string {
	if o == nil {
		return "none"
	}
	switch o.Type {
	case OBJ_STRING:
		return "string"
	case OBJ_LIST:
		return "list"
	case OBJ_SET:
		return "set"
	case OBJ_ZSET:
		return "zset"
	case OBJ_HASH:
		return "hash"
	case OBJ_MODULE:
		return "module"
	case OBJ_STREAM:
		return "stream"
	case OBJ_ARRAY:
		return "array"
	default:
		return "unknown"
	}
}

// EncodingName returns the human-readable string name for the object encoding.
func (o *Robj) EncodingName() string {
	if o == nil {
		return "none"
	}
	switch o.Encoding {
	case OBJ_ENCODING_RAW:
		return "raw"
	case OBJ_ENCODING_INT:
		return "int"
	case OBJ_ENCODING_HT:
		return "hashtable"
	case OBJ_ENCODING_ZIPMAP:
		return "zipmap"
	case OBJ_ENCODING_LINKEDLIST:
		return "linkedlist"
	case OBJ_ENCODING_ZIPLIST:
		return "ziplist"
	case OBJ_ENCODING_INTSET:
		return "intset"
	case OBJ_ENCODING_SKIPLIST:
		return "skiplist"
	case OBJ_ENCODING_EMBSTR:
		return "embstr"
	case OBJ_ENCODING_QUICKLIST:
		return "quicklist"
	case OBJ_ENCODING_STREAM:
		return "stream"
	case OBJ_ENCODING_LISTPACK:
		return "listpack"
	default:
		return "unknown"
	}
}
