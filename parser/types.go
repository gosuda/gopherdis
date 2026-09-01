package parser

// Common parser return status codes.
const (
	C_OK  = 0
	C_ERR = -1
)

// ReplyParserCallbacks defines callback functions called by ReplyParser

// for each RESP reply type encountered.
type ReplyParserCallbacks struct {
	// NullArrayCallback is called when an empty/null multi-bulk array ('*-1\r\n') is parsed.
	NullArrayCallback func(ctx any, proto []byte)

	// NullBulkStringCallback is called when a null bulk string ('$-1\r\n') is parsed.
	NullBulkStringCallback func(ctx any, proto []byte)

	// BulkStringCallback is called when a bulk string ('$<len>\r\n<str>\r\n') is parsed.
	BulkStringCallback func(ctx any, str []byte, proto []byte)

	// ErrorCallback is called when an error reply ('-<str>\r\n') is parsed.
	ErrorCallback func(ctx any, str []byte, proto []byte)

	// SimpleStrCallback is called when a simple string ('+<str>\r\n') is parsed.
	SimpleStrCallback func(ctx any, str []byte, proto []byte)

	// LongCallback is called when an integer reply (':<val>\r\n') is parsed.
	LongCallback func(ctx any, val int64, proto []byte)

	// ArrayCallback is called when an array reply ('*<len>\r\n') is parsed.
	// Nested elements must be parsed via parser.ParseReply.
	ArrayCallback func(parser *ReplyParser, ctx any, length int64, proto []byte)

	// SetCallback is called when a set reply ('~<len>\r\n') is parsed.
	// Nested elements must be parsed via parser.ParseReply.
	SetCallback func(parser *ReplyParser, ctx any, length int64, proto []byte)

	// MapCallback is called when a map reply ('%<len>\r\n') is parsed.
	// Nested key-value pairs (len * 2 items) must be parsed via parser.ParseReply.
	MapCallback func(parser *ReplyParser, ctx any, length int64, proto []byte)

	// BoolCallback is called when a boolean reply ('#t\r\n' or '#f\r\n') is parsed.
	BoolCallback func(ctx any, val bool, proto []byte)

	// DoubleCallback is called when a double/float reply (',<val>\r\n') is parsed.
	DoubleCallback func(ctx any, val float64, proto []byte)

	// BigNumberCallback is called when a big number reply ('(<str>\r\n') is parsed.
	BigNumberCallback func(ctx any, str []byte, proto []byte)

	// VerbatimStringCallback is called when a verbatim string ('=<len>\r\n<format>:<str>\r\n') is parsed.
	VerbatimStringCallback func(ctx any, format []byte, str []byte, proto []byte)

	// AttributeCallback is called when an attribute reply ('|<len>\r\n') is parsed.
	AttributeCallback func(parser *ReplyParser, ctx any, length int64, proto []byte)

	// NullCallback is called when a RESP3 null reply ('_\r\n') is parsed.
	NullCallback func(ctx any, proto []byte)

	// Error is called when a syntax or parsing error occurs.
	Error func(ctx any)
}

// ReplyParser tracks the current byte slice position and callbacks for RESP parsing.
type ReplyParser struct {
	// CurrLocation points to the remaining unparsed RESP byte slice.
	CurrLocation []byte
	// Callbacks contains handler functions for each RESP type.
	Callbacks ReplyParserCallbacks
}


