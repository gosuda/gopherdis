package hll

import (
	"encoding/binary"
	"math"
	"math/bits"
)

const (
	HLL_P             = 14                // 2^14 = 16384 registers
	HLL_Q             = 64 - HLL_P        // 50 bits
	HLL_REGISTERS     = 1 << HLL_P        // 16384
	HLL_BITS          = 6                 // 6 bits per register
	HLL_REGISTER_MAX  = (1 << HLL_BITS) - 1 // 63
	HLL_HDR_SIZE      = 16                // 16 bytes header
	HLL_DENSE_SIZE    = HLL_HDR_SIZE + ((HLL_REGISTERS * HLL_BITS) / 8) // 16 + 12288 = 12304 bytes
	HLL_ALPHA_INF     = 0.721347520444481703680
	HLL_HASH_SEED     = 0xadc83b19
)

// HLL represents a Redis-compatible Dense HyperLogLog instance.
type HLL struct {
	data []byte // 12304 bytes
}

// NewHLL initializes a fresh Dense HLL data structure with the standard "HYLL" header.
func NewHLL() *HLL {
	buf := make([]byte, HLL_DENSE_SIZE+2)
	// Header: "HYLL" + encoding 0 (Dense) + 3 zero padding + 8 bytes invalid cache (MSB set)
	copy(buf[0:4], "HYLL")
	buf[4] = 0    // Dense encoding
	buf[15] = 0x80 // Invalidate cardinality cache
	return &HLL{data: buf}
}

// FromBytes wraps an existing raw string byte slice into an HLL structure.
func FromBytes(buf []byte) *HLL {
	if len(buf) < HLL_DENSE_SIZE+2 {
		fresh := NewHLL()
		copy(fresh.data, buf)
		return fresh
	}
	return &HLL{data: buf}
}

// Bytes returns the raw underlying byte slice for storage in DB/RDB/AOF.
func (h *HLL) Bytes() []byte {
	if len(h.data) >= HLL_DENSE_SIZE {
		return h.data[:HLL_DENSE_SIZE]
	}
	return h.data
}

// MurmurHash64A computes 64-bit MurmurHash2 matching Redis's implementation.
func MurmurHash64A(key []byte, seed uint32) uint64 {
	const m = uint64(0xc6a4a7935bd1e995)
	const r = 47

	length := len(key)
	h := uint64(seed) ^ (uint64(length) * m)

	nblocks := length / 8
	for i := 0; i < nblocks; i++ {
		k := binary.LittleEndian.Uint64(key[i*8 : i*8+8])
		k *= m
		k ^= k >> r
		k *= m
		h ^= k
		h *= m
	}

	tail := key[nblocks*8:]
	switch len(tail) {
	case 7:
		h ^= uint64(tail[6]) << 48
		fallthrough
	case 6:
		h ^= uint64(tail[5]) << 40
		fallthrough
	case 5:
		h ^= uint64(tail[4]) << 32
		fallthrough
	case 4:
		h ^= uint64(tail[3]) << 24
		fallthrough
	case 3:
		h ^= uint64(tail[2]) << 16
		fallthrough
	case 2:
		h ^= uint64(tail[1]) << 8
		fallthrough
	case 1:
		h ^= uint64(tail[0])
		h *= m
	}

	h ^= h >> r
	h *= m
	h ^= h >> r
	return h
}

// GetRegister retrieves the 6-bit register value at index regnum (0..16383).
func (h *HLL) GetRegister(regnum int) uint8 {
	byteOffset := HLL_HDR_SIZE + (regnum * HLL_BITS / 8)
	fb := uint(regnum * HLL_BITS & 7)
	fb8 := 8 - fb

	b0 := uint32(h.data[byteOffset])
	var b1 uint32 = 0
	if byteOffset+1 < len(h.data) {
		b1 = uint32(h.data[byteOffset+1])
	}

	return uint8(((b0 >> fb) | (b1 << fb8)) & HLL_REGISTER_MAX)
}

// SetRegister stores a 6-bit value into register regnum (0..16383).
func (h *HLL) SetRegister(regnum int, val uint8) {
	byteOffset := HLL_HDR_SIZE + (regnum * HLL_BITS / 8)
	fb := uint(regnum * HLL_BITS & 7)
	fb8 := 8 - fb
	v := uint32(val & HLL_REGISTER_MAX)

	h.data[byteOffset] &= ^byte(HLL_REGISTER_MAX << fb)
	h.data[byteOffset] |= byte(v << fb)

	if byteOffset+1 < len(h.data) {
		h.data[byteOffset+1] &= ^byte(HLL_REGISTER_MAX >> fb8)
		h.data[byteOffset+1] |= byte(v >> fb8)
	}

	// Invalidate cache
	h.data[15] |= 0x80
}

