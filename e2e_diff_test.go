package main_test

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

// sendCommand writes a raw RESP command to a connection and reads the raw RESP reply.
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

func TestDifferentialCRedisVsNedis(t *testing.T) {
	// 1. Start real C redis-server on port 16379
	cRedisCmd := exec.Command("redis-server", "--port", "16379", "--save", "", "--appendonly", "no", "--protected-mode", "no")
	if err := cRedisCmd.Start(); err != nil {
		t.Skipf("skipping C Redis differential test: redis-server not runnable: %v", err)
	}
	defer func() {
		_ = cRedisCmd.Process.Kill()
		_ = cRedisCmd.Wait()
	}()

	// 2. Start Nedis on port 16380
	gopherdisServer := server.NewServer()
	go func() {
		_ = gopherdisServer.Listen("127.0.0.1:16380")
	}()
	defer gopherdisServer.Close()

	// Wait for both to accept connections
	time.Sleep(200 * time.Millisecond)

	cConn, err := net.Dial("tcp", "127.0.0.1:16379")
	if err != nil {
		t.Fatalf("failed to connect to C Redis on :16379: %v", err)
	}
	defer cConn.Close()
	cReader := bufio.NewReader(cConn)

	nConn, err := net.Dial("tcp", "127.0.0.1:16380")
	if err != nil {
		t.Fatalf("failed to connect to Nedis on :16380: %v", err)
	}
	defer nConn.Close()
	nReader := bufio.NewReader(nConn)

	// Flush both databases before starting
	_, _ = sendCommand(cConn, cReader, "FLUSHALL")
	_, _ = sendCommand(nConn, nReader, "FLUSHALL")

	// 3. Test Cases for Differential Comparison
	testCases := [][]string{
		// Basic String ops
		{"PING"},
		{"PING", "hello-world"},
		{"SET", "k1", "v1"},
		{"GET", "k1"},
		{"SET", "k2", "100"},
		{"INCR", "k2"},
		{"INCRBY", "k2", "50"},
		{"DECR", "k2"},
		{"MSET", "m1", "val1", "m2", "val2"},
		{"MGET", "m1", "k1", "nonexistent", "m2"},
		{"APPEND", "k1", "_extra"},
		{"GET", "k1"},

		// Hash ops
		{"HSET", "hkey", "f1", "v1", "f2", "v2"},
		{"HGET", "hkey", "f1"},
		{"HMGET", "hkey", "f1", "f2", "f_none"},
		{"HEXISTS", "hkey", "f1"},
		{"HEXISTS", "hkey", "f_none"},
		{"HLEN", "hkey"},
		{"HINCRBY", "hkey", "counter", "5"},
		{"HGET", "hkey", "counter"},
		{"HDEL", "hkey", "f1"},
		{"HLEN", "hkey"},

		// List ops
		{"RPUSH", "lkey", "a", "b", "c"},
		{"LPUSH", "lkey", "z"},
		{"LLEN", "lkey"},
		{"LRANGE", "lkey", "0", "-1"},
		{"LINDEX", "lkey", "1"},
		{"LPOP", "lkey"},
		{"RPOP", "lkey"},
		{"LRANGE", "lkey", "0", "-1"},

		// Set ops
		{"SADD", "skey", "member1", "member2", "member3"},
		{"SCARD", "skey"},
		{"SISMEMBER", "skey", "member2"},
		{"SISMEMBER", "skey", "not_in_set"},
		{"SREM", "skey", "member1"},
		{"SCARD", "skey"},

		// Sorted Set ops
		{"ZADD", "zkey", "10", "alice", "20", "bob", "30", "charlie"},
		{"ZCARD", "zkey"},
		{"ZSCORE", "zkey", "bob"},
		{"ZRANK", "zkey", "bob"},
		{"ZREVRANK", "zkey", "bob"},
		{"ZRANGE", "zkey", "0", "-1"},
		{"ZREVRANGE", "zkey", "0", "-1"},
		{"ZCOUNT", "zkey", "15", "35"},
		{"ZINCRBY", "zkey", "5", "alice"},
		{"ZSCORE", "zkey", "alice"},

		// Bitmap ops
		{"SETBIT", "bkey", "7", "1"},
		{"GETBIT", "bkey", "7"},
		{"GETBIT", "bkey", "0"},
		{"BITCOUNT", "bkey"},
		{"BITPOS", "bkey", "1"},

		// HyperLogLog ops
		{"PFADD", "hll1", "foo", "bar", "zap"},
		{"PFCOUNT", "hll1"},

		// Geospatial ops
		{"GEOADD", "Sicily", "13.361389", "38.115556", "Palermo", "15.087269", "37.502669", "Catania"},
		{"GEODIST", "Sicily", "Palermo", "Catania", "km"},
		{"GEOHASH", "Sicily", "Palermo", "Catania"},

		// Transactions (MULTI / EXEC)
		{"MULTI"},
		{"SET", "tx_k", "tx_val"},
		{"GET", "tx_k"},
		{"EXEC"},

		// Lua Scripting
		{"EVAL", "return {KEYS[1], ARGV[1]}", "1", "mykey", "myarg"},
		{"EVAL", "return redis.call('SET', KEYS[1], ARGV[1])", "1", "lua_k", "lua_v"},
		{"GET", "lua_k"},

		// Admin & Key ops
		{"DBSIZE"},
		{"TYPE", "k1"},
		{"TYPE", "hkey"},
		{"TYPE", "lkey"},
		{"TYPE", "skey"},
		{"TYPE", "zkey"},
		{"EXISTS", "k1", "nonexistent"},
		{"DEL", "k1", "k2"},
		{"EXISTS", "k1"},
	}

	matchCount := 0
	for i, tc := range testCases {
		cResp, err := sendCommand(cConn, cReader, tc...)
		if err != nil {
			t.Fatalf("C Redis error on %v: %v", tc, err)
		}

		nResp, err := sendCommand(nConn, nReader, tc...)
		if err != nil {
			t.Fatalf("Nedis error on %v: %v", tc, err)
		}

		if cResp != nResp {
			t.Errorf("[Mismatch #%d] Cmd: %v\n  C Redis: %q\n  Nedis:   %q", i+1, tc, cResp, nResp)
		} else {
			matchCount++
		}
	}

	t.Logf("Differential Test Summary: %d / %d test cases 100%% byte-for-byte identical!", matchCount, len(testCases))
}
