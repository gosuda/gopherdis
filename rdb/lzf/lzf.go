package lzf

import (
	"errors"
)

const (
	hLog   = 14
	hSize  = 1 << hLog
	maxOff = 1 << 13 // 8192
	maxRef = 255 + 7 + 2 // 264 max match length
	minLen = 20
)

var (
	ErrCorruptedData = errors.New("lzf: corrupted compressed data")
)

// Compress compresses input using standard Redis LZF algorithm.
func Compress(in []byte) ([]byte, bool) {
	inLen := len(in)
	if inLen < minLen {
		return nil, false
	}

	var htab [hSize]int
	for i := range htab {
		htab[i] = -1
	}

	out := make([]byte, inLen+32)
	ip := 0
	op := 1 // reserve for first lit control byte
	lit := 0

	hash3 := func(p int) int {
		v := (uint32(in[p]) << 16) | (uint32(in[p+1]) << 8) | uint32(in[p+2])
		return int((v * 16777619) >> (32 - hLog)) & (hSize - 1)
	}

	for ip < inLen-2 {
		h := hash3(ip)
		ref := htab[h]
		htab[h] = ip

		off := ip - ref - 1
		if ref >= 0 && off < maxOff && in[ref] == in[ip] && in[ref+1] == in[ip+1] && in[ref+2] == in[ip+2] {
			maxMatch := inLen - ip
			if maxMatch > maxRef {
				maxMatch = maxRef
			}
			matchLen := 3 + findMatchLen(in[ip+3:], in[ref+3:], maxMatch-3)

			// Finalize literal run before match
			if lit > 0 {
				out[op-lit-1] = byte(lit - 1)
			} else {
				op-- // undo reserved literal control byte
			}

			mLen := matchLen - 2 // mLen is in [1..262]
			if mLen < 7 {
				out[op] = byte((off >> 8) | (mLen << 5))
				op++
			} else {
				out[op] = byte((off >> 8) | (7 << 5))
				op++
				out[op] = byte(mLen - 7)
				op++
			}
			out[op] = byte(off & 0xFF)
			op++

			lit = 0
			op++ // reserve for next literal control byte

			ip += matchLen

			if ip >= inLen-2 {
				break
			}
		} else {
			lit++
			out[op] = in[ip]
			op++
			ip++

			if lit == 32 {
				out[op-32-1] = 31 // 32 literals
				lit = 0
				op++ // reserve for next literal control byte
			}
		}

		if op >= inLen {
			return nil, false
		}
	}

	// Copy remaining trailing bytes as literals
	for ip < inLen {
		lit++
		out[op] = in[ip]
		op++
		ip++

		if lit == 32 {
			out[op-32-1] = 31
			lit = 0
			op++
		}
	}

	if lit > 0 {
		out[op-lit-1] = byte(lit - 1)
	} else {
		op--
	}

	if op >= inLen {
		return nil, false
	}

	return out[:op], true
}

// Decompress decompresses LZF compressed data into an expected output length.
func Decompress(in []byte, expectedLen int) ([]byte, error) {
	out := make([]byte, expectedLen)
	ip := 0
	op := 0
	inLen := len(in)

	for ip < inLen {
		ctrl := int(in[ip])
		ip++

		if ctrl < (1 << 5) {
			// Literal run: ctrl + 1 bytes
			litLen := ctrl + 1
			if ip+litLen > inLen || op+litLen > expectedLen {
				return nil, ErrCorruptedData
			}
			copyChunk(out[op:], in[ip:ip+litLen], litLen)
			ip += litLen
			op += litLen
		} else {
			// Back-reference
			mLen := ctrl >> 5
			off := ((ctrl & 0x1F) << 8) + 1

			if mLen == 7 {
				if ip >= inLen {
					return nil, ErrCorruptedData
				}
				mLen += int(in[ip])
				ip++
			}

			if ip >= inLen {
				return nil, ErrCorruptedData
			}
			off += int(in[ip])
			ip++

			mLen += 2 // total length is mLen + 2
			ref := op - off

			if ref < 0 || op+mLen > expectedLen {
				return nil, ErrCorruptedData
			}

			if op >= ref+mLen {
				copyChunk(out[op:], out[ref:ref+mLen], mLen)
			} else {
				for i := 0; i < mLen; i++ {
					out[op+i] = out[ref+i]
				}
			}
			op += mLen
		}
	}

	if op != expectedLen {
		return nil, ErrCorruptedData
	}

	return out, nil
}
