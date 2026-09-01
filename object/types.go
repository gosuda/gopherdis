package object

// ObjectType represents the high-level logical data type in Redis.
type ObjectType uint8

// ObjectEncoding represents the physical memory representation format of an object.
type ObjectEncoding uint8

// Logical Redis object types corresponding to server.h definitions.
const (
	OBJ_STRING ObjectType = 0 // String object
	OBJ_LIST   ObjectType = 1 // List object
	OBJ_SET    ObjectType = 2 // Set object
	OBJ_ZSET   ObjectType = 3 // Sorted set object
	OBJ_HASH   ObjectType = 4 // Hash object
	OBJ_MODULE ObjectType = 5 // Module object
	OBJ_STREAM ObjectType = 6 // Stream object
	OBJ_ARRAY  ObjectType = 7 // Array object
)

// In-memory encodings corresponding to object.h definitions.
const (
	OBJ_ENCODING_RAW        ObjectEncoding = 0  // Raw string representation
	OBJ_ENCODING_INT        ObjectEncoding = 1  // Encoded as integer (int64)
	OBJ_ENCODING_HT         ObjectEncoding = 2  // Encoded as hash table / map
	OBJ_ENCODING_ZIPMAP     ObjectEncoding = 3  // Obsolete zipmap encoding
	OBJ_ENCODING_LINKEDLIST ObjectEncoding = 4  // Obsolete linked list encoding
	OBJ_ENCODING_ZIPLIST    ObjectEncoding = 5  // Obsolete ziplist encoding
	OBJ_ENCODING_INTSET     ObjectEncoding = 6  // Integer set encoding
	OBJ_ENCODING_SKIPLIST   ObjectEncoding = 7  // Skiplist sorted set encoding
	OBJ_ENCODING_EMBSTR     ObjectEncoding = 8  // Embedded string encoding
	OBJ_ENCODING_QUICKLIST  ObjectEncoding = 9  // Quicklist list encoding
	OBJ_ENCODING_STREAM     ObjectEncoding = 10 // Radix tree stream encoding
	OBJ_ENCODING_LISTPACK   ObjectEncoding = 11 // Listpack encoding
)

// Robj is the fundamental in-memory container holding Redis values.
// In Go, reference counting and lifetime management are handled by the GC,
// while Ptr stores the concrete payload (string, int64, map, slice, etc.).
type Robj struct {
	Type     ObjectType     // Logical data type (OBJ_STRING, OBJ_LIST, etc.)
	Encoding ObjectEncoding // Physical storage encoding (OBJ_ENCODING_RAW, OBJ_ENCODING_INT, etc.)
	Lru      uint32         // LRU timestamp or LFU frequency counter
	Ptr      any            // Underlying payload value
}
