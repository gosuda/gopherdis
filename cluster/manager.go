package cluster

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// ClusterManager coordinates distributed hash slots, proactive failure forecasting, and live handover.
type ClusterManager struct {
	mu             sync.RWMutex
	selfNodeID     string
	selfAddr       string
	epoch          atomic.Uint64
	slots          [NumSlots]string // Slot -> NodeID
	graph          *TopologyGraph
	predictors     map[string]*NodePredictor
	simulator      *Simulator
	shadows        map[string]string // failingNodeID -> shadowNodeID
	ClusterEnabled bool
}

// NewClusterManager initializes the cluster coordinator.
func NewClusterManager(selfID, selfAddr string) *ClusterManager {
	cm := &ClusterManager{
		selfNodeID: selfID,
		selfAddr:   selfAddr,
		graph:      NewTopologyGraph(),
		predictors: make(map[string]*NodePredictor),
		simulator:  NewSimulator(),
		shadows:    make(map[string]string),
	}

	// Register self node
	cm.graph.AddNode(&NodeVertex{
		ID:   selfID,
		Addr: selfAddr,
		Role: RoleMaster,
	})

	return cm
}

// SetClusterEnabled toggles cluster slot enforcement.
func (cm *ClusterManager) SetClusterEnabled(enabled bool) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.ClusterEnabled = enabled
}

// CurrentEpoch returns the current monotonic fencing epoch.
func (cm *ClusterManager) CurrentEpoch() uint64 {
	return cm.epoch.Load()
}

// AddNode registers a peer node in the cluster topology.
func (cm *ClusterManager) AddNode(id, addr string, role NodeRole, domainID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.graph.AddNode(&NodeVertex{
		ID:       id,
		Addr:     addr,
		Role:     role,
		DomainID: domainID,
	})
	if _, exists := cm.predictors[id]; !exists {
		cm.predictors[id] = NewNodePredictor()
	}
}

// AssignSlotRange assigns a contiguous range of slots [start, end] to a node.
func (cm *ClusterManager) AssignSlotRange(nodeID string, start, end uint16) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	node := cm.graph.GetNode(nodeID)
	for s := start; s <= end && s < NumSlots; s++ {
		cm.slots[s] = nodeID
		if node != nil {
			node.SetSlot(s)
		}
	}
}

// GetSlotNode returns the node ID owning the given slot.
func (cm *ClusterManager) GetSlotNode(slot uint16) string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if slot >= NumSlots {
		return ""
	}
	return cm.slots[slot]
}

// IsSlotLocal checks if the key's slot belongs to the local node. Returns (isLocal, slot, ownerAddr).
func (cm *ClusterManager) IsSlotLocal(key string) (bool, uint16, string) {
	slot := KeySlot(key)

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if !cm.ClusterEnabled {
		return true, slot, cm.selfAddr
	}

	ownerID := cm.slots[slot]
	if ownerID == "" || ownerID == cm.selfNodeID {
		return true, slot, cm.selfAddr
	}

	ownerNode := cm.graph.GetNode(ownerID)
	if ownerNode != nil {
		return false, slot, ownerNode.Addr
	}
	return false, slot, cm.selfAddr
}

// UpdateTelemetry records point-in-time metrics and updates the node predictor.
func (cm *ClusterManager) UpdateTelemetry(nodeID string, memBytes, latencyMs, cpu float64, queueDepth int64) {
	cm.graph.UpdateNodeMetrics(nodeID, memBytes, latencyMs, cpu, queueDepth)

	cm.mu.Lock()
	pred, exists := cm.predictors[nodeID]
	if !exists {
		pred = NewNodePredictor()
		cm.predictors[nodeID] = pred
	}
	cm.mu.Unlock()

	pred.RecordSample(memBytes, latencyMs, queueDepth)
}

// CheckAndAutoMitigate evaluates risk and triggers What-If simulation + Shadow Master assignment if critical.
func (cm *ClusterManager) CheckAndAutoMitigate(nodeID string) (*ReliefPlan, error) {
	cm.mu.RLock()
	pred := cm.predictors[nodeID]
	cm.mu.RUnlock()

	if pred == nil {
		return nil, nil
	}

	score, level := pred.EvaluateRisk(cm.graph, nodeID)
	if level != RiskCritical {
		return nil, nil
	}

	// 1. Run What-If topology simulation to find optimal destination
	plan, err := cm.simulator.FindOptimalReliefPlan(cm.graph, nodeID)
	if err != nil {
		return nil, err
	}

	// 2. Assign Shadow Master role on target node
	cm.mu.Lock()
	cm.shadows[nodeID] = plan.TargetNodeID
	targetNode := cm.graph.GetNode(plan.TargetNodeID)
	if targetNode != nil {
		targetNode.Role = RoleShadow
	}
	cm.mu.Unlock()

	_ = score
	return plan, nil
}

