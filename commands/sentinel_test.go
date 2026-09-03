package commands

import (
	"strings"
	"testing"

	"github.com/gosuda/gopherdis/cluster"
	"github.com/gosuda/gopherdis/db"
	"github.com/gosuda/gopherdis/pubsub"
)

func TestSentinelCommands(t *testing.T) {
	database := db.NewShardedDB()
	hub := pubsub.NewShardedHub()
	cm := cluster.NewClusterManager("node_1", "127.0.0.1:6379")

	ctx := &Context{
		DB:      database,
		PubSub:  hub,
		Cluster: cm,
	}

	// 1. SENTINEL get-master-addr-by-name
	res := DefaultTable.Execute(ctx, [][]byte{
		[]byte("SENTINEL"), []byte("get-master-addr-by-name"), []byte("mymaster"),
	})
	if string(res) != "*2\r\n$9\r\n127.0.0.1\r\n$4\r\n6379\r\n" {
		t.Fatalf("unexpected get-master-addr-by-name response: %q", res)
	}

	// 2. SENTINEL masters
	res = DefaultTable.Execute(ctx, [][]byte{
		[]byte("SENTINEL"), []byte("masters"),
	})
	if !strings.Contains(string(res), "mymaster") || !strings.Contains(string(res), "127.0.0.1") {
		t.Fatalf("unexpected masters response: %q", res)
	}

	// 3. SENTINEL master mymaster
	res = DefaultTable.Execute(ctx, [][]byte{
		[]byte("SENTINEL"), []byte("master"), []byte("mymaster"),
	})
	if !strings.Contains(string(res), "mymaster") {
		t.Fatalf("unexpected master response: %q", res)
	}

	// 4. SENTINEL replicas
	res = DefaultTable.Execute(ctx, [][]byte{
		[]byte("SENTINEL"), []byte("replicas"), []byte("mymaster"),
	})
	if string(res) != "*0\r\n" {
		t.Fatalf("expected empty replicas array, got %q", res)
	}

	// 5. SENTINEL is-master-down-by-addr
	res = DefaultTable.Execute(ctx, [][]byte{
		[]byte("SENTINEL"), []byte("is-master-down-by-addr"), []byte("127.0.0.1"), []byte("6379"), []byte("1"), []byte("*"),
	})
	if !strings.HasPrefix(string(res), "*3\r\n:0\r\n") {
		t.Fatalf("expected master up status from is-master-down-by-addr, got %q", res)
	}

	// 6. SENTINEL failover & PubSub broadcast
	sub := pubsub.NewSubscriber(1)
	hub.Subscribe(sub, "+switch-master")

	res = DefaultTable.Execute(ctx, [][]byte{
		[]byte("SENTINEL"), []byte("failover"), []byte("mymaster"),
	})
	if string(res) != "+OK\r\n" {
		t.Fatalf("expected +OK from SENTINEL failover, got %q", res)
	}

	select {
	case msg := <-sub.MsgCh:
		if !strings.Contains(string(msg), "mymaster") {
			t.Fatalf("expected +switch-master message on pubsub channel, got %s", msg)
		}
	default:
		t.Fatalf("expected pubsub notification for +switch-master")
	}
}
