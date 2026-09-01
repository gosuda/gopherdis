//go:build !(go1.27 && goexperiment.simd)

package lzf

// findMatchLen compares two byte slices up to maxLen and returns the matching prefix length.
func findMatchLen(a, b []byte, maxLen int) int {
	matched := 0
	for matched < maxLen && a[matched] == b[matched] {
		matched++
	}
	return matched
}

// copyChunk copies n bytes from src to dst.
func copyChunk(dst, src []byte, n int) {
	copy(dst[:n], src[:n])
}