// LiveHandover atomically bumps Epoch, fencing the failing node and promoting the shadow master to active.
func (cm *ClusterManager) LiveHandover(plan *ReliefPlan) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 1. Bump Monotonic Epoch (Fencing Token)
	newEpoch := cm.epoch.Add(1)

	// 2. Fence old source node
	srcNode := cm.graph.GetNode(plan.SourceNodeID)
	if srcNode != nil {
		srcNode.Role = RoleFenced
		for _, s := range plan.SlotsToShed {
			srcNode.ClearSlot(s)
		}
	}

	// 3. Promote target node to active Master
	targetNode := cm.graph.GetNode(plan.TargetNodeID)
	if targetNode != nil {
		targetNode.Role = RoleMaster
		for _, s := range plan.SlotsToShed {
			targetNode.SetSlot(s)
			cm.slots[s] = plan.TargetNodeID
		}
	}

	delete(cm.shadows, plan.SourceNodeID)
	_ = newEpoch

	return nil
}

// FormatClusterSlots produces standard RESP array format for CLUSTER SLOTS.
func (cm *ClusterManager) FormatClusterSlots() [][]byte {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var result [][]byte
	var curStart uint16 = 0
	var curOwner string = ""

	for s := uint16(0); s < NumSlots; s++ {
		owner := cm.slots[s]
		if owner != curOwner {
			if curOwner != "" {
				node := cm.graph.GetNode(curOwner)
				if node != nil {
					result = append(result, formatSlotRange(curStart, s-1, node))
				}
			}
			curStart = s
			curOwner = owner
		}
	}

	if curOwner != "" {
		node := cm.graph.GetNode(curOwner)
		if node != nil {
			result = append(result, formatSlotRange(curStart, NumSlots-1, node))
		}
	}

	return result
}

func formatSlotRange(start, end uint16, master *NodeVertex) []byte {
	host, portStr, _ := net.SplitHostPort(master.Addr)
	port, _ := strconv.Atoi(portStr)

	// Format: [start, end, [ip, port, nodeID]]
	var buf strings.Builder
	buf.WriteString("*3\r\n")
	buf.WriteString(fmt.Sprintf(":%d\r\n", start))
	buf.WriteString(fmt.Sprintf(":%d\r\n", end))
	buf.WriteString(fmt.Sprintf("*3\r\n$%d\r\n%s\r\n:%d\r\n$%d\r\n%s\r\n", len(host), host, port, len(master.ID), master.ID))
	return []byte(buf.String())
}

// FormatClusterNodes produces the standard text representation for CLUSTER NODES.
func (cm *ClusterManager) FormatClusterNodes() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var sb strings.Builder
	nodes := cm.graph.GetAllNodes()

	for _, n := range nodes {
		flags := string(n.Role)
		if n.ID == cm.selfNodeID {
			flags += ",myself"
		}
		masterID := n.MasterID
		if masterID == "" {
			masterID = "-"
		}

		// Slot ranges
		var slotStrs []string
		slots := n.OwnedSlots()
		if len(slots) > 0 {
			start := slots[0]
			prev := slots[0]
			for i := 1; i < len(slots); i++ {
				if slots[i] == prev+1 {
					prev = slots[i]
				} else {
					if start == prev {
						slotStrs = append(slotStrs, fmt.Sprintf("%d", start))
					} else {
						slotStrs = append(slotStrs, fmt.Sprintf("%d-%d", start, prev))
					}
					start = slots[i]
					prev = slots[i]
				}
			}
			if start == prev {
				slotStrs = append(slotStrs, fmt.Sprintf("%d", start))
			} else {
				slotStrs = append(slotStrs, fmt.Sprintf("%d-%d", start, prev))
			}
		}

		slotPart := strings.Join(slotStrs, " ")
		sb.WriteString(fmt.Sprintf("%s %s %s %s 0 0 %d connected %s\n",
			n.ID, n.Addr, flags, masterID, cm.epoch.Load(), slotPart))
	}

	return sb.String()
}
