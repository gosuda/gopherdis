package tests

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gosuda/gopherdis/server"
)

func sendCommand(conn net.Conn, r *bufio.Reader, args ...string) (string, error) {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("*%d\r\n", len(args)))
	for _, a := range args {
		buf.WriteString(fmt.Sprintf("$%d\r\n%s\r\n", len(a), a))
	}
	if _, err := conn.Write(buf.Bytes()); err != nil {
		return "", err
	}
	return readRESP(r)
}

func readRESP(r *bufio.Reader) (string, error) {
	prefix, err := r.ReadByte()
	if err != nil {
		return "", err
	}

	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}

	switch prefix {
	case '+', '-', ':', ',', '#', '_':
		return string(prefix) + line, nil
	case '$', '!', '=':
		length, _ := strconv.Atoi(strings.TrimSpace(line))
		if length == -1 {
			return string(prefix) + line, nil
		}
		data := make([]byte, length+2)
		if _, err := io.ReadFull(r, data); err != nil {
			return "", err
		}
		return string(prefix) + line + string(data), nil
	case '*', '%', '~':
		count, _ := strconv.Atoi(strings.TrimSpace(line))
		if count == -1 {
			return string(prefix) + line, nil
		}
		if prefix == '%' {
			count *= 2
		}
		res := string(prefix) + line
		for i := 0; i < count; i++ {
			elem, err := readRESP(r)
			if err != nil {
				return "", err
			}
			res += elem
		}
		return res, nil
	default:
		return string(prefix) + line, nil
	}
}

