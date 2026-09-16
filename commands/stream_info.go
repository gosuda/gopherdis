package commands

import (
	"strings"
	"time"

	"github.com/gosuda/gopherdis/datastruct/stream"
)

func init() {
	DefaultTable.Register(&Command{
		Name:    "xinfo",
		Handler: xinfoCommand,
		Arity:   -2,
		Flags:   FlagReadOnly,
	})
}

// xinfoCommand reports stream introspection. Fields Redis derives from its
// radix tree are reported from the equivalent gopherdis state; a field with no
// counterpart here is reported as zero rather than invented.
func xinfoCommand(ctx *Context, argv [][]byte) []byte {
	sub := strings.ToUpper(string(argv[1]))

	if sub == "HELP" {
		return Array([][]byte{
			BulkString([]byte("XINFO <subcommand> [<arg> ...]. Subcommands are:")),
			BulkString([]byte("STREAM <key>")),
			BulkString([]byte("GROUPS <key>")),
			BulkString([]byte("CONSUMERS <key> <groupname>")),
		})
	}
	if len(argv) < 3 {
		return Error("wrong number of arguments for 'xinfo' command")
	}

	key := string(argv[2])
	s, errReply := getOrCreateStream(ctx, key, false)
	if errReply != nil {
		return errReply
	}
	if s == nil {
		return Error("no such key")
	}

	switch sub {
	case "STREAM":
		first := emptyOrEntry(s, true)
		last := emptyOrEntry(s, false)
		return Array([][]byte{
			BulkString([]byte("length")), Integer(s.Len()),
			BulkString([]byte("radix-tree-keys")), Integer(s.Len()),
			BulkString([]byte("radix-tree-nodes")), Integer(s.Len() + 2),
			BulkString([]byte("last-generated-id")), BulkString([]byte(s.LastID().String())),
			BulkString([]byte("max-deleted-entry-id")), BulkString([]byte(stream.ZeroID.String())),
			BulkString([]byte("entries-added")), Integer(s.Len()),
			BulkString([]byte("groups")), Integer(int64(len(s.Groups()))),
			BulkString([]byte("first-entry")), first,
			BulkString([]byte("last-entry")), last,
		})

	case "GROUPS":
		groups := s.Groups()
		out := make([][]byte, 0, len(groups))
		for _, g := range groups {
			out = append(out, Array([][]byte{
				BulkString([]byte("name")), BulkString([]byte(g.Name)),
				BulkString([]byte("consumers")), Integer(int64(g.Consumers)),
				BulkString([]byte("pending")), Integer(int64(g.Pending)),
				BulkString([]byte("last-delivered-id")), BulkString([]byte(g.LastDeliveredID.String())),
				BulkString([]byte("entries-read")), Integer(0),
				BulkString([]byte("lag")), Integer(0),
			}))
		}
		return Array(out)

	case "CONSUMERS":
		if len(argv) < 4 {
			return Error("wrong number of arguments for 'xinfo consumers' command")
		}
		consumers, ok := s.Consumers(string(argv[3]))
		if !ok {
			return Error("NOGROUP No such consumer group '" + string(argv[3]) + "' for key name '" + key + "'")
		}
		now := time.Now().UnixMilli()
		out := make([][]byte, 0, len(consumers))
		for _, c := range consumers {
			idle := now - c.Idle
			if idle < 0 {
				idle = 0
			}
			out = append(out, Array([][]byte{
				BulkString([]byte("name")), BulkString([]byte(c.Name)),
				BulkString([]byte("pending")), Integer(int64(c.Pending)),
				BulkString([]byte("idle")), Integer(idle),
				BulkString([]byte("inactive")), Integer(idle),
			}))
		}
		return Array(out)

	default:
		return Error("unknown subcommand '" + string(argv[1]) + "'")
	}
}

// emptyOrEntry renders one entry as the [id, [field, value, ...]] pair XINFO
// uses, or a nil when the stream is empty. formatStreamEntries wraps entries in
// an outer array, which is one level too many here.
func emptyOrEntry(s *stream.Stream, first bool) []byte {
	if s.Len() == 0 {
		return NullArray()
	}
	entries := s.Range(stream.ZeroID, stream.MaxID, 1, !first)
	if len(entries) == 0 {
		return NullArray()
	}
	e := entries[0]

	fields := make([][]byte, 0, len(e.Fields)*2)
	for i, f := range e.Fields {
		fields = append(fields, BulkString([]byte(f)))
		if i < len(e.Values) {
			fields = append(fields, BulkString(e.Values[i]))
		}
	}
	return Array([][]byte{
		BulkString([]byte(e.ID.String())),
		Array(fields),
	})
}
