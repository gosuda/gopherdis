package commands

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gosuda/nedis/datastruct/stream"
	"github.com/gosuda/nedis/object"
)

func init() {
	DefaultTable.Register(&Command{
		Name:    "xadd",
		Handler: xaddCommand,
		Arity:   -4,
		Flags:   FlagWrite | FlagFast,
	})
	DefaultTable.Register(&Command{
		Name:    "xrange",
		Handler: xrangeCommand,
		Arity:   -4,
		Flags:   FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "xrevrange",
		Handler: xrevrangeCommand,
		Arity:   -4,
		Flags:   FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "xlen",
		Handler: xlenCommand,
		Arity:   2,
		Flags:   FlagReadOnly | FlagFast,
	})
	DefaultTable.Register(&Command{
		Name:    "xdel",
		Handler: xdelCommand,
		Arity:   -3,
		Flags:   FlagWrite | FlagFast,
	})
	DefaultTable.Register(&Command{
		Name:    "xtrim",
		Handler: xtrimCommand,
		Arity:   -4,
		Flags:   FlagWrite,
	})
	DefaultTable.Register(&Command{
		Name:    "xgroup",
		Handler: xgroupCommand,
		Arity:   -3,
		Flags:   FlagWrite,
	})
	DefaultTable.Register(&Command{
		Name:    "xread",
		Handler: xreadCommand,
		Arity:   -4,
		Flags:   FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "xreadgroup",
		Handler: xreadgroupCommand,
		Arity:   -7,
		Flags:   FlagWrite,
	})
	DefaultTable.Register(&Command{
		Name:    "xack",
		Handler: xackCommand,
		Arity:   -4,
		Flags:   FlagWrite | FlagFast,
	})
	DefaultTable.Register(&Command{
		Name:    "xpending",
		Handler: xpendingCommand,
		Arity:   -3,
		Flags:   FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "xclaim",
		Handler: xclaimCommand,
		Arity:   -6,
		Flags:   FlagWrite,
	})
}

func getOrCreateStream(ctx *Context, key string, createIfMissing bool) (*stream.Stream, []byte) {
	obj, exists := ctx.DB.Get(key)
	if exists {
		if obj.Type != object.OBJ_STREAM {
			return nil, Error("WRONGTYPE Operation against a key holding the wrong kind of value")
		}
		return obj.Ptr.(*stream.Stream), nil
	}
	if !createIfMissing {
		return nil, nil
	}
	s := stream.NewStream()
	_ = ctx.DB.Set(key, object.CreateObject(object.OBJ_STREAM, s))
	return s, nil
}

func xaddCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	idx := 2
	maxLen := int64(0)
	approx := false
	nomkstream := false

	// Parse options (NOMKSTREAM, MAXLEN, MINID)
	for idx < len(argv) {
		opt := strings.ToUpper(string(argv[idx]))
		if opt == "NOMKSTREAM" {
			nomkstream = true
			idx++
		} else if opt == "MAXLEN" || opt == "MINID" {
			idx++
			if idx < len(argv) && (string(argv[idx]) == "=" || string(argv[idx]) == "~") {
				if string(argv[idx]) == "~" {
					approx = true
				}
				idx++
			}
			if idx >= len(argv) {
				return Error("syntax error in XADD")
			}
			lim, err := strconv.ParseInt(string(argv[idx]), 10, 64)
			if err != nil {
				return Error("value is not an integer or out of range")
			}
			maxLen = lim
			idx++
		} else {
			break
		}
	}

	if idx >= len(argv) {
		return Error("wrong number of arguments for 'xadd' command")
	}

	idStr := string(argv[idx])
	idx++

	fieldsRaw := argv[idx:]
	if len(fieldsRaw) == 0 || len(fieldsRaw)%2 != 0 {
		return Error("wrong number of arguments for 'xadd' command")
	}

	s, errReply := getOrCreateStream(ctx, key, !nomkstream)
	if errReply != nil {
		return errReply
	}
	if s == nil {
		return NullBulkString()
	}

	lastID := s.LastID()
	id, err := stream.ParseID(idStr, lastID)
	if err != nil {
		return Error(err.Error())
	}

	addedID, err := s.AddRaw(id, fieldsRaw, maxLen, approx)
	if err != nil {
		return Error(err.Error())
	}

	return BulkString([]byte(addedID.String()))
}

