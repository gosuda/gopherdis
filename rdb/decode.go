package rdb

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/gosuda/nedis/datastruct/quicklist"
	"github.com/gosuda/nedis/datastruct/skiplist"
	"github.com/gosuda/nedis/db"
	"github.com/gosuda/nedis/object"
	"github.com/gosuda/nedis/rdb/lzf"
)

var (
	ErrInvalidMagicHeader = errors.New("rdb: invalid magic header")
	ErrUnsupportedVersion = errors.New("rdb: unsupported RDB version")
	ErrChecksumMismatch   = errors.New("rdb: CRC64 checksum mismatch")
	ErrCorruptedRDB       = errors.New("rdb: corrupted RDB stream")
)

// Decoder decodes standard Redis RDB binary streams into memory.
type Decoder struct {
	r   io.Reader
	crc uint64
}

// NewDecoder creates a new RDB decoder reading from r.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: r}
}

func (dec *Decoder) readFull(buf []byte) error {
	if _, err := io.ReadFull(dec.r, buf); err != nil {
		return err
	}
	dec.crc = CRC64(dec.crc, buf)
	return nil
}

func (dec *Decoder) readByte() (byte, error) {
	var b [1]byte
	if err := dec.readFull(b[:]); err != nil {
		return 0, err
	}
	return b[0], nil
}

// ReadHeader parses and validates the 9-byte "REDIS0009" header.
func (dec *Decoder) ReadHeader() (int, error) {
	var header [9]byte
	if err := dec.readFull(header[:]); err != nil {
		return 0, err
	}
	if string(header[:5]) != "REDIS" {
		return 0, ErrInvalidMagicHeader
	}
	ver, err := strconv.Atoi(string(header[5:]))
	if err != nil || ver < 1 || ver > 15 {
		return 0, fmt.Errorf("%w: %d", ErrUnsupportedVersion, ver)
	}
	return ver, nil
}

// ReadLen reads length or special encoding format.
func (dec *Decoder) ReadLen() (length uint64, isEncoded bool, encType byte, err error) {
	first, err := dec.readByte()
	if err != nil {
		return 0, false, 0, err
	}

	flag := (first & 0xC0) >> 6
	switch flag {
	case 0:
		// 6-bit length (00xxxxxx)
		return uint64(first & 0x3F), false, 0, nil
	case 1:
		// 14-bit length (01xxxxxx xxxxxxxx)
		second, err := dec.readByte()
		if err != nil {
			return 0, false, 0, err
		}
		val := (uint64(first&0x3F) << 8) | uint64(second)
		return val, false, 0, nil
	case 2:
		// 32-bit or 64-bit length (10xxxxxx)
		if first == Enc32BitLen {
			var buf [4]byte
			if err := dec.readFull(buf[:]); err != nil {
				return 0, false, 0, err
			}
			return uint64(binary.BigEndian.Uint32(buf[:])), false, 0, nil
		} else if first == Enc64BitLen {
			var buf [8]byte
			if err := dec.readFull(buf[:]); err != nil {
				return 0, false, 0, err
			}
			return binary.BigEndian.Uint64(buf[:]), false, 0, nil
		}
		return 0, false, 0, ErrCorruptedRDB
	case 3:
		// Special encoding (11xxxxxx)
		return 0, true, first & 0x3F, nil
	}

	return 0, false, 0, ErrCorruptedRDB
}

// ReadString reads a length-prefixed or integer-encoded string.
func (dec *Decoder) ReadString() ([]byte, error) {
	length, isEncoded, encType, err := dec.ReadLen()
	if err != nil {
		return nil, err
	}

	if isEncoded {
		switch encType {
		case EncInt8:
			b, err := dec.readByte()
			if err != nil {
				return nil, err
			}
			return []byte(strconv.FormatInt(int64(int8(b)), 10)), nil
		case EncInt16:
			var buf [2]byte
			if err := dec.readFull(buf[:]); err != nil {
				return nil, err
			}
			val := int16(binary.LittleEndian.Uint16(buf[:]))
			return []byte(strconv.FormatInt(int64(val), 10)), nil
		case EncInt32:
			var buf [4]byte
			if err := dec.readFull(buf[:]); err != nil {
				return nil, err
			}
			val := int32(binary.LittleEndian.Uint32(buf[:]))
			return []byte(strconv.FormatInt(int64(val), 10)), nil
		case EncLZF:
			// LZF compressed string: clen (compressed len), ulen (uncompressed len)
			cLen, _, _, err := dec.ReadLen()
			if err != nil {
				return nil, err
			}
			uLen, _, _, err := dec.ReadLen()
			if err != nil {
				return nil, err
			}
			cData := make([]byte, cLen)
			if err := dec.readFull(cData); err != nil {
				return nil, err
			}
			return lzf.Decompress(cData, int(uLen))
		default:
			return nil, ErrCorruptedRDB
		}
	}

	data := make([]byte, length)
	if err := dec.readFull(data); err != nil {
		return nil, err
	}
	return data, nil
}

