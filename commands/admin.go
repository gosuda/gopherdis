package commands

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gosuda/gopherdis/db"
	"github.com/gosuda/gopherdis/encoding"
)

var (
	serverStartTime = time.Now()
	slowlogMu       sync.RWMutex
	slowlogEntries  []SlowlogEntry
	slowlogMaxLen   = 128
	slowlogIDSeq    atomic.Int64

	configMu sync.RWMutex
	configs  = map[string]string{
		"maxmemory":               "0",
		"maxmemory-policy":        "noeviction",
		"timeout":                 "0",
		"databases":               "16",
		"appendonly":              "no",
		"slowlog-log-slower-than": "10000",
		"slowlog-max-len":         "128",
		"proto-max-bulk-len":      "536870912",
	}
)

type SlowlogEntry struct {
	ID         int64
	Timestamp  int64
	Duration   int64 // microseconds
	Args       []string
	ClientIP   string
	ClientName string
}

func RecordSlowlog(durationUs int64, argv [][]byte, clientIP, clientName string) {
	thresholdUs := int64(10000)
	configMu.RLock()
	if thStr, ok := configs["slowlog-log-slower-than"]; ok {
		if th, err := strconv.ParseInt(thStr, 10, 64); err == nil {
			thresholdUs = th
		}
	}
	configMu.RUnlock()

	if thresholdUs < 0 || durationUs < thresholdUs {
		return
	}

	args := make([]string, len(argv))
	for i, a := range argv {
		args[i] = string(a)
	}

	entry := SlowlogEntry{
		ID:         slowlogIDSeq.Add(1),
		Timestamp:  time.Now().Unix(),
		Duration:   durationUs,
		Args:       args,
		ClientIP:   clientIP,
		ClientName: clientName,
	}

	slowlogMu.Lock()
	if len(slowlogEntries) >= slowlogMaxLen {
		slowlogEntries = slowlogEntries[1:]
	}
	slowlogEntries = append(slowlogEntries, entry)
	slowlogMu.Unlock()
}

func init() {
	DefaultTable.Register(&Command{
		Name:    "hello",
		Handler: helloCommand,
		Arity:   -1,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "info",
		Handler: infoCommand,
		Arity:   -1,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "time",
		Handler: timeCommand,
		Arity:   1,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "dbsize",
		Handler: dbsizeCommand,
		Arity:   1,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "flushdb",
		Handler: flushdbCommand,
		Arity:   -1,
		Flags:   FlagWrite,
	})
	// FLUSHALL was missing entirely, so clients got "unknown command" for one of
	// the commands the README claims support for.
	DefaultTable.Register(&Command{
		Name:    "flushall",
		Handler: flushdbCommand,
		Arity:   -1,
		Flags:   FlagWrite,
	})
	DefaultTable.Register(&Command{
		Name:    "client",
		Handler: clientCommand,
		Arity:   -2,
		Flags:   FlagAdmin,
	})
	DefaultTable.Register(&Command{
		Name:    "slowlog",
		Handler: slowlogCommand,
		Arity:   -2,
		Flags:   FlagAdmin,
	})
	DefaultTable.Register(&Command{
		Name:    "config",
		Handler: configCommand,
		Arity:   -2,
		Flags:   FlagAdmin,
	})
	DefaultTable.Register(&Command{
		Name:    "command",
		Handler: commandCommand,
		Arity:   -1,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "monitor",
		Handler: monitorCommand,
		Arity:   1,
		Flags:   FlagAdmin,
	})
}

