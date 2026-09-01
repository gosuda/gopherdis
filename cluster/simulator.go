package cluster

import (
	"errors"
	"math"
)

var (
	ErrNoSuitableShadowTarget = errors.New("cluster: no suitable healthy node available for shadow hosting")
)

// ReliefPlan specifies the optimal migration action computed by the topology simulator.
type ReliefPlan struct {
	SourceNodeID       string
	TargetNodeID       string
	SlotsToShed        []uint16
	EstimatedRiskAfter float64
}

// Simulator executes What-If graph simulations to evaluate candidate target nodes without cascading failures.
type Simulator struct{}

// NewSimulator initializes a topology placement simulator.
func NewSimulator() *Simulator {
	return &Simulator{}
}

// FindOptimalReliefPlan searches the topology graph for the best target node with minimum blast radius.
func (s *Simulator) FindOptimalReliefPlan(g *TopologyGraph, failingNodeID string) (*ReliefPlan, error) {
	virtualGraph := g.Clone()
	failingNode := virtualGraph.GetNode(failingNodeID)
	if failingNode == nil {
		return nil, errors.New("failing node not found in topology graph")
	}

	slots := failingNode.OwnedSlots()
	if len(slots) == 0 {
		return nil, errors.New("failing node holds no slots to relieve")
	}

	nodes := virtualGraph.GetAllNodes()
	var bestTargetID string
	minCost := math.Inf(1)

	for _, candidate := range nodes {
		if candidate.ID == failingNodeID || candidate.Role == RoleFenced {
			continue
		}

		// Cost Function Evaluation
		// 1. Residual resource load penalty (CPU + EWMA Memory)
		loadCost := (candidate.Metrics.CPUUsage * 0.5) + ((candidate.Metrics.EWMAMemory / (1024 * 1024 * 1024)) * 0.3)

		// 2. Replication link distance/latency penalty
		linkCost := 1.0
		virtualGraph.mu.RLock()
		for _, e := range virtualGraph.adj[failingNodeID] {
			if e.To == candidate.ID {
				linkCost = e.Weight
				break
			}
		}
		virtualGraph.mu.RUnlock()

		// 3. Fault Domain Isolation Penalty
		domainPenalty := 0.0
		if candidate.DomainID != "" && candidate.DomainID == failingNode.DomainID {
			domainPenalty = 50.0 // Heavy penalty for sharing the same physical failure domain
		}

		totalCost := loadCost + (linkCost * 0.2) + domainPenalty

		if totalCost < minCost {
			minCost = totalCost
			bestTargetID = candidate.ID
		}
	}

	if bestTargetID == "" {
		return nil, ErrNoSuitableShadowTarget
	}

	return &ReliefPlan{
		SourceNodeID:       failingNodeID,
		TargetNodeID:       bestTargetID,
		SlotsToShed:        slots,
		EstimatedRiskAfter: minCost,
	}, nil
}
