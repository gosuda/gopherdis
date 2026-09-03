package rdb

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strconv"
	"time"

	"github.com/gosuda/gopherdis/datastruct/dict"
	"github.com/gosuda/gopherdis/datastruct/quicklist"
	"github.com/gosuda/gopherdis/datastruct/skiplist"
	"github.com/gosuda/gopherdis/db"
	"github.com/gosuda/gopherdis/object"
	"github.com/gosuda/gopherdis/rdb/lzf"
)

// Encoder encodes in-memory Redis structures to the standard RDB binary stream.
type Encoder struct {
	w   io.Writer
	crc uint64
}

// NewEncoder creates a new RDB encoder writing to w.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

func (enc *Encoder) writeRaw(b []byte) error {
	enc.crc = CRC64(enc.crc, b)
	_, err := enc.w.Write(b)
	return err
}

func (enc *Encoder) writeByte(b byte) error {
	return enc.writeRaw([]byte{b})
}

// WriteHeader writes the 9-byte RDB magic header (e.g. "REDIS0009").
func (enc *Encoder) WriteHeader() error {
	header := fmt.Sprintf("REDIS%04d", RDBVersion)
	return enc.writeRaw([]byte(header))
}

// WriteAux writes an auxiliary metadata key-value pair.
func (enc *Encoder) WriteAux(key, val string) error {
	if err := enc.writeByte(OpcodeAux); err != nil {
		return err
	}
	if err := enc.WriteString([]byte(key)); err != nil {
		return err
	}
	return enc.WriteString([]byte(val))
}

// WriteSelectDB writes the SELECTDB opcode and database index.
func (enc *Encoder) WriteSelectDB(dbIndex uint64) error {
	if err := enc.writeByte(OpcodeSelectDB); err != nil {
		return err
	}
	return enc.WriteLen(dbIndex)
}

// WriteResizeDB writes the RESIZEDB opcode with DB size and expires count hints.
func (enc *Encoder) WriteResizeDB(dbSize, expiresSize uint64) error {
	if err := enc.writeByte(OpcodeResizeDB); err != nil {
		return err
	}
	if err := enc.WriteLen(dbSize); err != nil {
		return err
	}
	return enc.WriteLen(expiresSize)
}

// WriteLen writes a length using Redis RDB length encoding.
func (enc *Encoder) WriteLen(length uint64) error {
	if length < (1 << 6) {
		// 6-bit length (00xxxxxx)
		return enc.writeByte(byte(length & 0x3F))
	} else if length < (1 << 14) {
		// 14-bit length (01xxxxxx xxxxxxxx)
		buf := []byte{
			byte(((length >> 8) & 0x3F) | 0x40),
			byte(length & 0xFF),
		}
		return enc.writeRaw(buf)
	} else if length <= math.MaxUint32 {
		// 32-bit length (10000000 + 4 bytes big endian)
		buf := make([]byte, 5)
		buf[0] = Enc32BitLen
		binary.BigEndian.PutUint32(buf[1:], uint32(length))
		return enc.writeRaw(buf)
	} else {
		// 64-bit length (10000001 + 8 bytes big endian)
		buf := make([]byte, 9)
		buf[0] = Enc64BitLen
		binary.BigEndian.PutUint64(buf[1:], length)
		return enc.writeRaw(buf)
	}
}

// WriteString writes a length-prefixed raw or LZF-compressed string to the RDB stream.
func (enc *Encoder) WriteString(data []byte) error {
	if len(data) >= 20 {
		if compressed, ok := lzf.Compress(data); ok {
			// Write LZF opcode (11000011 = EncVal << 6 | EncLZF)
			if err := enc.writeByte(byte(EncVal<<6 | EncLZF)); err != nil {
				return err
			}
			if err := enc.WriteLen(uint64(len(compressed))); err != nil {
				return err
			}
			if err := enc.WriteLen(uint64(len(data))); err != nil {
				return err
			}
			return enc.writeRaw(compressed)
		}
	}

	if err := enc.WriteLen(uint64(len(data))); err != nil {
		return err
	}
	return enc.writeRaw(data)
}

