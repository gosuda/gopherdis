//go:build !(go1.27 && goexperiment.simd)

package parser

import "bytes"

// FindCRLF locates the index of "\r\n" in data using standard byte search.
func FindCRLF(data []byte) int {
	return bytes.Index(data, []byte("\r\n"))
}

// FindCR locates the index of '\r' in data.
func FindCR(data []byte) int {
	return bytes.IndexByte(data, '\r')
}

// ScanRESP3Prefix locates the next RESP3 type token using scalar byte scanning.
func ScanRESP3Prefix(data []byte) int {
	for i, b := range data {
		if b == '+' || b == '-' || b == ':' || b == '$' || b == '*' || b == '%' || b == '~' || b == '#' || b == ',' || b == '_' || b == '!' || b == '=' {
			return i
		}
	}
	return -1
}
