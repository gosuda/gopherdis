package parser

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strconv"

	"github.com/gosuda/beaver/pure"
)

var (
	ErrInvalidMultiBulkLength = errors.New("protocol error: invalid multibulk length")
	ErrInvalidBulkLength      = errors.New("protocol error: invalid bulk length")
	ErrUnexpectedEOF          = errors.New("protocol error: unexpected EOF")
)

// ParseRequest parses an incoming client command from a bufio.Reader.
// If arena is provided, byte slices are copied to the arena to minimize GC pressure.
func ParseRequest(r *bufio.Reader, a *pure.Arena) ([][]byte, error) {
	firstByte, err := r.Peek(1)
	if err != nil {
		return nil, err
	}

	if firstByte[0] == '*' {
		// Multi-bulk RESP format (e.g. *3\r\n$3\r\nSET\r\n$3\r\nkey\r\n$3\r\nval\r\n)
		return parseMultiBulkRequest(r, a)
	}

	// Inline command format (e.g. PING\r\n or GET foo\r\n)
	return parseInlineRequest(r, a)
}

func parseMultiBulkRequest(r *bufio.Reader, a *pure.Arena) ([][]byte, error) {
	line, err := readLine(r)
	if err != nil {
		return nil, err
	}
	if len(line) < 2 || line[0] != '*' {
		return nil, ErrInvalidMultiBulkLength
	}

	argc, err := strconv.ParseInt(string(line[1:]), 10, 64)
	if err != nil || argc <= 0 {
		return nil, ErrInvalidMultiBulkLength
	}

	argv := make([][]byte, 0, argc)

	for i := int64(0); i < argc; i++ {
		bLine, err := readLine(r)
		if err != nil {
			return nil, err
		}
		if len(bLine) < 2 || bLine[0] != '$' {
			return nil, ErrInvalidBulkLength
		}

		bulkLen, err := strconv.ParseInt(string(bLine[1:]), 10, 64)
		if err != nil || bulkLen < 0 {
			return nil, ErrInvalidBulkLength
		}

		buf := make([]byte, bulkLen+2) // bulk data + \r\n
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, ErrUnexpectedEOF
		}

		arg := buf[:bulkLen]
		if a != nil {
			allocBuf, err := a.Alloc(len(arg))
			if err == nil {
				copy(allocBuf, arg)
				arg = allocBuf
			}
		}
		argv = append(argv, arg)
	}

	return argv, nil
}

func parseInlineRequest(r *bufio.Reader, a *pure.Arena) ([][]byte, error) {
	line, err := readLine(r)
	if err != nil {
		return nil, err
	}

	fields := bytes.Fields(line)
	if len(fields) == 0 {
		return nil, nil
	}

	argv := make([][]byte, 0, len(fields))
	for _, f := range fields {
		arg := f
		if a != nil {
			allocBuf, err := a.Alloc(len(f))
			if err == nil {
				copy(allocBuf, f)
				arg = allocBuf
			}
		}
		argv = append(argv, arg)
	}
	return argv, nil
}

// readLine reads until CRLF and strips \r\n.
func readLine(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	n := len(line)
	if n >= 2 && line[n-2] == '\r' {
		return line[:n-2], nil
	}
	if n >= 1 && line[n-1] == '\n' {
		return line[:n-1], nil
	}
	return line, nil
}
