package parser

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/gosuda/beaver/pure"
)

func TestParseMultiBulkRequest(t *testing.T) {
	raw := []byte("*3\r\n$3\r\nSET\r\n$3\r\nfoo\r\n$3\r\nbar\r\n")
	br := bufio.NewReader(bytes.NewReader(raw))
	arena := pure.New(1024)
	defer arena.Close()

	argv, err := ParseRequest(br, arena)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(argv) != 3 {
		t.Fatalf("expected 3 arguments, got %d", len(argv))
	}
	if string(argv[0]) != "SET" || string(argv[1]) != "foo" || string(argv[2]) != "bar" {
		t.Fatalf("argument mismatch: %q, %q, %q", argv[0], argv[1], argv[2])
	}
}

func TestParseInlineRequest(t *testing.T) {
	raw := []byte("PING hello\r\n")
	br := bufio.NewReader(bytes.NewReader(raw))
	arena := pure.New(1024)
	defer arena.Close()

	argv, err := ParseRequest(br, arena)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(argv) != 2 {
		t.Fatalf("expected 2 arguments, got %d", len(argv))
	}
	if string(argv[0]) != "PING" || string(argv[1]) != "hello" {
		t.Fatalf("argument mismatch: %q, %q", argv[0], argv[1])
	}
}

func TestParseRequest_Errors(t *testing.T) {
	// 1. Invalid multibulk length
	raw1 := []byte("*-5\r\n")
	_, err := ParseRequest(bufio.NewReader(bytes.NewReader(raw1)), nil)
	if err != ErrInvalidMultiBulkLength {
		t.Fatalf("expected ErrInvalidMultiBulkLength, got %v", err)
	}

	// 2. Invalid bulk length prefix
	raw2 := []byte("*1\r\n!3\r\nFOO\r\n")
	_, err = ParseRequest(bufio.NewReader(bytes.NewReader(raw2)), nil)
	if err != ErrInvalidBulkLength {
		t.Fatalf("expected ErrInvalidBulkLength, got %v", err)
	}

	// 3. Unexpected EOF in bulk payload
	raw3 := []byte("*1\r\n$10\r\n123")
	_, err = ParseRequest(bufio.NewReader(bytes.NewReader(raw3)), nil)
	if err != ErrUnexpectedEOF {
		t.Fatalf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestFindCR_And_CRLF(t *testing.T) {
	cases := []struct {
		input       string
		expectedCR  int
		expectedCRLF int
	}{
		{"", -1, -1},
		{"abc", -1, -1},
		{"\r", 0, -1},
		{"\r\n", 0, 0},
		{"hello\r\nworld", 5, 5},
		{"prefix_\r_not_lf\r\n", 7, 15},
	}

	for _, c := range cases {
		b := []byte(c.input)
		cr := FindCR(b)
		if cr != c.expectedCR {
			t.Errorf("FindCR(%q) = %d, expected %d", c.input, cr, c.expectedCR)
		}
		crlf := FindCRLF(b)
		if crlf != c.expectedCRLF {
			t.Errorf("FindCRLF(%q) = %d, expected %d", c.input, crlf, c.expectedCRLF)
		}
	}
}