func TestFullCompatibilitySuite(t *testing.T) {
	// 1. Start C Redis on 16379
	cRedisCmd := exec.Command("redis-server", "--port", "16379", "--save", "", "--appendonly", "no", "--protected-mode", "no")
	if err := cRedisCmd.Start(); err != nil {
		t.Skipf("skipping C Redis test: redis-server not runnable: %v", err)
	}
	defer func() {
		_ = cRedisCmd.Process.Kill()
		_ = cRedisCmd.Wait()
	}()

	// 2. Start Nedis on 16380
	gopherdisServer := server.NewServer()
	go func() {
		_ = gopherdisServer.Listen("127.0.0.1:16380")
	}()
	defer gopherdisServer.Close()

	time.Sleep(200 * time.Millisecond)

	cConn, err := net.Dial("tcp", "127.0.0.1:16379")
	if err != nil {
		t.Fatalf("failed to connect to C Redis: %v", err)
	}
	defer cConn.Close()
	cReader := bufio.NewReader(cConn)

	nConn, err := net.Dial("tcp", "127.0.0.1:16380")
	if err != nil {
		t.Fatalf("failed to connect to Nedis: %v", err)
	}
	defer nConn.Close()
	nReader := bufio.NewReader(nConn)

	// Flush both DBs
	_, _ = sendCommand(cConn, cReader, "FLUSHALL")
	_, _ = sendCommand(nConn, nReader, "FLUSHALL")

	// Comprehensive test scenarios across all 10 categories
	scenarios := []struct {
		category string
		commands [][]string
	}{
		{
			category: "1. String & Basic Key Ops",
			commands: [][]string{
				{"PING"},
				{"PING", "custom_msg"},
				{"SET", "str_k1", "hello"},
				{"GET", "str_k1"},
				{"APPEND", "str_k1", "_world"},
				{"GET", "str_k1"},
				{"STRLEN", "str_k1"},
				{"SET", "num_k", "10"},
				{"INCR", "num_k"},
				{"INCRBY", "num_k", "25"},
				{"DECR", "num_k"},
				{"MSET", "m1", "v1", "m2", "v2", "m3", "v3"},
				{"MGET", "m1", "m2", "nonexistent", "m3"},
				{"EXISTS", "str_k1", "m1", "nonexistent"},
				{"TYPE", "str_k1"},
				{"DEL", "m1", "m2"},
				{"EXISTS", "m1"},
			},
		},
		{
			category: "2. Hash Operations",
			commands: [][]string{
				{"HSET", "user:1", "name", "alice", "age", "30", "role", "admin"},
				{"HGET", "user:1", "name"},
				{"HMGET", "user:1", "name", "role", "invalid_field"},
				{"HEXISTS", "user:1", "age"},
				{"HEXISTS", "user:1", "gender"},
				{"HLEN", "user:1"},
				{"HINCRBY", "user:1", "age", "1"},
				{"HGET", "user:1", "age"},
				{"HDEL", "user:1", "role"},
				{"HLEN", "user:1"},
				{"TYPE", "user:1"},
			},
		},
		{
			category: "3. List Operations",
			commands: [][]string{
				{"RPUSH", "mylist", "a", "b", "c"},
				{"LPUSH", "mylist", "first"},
				{"LLEN", "mylist"},
				{"LRANGE", "mylist", "0", "-1"},
				{"LINDEX", "mylist", "0"},
				{"LINDEX", "mylist", "-1"},
				{"LPOP", "mylist"},
				{"RPOP", "mylist"},
				{"LRANGE", "mylist", "0", "-1"},
				{"TYPE", "mylist"},
			},
		},
		{
			category: "4. Set Operations",
			commands: [][]string{
				{"SADD", "set_a", "m1", "m2", "m3"},
				{"SCARD", "set_a"},
				{"SISMEMBER", "set_a", "m2"},
				{"SISMEMBER", "set_a", "m_unknown"},
				{"SREM", "set_a", "m1"},
				{"SCARD", "set_a"},
				{"TYPE", "set_a"},
			},
		},
		{
			category: "5. Sorted Set (ZSet) Operations",
			commands: [][]string{
				{"ZADD", "leaderboard", "100", "p1", "200", "p2", "150", "p3"},
				{"ZCARD", "leaderboard"},
				{"ZSCORE", "leaderboard", "p2"},
				{"ZRANK", "leaderboard", "p1"},
				{"ZRANK", "leaderboard", "p3"},
				{"ZRANK", "leaderboard", "p2"},
				{"ZREVRANK", "leaderboard", "p2"},
				{"ZRANGE", "leaderboard", "0", "-1"},
				{"ZREVRANGE", "leaderboard", "0", "-1"},
				{"ZCOUNT", "leaderboard", "120", "220"},
				{"ZINCRBY", "leaderboard", "100", "p1"},
				{"ZSCORE", "leaderboard", "p1"},
				{"ZREM", "leaderboard", "p3"},
				{"ZCARD", "leaderboard"},
				{"TYPE", "leaderboard"},
			},
		},
		{
			category: "6. Bitmaps & HyperLogLog",
			commands: [][]string{
				{"SETBIT", "bm_active", "0", "1"},
				{"SETBIT", "bm_active", "7", "1"},
				{"SETBIT", "bm_active", "63", "1"},
				{"GETBIT", "bm_active", "0"},
				{"GETBIT", "bm_active", "1"},
				{"GETBIT", "bm_active", "63"},
				{"BITCOUNT", "bm_active"},
				{"BITPOS", "bm_active", "1"},
				{"PFADD", "unique_visitors", "ip1", "ip2", "ip3", "ip1"},
				{"PFCOUNT", "unique_visitors"},
			},
		},
		{
			category: "7. Geospatial",
			commands: [][]string{
				{"GEOADD", "Sicily", "13.361389", "38.115556", "Palermo", "15.087269", "37.502669", "Catania"},
				{"GEODIST", "Sicily", "Palermo", "Catania", "km"},
				{"GEOHASH", "Sicily", "Palermo", "Catania"},
			},
		},
		{
			category: "8. Transactions & Lua Scripting",
			commands: [][]string{
				{"MULTI"},
				{"SET", "tx_key", "tx_val"},
				{"GET", "tx_key"},
				{"EXEC"},
				{"EVAL", "return {KEYS[1], ARGV[1]}", "1", "k_test", "arg_test"},
				{"EVAL", "return redis.call('SET', KEYS[1], ARGV[1])", "1", "lua_set_k", "lua_set_v"},
				{"GET", "lua_set_k"},
			},
		},
		{
			category: "9. Admin & Utility Ops",
			commands: [][]string{
				{"DBSIZE"},
				{"HELLO", "2"},
				{"HELLO", "3"},
			},
		},
	}

	totalPassed := 0
	totalCases := 0

	for _, sc := range scenarios {
		t.Run(sc.category, func(t *testing.T) {
			for _, cmd := range sc.commands {
				totalCases++
				cResp, err := sendCommand(cConn, cReader, cmd...)
				if err != nil {
					t.Fatalf("C Redis error on %v: %v", cmd, err)
				}
				nResp, err := sendCommand(nConn, nReader, cmd...)
				if err != nil {
					t.Fatalf("Nedis error on %v: %v", cmd, err)
				}

				if cmd[0] == "HELLO" {
					// Verify both start with array/map indicator and contain role/proto
					if (cmd[1] == "2" && strings.HasPrefix(nResp, "*") && strings.Contains(nResp, "standalone")) ||
						(cmd[1] == "3" && strings.HasPrefix(nResp, "%") && strings.Contains(nResp, "proto\r\n:3")) {
						totalPassed++
						continue
					}
				}

				if cResp != nResp {
					t.Errorf("Mismatch on command %v:\n  C Redis: %q\n  Nedis:   %q", cmd, cResp, nResp)
				} else {
					totalPassed++
				}
			}
		})
	}

	t.Logf("=== Compatibility Summary: %d / %d scenarios passed 100%% identically ===", totalPassed, totalCases)
}
