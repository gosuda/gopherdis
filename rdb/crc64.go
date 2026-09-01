package rdb

// CRC64-Jones polynomial used by Redis (0xad93d23594c935a9)
const crc64Poly = uint64(0xad93d23594c935a9)

var crc64Table [256]uint64

func init() {
	for i := 0; i < 256; i++ {
		crc := uint64(i)
		for j := 0; j < 8; j++ {
			if crc&1 == 1 {
				crc = (crc >> 1) ^ crc64Poly
			} else {
				crc >>= 1
			}
		}
		crc64Table[i] = crc
	}
}

// CRC64 calculates the 64-bit CRC checksum matching Redis's crc64() function.
func CRC64(crc uint64, data []byte) uint64 {
	for _, b := range data {
		crc = crc64Table[byte(crc)^b] ^ (crc >> 8)
	}
	return crc
}
