//go:build go1.27 && goexperiment.simd

package parser

import "simd" // Go 1.27+ standard SIMD package under goexperiment.simd

// FindCRLF locates the index of "\r\n" using Go 1.27 SIMD vector operations.
func FindCRLF(data []byte) int {
	n := len(data)
	if n < 2 {
		return -1
	}

	// 16-byte vector chunk scan
	const vecSize = 16
	crVec := simd.BroadcastUint8('\r')
	lfVec := simd.BroadcastUint8('\n')

	i := 0
	for ; i+vecSize <= n-1; i += vecSize {
		chunk1 := simd.LoadUint8Slice(data[i : i+vecSize])
		chunk2 := simd.LoadUint8Slice(data[i+1 : i+1+vecSize])

		maskCR := simd.Equal(chunk1, crVec)
		maskLF := simd.Equal(chunk2, lfVec)
		matched := simd.And(maskCR, maskLF)

		if mask := matched.ToBitMask(); mask != 0 {
			firstBit := simd.TrailingZeros64(uint64(mask))
			return i + firstBit
		}
	}

	// Remainder scalar fallback
	for ; i < n-1; i++ {
		if data[i] == '\r' && data[i+1] == '\n' {
			return i
		}
	}
	return -1
}

// FindCR locates the index of '\r' using Go 1.27 SIMD vector operations.
func FindCR(data []byte) int {
	n := len(data)
	const vecSize = 16
	crVec := simd.BroadcastUint8('\r')

	i := 0
	for ; i+vecSize <= n; i += vecSize {
		chunk := simd.LoadUint8Slice(data[i : i+vecSize])
		maskCR := simd.Equal(chunk, crVec)
		if mask := maskCR.ToBitMask(); mask != 0 {
			firstBit := simd.TrailingZeros64(uint64(mask))
			return i + firstBit
		}
	}

	for ; i < n; i++ {
		if data[i] == '\r' {
			return i
		}
	}
	return -1
}

// ScanRESP3Prefix locates the next RESP3 type token (+, -, :, $, *, %, ~, #, ,, _, !, =) using SIMD vector lookup.
func ScanRESP3Prefix(data []byte) int {
	n := len(data)
	if n == 0 {
		return -1
	}

	const vecSize = 16
	i := 0
	for ; i+vecSize <= n; i += vecSize {
		chunk := simd.LoadUint8Slice(data[i : i+vecSize])
		m1 := simd.Or(simd.Equal(chunk, simd.BroadcastUint8('+')), simd.Equal(chunk, simd.BroadcastUint8('-')))
		m2 := simd.Or(simd.Equal(chunk, simd.BroadcastUint8(':')), simd.Equal(chunk, simd.BroadcastUint8('$')))
		m3 := simd.Or(simd.Equal(chunk, simd.BroadcastUint8('*')), simd.Equal(chunk, simd.BroadcastUint8('%')))
		m4 := simd.Or(simd.Equal(chunk, simd.BroadcastUint8('~')), simd.Equal(chunk, simd.BroadcastUint8('#')))
		m5 := simd.Or(simd.Equal(chunk, simd.BroadcastUint8(',')), simd.Equal(chunk, simd.BroadcastUint8('_')))
		mAll := simd.Or(simd.Or(m1, m2), simd.Or(m3, simd.Or(m4, m5)))

		if mask := mAll.ToBitMask(); mask != 0 {
			firstBit := simd.TrailingZeros64(uint64(mask))
			return i + firstBit
		}
	}

	for ; i < n; i++ {
		b := data[i]
		if b == '+' || b == '-' || b == ':' || b == '$' || b == '*' || b == '%' || b == '~' || b == '#' || b == ',' || b == '_' || b == '!' || b == '=' {
			return i
		}
	}
	return -1
}