func helloCommand(ctx *Context, argv [][]byte) []byte {
	proto := 2
	idx := 1

	if len(argv) >= 2 {
		ver, err := strconv.Atoi(string(argv[1]))
		if err != nil || (ver != 2 && ver != 3) {
			return Error("NOPROTO unsupported protocol version")
		}
		proto = ver
		idx = 2
	}
	// Record it: without this the handshake succeeds and every later reply is
	// still encoded as RESP2, which a RESP3 client cannot decode.
	if ctx != nil {
		ctx.Proto = proto
	}

	for idx < len(argv) {
		opt := strings.ToUpper(string(argv[idx]))
		if opt == "AUTH" && idx+2 < len(argv) {
			user := string(argv[idx+1])
			pass := string(argv[idx+2])
			if ctx.ACL != nil {
				u, err := ctx.ACL.Auth(user, pass)
				if err != nil {
					return Error("WRONGPASS invalid username-password pair or user is disabled")
				}
				ctx.User = u
			}
			idx += 3
		} else if opt == "SETNAME" && idx+1 < len(argv) {
			idx += 2
		} else {
			break
		}
	}

	role := "master"
	if ctx.Replication != nil && ctx.Replication.Role() == "slave" {
		role = "replica"
	}

	if proto == 3 {
		return []byte(fmt.Sprintf("%%7\r\n$6\r\nserver\r\n$9\r\ngopherdis\r\n$7\r\nversion\r\n$5\r\n7.2.0\r\n$5\r\nproto\r\n:3\r\n$2\r\nid\r\n:1\r\n$4\r\nmode\r\n$10\r\nstandalone\r\n$4\r\nrole\r\n$%d\r\n%s\r\n$7\r\nmodules\r\n*0\r\n", len(role), role))
	}

	return Array([][]byte{
		BulkString([]byte("server")),
		BulkString([]byte("gopherdis")),
		BulkString([]byte("version")),
		BulkString([]byte("7.2.0")),
		BulkString([]byte("proto")),
		Integer(int64(proto)),
		BulkString([]byte("id")),
		Integer(1),
		BulkString([]byte("mode")),
		BulkString([]byte("standalone")),
		BulkString([]byte("role")),
		BulkString([]byte(role)),
		BulkString([]byte("modules")),
		Array(nil),
	})
}

// procRSS reads the process resident set size from /proc/self/statm, matching
// how C Redis computes used_memory_rss. Falls back to MemStats.Sys on error.
func procRSS(m runtime.MemStats) uint64 {
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return m.Sys
	}
	// statm fields: size resident shared text lib data dt (in pages)
	var size, resident uint64
	if _, err := fmt.Sscanf(string(data), "%d %d", &size, &resident); err != nil {
		return m.Sys
	}
	return resident * uint64(os.Getpagesize())
}

func infoCommand(ctx *Context, argv [][]byte) []byte {
	section := "all"
	if len(argv) >= 2 {
		section = strings.ToLower(string(argv[1]))
	}

	var sb strings.Builder
	uptime := int64(time.Since(serverStartTime).Seconds())

	// Report the runtime's numbers as they are. Forcing runtime.GC() plus
	// debug.FreeOSMemory() here made every metrics scrape a stop-the-world pause
	// followed by a madvise sweep, so monitoring INFO degraded the server it was
	// measuring (and the reported figures did not describe steady state anyway).
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	keysCount := int64(0)
	if ctx != nil && ctx.DB != nil {
		keysCount = ctx.DB.Len()
	}

	role := "master"
	if ctx != nil && ctx.Replication != nil && ctx.Replication.Role() == "slave" {
		role = "slave"
	}

	if section == "all" || section == "server" || section == "default" {
		sb.WriteString("# Server\r\n")
		sb.WriteString("redis_version:7.2.0\r\n")
		sb.WriteString("redis_mode:standalone\r\n")
		sb.WriteString("os:linux\r\n")
		sb.WriteString(fmt.Sprintf("uptime_in_seconds:%d\r\n", uptime))
		sb.WriteString(fmt.Sprintf("uptime_in_days:%d\r\n", uptime/86400))
		sb.WriteString("tcp_port:6379\r\n")
	}

	if section == "all" || section == "clients" || section == "default" {
		sb.WriteString("# Clients\r\n")
		sb.WriteString("connected_clients:1\r\n")
		sb.WriteString("blocked_clients:0\r\n")
	}

	if section == "all" || section == "memory" || section == "default" {
		sb.WriteString("# Memory\r\n")
		sb.WriteString(fmt.Sprintf("used_memory:%d\r\n", m.Alloc))
		sb.WriteString(fmt.Sprintf("used_memory_human:%.2fM\r\n", float64(m.Alloc)/(1024*1024)))
		sb.WriteString(fmt.Sprintf("used_memory_rss:%d\r\n", procRSS(m)))
		sb.WriteString(fmt.Sprintf("used_memory_peak:%d\r\n", m.TotalAlloc))
		sb.WriteString(fmt.Sprintf("heap_sys:%d\r\n", m.HeapSys))
		sb.WriteString(fmt.Sprintf("heap_idle:%d\r\n", m.HeapIdle))
		sb.WriteString(fmt.Sprintf("heap_released:%d\r\n", m.HeapReleased))
	}

	if section == "all" || section == "persistence" || section == "default" {
		sb.WriteString("# Persistence\r\n")
		sb.WriteString("rdb_changes_since_last_save:0\r\n")
		sb.WriteString("rdb_last_bgsave_status:ok\r\n")
		sb.WriteString("aof_enabled:0\r\n")
	}

	if section == "all" || section == "stats" || section == "default" {
		sb.WriteString("# Stats\r\n")
		if ctx != nil && ctx.DB != nil {
			sb.WriteString(fmt.Sprintf("expired_keys:%d\r\n", ctx.DB.ExpiredKeys()))
		}
		sb.WriteString("total_connections_received:1\r\n")
		sb.WriteString("total_commands_processed:1\r\n")
		sb.WriteString("instantaneous_ops_per_sec:0\r\n")
	}

	if section == "all" || section == "replication" || section == "default" {
		sb.WriteString("# Replication\r\n")
		sb.WriteString(fmt.Sprintf("role:%s\r\n", role))
		sb.WriteString("connected_slaves:0\r\n")
		sb.WriteString("master_repl_offset:0\r\n")
	}

	if section == "all" || section == "cpu" || section == "default" {
		sb.WriteString("# CPU\r\n")
		sb.WriteString("used_cpu_sys:0.00\r\n")
		sb.WriteString("used_cpu_user:0.00\r\n")
	}

	if section == "all" || section == "cluster" || section == "default" {
		sb.WriteString("# Cluster\r\n")
		clusterEnabled := "0"
		if ctx != nil && ctx.Cluster != nil && ctx.Cluster.ClusterEnabled {
			clusterEnabled = "1"
		}
		sb.WriteString(fmt.Sprintf("cluster_enabled:%s\r\n", clusterEnabled))
	}

	if section == "all" || section == "keyspace" || section == "default" {
		sb.WriteString("# Keyspace\r\n")
		sb.WriteString(fmt.Sprintf("db0:keys=%d,expires=0,avg_ttl=0\r\n", keysCount))
	}

	return BulkString([]byte(sb.String()))
}

