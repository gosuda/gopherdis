package cluster

import (
	"testing"
	"time"
)

func TestCluster_HashTagSlot(t *testing.T) {
	// Matching hashtags must map to identical slots
	s1 := KeySlot("{user_100}:profile")
	s2 := KeySlot("{user_100}:orders")
	if s1 != s2 {
		t.Fatalf("expected identical slot for same hashtag, got %d vs %d", s1, s2)
	}

	// Empty bracket {} falls back to full key
	s3 := KeySlot("{}user_100")
	s4 := KeySlot("{}user_100")
	if s3 != s4 {
		t.Fatalf("expected identical slot, got %d vs %d", s3, s4)
	}
}

func TestCluster_ProactiveShadowMitigation(t *testing.T) {
	cm := NewClusterManager("node_A", "127.0.0.1:7000")
	cm.SetClusterEnabled(true)

	// Add Node B (Heavy load, Rack 1) and Node C (Light load, Rack 2)
	cm.AddNode("node_B", "127.0.0.1:7001", RoleMaster, "rack_1")
	cm.AddNode("node_C", "127.0.0.1:7002", RoleMaster, "rack_2")

	// Set Node A domain to rack_1
	nodeA := cm.graph.GetNode("node_A")
	nodeA.DomainID = "rack_1"

	// Assign slots 0..5000 to Node A
	cm.AssignSlotRange("node_A", 0, 5000)

	// Update telemetry for Node B (High load: CPU 80%, RAM 4GB)
	cm.UpdateTelemetry("node_B", 4*1024*1024*1024, 30.0, 80.0, 50)

	// Update telemetry for Node C (Low load: CPU 10%, RAM 500MB)
	cm.UpdateTelemetry("node_C", 500*1024*1024, 2.0, 10.0, 0)

	// Simulate rapid memory surge on Node A (Surge 100MB/sec -> triggers RiskCritical)
	cm.UpdateTelemetry("node_A", 100*1024*1024, 1.0, 20.0, 0)
	time.Sleep(5 * time.Millisecond)
	cm.UpdateTelemetry("node_A", 200*1024*1024, 25.0, 50.0, 10)
	time.Sleep(5 * time.Millisecond)
	cm.UpdateTelemetry("node_A", 350*1024*1024, 55.0, 90.0, 20)

	// Evaluate and Auto-Mitigate
	plan, err := cm.CheckAndAutoMitigate("node_A")
	if err != nil {
		t.Fatalf("CheckAndAutoMitigate failed: %v", err)
	}
	if plan == nil {
		t.Fatalf("expected plan to be generated for critical risk on node_A")
	}

	// Simulator MUST select Node C (Low load + Different Rack) over Node B
	if plan.TargetNodeID != "node_C" {
		t.Fatalf("expected Simulator to pick node_C, but got %s", plan.TargetNodeID)
	}

	// Verify Target Node C became RoleShadow
	targetNode := cm.graph.GetNode("node_C")
	if targetNode.Role != RoleShadow {
		t.Fatalf("expected node_C to be RoleShadow, got %s", targetNode.Role)
	}

	// Execute Live Handover
	oldEpoch := cm.CurrentEpoch()
	err = cm.LiveHandover(plan)
	if err != nil {
		t.Fatalf("LiveHandover failed: %v", err)
	}

	// Verify Epoch incremented (Fencing Token)
	if cm.CurrentEpoch() != oldEpoch+1 {
		t.Fatalf("expected epoch to increment to %d, got %d", oldEpoch+1, cm.CurrentEpoch())
	}

	// Verify Node A is Fenced and Node C is active Master owning slots
	if nodeA.Role != RoleFenced {
		t.Fatalf("expected node_A to be RoleFenced, got %s", nodeA.Role)
	}
	if targetNode.Role != RoleMaster {
		t.Fatalf("expected node_C to be RoleMaster, got %s", targetNode.Role)
	}
	if cm.GetSlotNode(100) != "node_C" {
		t.Fatalf("expected slot 100 to be owned by node_C, got %s", cm.GetSlotNode(100))
	}
}
