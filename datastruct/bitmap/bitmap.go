package bitmap

import (
	"encoding/binary"
	"math/bits"
	"strings"
)

// SetBit sets or clears the bit at offset in buf. Returns the modified slice and old bit value (0 or 1).
func SetBit(buf []byte, offset int64, val int) ([]byte, int) {
	if offset < 0 {
		return buf, 0
	}

	byteIdx := int(offset / 8)
	bitIdx := 7 - (offset % 8)

	// Expand buffer with zero padding if needed
	if byteIdx >= len(buf) {
		if byteIdx < cap(buf) {
			buf = buf[:byteIdx+1]
		} else {
			newCap := (byteIdx + 1) * 2
			if newCap < 64 {
				newCap = 64
			}
			newBuf := make([]byte, byteIdx+1, newCap)
			copy(newBuf, buf)
			buf = newBuf
		}
	}

	oldBit := int((buf[byteIdx] >> bitIdx) & 1)

	if val == 1 {
		buf[byteIdx] |= (1 << bitIdx)
	} else {
		buf[byteIdx] &= ^(1 << bitIdx)
	}

	return buf, oldBit
}

// GetBit retrieves the bit value at offset in buf. Returns 0 if offset is beyond buffer length.
func GetBit(buf []byte, offset int64) int {
	if offset < 0 {
		return 0
	}
	byteIdx := int(offset / 8)
	if byteIdx >= len(buf) {
		return 0
	}
	bitIdx := 7 - (offset % 8)
	return int((buf[byteIdx] >> bitIdx) & 1)
}

// BitCount counts the number of set bits (1s) in the range [start, end] bytes using hardware POPCNT.
func BitCount(buf []byte, start, end int) int64 {
	n := len(buf)
	if n == 0 {
		return 0
	}

	if start < 0 {
		start = n + start
	}
	if end < 0 {
		end = n + end
	}

	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 0
	}
	if start >= n || start > end {
		return 0
	}
	if end >= n {
		end = n - 1
	}

	sub := buf[start : end+1]
	var count int64
	i := 0
	subLen := len(sub)

	// 256-bit (4x uint64) unrolled vector POPCNT
	for ; i+32 <= subLen; i += 32 {
		w0 := binary.LittleEndian.Uint64(sub[i : i+8])
		w1 := binary.LittleEndian.Uint64(sub[i+8 : i+16])
		w2 := binary.LittleEndian.Uint64(sub[i+16 : i+24])
		w3 := binary.LittleEndian.Uint64(sub[i+24 : i+32])
		count += int64(bits.OnesCount64(w0) + bits.OnesCount64(w1) + bits.OnesCount64(w2) + bits.OnesCount64(w3))
	}

	// 64-bit word POPCNT
	for ; i+8 <= subLen; i += 8 {
		w := binary.LittleEndian.Uint64(sub[i : i+8])
		count += int64(bits.OnesCount64(w))
	}

	// Remainder bytes
	for ; i < subLen; i++ {
		count += int64(bits.OnesCount8(sub[i]))
	}

	return count
}

