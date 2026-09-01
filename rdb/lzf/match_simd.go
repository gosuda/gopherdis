//go:build go1.27 && goexperiment.simd

package lzf

import "simd"

// findMatchLen compares two byte slices using 16-byte SIMD vector comparison.
func findMatchLen(a, b []byte, maxLen int) int {
	matched := 0
	const vecSize = 16

	for matched+vecSize <= maxLen {
		vA := simd.LoadUint8Slice(a[matched : matched+vecSize])
		vB := simd.LoadUint8Slice(b[matched : matched+vecSize])

		eq := simd.Equal(vA, vB)
		mask := eq.ToBitMask()

		if mask != 0xFFFF { // If any byte in the 16-byte chunk mismatched
			firstDiff := simd.TrailingZeros64(^uint64(mask))
			return matched + firstDiff
		}
		matched += vecSize
	}

	// Remainder scalar comparison
	for matched < maxLen && a[matched] == b[matched] {
		matched++
	}
	return matched
}

// copyChunk copies n bytes from src to dst using 16-byte SIMD vectorized writes where non-overlapping.
func copyChunk(dst, src []byte, n int) {
	const vecSize = 16
	i := 0
	for ; i+vecSize <= n; i += vecSize {
		chunk := simd.LoadUint8Slice(src[i : i+vecSize])
		simd.StoreUint8Slice(dst[i:i+vecSize], chunk)
	}
	if i < n {
		copy(dst[i:n], src[i:n])
	}
}