// Add adds an element to the HyperLogLog. Returns true if cardinality approximated count was updated.
func (h *HLL) Add(elem []byte) bool {
	hash := MurmurHash64A(elem, HLL_HASH_SEED)
	index := int(hash & (HLL_REGISTERS - 1)) // 14-bit index
	hash >>= HLL_P
	hash |= (uint64(1) << HLL_Q)

	count := uint8(bits.TrailingZeros64(hash) + 1)

	// Inlined GetRegister & SetRegister fast path
	bitOffset := index * HLL_BITS
	byteOffset := HLL_HDR_SIZE + (bitOffset >> 3)
	fb := uint(bitOffset & 7)

	w := binary.LittleEndian.Uint16(h.data[byteOffset : byteOffset+2])
	oldCount := uint8((w >> fb) & HLL_REGISTER_MAX)

	if count > oldCount {
		// Update 6-bit register inside 16-bit word
		w &= ^uint16(HLL_REGISTER_MAX << fb)
		w |= uint16(uint16(count&HLL_REGISTER_MAX) << fb)
		binary.LittleEndian.PutUint16(h.data[byteOffset:byteOffset+2], w)

		// Invalidate cache
		h.data[15] |= 0x80
		return true
	}
	return false
}

// Sigma helper function (Otmar Ertl).
func hllSigma(x double) double {
	if x == 1.0 {
		return math.Inf(1)
	}
	var zPrime double
	var y double = 1.0
	z := x
	for {
		x *= x
		zPrime = z
		z += x * y
		y += y
		if zPrime == z {
			break
		}
	}
	return z
}

// Tau helper function (Otmar Ertl).
func hllTau(x double) double {
	if x == 0.0 || x == 1.0 {
		return 0.0
	}
	var zPrime double
	var y double = 1.0
	z := 1.0 - x
	for {
		x = math.Sqrt(x)
		zPrime = z
		y *= 0.5
		z -= math.Pow(1.0-x, 2) * y
		if zPrime == z {
			break
		}
	}
	return z / 3.0
}

type double = float64

// Count returns the estimated cardinality of the HyperLogLog set.
func (h *HLL) Count() uint64 {
	// Check cached cardinality
	if (h.data[15] & 0x80) == 0 {
		return binary.LittleEndian.Uint64(h.data[8:16])
	}

	var reghisto [64]int
	// Process 8 6-bit registers (6 bytes) at a time across all 16384 registers (2048 iterations)
	data := h.data[HLL_HDR_SIZE:]
	for byteIdx := 0; byteIdx < 12288; byteIdx += 6 {
		b0 := uint32(data[byteIdx])
		b1 := uint32(data[byteIdx+1])
		b2 := uint32(data[byteIdx+2])
		b3 := uint32(data[byteIdx+3])
		b4 := uint32(data[byteIdx+4])
		b5 := uint32(data[byteIdx+5])

		reghisto[b0&0x3F]++
		reghisto[((b0>>6)|(b1<<2))&0x3F]++
		reghisto[((b1>>4)|(b2<<4))&0x3F]++
		reghisto[b2>>2&0x3F]++
		reghisto[b3&0x3F]++
		reghisto[((b3>>6)|(b4<<2))&0x3F]++
		reghisto[((b4>>4)|(b5<<4))&0x3F]++
		reghisto[b5>>2&0x3F]++
	}

	m := double(HLL_REGISTERS)
	z := m * hllTau((m-double(reghisto[HLL_Q+1]))/m)
	for j := HLL_Q; j >= 1; j-- {
		z += double(reghisto[j])
		z *= 0.5
	}
	z += m * hllSigma(double(reghisto[0])/m)
	est := uint64(math.Round(HLL_ALPHA_INF * m * m / z))

	// Cache cardinality
	binary.LittleEndian.PutUint64(h.data[8:16], est)
	h.data[15] &= 0x7F // Clear MSB to validate cache

	return est
}

// Merge combines another HLL into this instance taking maximum register values.
func (h *HLL) Merge(other *HLL) {
	for i := 0; i < HLL_REGISTERS; i++ {
		rA := h.GetRegister(i)
		rB := other.GetRegister(i)
		if rB > rA {
			h.SetRegister(i, rB)
		}
	}
	h.data[15] |= 0x80
}