func timeCommand(ctx *Context, argv [][]byte) []byte {
	now := time.Now()
	sec := now.Unix()
	micro := now.Nanosecond() / 1000

	return Array([][]byte{
		BulkString([]byte(strconv.FormatInt(sec, 10))),
		BulkString([]byte(strconv.Itoa(micro))),
	})
}

func dbsizeCommand(ctx *Context, argv [][]byte) []byte {
	if ctx == nil || ctx.DB == nil {
		return Integer(0)
	}
	return Integer(ctx.DB.Len())
}

func flushdbCommand(ctx *Context, argv [][]byte) []byte {
	if ctx != nil && ctx.DB != nil {
		ctx.DB.FlushAll()
	}
	return OK()
}

func clientCommand(ctx *Context, argv [][]byte) []byte {
	subCmd := strings.ToUpper(string(argv[1]))

	switch subCmd {
	case "LIST":
		user := "default"
		if ctx.User != nil {
			user = ctx.User.Name
		}
		line := fmt.Sprintf("id=1 addr=127.0.0.1:0 fd=6 name= age=10 idle=0 flags=N db=0 sub=0 psub=0 multi=-1 qbuf=0 qbuf-free=20474 argv-mem=0 obl=0 oll=0 omem=0 events=r cmd=client user=%s redir=-1 resp=2\n", user)
		return BulkString([]byte(line))

	case "ID":
		return Integer(1)

	case "SETNAME":
		return OK()

	case "GETNAME":
		return NullBulkString()

	case "KILL":
		return OK()

	case "PAUSE", "UNPAUSE":
		return OK()

	default:
		return Error(fmt.Sprintf("unknown subcommand '%s'", subCmd))
	}
}

func slowlogCommand(ctx *Context, argv [][]byte) []byte {
	subCmd := strings.ToUpper(string(argv[1]))

	switch subCmd {
	case "GET":
		slowlogMu.RLock()
		defer slowlogMu.RUnlock()

		max := len(slowlogEntries)
		if len(argv) >= 3 {
			if count, err := strconv.Atoi(string(argv[2])); err == nil && count < max {
				max = count
			}
		}

		replies := make([][]byte, 0, max)
		// Return entries in reverse chronological order
		for i := len(slowlogEntries) - 1; i >= 0 && len(replies) < max; i-- {
			e := slowlogEntries[i]
			argsArray := make([][]byte, len(e.Args))
			for j, a := range e.Args {
				argsArray[j] = BulkString([]byte(a))
			}

			entryArray := [][]byte{
				Integer(e.ID),
				Integer(e.Timestamp),
				Integer(e.Duration),
				Array(argsArray),
				BulkString([]byte(e.ClientIP)),
				BulkString([]byte(e.ClientName)),
			}
			replies = append(replies, Array(entryArray))
		}
		return Array(replies)

	case "LEN":
		slowlogMu.RLock()
		defer slowlogMu.RUnlock()
		return Integer(int64(len(slowlogEntries)))

	case "RESET":
		slowlogMu.Lock()
		slowlogEntries = make([]SlowlogEntry, 0, slowlogMaxLen)
		slowlogMu.Unlock()
		return OK()

	default:
		return Error(fmt.Sprintf("unknown subcommand '%s'", subCmd))
	}
}

