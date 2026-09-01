package server

import (
	"bufio"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gosuda/nedis/aof"
	"github.com/gosuda/nedis/db"
	"github.com/gosuda/nedis/rdb"
)

func TestServerEndToEnd(t *testing.T) {
	srv := NewServer()
	go func() {
		_ = srv.Listen("127.0.0.1:0")
	}()
	defer srv.Close()

	// Wait for listener to bind
	var addr net.Addr
	for i := 0; i < 20; i++ {
		addr = srv.Addr()
		if addr != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if addr == nil {
		t.Fatalf("server failed to bind")
	}

	conn, err := net.Dial("tcp", addr.String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// Test PING
	conn.Write([]byte("PING\r\n"))
	line, _ := reader.ReadString('\n')
	if strings.TrimSpace(line) != "+PONG" {
		t.Fatalf("expected +PONG, got %q", line)
	}

	// Test SET
	conn.Write([]byte("*3\r\n$3\r\nSET\r\n$3\r\nfoo\r\n$3\r\nbar\r\n"))
	line, _ = reader.ReadString('\n')
	if strings.TrimSpace(line) != "+OK" {
		t.Fatalf("expected +OK, got %q", line)
	}

	// Test GET
	conn.Write([]byte("*2\r\n$3\r\nGET\r\n$3\r\nfoo\r\n"))
	lenLine, _ := reader.ReadString('\n')
	valLine, _ := reader.ReadString('\n')
	if strings.TrimSpace(lenLine) != "$3" || strings.TrimSpace(valLine) != "bar" {
		t.Fatalf("expected $3\\r\\nbar\\r\\n, got %q %q", lenLine, valLine)
	}

	// Test DEL
	conn.Write([]byte("*2\r\n$3\r\nDEL\r\n$3\r\nfoo\r\n"))
	line, _ = reader.ReadString('\n')
	if strings.TrimSpace(line) != ":1" {
		t.Fatalf("expected :1, got %q", line)
	}

	// Test GET after DEL
	conn.Write([]byte("*2\r\n$3\r\nGET\r\n$3\r\nfoo\r\n"))
	line, _ = reader.ReadString('\n')
	if strings.TrimSpace(line) != "$-1" {
		t.Fatalf("expected $-1, got %q", line)
	}
}

func TestServerAOFIntegration(t *testing.T) {
	tempDir := t.TempDir()
	aofPath := filepath.Join(tempDir, "server_test.aof")

	aofEngine, err := aof.OpenAOF(aofPath, aof.FsyncAlways)
	if err != nil {
		t.Fatalf("OpenAOF failed: %v", err)
	}

	srv := NewServer()
	srv.SetAOF(aofEngine)

	go func() {
		_ = srv.Listen("127.0.0.1:0")
	}()
	defer srv.Close()

	var addr net.Addr
	for i := 0; i < 20; i++ {
		addr = srv.Addr()
		if addr != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if addr == nil {
		t.Fatalf("server failed to bind")
	}

	conn, err := net.Dial("tcp", addr.String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// Send write commands
	conn.Write([]byte("*3\r\n$3\r\nSET\r\n$7\r\naof_key\r\n$7\r\naof_val\r\n"))
	line, _ := reader.ReadString('\n')
	if strings.TrimSpace(line) != "+OK" {
		t.Fatalf("expected +OK, got %q", line)
	}

	conn.Write([]byte("*3\r\n$6\r\nEXPIRE\r\n$7\r\naof_key\r\n$3\r\n100\r\n"))
	line, _ = reader.ReadString('\n')
	if strings.TrimSpace(line) != ":1" {
		t.Fatalf("expected :1, got %q", line)
	}

	_ = aofEngine.Close()

	// Replay AOF into fresh DB
	freshDB := db.NewShardedDB()
	loader, err := aof.OpenAOF(aofPath, aof.FsyncNo)
	if err != nil {
		t.Fatalf("open loader failed: %v", err)
	}
	defer loader.Close()

	if err := loader.Load(freshDB); err != nil {
		t.Fatalf("load failed: %v", err)
	}

	val, ok := freshDB.Get("aof_key")
	if !ok || val.String() != "aof_val" {
		t.Fatalf("expected aof_key=aof_val after AOF load")
	}

	ttl, code := freshDB.TTL("aof_key")
	if code != 0 || ttl <= 0 {
		t.Fatalf("expected positive TTL after AOF load, got code %d ttl %v", code, ttl)
	}
}

func TestServerRDBIntegration(t *testing.T) {
	tempDir := t.TempDir()
	rdbPath := filepath.Join(tempDir, "server_dump.rdb")

	rdbMgr := rdb.NewManager(rdbPath)

	srv := NewServer()
	srv.SetRDB(rdbMgr)

	go func() {
		_ = srv.Listen("127.0.0.1:0")
	}()
	defer srv.Close()

	var addr net.Addr
	for i := 0; i < 20; i++ {
		addr = srv.Addr()
		if addr != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if addr == nil {
		t.Fatalf("server failed to bind")
	}

	conn, err := net.Dial("tcp", addr.String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// Write data
	conn.Write([]byte("*3\r\n$3\r\nSET\r\n$7\r\nrdb_key\r\n$7\r\nrdb_val\r\n"))
	line, _ := reader.ReadString('\n')
	if strings.TrimSpace(line) != "+OK" {
		t.Fatalf("expected +OK, got %q", line)
	}

	// Trigger SAVE
	conn.Write([]byte("*1\r\n$4\r\nSAVE\r\n"))
	line, _ = reader.ReadString('\n')
	if strings.TrimSpace(line) != "+OK" {
		t.Fatalf("expected +OK on SAVE, got %q", line)
	}

	// Load RDB into a fresh DB
	freshDB := db.NewShardedDB()
	if err := rdbMgr.Load(freshDB); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	val, ok := freshDB.Get("rdb_key")
	if !ok || val.String() != "rdb_val" {
		t.Fatalf("expected rdb_key=rdb_val after RDB load, got %v", val)
	}
}

func TestServerPubSubIntegration(t *testing.T) {
	srv := NewServer()
	go func() {
		_ = srv.Listen("127.0.0.1:0")
	}()
	defer srv.Close()

	var addr net.Addr
	for i := 0; i < 20; i++ {
		addr = srv.Addr()
		if addr != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if addr == nil {
		t.Fatalf("server failed to bind")
	}

	// Subscriber client
	subConn, err := net.Dial("tcp", addr.String())
	if err != nil {
		t.Fatalf("failed to dial sub: %v", err)
	}
	defer subConn.Close()
	subReader := bufio.NewReader(subConn)

	// Send SUBSCRIBE
	subConn.Write([]byte("*2\r\n$9\r\nSUBSCRIBE\r\n$9\r\nnews.live\r\n"))
	line, _ := subReader.ReadString('\n')
	if strings.TrimSpace(line) != "*3" {
		t.Fatalf("expected *3 header, got %q", line)
	}
	// read rest of subscribe reply
	subReader.ReadString('\n') // $9
	subReader.ReadString('\n') // subscribe
	subReader.ReadString('\n') // $9
	subReader.ReadString('\n') // news.live
	subReader.ReadString('\n') // :1

	// Publisher client
	pubConn, err := net.Dial("tcp", addr.String())
	if err != nil {
		t.Fatalf("failed to dial pub: %v", err)
	}
	defer pubConn.Close()
	pubReader := bufio.NewReader(pubConn)

	// Send PUBLISH
	pubConn.Write([]byte("*3\r\n$7\r\nPUBLISH\r\n$9\r\nnews.live\r\n$11\r\nflash alert\r\n"))
	pubLine, _ := pubReader.ReadString('\n')
	if strings.TrimSpace(pubLine) != ":1" {
		t.Fatalf("expected :1 receiver from PUBLISH, got %q", pubLine)
	}

	// Verify subscriber received message
	subMsgLine, _ := subReader.ReadString('\n')
	if strings.TrimSpace(subMsgLine) != "*3" {
		t.Fatalf("expected *3 message header on sub, got %q", subMsgLine)
	}
}

func TestServerReplicationIntegration(t *testing.T) {
	// 1. Start Master Server
	masterSrv := NewServer()
	go func() {
		_ = masterSrv.Listen("127.0.0.1:0")
	}()
	defer masterSrv.Close()

	var masterAddr net.Addr
	for i := 0; i < 20; i++ {
		masterAddr = masterSrv.Addr()
		if masterAddr != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if masterAddr == nil {
		t.Fatalf("master failed to bind")
	}

	// Write initial data to master
	masterConn, err := net.Dial("tcp", masterAddr.String())
	if err != nil {
		t.Fatalf("failed to dial master: %v", err)
	}
	defer masterConn.Close()
	masterReader := bufio.NewReader(masterConn)

	masterConn.Write([]byte("*3\r\n$3\r\nSET\r\n$11\r\ninit_master\r\n$8\r\ninit_val\r\n"))
	line, _ := masterReader.ReadString('\n')
	if strings.TrimSpace(line) != "+OK" {
		t.Fatalf("expected +OK on master SET, got %q", line)
	}

	// 2. Start Replica Server
	replicaSrv := NewServer()
	go func() {
		_ = replicaSrv.Listen("127.0.0.1:0")
	}()
	defer replicaSrv.Close()

	var replicaAddr net.Addr
	for i := 0; i < 20; i++ {
		replicaAddr = replicaSrv.Addr()
		if replicaAddr != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if replicaAddr == nil {
		t.Fatalf("replica failed to bind")
	}

	// Tell Replica to replicate Master
	replicaConn, err := net.Dial("tcp", replicaAddr.String())
	if err != nil {
		t.Fatalf("failed to dial replica: %v", err)
	}
	defer replicaConn.Close()
	replicaReader := bufio.NewReader(replicaConn)

	_, masterPortStr, _ := net.SplitHostPort(masterAddr.String())
	cmd := fmt.Sprintf("*3\r\n$9\r\nREPLICAOF\r\n$9\r\n127.0.0.1\r\n$%d\r\n%s\r\n", len(masterPortStr), masterPortStr)
	replicaConn.Write([]byte(cmd))
	line, _ = replicaReader.ReadString('\n')
	if strings.TrimSpace(line) != "+OK" {
		t.Fatalf("expected +OK on REPLICAOF, got %q", line)
	}

	// 3. Wait for Full Sync RDB load on replica
	synced := false
	for i := 0; i < 50; i++ {
		val, ok := replicaSrv.GetDB().Get("init_master")
		if ok && val.String() == "init_val" {
			synced = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !synced {
		t.Fatalf("replica failed to sync initial RDB data")
	}

	// 4. Send live write to Master and verify streaming replication to Replica
	masterConn.Write([]byte("*3\r\n$3\r\nSET\r\n$11\r\nlive_stream\r\n$8\r\nlive_val\r\n"))
	line, _ = masterReader.ReadString('\n')
	if strings.TrimSpace(line) != "+OK" {
		t.Fatalf("expected +OK on live SET, got %q", line)
	}

	liveSynced := false
	for i := 0; i < 50; i++ {
		val, ok := replicaSrv.GetDB().Get("live_stream")
		if ok && val.String() == "live_val" {
			liveSynced = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !liveSynced {
		t.Fatalf("replica failed to receive live streamed write")
	}
}
