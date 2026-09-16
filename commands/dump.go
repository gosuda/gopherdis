package commands

import (
	"bytes"
	"encoding/binary"
	"strconv"
	"strings"
	"time"

	"github.com/gosuda/gopherdis/db"
	"github.com/gosuda/gopherdis/rdb"
)

func init() {
	reg := func(name string, h CommandHandler, arity int, flags CommandFlags) {
		DefaultTable.Register(&Command{Name: name, Handler: h, Arity: arity, Flags: flags})
	}
	reg("dump", dumpCommand, 2, FlagReadOnly)
	reg("restore", restoreCommand, -4, FlagWrite)
}

// dumpFooterLen is the trailing 2 byte format version plus 8 byte CRC64 that
// every serialized value carries, matching the shape of Redis' footer.
const dumpFooterLen = 10

// dumpVersion identifies the payload layout. RESTORE refuses anything else, so
// a payload from a future format cannot be silently misread.
const dumpVersion uint16 = 1

// dumpCommand serializes a value into a self describing, checksummed blob.
//
// The payload is the same entry encoding the RDB writer produces, so DUMP and
// the snapshot format stay in step, followed by a version and a CRC64 over
// everything before it.
func dumpCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	obj, expireAt, ok := ctx.DB.GetWithTTL(key)
	if !ok || obj == nil {
		return NullBulkString()
	}

	// The payload is a self contained mini RDB: header, the single entry, then
	// the RDB footer. RESTORE replays it through the ordinary decoder, so DUMP
	// and the snapshot format cannot drift apart.
	var body bytes.Buffer
	enc := rdb.NewEncoder(&body)
	if err := enc.WriteHeader(); err != nil {
		return Error("failed to serialize value")
	}
	if err := enc.WriteEntry(db.DBEntry{Key: key, Val: obj, ExpireAt: expireAt}); err != nil {
		return Error("failed to serialize value")
	}
	if err := enc.WriteFooter(); err != nil {
		return Error("failed to serialize value")
	}

	payload := body.Bytes()
	out := make([]byte, 0, len(payload)+dumpFooterLen)
	out = append(out, payload...)
	out = binary.LittleEndian.AppendUint16(out, dumpVersion)
	out = binary.LittleEndian.AppendUint64(out, rdb.CRC64(0, out))
	return BulkString(out)
}

func restoreCommand(ctx *Context, argv [][]byte) []byte {
	key := string(argv[1])
	ttlMs, err := strconv.ParseInt(string(argv[2]), 10, 64)
	if err != nil {
		return Error("value is not an integer or out of range")
	}
	if ttlMs < 0 {
		return Error("Invalid TTL value, must be >= 0")
	}
	payload := argv[3]

	replace, absTTL := false, false
	for i := 4; i < len(argv); i++ {
		switch strings.ToUpper(string(argv[i])) {
		case "REPLACE":
			replace = true
		case "ABSTTL":
			absTTL = true
		case "IDLETIME", "FREQ":
			if i+1 >= len(argv) {
				return Error("syntax error")
			}
			if _, err := strconv.ParseInt(string(argv[i+1]), 10, 64); err != nil {
				return Error("value is not an integer or out of range")
			}
			i++
		default:
			return Error("syntax error")
		}
	}

	if len(payload) < dumpFooterLen {
		return Error("DUMP payload version or checksum are wrong")
	}
	body := payload[:len(payload)-dumpFooterLen]
	verOff := len(payload) - dumpFooterLen
	version := binary.LittleEndian.Uint16(payload[verOff : verOff+2])
	want := binary.LittleEndian.Uint64(payload[verOff+2:])
	if version != dumpVersion || rdb.CRC64(0, payload[:verOff+2]) != want {
		return Error("DUMP payload version or checksum are wrong")
	}

	ctx.DB.LockKey(key)
	defer ctx.DB.UnlockKey(key)

	if !replace && ctx.DB.Exists(key) {
		return Error("BUSYKEY Target key name already exists.")
	}

	// Replay into a scratch database, then move the single value across. This
	// reuses the snapshot decoder rather than carrying a second parser.
	scratch := db.NewShardedDB()
	if err := rdb.NewDecoder(bytes.NewReader(body)).Load(scratch); err != nil {
		return Error("Bad data format")
	}
	names := scratch.Keys("*")
	if len(names) != 1 {
		return Error("Bad data format")
	}
	restored, _ := scratch.Get(names[0])
	if restored == nil {
		return Error("Bad data format")
	}

	if err := ctx.DB.Set(key, restored); err != nil {
		return Error(err.Error())
	}
	if ttlMs > 0 {
		if absTTL {
			ctx.DB.SetExpireAt(key, ttlMs)
		} else {
			ctx.DB.SetExpire(key, time.Duration(ttlMs)*time.Millisecond)
		}
	}
	return OK()
}
