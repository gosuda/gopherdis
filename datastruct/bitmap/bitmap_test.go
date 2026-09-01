package bitmap

import (
	"bytes"
	"testing"
)

func TestBitmap_SetAndGetBit(t *testing.T) {
	var buf []byte

	// Set bit 0 to 1 -> 1000 0000 = 0x80
	buf, old := SetBit(buf, 0, 1)
	if old != 0 || len(buf) != 1 || buf[0] != 0x80 {
		t.Fatalf("expected buf[0]=0x80, got %v", buf)
	}

	// Set bit 7 to 1 -> 1000 0001 = 0x81
	buf, old = SetBit(buf, 7, 1)
	if old != 0 || buf[0] != 0x81 {
		t.Fatalf("expected buf[0]=0x81, got %v", buf)
	}

	// GetBit
	if GetBit(buf, 0) != 1 || GetBit(buf, 1) != 0 || GetBit(buf, 7) != 1 {
		t.Fatalf("GetBit mismatch")
	}

	// Clear bit 0
	buf, old = SetBit(buf, 0, 0)
	if old != 1 || buf[0] != 0x01 {
		t.Fatalf("expected buf[0]=0x01 after clear, got %v", buf)
	}

	// Expand to offset 100 (byte 12, bit 4)
	buf, old = SetBit(buf, 100, 1)
	if old != 0 || len(buf) != 13 {
		t.Fatalf("expected len 13, got %d", len(buf))
	}
	if GetBit(buf, 100) != 1 || GetBit(buf, 99) != 0 {
		t.Fatalf("GetBit at offset 100 failed")
	}
}

func TestBitmap_BitCount(t *testing.T) {
	// 0xFF 0x0F = 8 + 4 = 12 bits
	buf := []byte{0xFF, 0x0F}
	if BitCount(buf, 0, -1) != 12 {
		t.Fatalf("expected 12, got %d", BitCount(buf, 0, -1))
	}

	// Range [0, 0] = 8 bits
	if BitCount(buf, 0, 0) != 8 {
		t.Fatalf("expected 8, got %d", BitCount(buf, 0, 0))
	}

	// Large 128-byte bitmap (1024 bits of 1s)
	large := bytes.Repeat([]byte{0xFF}, 128)
	if BitCount(large, 0, -1) != 1024 {
		t.Fatalf("expected 1024, got %d", BitCount(large, 0, -1))
	}
}

func TestBitmap_BitPos(t *testing.T) {
	// 0x00 0x00 0x10 -> byte 0: 00000000, byte 1: 00000000, byte 2: 00010000 (bit 19)
	buf := []byte{0x00, 0x00, 0x10}

	pos := BitPos(buf, 1, 0, -1, false)
	if pos != 19 {
		t.Fatalf("expected pos 19, got %d", pos)
	}

	// Finding 0 in all 1s
	allOnes := []byte{0xFF, 0xFE} // bit 15 is 0
	pos = BitPos(allOnes, 0, 0, -1, false)
	if pos != 15 {
		t.Fatalf("expected pos 15, got %d", pos)
	}
}

func TestBitmap_BitOp(t *testing.T) {
	k1 := []byte{0x0F} // 0000 1111
	k2 := []byte{0xF0} // 1111 0000

	andRes := BitOp("AND", [][]byte{k1, k2})
	if andRes[0] != 0x00 {
		t.Fatalf("AND mismatch: %x", andRes[0])
	}

	orRes := BitOp("OR", [][]byte{k1, k2})
	if orRes[0] != 0xFF {
		t.Fatalf("OR mismatch: %x", orRes[0])
	}

	xorRes := BitOp("XOR", [][]byte{k1, k2})
	if xorRes[0] != 0xFF {
		t.Fatalf("XOR mismatch: %x", xorRes[0])
	}

	notRes := BitOp("NOT", [][]byte{k1})
	if notRes[0] != 0xF0 {
		t.Fatalf("NOT mismatch: %x", notRes[0])
	}
}
