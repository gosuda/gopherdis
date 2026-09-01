package stream

import (
	"time"
)

// NackEntry represents an unacknowledged message in the Pending Entries List (PEL).
type NackEntry struct {
	ID            StreamID
	ConsumerName  string
	DeliveryTime  int64 // Unix timestamp in milliseconds
	DeliveryCount int
}

// Consumer represents a worker consumer inside a consumer group.
type Consumer struct {
	Name       string
	ActiveTime int64
	Pel        map[StreamID]*NackEntry
}

// ConsumerGroup manages group-level offset and PEL for a set of consumers.
type ConsumerGroup struct {
	Name            string
	LastDeliveredID StreamID
	Pel             map[StreamID]*NackEntry
	Consumers       map[string]*Consumer
}

// NewConsumerGroup creates an initialized consumer group starting from startID.
func NewConsumerGroup(name string, startID StreamID) *ConsumerGroup {
	return &ConsumerGroup{
		Name:            name,
		LastDeliveredID: startID,
		Pel:             make(map[StreamID]*NackEntry),
		Consumers:       make(map[string]*Consumer),
	}
}

// GetOrCreateConsumer returns an existing consumer or initializes a new one.
func (cg *ConsumerGroup) GetOrCreateConsumer(name string) *Consumer {
	c, exists := cg.Consumers[name]
	if !exists {
		c = &Consumer{
			Name:       name,
			ActiveTime: time.Now().UnixMilli(),
			Pel:        make(map[StreamID]*NackEntry),
		}
		cg.Consumers[name] = c
	} else {
		c.ActiveTime = time.Now().UnixMilli()
	}
	return c
}

// AddPending records a delivered message into global and consumer PEL.
func (cg *ConsumerGroup) AddPending(consumerName string, id StreamID) {
	c := cg.GetOrCreateConsumer(consumerName)
	now := time.Now().UnixMilli()

	entry, exists := cg.Pel[id]
	if exists {
		entry.DeliveryTime = now
		entry.DeliveryCount++
		entry.ConsumerName = consumerName
		c.Pel[id] = entry
	} else {
		nack := &NackEntry{
			ID:            id,
			ConsumerName:  consumerName,
			DeliveryTime:  now,
			DeliveryCount: 1,
		}
		cg.Pel[id] = nack
		c.Pel[id] = nack
	}
}

// Ack removes an entry from PEL upon confirmation. Returns true if key was in PEL.
func (cg *ConsumerGroup) Ack(id StreamID) bool {
	nack, exists := cg.Pel[id]
	if !exists {
		return false
	}

	delete(cg.Pel, id)
	if c, ok := cg.Consumers[nack.ConsumerName]; ok {
		delete(c.Pel, id)
	}
	return true
}

// PendingReportEntry represents summary info for XPENDING.
type PendingReportEntry struct {
	ID            StreamID
	ConsumerName  string
	IdleTimeMs    int64
	DeliveryCount int
}