func xrangeCommand(ctx *Context, argv [][]byte) []byte {
	return genericRange(ctx, argv, false)
}

func xrevrangeCommand(ctx *Context, argv [][]byte) []byte {
	return genericRange(ctx, argv, true)
}

func genericRange(ctx *Context, argv [][]byte, reverse bool) []byte {
	key := string(argv[1])
	startStr := string(argv[2])
	endStr := string(argv[3])
	count := 0

	if len(argv) >= 6 && strings.ToUpper(string(argv[4])) == "COUNT" {
		c, err := strconv.Atoi(string(argv[5]))
		if err != nil || c < 0 {
			return Error("value is not an integer or out of range")
		}
		count = c
	}

	s, errReply := getOrCreateStream(ctx, key, false)
	if errReply != nil {
		return errReply
	}
	if s == nil {
		return Array(nil)
	}

	startID, err := stream.ParseID(startStr, stream.ZeroID)
	if err != nil {
		return Error(err.Error())
	}
	endID, err := stream.ParseID(endStr, stream.ZeroID)
	if err != nil {
		return Error(err.Error())
	}

	entries := s.Range(startID, endID, count, reverse)
	return formatStreamEntries(entries)
}

func formatStreamEntries(entries []stream.StreamEntry) []byte {
	if len(entries) == 0 {
		return []byte("*0\r\n")
	}

	var buf bytes.Buffer
	buf.Grow(len(entries) * 128)
	buf.WriteString(fmt.Sprintf("*%d\r\n", len(entries)))

	for _, e := range entries {
		buf.WriteString("*2\r\n")
		idStr := e.ID.String()
		buf.WriteString(fmt.Sprintf("$%d\r\n%s\r\n", len(idStr), idStr))

		buf.WriteString(fmt.Sprintf("*%d\r\n", len(e.Fields)*2))
		for j := 0; j < len(e.Fields); j++ {
			buf.WriteString(fmt.Sprintf("$%d\r\n%s\r\n", len(e.Fields[j]), e.Fields[j]))
			buf.WriteString(fmt.Sprintf("$%d\r\n", len(e.Values[j])))
			buf.Write(e.Values[j])
			buf.WriteString("\r\n")
		}
	}
	return buf.Bytes()
}

func xlenCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	s, errReply := getOrCreateStream(ctx, key, false)
	if errReply != nil {
		return errReply
	}
	if s == nil {
		return Integer(0)
	}
	return Integer(s.Len())
}

func xdelCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	s, errReply := getOrCreateStream(ctx, key, false)
	if errReply != nil {
		return errReply
	}
	if s == nil {
		return Integer(0)
	}

	ids := make([]stream.StreamID, 0, len(argv)-2)
	for i := 2; i < len(argv); i++ {
		id, err := stream.ParseID(string(argv[i]), stream.ZeroID)
		if err == nil {
			ids = append(ids, id)
		}
	}

	deleted := s.Delete(ids)
	return Integer(deleted)
}

func xtrimCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	idx := 2
	approx := false

	opt := strings.ToUpper(string(argv[idx]))
	if opt != "MAXLEN" && opt != "MINID" {
		return Error("syntax error in XTRIM")
	}
	idx++

	if idx < len(argv) && (string(argv[idx]) == "=" || string(argv[idx]) == "~") {
		if string(argv[idx]) == "~" {
			approx = true
		}
		idx++
	}
	if idx >= len(argv) {
		return Error("syntax error in XTRIM")
	}

	lim, err := strconv.ParseInt(string(argv[idx]), 10, 64)
	if err != nil {
		return Error("value is not an integer or out of range")
	}

	s, errReply := getOrCreateStream(ctx, key, false)
	if errReply != nil {
		return errReply
	}
	if s == nil {
		return Integer(0)
	}

	deleted := s.Trim(lim, approx)
	return Integer(deleted)
}

