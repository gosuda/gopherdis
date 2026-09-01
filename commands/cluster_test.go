package commands

import (
	"strings"
	"testing"

	"github.com/gosuda/nedis/cluster"
	"github.com/gosuda/nedis/db"
)

func TestClusterCommands(t *testing.T) {
	database := db.NewShardedDB()
	cm := cluster.NewClusterManager("node_self", "127.0.0.1:7000")
	ctx := &Context{
		DB:      database,
		Cluster: cm,
	}

	// 1. CLUSTER KEYSLOT
	res := DefaultTable.Execute(ctx, [][]byte{[]byte("CLUSTER"), []byte("KEYSLOT"), []byte("{user}:data")})
	if !strings.HasPrefix(string(res), ":") {
		t.Fatalf("expected integer reply from CLUSTER KEYSLOT, got %q", res)
	}

	// 2. CLUSTER INFO
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("CLUSTER"), []byte("INFO")})
	if !strings.Contains(string(res), "cluster_state:ok") {
		t.Fatalf("expected cluster_state:ok in CLUSTER INFO, got %q", res)
	}

	// 3. CLUSTER MEET
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("CLUSTER"), []byte("MEET"), []byte("127.0.0.1"), []byte("7001")})
	if string(res) != "+OK\r\n" {
		t.Fatalf("expected +OK on CLUSTER MEET, got %q", res)
	}

	// 4. CLUSTER NODES
	res = DefaultTable.Execute(ctx, [][]byte{[]byte("CLUSTER"), []byte("NODES")})
	if !strings.Contains(string(res), "node_self") || !strings.Contains(string(res), "node_127.0.0.1_7001") {
		t.Fatalf("expected both nodes in CLUSTER NODES, got %q", res)
	}

	// 5. Test -MOVED redirection when cluster enabled
	cm.SetClusterEnabled(true)
	cm.AddNode("node_remote", "127.0.0.1:7002", cluster.RoleMaster, "")
	// Assign slots 0..5000 to remote node
	cm.AssignSlotRange("node_remote", 0, 5000)

	// Key that hashes into slot < 5000
	// Let's find a key that hashes to slot < 5000
	var remoteKey string
	var remoteSlot uint16
	for i := 0; i < 100; i++ {
		k := string([]byte{byte('a' + i)})
		s := cluster.KeySlot(k)
		if s <= 5000 {
			remoteKey = k
			remoteSlot = s
			break
		}
	}

	res = DefaultTable.Execute(ctx, [][]byte{[]byte("SET"), []byte(remoteKey), []byte("val")})
	expectedMoved := strings.TrimSpace(string(res))
	if !strings.HasPrefix(expectedMoved, "-MOVED") || !strings.Contains(expectedMoved, "127.0.0.1:7002") {
		t.Fatalf("expected -MOVED redirection for remote key (slot %d), got %q", remoteSlot, res)
	}
}
