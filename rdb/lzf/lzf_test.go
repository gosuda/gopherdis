package lzf

import (
	"bytes"
	"strings"
	"testing"
)

func TestLZF_Roundtrip(t *testing.T) {
	testCases := []string{
		strings.Repeat("Hello world! Redis RDB LZF Compression test string. ", 10),
		strings.Repeat("ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", 50),
		"This is a relatively short string that should compress well because this is a relatively short string.",
	}

	for i, original := range testCases {
		data := []byte(original)
		compressed, ok := Compress(data)
		if !ok {
			t.Fatalf("case %d: expected compression to succeed", i)
		}
		if len(compressed) >= len(data) {
			t.Fatalf("case %d: compressed size (%d) not smaller than original (%d)", i, len(compressed), len(data))
		}

		decompressed, err := Decompress(compressed, len(data))
		if err != nil {
			t.Fatalf("case %d: decompress error: %v", i, err)
		}

		if !bytes.Equal(data, decompressed) {
			t.Fatalf("case %d: decompressed content mismatch", i)
		}
	}
}

func TestLZF_ShortString(t *testing.T) {
	short := []byte("short")
	_, ok := Compress(short)
	if ok {
		t.Fatalf("expected short string (< 20 bytes) to skip compression")
	}
}

func TestLZF_CorruptedData(t *testing.T) {
	corrupted := []byte{0x20, 0x01} // invalid match back-ref
	_, err := Decompress(corrupted, 100)
	if err == nil {
		t.Fatalf("expected error on corrupted data")
	}
}