func xgroupCommand(ctx *Context, argv [][]byte) []byte {
	subCmd := strings.ToUpper(string(argv[1]))
	key := string(argv[2])

	switch subCmd {
	case "CREATE":
		if len(argv) < 5 {
			return Error("wrong number of arguments for 'xgroup create' command")
		}
		groupName := string(argv[3])
		startStr := string(argv[4])

		s, errReply := getOrCreateStream(ctx, key, true)
		if errReply != nil {
			return errReply
		}

		var startID stream.StreamID
		if startStr == "$" {
			startID = s.LastID()
		} else {
			id, err := stream.ParseID(startStr, stream.ZeroID)
			if err != nil {
				return Error(err.Error())
			}
			startID = id
		}

		if err := s.CreateGroup(groupName, startID); err != nil {
			return Error(err.Error())
		}
		return OK()

	case "DESTROY":
		if len(argv) < 4 {
			return Error("wrong number of arguments for 'xgroup destroy' command")
		}
		groupName := string(argv[3])
		s, errReply := getOrCreateStream(ctx, key, false)
		if errReply != nil {
			return errReply
		}
		if s == nil || !s.DestroyGroup(groupName) {
			return Integer(0)
		}
		return Integer(1)

	default:
		return Error(fmt.Sprintf("unknown subcommand '%s'", subCmd))
	}
}

func xreadCommand(ctx *Context, argv [][]byte) []byte {
	idx := 1
	count := 0
	blockMs := int64(-1)

	for idx < len(argv) {
		opt := strings.ToUpper(string(argv[idx]))
		if opt == "COUNT" {
			idx++
			c, err := strconv.Atoi(string(argv[idx]))
			if err != nil {
				return Error("value is not an integer or out of range")
			}
			count = c
			idx++
		} else if opt == "BLOCK" {
			idx++
			b, err := strconv.ParseInt(string(argv[idx]), 10, 64)
			if err != nil {
				return Error("value is not an integer or out of range")
			}
			blockMs = b
			idx++
		} else if opt == "STREAMS" {
			idx++
			break
		} else {
			return Error("syntax error in XREAD")
		}
	}

	rem := argv[idx:]
	if len(rem) == 0 || len(rem)%2 != 0 {
		return Error("syntax error in XREAD STREAMS")
	}

	numStreams := len(rem) / 2
	keys := make([]string, numStreams)
	startIDs := make([]stream.StreamID, numStreams)

	for i := 0; i < numStreams; i++ {
		keys[i] = string(rem[i])
		idStr := string(rem[numStreams+i])

		s, _ := getOrCreateStream(ctx, keys[i], false)
		lastID := stream.ZeroID
		if s != nil {
			lastID = s.LastID()
		}

		if idStr == "$" {
			startIDs[i] = lastID
		} else {
			id, err := stream.ParseID(idStr, stream.ZeroID)
			if err != nil {
				return Error(err.Error())
			}
			startIDs[i] = id
		}
	}

	// Fetch entries
	fetch := func() [][]byte {
		var results [][]byte
		for i, k := range keys {
			s, _ := getOrCreateStream(ctx, k, false)
			if s != nil {
				// Exclusive range > startID
				entries := s.Range(startIDs[i], stream.MaxID, count+1, false)
				filtered := make([]stream.StreamEntry, 0, len(entries))
				for _, e := range entries {
					if e.ID.Compare(startIDs[i]) > 0 {
						filtered = append(filtered, e)
						if count > 0 && len(filtered) >= count {
							break
						}
					}
				}
				if len(filtered) > 0 {
					results = append(results, Array([][]byte{
						BulkString([]byte(k)),
						formatStreamEntries(filtered),
					}))
				}
			}
		}
		return results
	}

	results := fetch()
	if len(results) > 0 || blockMs < 0 {
		if len(results) == 0 {
			return Array(nil)
		}
		return Array(results)
	}

	// Handle BLOCK wait
	s, _ := getOrCreateStream(ctx, keys[0], true)
	if s == nil {
		return Array(nil)
	}
	waiter := s.RegisterWaiter()
	defer s.UnregisterWaiter(waiter)

	timeout := time.Duration(blockMs) * time.Millisecond
	if blockMs == 0 {
		timeout = 10 * time.Minute
	}

	select {
	case <-waiter.WakeCh:
		results = fetch()
		if len(results) == 0 {
			return Array(nil)
		}
		return Array(results)
	case <-time.After(timeout):
		return NullArray()
	}
}

