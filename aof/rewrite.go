package aof

import (
	"bytes"
	"fmt"
	"io"
	"strconv"

	"github.com/gosuda/gopherdis/datastruct/dict"
	"github.com/gosuda/gopherdis/datastruct/intset"
	"github.com/gosuda/gopherdis/datastruct/listpack"
	"github.com/gosuda/gopherdis/datastruct/quicklist"
	"github.com/gosuda/gopherdis/datastruct/set"
	"github.com/gosuda/gopherdis/datastruct/skiplist"
	"github.com/gosuda/gopherdis/datastruct/stream"
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
		var items [][]byte
		switch v := obj.Ptr.(type) {
		case *listpack.Listpack:
			items = v.All()
		case *quicklist.Quicklist:
			if v != nil {
				items = v.LRange(0, -1)
			}
		}
		{
			{
				argv := make([][]byte, 0, len(items)+2)
				argv = append(argv, []byte("RPUSH"), []byte(key))
				argv = append(argv, items...)
				cmds = append(cmds, argv)
			}
		}

	case object.OBJ_HASH:
		if lp, ok := obj.Ptr.(*listpack.Listpack); ok && lp != nil && lp.Len() > 0 {
			argv := make([][]byte, 0, lp.Len()+2)
			argv = append(argv, []byte("HSET"), []byte(key))
			argv = append(argv, lp.All()...)
			cmds = append(cmds, argv)
		} else if d, ok := obj.Ptr.(*dict.Dict); ok && d != nil && d.Len() > 0 {
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
		// Commands store sets as *set.Set; matching only the bare map dropped every
		// set from the rewritten AOF.
		var members []string
		switch v := obj.Ptr.(type) {
		case *intset.IntSet:
			members = v.Members()
		case *listpack.Listpack:
			for _, m := range v.All() {
				members = append(members, string(m))
			}
		case *set.Set:
			if v != nil {
				members = v.Members()
			}
		case map[string]struct{}:
			members = make([]string, 0, len(v))
			for mem := range v {
				members = append(members, mem)
			}
		}
		if len(members) > 0 {
			argv := make([][]byte, 0, len(members)+2)
			argv = append(argv, []byte("SADD"), []byte(key))
			for _, mem := range members {
				argv = append(argv, []byte(mem))
			}
			cmds = append(cmds, argv)
		}

	case object.OBJ_STREAM:
		// Streams have no RDB opcode here, so the AOF is their only persistence
		// path: replay them as the XADDs that produced them.
		if st, ok := obj.Ptr.(*stream.Stream); ok && st != nil {
			for _, e := range st.Range(stream.ZeroID, stream.MaxID, 0, false) {
				argv := make([][]byte, 0, len(e.Fields)*2+3)
				argv = append(argv, []byte("XADD"), []byte(key), []byte(e.ID.String()))
				for i, f := range e.Fields {
					if i >= len(e.Values) {
						break
					}
					argv = append(argv, []byte(f), e.Values[i])
				}
				cmds = append(cmds, argv)
			}
		}

	case object.OBJ_ZSET:
		if lp, ok := obj.Ptr.(*listpack.Listpack); ok && lp != nil && lp.Len() > 0 {
			all := lp.All()
			argv := make([][]byte, 0, len(all)+2)
			argv = append(argv, []byte("ZADD"), []byte(key))
			// ZADD takes score before member; the listpack holds the reverse.
			for i := 0; i+1 < len(all); i += 2 {
				argv = append(argv, all[i+1], all[i])
			}
			cmds = append(cmds, argv)
			break
		}
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