// Load reads the RDB stream and populates targetDB.
func (dec *Decoder) Load(targetDB *db.ShardedDB) error {
	_, err := dec.ReadHeader()
	if err != nil {
		return err
	}

	var expireAt int64 = 0

	for {
		b, err := dec.readByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		// Handle opcodes
		switch b {
		case OpcodeAux:
			_, _ = dec.ReadString() // aux key
			_, _ = dec.ReadString() // aux value
			continue

		case OpcodeSelectDB:
			_, _, _, _ = dec.ReadLen() // select db index
			continue

		case OpcodeResizeDB:
			_, _, _, _ = dec.ReadLen() // db size
			_, _, _, _ = dec.ReadLen() // expires size
			continue

		case OpcodeExpireTimeMs:
			var expBuf [8]byte
			if err := dec.readFull(expBuf[:]); err != nil {
				return err
			}
			expireAt = int64(binary.LittleEndian.Uint64(expBuf[:]))
			continue

		case OpcodeExpireTimeSec:
			var expBuf [4]byte
			if err := dec.readFull(expBuf[:]); err != nil {
				return err
			}
			expireAt = int64(binary.LittleEndian.Uint32(expBuf[:])) * 1000
			continue

		case OpcodeEOF:
			// Read 8 bytes checksum
			var expectedCRCBuf [8]byte
			if _, err := io.ReadFull(dec.r, expectedCRCBuf[:]); err != nil {
				return nil
			}
			expectedCRC := binary.LittleEndian.Uint64(expectedCRCBuf[:])
			if expectedCRC != 0 && expectedCRC != dec.crc {
				return ErrChecksumMismatch
			}
			return nil
		}

		// It's a key-value object
		valType := b
		keyBytes, err := dec.ReadString()
		if err != nil {
			return err
		}
		key := string(keyBytes)

		var robj *object.Robj

		switch valType {
		case TypeString:
			valBytes, err := dec.ReadString()
			if err != nil {
				return err
			}
			robj = object.CreateRawStringObject(valBytes)

		case TypeList, TypeListQuicklist:
			count, _, _, err := dec.ReadLen()
			if err != nil {
				return err
			}
			ql := quicklist.NewQuicklist()
			for i := uint64(0); i < count; i++ {
				item, err := dec.ReadString()
				if err != nil {
					return err
				}
				ql.RPush(item)
			}
			robj = object.CreateObject(object.OBJ_LIST, ql)
			robj.Encoding = object.OBJ_ENCODING_QUICKLIST

		case TypeSet:
			count, _, _, err := dec.ReadLen()
			if err != nil {
				return err
			}
			smap := make(map[string]struct{}, count)
			for i := uint64(0); i < count; i++ {
				mem, err := dec.ReadString()
				if err != nil {
					return err
				}
				smap[string(mem)] = struct{}{}
			}
			robj = object.CreateObject(object.OBJ_SET, smap)
			robj.Encoding = object.OBJ_ENCODING_HT

		case TypeHash:
			count, _, _, err := dec.ReadLen()
			if err != nil {
				return err
			}
			hmap := make(map[string][]byte, count)
			for i := uint64(0); i < count; i++ {
				field, err := dec.ReadString()
				if err != nil {
					return err
				}
				val, err := dec.ReadString()
				if err != nil {
					return err
				}
				hmap[string(field)] = val
			}
			robj = object.CreateObject(object.OBJ_HASH, hmap)
			robj.Encoding = object.OBJ_ENCODING_HT

		case TypeZSet, TypeZSet2:
			count, _, _, err := dec.ReadLen()
			if err != nil {
				return err
			}
			zs := skiplist.NewZSet()
			for i := uint64(0); i < count; i++ {
				member, err := dec.ReadString()
				if err != nil {
					return err
				}
				scoreBytes, err := dec.ReadString()
				if err != nil {
					return err
				}
				score, _ := strconv.ParseFloat(string(scoreBytes), 64)
				zs.Add(string(member), score)
			}
			robj = object.CreateObject(object.OBJ_ZSET, zs)
			robj.Encoding = object.OBJ_ENCODING_SKIPLIST

		default:
			// Skip unknown object value string
			_, _ = dec.ReadString()
		}

		if robj != nil {
			if expireAt > 0 {
				ttlMs := expireAt - time.Now().UnixMilli()
				if ttlMs > 0 {
					_ = targetDB.SetWithExpire(key, robj, time.Duration(ttlMs)*time.Millisecond)
				}
			} else {
				_ = targetDB.Set(key, robj)
			}
		}

		expireAt = 0
	}

	return nil
}
