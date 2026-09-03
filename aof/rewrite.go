package aof

import (
	"bytes"
	"fmt"
	"io"
	"strconv"

	"github.com/gosuda/gopherdis/datastruct/dict"
	"github.com/gosuda/gopherdis/datastruct/quicklist"
	"github.com/gosuda/gopherdis/datastruct/skiplist"
	"github.com/gosuda/gopherdis/db"
	"github.com/gosuda/gopherdis/object"
)

// encodeCommand formats an argv command slice into RESP Multi-bulk format.
func encodeCommand(argv [][]byte) []byte {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("*%d\r\n", len(argv)))
	for _, arg := range argv {
		buf.WriteString(fmt.Sprintf("$%d\r\n", len(arg)))
		buf.Write(arg)
		buf.WriteString("\r\n")
	}
	return buf.Bytes()
}

// writeEntryToRESP converts a single DBEntry into RESP commands and writes to w.
func writeEntryToRESP(w io.Writer, entry db.DBEntry) error {
	key := entry.Key
	obj := entry.Val
	if obj == nil {
		return nil
	}

	var cmds [][][]byte

	switch obj.Type {
	case object.OBJ_STRING:
		valBytes := obj.Bytes()
		cmds = append(cmds, [][]byte{
			[]byte("SET"),
			[]byte(key),
			valBytes,
		})

	case object.OBJ_LIST:
		ql, ok := obj.Ptr.(*quicklist.Quicklist)
		if ok && ql != nil && ql.Len() > 0 {
			items := ql.LRange(0, -1)
			if len(items) > 0 {
				argv := make([][]byte, 0, len(items)+2)
				argv = append(argv, []byte("RPUSH"), []byte(key))
				argv = append(argv, items...)
				cmds = append(cmds, argv)
			}
		}

	case object.OBJ_HASH:
		if d, ok := obj.Ptr.(*dict.Dict); ok && d != nil && d.Len() > 0 {
			argv := make([][]byte, 0, d.Len()*2+2)
			argv = append(argv, []byte("HSET"), []byte(key))
			d.ForEach(func(f string, v []byte) {
				argv = append(argv, []byte(f), v)
			})
			cmds = append(cmds, argv)
		} else if hmap, ok := obj.Ptr.(map[string][]byte); ok && len(hmap) > 0 {
			argv := make([][]byte, 0, len(hmap)*2+2)
			argv = append(argv, []byte("HSET"), []byte(key))
			for f, v := range hmap {
				argv = append(argv, []byte(f), v)
			}
			cmds = append(cmds, argv)
		}

	case object.OBJ_SET:
		smap, ok := obj.Ptr.(map[string]struct{})
		if ok && len(smap) > 0 {
			argv := make([][]byte, 0, len(smap)+2)
			argv = append(argv, []byte("SADD"), []byte(key))
			for mem := range smap {
				argv = append(argv, []byte(mem))
			}
			cmds = append(cmds, argv)
		}

	case object.OBJ_ZSET:
		zs, ok := obj.Ptr.(*skiplist.ZSet)
		if ok && zs != nil {
			elements := zs.Range(0, -1, false)
			if len(elements) > 0 {
				argv := make([][]byte, 0, len(elements)*2+2)
				argv = append(argv, []byte("ZADD"), []byte(key))
				for _, el := range elements {
					scoreStr := strconv.FormatFloat(el.Score, 'f', -1, 64)
					argv = append(argv, []byte(scoreStr), []byte(el.Member))
				}
				cmds = append(cmds, argv)
			}
		}
	}

	// If entry has TTL, append PEXPIREAT command with absolute millisecond timestamp
	if entry.ExpireAt > 0 {
		expStr := strconv.FormatInt(entry.ExpireAt, 10)
		cmds = append(cmds, [][]byte{
			[]byte("PEXPIREAT"),
			[]byte(key),
			[]byte(expStr),
		})
	}

	for _, cmd := range cmds {
		encoded := encodeCommand(cmd)
		if _, err := w.Write(encoded); err != nil {
			return err
		}
	}

	return nil
}
