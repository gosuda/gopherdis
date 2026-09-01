package dict

import (
	"fmt"
	"testing"
)

func TestDict_Basic(t *testing.T) {
	d := New()
	if !d.Set("f1", []byte("v1")) {
		t.Fatal("expected new")
	}
	if d.Set("f1", []byte("v1_new")) {
		t.Fatal("expected overwrite")
	}
	if v, ok := d.Get("f1"); !ok || string(v) != "v1_new" {
		t.Fatalf("unexpected get: %s, %v", v, ok)
	}
	if d.Len() != 1 {
		t.Fatalf("unexpected len: %d", d.Len())
	}
	if !d.Del("f1") {
		t.Fatal("expected del")
	}
	if d.Len() != 0 {
		t.Fatalf("unexpected len: %d", d.Len())
	}
}

func TestDict_Promotion(t *testing.T) {
	d := New()
	for i := 0; i < 100; i++ {
		k := fmt.Sprintf("k%d", i)
		v := fmt.Sprintf("v%d", i)
		d.Set(k, []byte(v))
	}
	if d.Len() != 100 {
		t.Fatalf("unexpected len: %d", d.Len())
	}
	if d.table == nil {
		t.Fatal("expected promoted table")
	}
	for i := 0; i < 100; i++ {
		k := fmt.Sprintf("k%d", i)
		v := fmt.Sprintf("v%d", i)
		val, ok := d.Get(k)
		if !ok || string(val) != v {
			t.Fatalf("mismatch on key %s", k)
		}
	}
}
