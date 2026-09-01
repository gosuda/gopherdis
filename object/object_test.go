package object

import (
	"testing"
)

func TestCreateStringObject(t *testing.T) {
	o := CreateStringObject("hello")
	if o.Type != OBJ_STRING {
		t.Fatalf("expected OBJ_STRING, got %d", o.Type)
	}
	if o.Encoding != OBJ_ENCODING_EMBSTR {
		t.Fatalf("expected OBJ_ENCODING_EMBSTR, got %d", o.Encoding)
	}
	if o.String() != "hello" {
		t.Fatalf("expected 'hello', got '%s'", o.String())
	}
	if o.TypeName() != "string" {
		t.Fatalf("expected 'string', got '%s'", o.TypeName())
	}
	if o.EncodingName() != "embstr" {
		t.Fatalf("expected 'embstr', got '%s'", o.EncodingName())
	}
}

func TestCreateStringObjectFromLongLong(t *testing.T) {
	o := CreateStringObjectFromLongLong(42)
	if o.Type != OBJ_STRING {
		t.Fatalf("expected OBJ_STRING, got %d", o.Type)
	}
	if o.Encoding != OBJ_ENCODING_INT {
		t.Fatalf("expected OBJ_ENCODING_INT, got %d", o.Encoding)
	}
	val, err := o.Int64()
	if err != nil || val != 42 {
		t.Fatalf("expected 42, got %d (err: %v)", val, err)
	}
	if o.String() != "42" {
		t.Fatalf("expected '42', got '%s'", o.String())
	}
}

func TestCreateCompositeObjects(t *testing.T) {
	hash := CreateHashObject()
	if hash.Type != OBJ_HASH || hash.TypeName() != "hash" {
		t.Fatalf("unexpected hash type: %v", hash.TypeName())
	}

	list := CreateListObject()
	if list.Type != OBJ_LIST || list.TypeName() != "list" {
		t.Fatalf("unexpected list type: %v", list.TypeName())
	}

	set := CreateSetObject()
	if set.Type != OBJ_SET || set.TypeName() != "set" {
		t.Fatalf("unexpected set type: %v", set.TypeName())
	}
}