// WriteEntry encodes a single DBEntry into standard RDB binary format.
func (enc *Encoder) WriteEntry(entry db.DBEntry) error {
	key := []byte(entry.Key)
	obj := entry.Val
	if obj == nil {
		return nil
	}

	// 1. Expire timestamp (if set)
	if entry.ExpireAt > 0 {
		if err := enc.writeByte(OpcodeExpireTimeMs); err != nil {
			return err
		}
		var expBuf [8]byte
		binary.LittleEndian.PutUint64(expBuf[:], uint64(entry.ExpireAt))
		if err := enc.writeRaw(expBuf[:]); err != nil {
			return err
		}
	}

	// 2. Type opcode and value payload
	switch obj.Type {
	case object.OBJ_STRING:
		if err := enc.writeByte(TypeString); err != nil {
			return err
		}
		if err := enc.WriteString(key); err != nil {
			return err
		}
		return enc.WriteString(obj.Bytes())

	case object.OBJ_LIST:
		ql, ok := obj.Ptr.(*quicklist.Quicklist)
		if !ok || ql == nil {
			return nil
		}
		items := ql.LRange(0, -1)
		if err := enc.writeByte(TypeList); err != nil {
			return err
		}
		if err := enc.WriteString(key); err != nil {
			return err
		}
		if err := enc.WriteLen(uint64(len(items))); err != nil {
			return err
		}
		for _, item := range items {
			if err := enc.WriteString(item); err != nil {
				return err
			}
		}

	case object.OBJ_SET:
		smap, ok := obj.Ptr.(map[string]struct{})
		if !ok || smap == nil {
			return nil
		}
		if err := enc.writeByte(TypeSet); err != nil {
			return err
		}
		if err := enc.WriteString(key); err != nil {
			return err
		}
		if err := enc.WriteLen(uint64(len(smap))); err != nil {
			return err
		}
		for mem := range smap {
			if err := enc.WriteString([]byte(mem)); err != nil {
				return err
			}
		}

	case object.OBJ_HASH:
		if d, ok := obj.Ptr.(*dict.Dict); ok && d != nil {
			if err := enc.writeByte(TypeHash); err != nil {
				return err
			}
			if err := enc.WriteString(key); err != nil {
				return err
			}
			if err := enc.WriteLen(uint64(d.Len())); err != nil {
				return err
			}
			var encodeErr error
			d.ForEach(func(f string, v []byte) {
				if encodeErr != nil {
					return
				}
				if err := enc.WriteString([]byte(f)); err != nil {
					encodeErr = err
					return
				}
				if err := enc.WriteString(v); err != nil {
					encodeErr = err
					return
				}
			})
			return encodeErr
		} else if hmap, ok := obj.Ptr.(map[string][]byte); ok && hmap != nil {
			if err := enc.writeByte(TypeHash); err != nil {
				return err
			}
			if err := enc.WriteString(key); err != nil {
				return err
			}
			if err := enc.WriteLen(uint64(len(hmap))); err != nil {
				return err
			}
			for f, v := range hmap {
				if err := enc.WriteString([]byte(f)); err != nil {
					return err
				}
				if err := enc.WriteString(v); err != nil {
					return err
				}
			}
		}

	case object.OBJ_ZSET:
		zs, ok := obj.Ptr.(*skiplist.ZSet)
		if !ok || zs == nil {
			return nil
		}
		elements := zs.Range(0, -1, false)
		if err := enc.writeByte(TypeZSet); err != nil {
			return err
		}
		if err := enc.WriteString(key); err != nil {
			return err
		}
		if err := enc.WriteLen(uint64(len(elements))); err != nil {
			return err
		}
		for _, el := range elements {
			if err := enc.WriteString([]byte(el.Member)); err != nil {
				return err
			}
			scoreStr := strconv.FormatFloat(el.Score, 'f', -1, 64)
			if err := enc.WriteString([]byte(scoreStr)); err != nil {
				return err
			}
		}
	}

	return nil
}

// WriteFooter writes the EOF opcode and the calculated CRC64 checksum.
func (enc *Encoder) WriteFooter() error {
	if err := enc.writeByte(OpcodeEOF); err != nil {
		return err
	}
	var crcBuf [8]byte
	binary.LittleEndian.PutUint64(crcBuf[:], enc.crc)
	_, err := enc.w.Write(crcBuf[:]) // Checksum itself is not included in the CRC calculation
	return err
}

// WriteStandardAuxFields writes common standard Redis metadata fields.
func (enc *Encoder) WriteStandardAuxFields() error {
	_ = enc.WriteAux("redis-ver", "7.2.0")
	_ = enc.WriteAux("redis-bits", "64")
	_ = enc.WriteAux("ctime", strconv.FormatInt(time.Now().Unix(), 10))
	return nil
}