func xreadgroupCommand(ctx *Context, argv [][]byte) []byte {
	idx := 1
	groupName := ""
	consumerName := ""
	count := 0
	noAck := false

	for idx < len(argv) {
		opt := strings.ToUpper(string(argv[idx]))
		if opt == "GROUP" {
			idx++
			groupName = string(argv[idx])
			idx++
			consumerName = string(argv[idx])
			idx++
		} else if opt == "COUNT" {
			idx++
			c, err := strconv.Atoi(string(argv[idx]))
			if err != nil {
				return Error("value is not an integer or out of range")
			}
			count = c
			idx++
		} else if opt == "NOACK" {
			noAck = true
			idx++
		} else if opt == "STREAMS" {
			idx++
			break
		} else {
			return Error("syntax error in XREADGROUP")
		}
	}

	rem := argv[idx:]
	if len(rem) == 0 || len(rem)%2 != 0 {
		return Error("syntax error in XREADGROUP STREAMS")
	}

	numStreams := len(rem) / 2
	var streamResults [][]byte

	for i := 0; i < numStreams; i++ {
		k := string(rem[i])
		idStr := string(rem[numStreams+i])

		s, errReply := getOrCreateStream(ctx, k, false)
		if errReply != nil {
			return errReply
		}
		if s == nil {
			continue
		}

		isNew := (idStr == ">")
		var startID stream.StreamID
		if !isNew {
			id, err := stream.ParseID(idStr, stream.ZeroID)
			if err != nil {
				return Error(err.Error())
			}
			startID = id
		}

		entries, err := s.ReadGroup(groupName, consumerName, startID, isNew, count, noAck)
		if err != nil {
			return Error(err.Error())
		}

		if len(entries) > 0 {
			streamResults = append(streamResults, Array([][]byte{
				BulkString([]byte(k)),
				formatStreamEntries(entries),
			}))
		}
	}

	if len(streamResults) == 0 {
		return Array(nil)
	}
	return Array(streamResults)
}

func xackCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	groupName := string(argv[2])

	s, errReply := getOrCreateStream(ctx, key, false)
	if errReply != nil {
		return errReply
	}
	if s == nil {
		return Integer(0)
	}

	ids := make([]stream.StreamID, 0, len(argv)-3)
	for i := 3; i < len(argv); i++ {
		id, err := stream.ParseID(string(argv[i]), stream.ZeroID)
		if err == nil {
			ids = append(ids, id)
		}
	}

	acked := s.Ack(groupName, ids)
	return Integer(acked)
}

func xpendingCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	groupName := string(argv[2])

	s, errReply := getOrCreateStream(ctx, key, false)
	if errReply != nil {
		return errReply
	}
	if s == nil {
		return Array(nil)
	}

	startID := stream.ZeroID
	endID := stream.MaxID
	count := 10
	consumerFilter := ""

	if len(argv) >= 6 {
		sID, err := stream.ParseID(string(argv[3]), stream.ZeroID)
		if err == nil {
			startID = sID
		}
		eID, err := stream.ParseID(string(argv[4]), stream.MaxID)
		if err == nil {
			endID = eID
		}
		c, err := strconv.Atoi(string(argv[5]))
		if err == nil {
			count = c
		}
	}
	if len(argv) >= 7 {
		consumerFilter = string(argv[6])
	}

	entries := s.Pending(groupName, startID, endID, count, consumerFilter)
	replies := make([][]byte, len(entries))
	for i, e := range entries {
		replies[i] = Array([][]byte{
			BulkString([]byte(e.ID.String())),
			BulkString([]byte(e.ConsumerName)),
			Integer(e.IdleTimeMs),
			Integer(int64(e.DeliveryCount)),
		})
	}

	return Array(replies)
}

func xclaimCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	groupName := string(argv[2])
	consumerName := string(argv[3])
	minIdleMs, err := strconv.ParseInt(string(argv[4]), 10, 64)
	if err != nil {
		return Error("value is not an integer or out of range")
	}

	s, errReply := getOrCreateStream(ctx, key, false)
	if errReply != nil {
		return errReply
	}
	if s == nil {
		return Array(nil)
	}

	ids := make([]stream.StreamID, 0, len(argv)-5)
	for i := 5; i < len(argv); i++ {
		id, err := stream.ParseID(string(argv[i]), stream.ZeroID)
		if err == nil {
			ids = append(ids, id)
		}
	}

	claimed := s.Claim(groupName, consumerName, minIdleMs, ids)
	return formatStreamEntries(claimed)
}