// parseMemoryValue parses a maxmemory value, accepting a plain byte count or the
// usual "100mb" / "1gb" suffixes that redis-cli sends.
func parseMemoryValue(val string) (int64, error) {
	v := strings.TrimSpace(strings.ToLower(val))
	mult := int64(1)
	for _, suffix := range []struct {
		name string
		mult int64
	}{
		{"kb", 1024}, {"mb", 1024 * 1024}, {"gb", 1024 * 1024 * 1024},
		{"k", 1000}, {"m", 1000 * 1000}, {"g", 1000 * 1000 * 1000},
		{"b", 1},
	} {
		if strings.HasSuffix(v, suffix.name) {
			mult = suffix.mult
			v = strings.TrimSpace(strings.TrimSuffix(v, suffix.name))
			break
		}
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid memory value")
	}
	return n * mult, nil
}

func configCommand(ctx *Context, argv [][]byte) []byte {
	subCmd := strings.ToUpper(string(argv[1]))

	switch subCmd {
	case "GET":
		if len(argv) < 3 {
			return Error("wrong number of arguments for 'config get' command")
		}
		pattern := strings.ToLower(string(argv[2]))
		configMu.RLock()
		defer configMu.RUnlock()

		var replies [][]byte
		for k, v := range configs {
			if pattern == "*" || pattern == k || strings.Contains(k, pattern) {
				replies = append(replies, BulkString([]byte(k)), BulkString([]byte(v)))
			}
		}
		// The encoding thresholds live in the encoding package, so report them
		// from there rather than from a stale copy in the configs map.
		for _, k := range encoding.Names() {
			if pattern == "*" || pattern == k || strings.Contains(k, pattern) {
				if v, ok := encoding.Get(k); ok {
					replies = append(replies, BulkString([]byte(k)), BulkString([]byte(strconv.FormatInt(v, 10))))
				}
			}
		}
		return MapReply(ctx, replies)

	case "SET":
		if len(argv) < 4 {
			return Error("wrong number of arguments for 'config set' command")
		}
		param := strings.ToLower(string(argv[2]))
		val := string(argv[3])

		// Encoding thresholds drive real conversions, so they have to reach the
		// encoding package rather than only landing in the configs map.
		if n, err := strconv.ParseInt(val, 10, 64); err == nil {
			if encoding.Set(param, n) {
				configMu.Lock()
				configs[param] = val
				configMu.Unlock()
				return OK()
			}
		}

		// Parameters that back a live setting have to be applied, not just
		// recorded; CONFIG SET maxmemory used to return +OK and change nothing.
		if ctx != nil && ctx.DB != nil {
			switch param {
			case "maxmemory":
				bytesVal, err := parseMemoryValue(val)
				if err != nil {
					return Error("Invalid argument '" + val + "' for CONFIG SET 'maxmemory'")
				}
				ctx.DB.SetMaxMemory(bytesVal)
			case "maxmemory-policy":
				ctx.DB.SetEvictionPolicy(db.ParseEvictionPolicy(val))
			}
		}

		configMu.Lock()
		configs[param] = val
		configMu.Unlock()
		return OK()

	case "RESETSTAT", "REWRITE":
		return OK()

	default:
		return Error(fmt.Sprintf("unknown subcommand '%s'", subCmd))
	}
}

func commandCommand(ctx *Context, argv [][]byte) []byte {
	if len(argv) >= 2 {
		subCmd := strings.ToUpper(string(argv[1]))
		if subCmd == "COUNT" {
			return Integer(int64(DefaultTable.Count()))
		}
	}
	// Return list of command names
	names := DefaultTable.AllNames()
	replies := make([][]byte, len(names))
	for i, name := range names {
		replies[i] = BulkString([]byte(name))
	}
	return Array(replies)
}

func monitorCommand(ctx *Context, argv [][]byte) []byte {
	return OK()
}