// BitPos finds the first bit matching bitVal (0 or 1) in the range [start, end] bytes.
func BitPos(buf []byte, bitVal int, start, end int, hasEnd bool) int64 {
	n := len(buf)

	if start < 0 {
		start = n + start
	}
	if end < 0 {
		end = n + end
	}
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 0
	}

	if start >= n {
		if bitVal == 0 && !hasEnd {
			return int64(n * 8)
		}
		return -1
	}

	if end >= n {
		end = n - 1
	}
	if start > end {
		return -1
	}

	sub := buf[start : end+1]
	subLen := len(sub)
	i := 0

	if bitVal == 1 {
		// 256-bit fast zero-word skip
		for ; i+32 <= subLen; i += 32 {
			w0 := binary.LittleEndian.Uint64(sub[i : i+8])
			w1 := binary.LittleEndian.Uint64(sub[i+8 : i+16])
			w2 := binary.LittleEndian.Uint64(sub[i+16 : i+24])
			w3 := binary.LittleEndian.Uint64(sub[i+24 : i+32])
			if (w0 | w1 | w2 | w3) != 0 {
				break
			}
		}

		// Fast skip 64-bit zero words
		for ; i+8 <= subLen; i += 8 {
			w := binary.LittleEndian.Uint64(sub[i : i+8])
			if w != 0 {
				break
			}
		}

		for ; i < subLen; i++ {
			b := sub[i]
			if b != 0 {
				leading := bits.LeadingZeros8(b)
				return int64((start+i)*8 + leading)
			}
		}
		return -1
	}

	// Finding 0 bit
	// 256-bit fast all-ones skip
	for ; i+32 <= subLen; i += 32 {
		w0 := binary.LittleEndian.Uint64(sub[i : i+8])
		w1 := binary.LittleEndian.Uint64(sub[i+8 : i+16])
		w2 := binary.LittleEndian.Uint64(sub[i+16 : i+24])
		w3 := binary.LittleEndian.Uint64(sub[i+24 : i+32])
		if (w0 & w1 & w2 & w3) != 0xFFFFFFFFFFFFFFFF {
			break
		}
	}

	// Fast skip 64-bit all-ones words
	for ; i+8 <= subLen; i += 8 {
		w := binary.LittleEndian.Uint64(sub[i : i+8])
		if w != 0xFFFFFFFFFFFFFFFF {
			break
		}
	}

	for ; i < subLen; i++ {
		b := sub[i]
		if b != 0xFF {
			leading := bits.LeadingZeros8(^b)
			return int64((start+i)*8 + leading)
		}
	}

	if !hasEnd {
		// Implicit trailing zero bit at the end of the string
		return int64(n * 8)
	}

	return -1
}

