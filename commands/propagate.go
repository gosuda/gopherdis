package commands

import (
	"bytes"
	"strconv"
	"strings"
	"time"
)

// normalizeForPropagation rewrites a write command into the form that is safe
// to replay later, on a replica or from the AOF.
//
// Every relative TTL becomes an absolute one. "SET k v EX 200" replayed an hour
// after it ran would give the key another 200 seconds of life, so Redis
// propagates "SET k v PXAT <deadline>" instead, and the same applies to SETEX,
// PSETEX, GETEX, EXPIRE and friends. An expiry that has already passed is
// propagated as a DEL, because the replica must delete rather than resurrect.
//
// Returning nil means the command propagates nothing.
func normalizeForPropagation(cmdName string, argv [][]byte, reply []byte) [][]byte {
	now := time.Now().UnixMilli()

	switch cmdName {
	case "expire", "pexpire", "expireat", "pexpireat":
		if !bytes.Equal(reply, []byte(":1\r\n")) || len(argv) < 3 {
			return argv
		}
		n, err := strconv.ParseInt(string(argv[2]), 10, 64)
		if err != nil {
			return argv
		}
		var absMs int64
		switch cmdName {
		case "expire":
			absMs = now + n*1000
		case "pexpire":
			absMs = now + n
		case "expireat":
			absMs = n * 1000
		default:
			absMs = n
		}
		if absMs <= now {
			return [][]byte{[]byte("DEL"), argv[1]}
		}
		return [][]byte{[]byte("PEXPIREAT"), argv[1], []byte(strconv.FormatInt(absMs, 10))}

	case "setex", "psetex":
		if len(argv) < 4 {
			return argv
		}
		n, err := strconv.ParseInt(string(argv[2]), 10, 64)
		if err != nil {
			return argv
		}
		absMs := now + n*1000
		if cmdName == "psetex" {
			absMs = now + n
		}
		return [][]byte{
			[]byte("SET"), argv[1], argv[3],
			[]byte("PXAT"), []byte(strconv.FormatInt(absMs, 10)),
		}

	case "set":
		return normalizeSet(argv, now)

	case "getex":
		return normalizeGetex(argv, now)

	case "getdel":
		// The value was removed, so the replica needs a DEL. Propagating GETDEL
		// would work too, but Redis normalises it and the two must agree.
		if bytes.Equal(reply, []byte("$-1\r\n")) || bytes.Equal(reply, []byte("_\r\n")) {
			return nil // nothing was deleted
		}
		return [][]byte{[]byte("DEL"), argv[1]}

	case "restore":
		return normalizeRestore(argv, now)

	default:
		return argv
	}
}

// normalizeSet replaces any relative expiry option on SET with PXAT.
func normalizeSet(argv [][]byte, now int64) [][]byte {
	out := make([][]byte, 0, len(argv))
	out = append(out, argv[0], argv[1], argv[2])

	rewritten := false
	for i := 3; i < len(argv); i++ {
		opt := strings.ToUpper(string(argv[i]))
		switch opt {
		case "PXAT":
			// Already absolute: pass the caller's own tokens through, so the
			// propagated form is byte for byte what was executed.
			if i+1 >= len(argv) {
				return argv
			}
			out = append(out, argv[i], argv[i+1])
			rewritten = true
			i++
		case "EX", "PX", "EXAT":
			if i+1 >= len(argv) {
				return argv
			}
			n, err := strconv.ParseInt(string(argv[i+1]), 10, 64)
			if err != nil {
				return argv
			}
			var absMs int64
			switch opt {
			case "EX":
				absMs = now + n*1000
			case "PX":
				absMs = now + n
			default:
				absMs = n * 1000
			}
			out = append(out, []byte("PXAT"), []byte(strconv.FormatInt(absMs, 10)))
			rewritten = true
			i++
		case "NX", "XX":
			// The outcome is already decided, so the conditional must not be
			// re-evaluated on the replica.
		case "GET":
			// Affects only the reply.
		default:
			out = append(out, argv[i])
		}
	}
	if !rewritten {
		// Nothing to rewrite, but NX/XX/GET still have to be dropped.
		return out
	}
	return out
}

// normalizeGetex propagates the expiry change GETEX made, not GETEX itself.
func normalizeGetex(argv [][]byte, now int64) [][]byte {
	for i := 2; i < len(argv); i++ {
		opt := strings.ToUpper(string(argv[i]))
		if opt == "PERSIST" {
			return [][]byte{[]byte("PERSIST"), argv[1]}
		}
		if i+1 >= len(argv) {
			break
		}
		n, err := strconv.ParseInt(string(argv[i+1]), 10, 64)
		if err != nil {
			break
		}
		var absMs int64
		switch opt {
		case "EX":
			absMs = now + n*1000
		case "PX":
			absMs = now + n
		case "EXAT":
			absMs = n * 1000
		case "PXAT":
			absMs = n
		default:
			return nil
		}
		if absMs <= now {
			// The key is already gone, so the replica must delete rather than
			// be told to expire it at a time in the past.
			return [][]byte{[]byte("DEL"), argv[1]}
		}
		return [][]byte{[]byte("PEXPIREAT"), argv[1], []byte(strconv.FormatInt(absMs, 10))}
	}
	// A plain GETEX changes nothing, so it propagates nothing.
	return nil
}

// normalizeRestore turns a relative TTL into an absolute one plus ABSTTL.
func normalizeRestore(argv [][]byte, now int64) [][]byte {
	if len(argv) < 4 {
		return argv
	}
	for i := 4; i < len(argv); i++ {
		if strings.EqualFold(string(argv[i]), "ABSTTL") {
			return argv // already absolute
		}
	}
	ttl, err := strconv.ParseInt(string(argv[2]), 10, 64)
	if err != nil || ttl == 0 {
		return argv
	}
	out := make([][]byte, 0, len(argv)+1)
	out = append(out, argv[0], argv[1], []byte(strconv.FormatInt(now+ttl, 10)))
	out = append(out, argv[3:]...)
	out = append(out, []byte("ABSTTL"))
	return out
}

// serverDelVerb is the verb a server-side delete propagates.
//
// UNLINK and DEL do the same thing here, since values are freed by the Go
// collector either way, but the verb a replica receives has to follow
// lazyfree-lazy-server-del the way it does upstream.
func serverDelVerb() []byte {
	configMu.RLock()
	v := configs["lazyfree-lazy-server-del"]
	configMu.RUnlock()
	if strings.EqualFold(v, "yes") {
		return []byte("UNLINK")
	}
	return []byte("DEL")
}
