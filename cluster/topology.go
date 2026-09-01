package cluster

import (
	"sync"
	"time"
)

// NodeRole represents a node's operational state in the cluster.
type NodeRole string

const (
	RoleMaster  NodeRole = "master"
	RoleReplica NodeRole = "slave"
	RoleShadow  NodeRole = "shadow"
	RoleFenced  NodeRole = "fenced"
)

// NodeMetrics encapsulates real-time resource and telemetry stats.
type NodeMetrics struct {
	EWMAMemory  float64
	EWMALatency float64
	CPUUsage    float64
	QueueDepth  int64
	LastUpdated int64
}

// NodeVertex represents a cluster node vertex in the topology graph.
type NodeVertex struct {
	ID       string
	Addr     string
	Role     NodeRole
	MasterID string
	Slots    [NumSlots / 8]byte // 2048 bytes for 16384 slot bits
	Metrics  NodeMetrics
	DomainID string // Rack or Zone ID for fault domain isolation
}

// SetSlot marks a slot as owned by this node.
func (nv *NodeVertex) SetSlot(slot uint16) {
	if slot >= NumSlots {
		return
	}
	nv.Slots[slot/8] |= (1 << (slot % 8))
}

// ClearSlot removes slot ownership.
func (nv *NodeVertex) ClearSlot(slot uint16) {
	if slot >= NumSlots {
		return
	}
	nv.Slots[slot/8] &= ^(1 << (slot % 8))
}

// HasSlot checks if node owns the slot.
func (nv *NodeVertex) HasSlot(slot uint16) bool {
	if slot >= NumSlots {
		return false
	}
	return (nv.Slots[slot/8] & (1 << (slot % 8))) != 0
}

// OwnedSlots returns a slice of all slots assigned to this node.
func (nv *NodeVertex) OwnedSlots() []uint16 {
	var slots []uint16
	for s := uint16(0); s < NumSlots; s++ {
		if nv.HasSlot(s) {
			slots = append(slots, s)
		}
	}
	return slots
}

// LinkType specifies the communication link between topology nodes.
type LinkType string

const (
	LinkReplication LinkType = "replication"
	LinkClient      LinkType = "client"
	LinkGossip       LinkType = "gossip"
)

// LinkEdge represents a directed weighted connection in the topology graph.
type LinkEdge struct {
	From   string
	To     string
	Type   LinkType
	Weight float64 // Latency in ms + Queue Depth penalty
}

// TopologyGraph maintains the directed weighted graph of all nodes and communication links.
type TopologyGraph struct {
	mu    sync.RWMutex
	nodes map[string]*NodeVertex
	adj   map[string][]*LinkEdge
}

// NewTopologyGraph initializes a new empty topology graph.
func NewTopologyGraph() *TopologyGraph {
	return &TopologyGraph{
		nodes: make(map[string]*NodeVertex),
		adj:   make(map[string][]*LinkEdge),
	}
}

// AddNode registers or updates a node vertex.
func (g *TopologyGraph) AddNode(n *NodeVertex) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nodes[n.ID] = n
}

// GetNode retrieves a node vertex by ID.
func (g *TopologyGraph) GetNode(id string) *NodeVertex {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.nodes[id]
}

// AddEdge registers a directed edge between nodes.
func (g *TopologyGraph) AddEdge(from, to string, linkType LinkType, weight float64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	edges := g.adj[from]
	for _, e := range edges {
		if e.To == to && e.Type == linkType {
			e.Weight = weight
			return
		}
	}
	g.adj[from] = append(g.adj[from], &LinkEdge{
		From:   from,
		To:     to,
		Type:   linkType,
		Weight: weight,
	})
}

// GetAllNodes returns a list of all registered node vertices.
func (g *TopologyGraph) GetAllNodes() []*NodeVertex {
	g.mu.RLock()
	defer g.mu.RUnlock()

	res := make([]*NodeVertex, 0, len(g.nodes))
	for _, n := range g.nodes {
		res = append(res, n)
	}
	return res
}

// Clone creates an isolated copy of the graph for What-If simulation.
func (g *TopologyGraph) Clone() *TopologyGraph {
	g.mu.RLock()
	defer g.mu.RUnlock()

	cloned := NewTopologyGraph()
	for id, n := range g.nodes {
		vCopy := *n
		cloned.nodes[id] = &vCopy
	}
	for from, edges := range g.adj {
		edgesCopy := make([]*LinkEdge, len(edges))
		for i, e := range edges {
			eCopy := *e
			edgesCopy[i] = &eCopy
		}
		cloned.adj[from] = edgesCopy
	}
	return cloned
}

// UpdateNodeMetrics updates the real-time resource stats of a node.
func (g *TopologyGraph) UpdateNodeMetrics(nodeID string, memBytes, latencyMs, cpu float64, queueDepth int64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	n, exists := g.nodes[nodeID]
	if !exists {
		return
	}

	const alpha = 0.3 // EWMA smoothing coefficient
	if n.Metrics.LastUpdated == 0 {
		n.Metrics.EWMAMemory = memBytes
		n.Metrics.EWMALatency = latencyMs
	} else {
		n.Metrics.EWMAMemory = alpha*memBytes + (1-alpha)*n.Metrics.EWMAMemory
		n.Metrics.EWMALatency = alpha*latencyMs + (1-alpha)*n.Metrics.EWMALatency
	}
	n.Metrics.CPUUsage = cpu
	n.Metrics.QueueDepth = queueDepth
	n.Metrics.LastUpdated = time.Now().UnixMilli()
}