// BitOp performs bitwise operations (AND, OR, XOR, NOT) across multiple byte slices.
func BitOp(op string, srcs [][]byte) []byte {
	op = strings.ToUpper(op)
	if len(srcs) == 0 {
		return []byte{}
	}

	if op == "NOT" {
		src := srcs[0]
		dst := make([]byte, len(src))
		i := 0
		n := len(src)
		// 256-bit unroll
		for ; i+32 <= n; i += 32 {
			w0 := binary.LittleEndian.Uint64(src[i : i+8])
			w1 := binary.LittleEndian.Uint64(src[i+8 : i+16])
			w2 := binary.LittleEndian.Uint64(src[i+16 : i+24])
			w3 := binary.LittleEndian.Uint64(src[i+24 : i+32])
			binary.LittleEndian.PutUint64(dst[i:i+8], ^w0)
			binary.LittleEndian.PutUint64(dst[i+8:i+16], ^w1)
			binary.LittleEndian.PutUint64(dst[i+16:i+24], ^w2)
			binary.LittleEndian.PutUint64(dst[i+24:i+32], ^w3)
		}
		for ; i+8 <= n; i += 8 {
			w := binary.LittleEndian.Uint64(src[i : i+8])
			binary.LittleEndian.PutUint64(dst[i:i+8], ^w)
		}
		for ; i < n; i++ {
			dst[i] = ^src[i]
		}
		return dst
	}

	maxLen := 0
	for _, s := range srcs {
		if len(s) > maxLen {
			maxLen = len(s)
		}
	}
	if maxLen == 0 {
		return []byte{}
	}

	dst := make([]byte, maxLen)

	switch op {
	case "AND":
		copy(dst, srcs[0])
		// Pad to maxLen with 0 for remaining part
		for i := len(srcs[0]); i < maxLen; i++ {
			dst[i] = 0
		}
		for _, s := range srcs[1:] {
			sLen := len(s)
			i := 0
			for ; i+32 <= sLen; i += 32 {
				wDst0 := binary.LittleEndian.Uint64(dst[i : i+8])
				wSrc0 := binary.LittleEndian.Uint64(s[i : i+8])
				wDst1 := binary.LittleEndian.Uint64(dst[i+8 : i+16])
				wSrc1 := binary.LittleEndian.Uint64(s[i+8 : i+16])
				wDst2 := binary.LittleEndian.Uint64(dst[i+16 : i+24])
				wSrc2 := binary.LittleEndian.Uint64(s[i+16 : i+24])
				wDst3 := binary.LittleEndian.Uint64(dst[i+24 : i+32])
				wSrc3 := binary.LittleEndian.Uint64(s[i+24 : i+32])
				binary.LittleEndian.PutUint64(dst[i:i+8], wDst0&wSrc0)
				binary.LittleEndian.PutUint64(dst[i+8:i+16], wDst1&wSrc1)
				binary.LittleEndian.PutUint64(dst[i+16:i+24], wDst2&wSrc2)
				binary.LittleEndian.PutUint64(dst[i+24:i+32], wDst3&wSrc3)
			}
			for ; i+8 <= sLen; i += 8 {
				wDst := binary.LittleEndian.Uint64(dst[i : i+8])
				wSrc := binary.LittleEndian.Uint64(s[i : i+8])
				binary.LittleEndian.PutUint64(dst[i:i+8], wDst&wSrc)
			}
			for ; i < sLen; i++ {
				dst[i] &= s[i]
			}
			for ; i < maxLen; i++ {
				dst[i] = 0
			}
		}

	case "OR":
		for _, s := range srcs {
			sLen := len(s)
			i := 0
			for ; i+32 <= sLen; i += 32 {
				wDst0 := binary.LittleEndian.Uint64(dst[i : i+8])
				wSrc0 := binary.LittleEndian.Uint64(s[i : i+8])
				wDst1 := binary.LittleEndian.Uint64(dst[i+8 : i+16])
				wSrc1 := binary.LittleEndian.Uint64(s[i+8 : i+16])
				wDst2 := binary.LittleEndian.Uint64(dst[i+16 : i+24])
				wSrc2 := binary.LittleEndian.Uint64(s[i+16 : i+24])
				wDst3 := binary.LittleEndian.Uint64(dst[i+24 : i+32])
				wSrc3 := binary.LittleEndian.Uint64(s[i+24 : i+32])
				binary.LittleEndian.PutUint64(dst[i:i+8], wDst0|wSrc0)
				binary.LittleEndian.PutUint64(dst[i+8:i+16], wDst1|wSrc1)
				binary.LittleEndian.PutUint64(dst[i+16:i+24], wDst2|wSrc2)
				binary.LittleEndian.PutUint64(dst[i+24:i+32], wDst3|wSrc3)
			}
			for ; i+8 <= sLen; i += 8 {
				wDst := binary.LittleEndian.Uint64(dst[i : i+8])
				wSrc := binary.LittleEndian.Uint64(s[i : i+8])
				binary.LittleEndian.PutUint64(dst[i:i+8], wDst|wSrc)
			}
			for ; i < sLen; i++ {
				dst[i] |= s[i]
			}
		}

	case "XOR":
		for _, s := range srcs {
			sLen := len(s)
			i := 0
			for ; i+32 <= sLen; i += 32 {
				wDst0 := binary.LittleEndian.Uint64(dst[i : i+8])
				wSrc0 := binary.LittleEndian.Uint64(s[i : i+8])
				wDst1 := binary.LittleEndian.Uint64(dst[i+8 : i+16])
				wSrc1 := binary.LittleEndian.Uint64(s[i+8 : i+16])
				wDst2 := binary.LittleEndian.Uint64(dst[i+16 : i+24])
				wSrc2 := binary.LittleEndian.Uint64(s[i+16 : i+24])
				wDst3 := binary.LittleEndian.Uint64(dst[i+24 : i+32])
				wSrc3 := binary.LittleEndian.Uint64(s[i+24 : i+32])
				binary.LittleEndian.PutUint64(dst[i:i+8], wDst0^wSrc0)
				binary.LittleEndian.PutUint64(dst[i+8:i+16], wDst1^wSrc1)
				binary.LittleEndian.PutUint64(dst[i+16:i+24], wDst2^wSrc2)
				binary.LittleEndian.PutUint64(dst[i+24:i+32], wDst3^wSrc3)
			}
			for ; i+8 <= sLen; i += 8 {
				wDst := binary.LittleEndian.Uint64(dst[i : i+8])
				wSrc := binary.LittleEndian.Uint64(s[i : i+8])
				binary.LittleEndian.PutUint64(dst[i:i+8], wDst^wSrc)
			}
			for ; i < sLen; i++ {
				dst[i] ^= s[i]
			}
		}
	}

	return dst
}
