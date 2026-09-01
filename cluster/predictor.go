package cluster

import (
	"sync"
	"time"
)

// RiskLevel categorizes node failure probability based on derivative trends.
type RiskLevel string

const (
	RiskNormal   RiskLevel = "normal"   // Score < 50
	RiskWarning  RiskLevel = "warning"  // Score 50..80 (Triggers Simulation)
	RiskCritical RiskLevel = "critical" // Score > 80 (Triggers Shadow Spawn & Live Handover)
)

// TelemetrySample captures a point-in-time performance measurement.
type TelemetrySample struct {
	TimestampNano int64
	MemoryBytes   float64
	LatencyMs     float64
	QueueDepth    int64
}

// NodePredictor tracks sliding window derivatives and downstream graph pressure.
type NodePredictor struct {
	mu            sync.Mutex
	history       []TelemetrySample
	maxHistory    int
	lastDMdt      float64 // 1st derivative of memory growth (bytes/sec)
	lastDLdt      float64 // 1st derivative of latency increase (ms/sec)
	criticalStart int64   // Timestamp when critical state began for damping
}

// NewNodePredictor initializes a predictor with a 20-sample sliding window.
func NewNodePredictor() *NodePredictor {
	return &NodePredictor{
		history:    make([]TelemetrySample, 0, 20),
		maxHistory: 20,
	}
}

// RecordSample appends a new measurement and updates derivative trends.
func (p *NodePredictor) RecordSample(memBytes, latencyMs float64, queueDepth int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now().UnixNano()
	sample := TelemetrySample{
		TimestampNano: now,
		MemoryBytes:   memBytes,
		LatencyMs:     latencyMs,
		QueueDepth:    queueDepth,
	}

	if len(p.history) >= p.maxHistory {
		p.history = p.history[1:]
	}
	p.history = append(p.history, sample)

	if len(p.history) >= 2 {
		first := p.history[0]
		last := p.history[len(p.history)-1]
		dt := float64(last.TimestampNano-first.TimestampNano) / 1e9 // seconds
		if dt > 1e-6 {
			p.lastDMdt = (last.MemoryBytes - first.MemoryBytes) / dt
			p.lastDLdt = (last.LatencyMs - first.LatencyMs) / dt
		}
	}
}

// EvaluateRisk computes the composite risk score (0..100) and risk level using graph topology context.
func (p *NodePredictor) EvaluateRisk(g *TopologyGraph, nodeID string) (float64, RiskLevel) {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := g.GetNode(nodeID)
	if node == nil {
		return 0, RiskNormal
	}

	score := 0.0

	// 1. Memory Derivative Risk (Weight: 50)
	if p.lastDMdt > 0 {
		memRisk := (p.lastDMdt / (50 * 1024 * 1024)) * 50.0
		if memRisk > 50.0 {
			memRisk = 50.0
		}
		score += memRisk
	}

	// 2. Latency Gradient Risk (Weight: 40)
	if p.lastDLdt > 0 {
		latRisk := (p.lastDLdt / 20.0) * 40.0
		if latRisk > 40.0 {
			latRisk = 40.0
		}
		score += latRisk
	}

	// 3. Topology Downstream Critical-Path Pressure (Weight: 30)
	// Check replication queue depths to replicas
	g.mu.RLock()
	edges := g.adj[nodeID]
	downstreamCongestion := 0.0
	for _, e := range edges {
		if e.Type == LinkReplication && e.Weight > 10.0 { // Latency > 10ms
			downstreamCongestion += 15.0
		}
	}
	g.mu.RUnlock()

	if downstreamCongestion > 30.0 {
		downstreamCongestion = 30.0
	}
	score += downstreamCongestion

	// State classification
	if score >= 80.0 {
		return score, RiskCritical
	}
	if score >= 50.0 {
		return score, RiskWarning
	}

	return score, RiskNormal
}
