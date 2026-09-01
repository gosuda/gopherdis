package reply

import (
	"testing"
)

func TestParseBulkString(t *testing.T) {
	raw := []byte("$5\r\nhello\r\n")
	rep, err := CallReplyCreate(raw, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rep.Type != REPLY_STRING {
		t.Fatalf("expected REPLY_STRING, got %d", rep.Type)
	}
	if string(CallReplyGetString(rep)) != "hello" {
		t.Fatalf("expected 'hello', got '%s'", string(CallReplyGetString(rep)))
	}
}

func TestParseInteger(t *testing.T) {
	raw := []byte(":12345\r\n")
	rep, err := CallReplyCreate(raw, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rep.Type != REPLY_INTEGER {
		t.Fatalf("expected REPLY_INTEGER, got %d", rep.Type)
	}
	if CallReplyGetLongLong(rep) != 12345 {
		t.Fatalf("expected 12345, got %d", CallReplyGetLongLong(rep))
	}
}

func TestParseArray(t *testing.T) {
	raw := []byte("*2\r\n$3\r\nfoo\r\n$3\r\nbar\r\n")
	rep, err := CallReplyCreate(raw, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rep.Type != REPLY_ARRAY {
		t.Fatalf("expected REPLY_ARRAY, got %d", rep.Type)
	}
	if CallReplyGetLen(rep) != 2 {
		t.Fatalf("expected len 2, got %d", CallReplyGetLen(rep))
	}
	elem0 := CallReplyGetArrayElement(rep, 0)
	if string(CallReplyGetString(elem0)) != "foo" {
		t.Fatalf("expected 'foo', got '%s'", string(CallReplyGetString(elem0)))
	}
	elem1 := CallReplyGetArrayElement(rep, 1)
	if string(CallReplyGetString(elem1)) != "bar" {
		t.Fatalf("expected 'bar', got '%s'", string(CallReplyGetString(elem1)))
	}
}

func TestParseMap(t *testing.T) {
	raw := []byte("%1\r\n+key\r\n:100\r\n")
	rep, err := CallReplyCreate(raw, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rep.Type != REPLY_MAP {
		t.Fatalf("expected REPLY_MAP, got %d", rep.Type)
	}
	if !CallReplyIsResp3(rep) {
		t.Fatalf("expected RESP3 flag to be set")
	}
	k, v := CallReplyGetMapElement(rep, 0)
	if string(CallReplyGetString(k)) != "key" || CallReplyGetLongLong(v) != 100 {
		t.Fatalf("unexpected map key/val: %v, %v", string(CallReplyGetString(k)), CallReplyGetLongLong(v))
	}
}
