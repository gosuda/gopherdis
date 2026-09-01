package skiplist

import (
	"math/rand/v2"
	"sync"
)

const (
	ZSkiplistMaxLevel = 32
	ZSkiplistP        = 0.25
)

// ZSetElement represents a single (member, score) pair.
type ZSetElement struct {
	Member string
	Score  float64
}

type zskiplistLevel struct {
	forward *zskiplistNode
	span    uint64
}

type zskiplistNode struct {
	member   string
	score    float64
	backward *zskiplistNode
	level    []zskiplistLevel
}

func newZSkiplistNode(level int, score float64, member string) *zskiplistNode {
	return &zskiplistNode{
		member: member,
		score:  score,
		level:  make([]zskiplistLevel, level),
	}
}

type zskiplist struct {
	header *zskiplistNode
	tail   *zskiplistNode
	length uint64
	level  int
}

func newZSkiplist() *zskiplist {
	sl := &zskiplist{
		level:  1,
		length: 0,
		header: newZSkiplistNode(ZSkiplistMaxLevel, 0, ""),
	}
	for i := 0; i < ZSkiplistMaxLevel; i++ {
		sl.header.level[i].forward = nil
		sl.header.level[i].span = 0
	}
	return sl
}

func randomLevel() int {
	level := 1
	for float64(rand.Uint32()&0xFFFF) < float64(0xFFFF)*ZSkiplistP {
		level++
	}
	if level < ZSkiplistMaxLevel {
		return level
	}
	return ZSkiplistMaxLevel
}

func (sl *zskiplist) insert(score float64, member string) *zskiplistNode {
	var update [ZSkiplistMaxLevel]*zskiplistNode
	var rank [ZSkiplistMaxLevel]uint64

	x := sl.header
	for i := sl.level - 1; i >= 0; i-- {
		if i == sl.level-1 {
			rank[i] = 0
		} else {
			rank[i] = rank[i+1]
		}

		for x.level[i].forward != nil &&
			(x.level[i].forward.score < score ||
				(x.level[i].forward.score == score && x.level[i].forward.member < member)) {
			rank[i] += x.level[i].span
			x = x.level[i].forward
		}
		update[i] = x
	}

	level := randomLevel()
	if level > sl.level {
		for i := sl.level; i < level; i++ {
			rank[i] = 0
			update[i] = sl.header
			update[i].level[i].span = sl.length
		}
		sl.level = level
	}

	x = newZSkiplistNode(level, score, member)
	for i := 0; i < level; i++ {
		x.level[i].forward = update[i].level[i].forward
		update[i].level[i].forward = x

		x.level[i].span = update[i].level[i].span - (rank[0] - rank[i])
		update[i].level[i].span = (rank[0] - rank[i]) + 1
	}

	for i := level; i < sl.level; i++ {
		update[i].level[i].span++
	}

	if update[0] == sl.header {
		x.backward = nil
	} else {
		x.backward = update[0]
	}
	if x.level[0].forward != nil {
		x.level[0].forward.backward = x
	} else {
		sl.tail = x
	}
	sl.length++
	return x
}

func (sl *zskiplist) deleteNode(x *zskiplistNode, update *[ZSkiplistMaxLevel]*zskiplistNode) {
	for i := 0; i < sl.level; i++ {
		if update[i].level[i].forward == x {
			update[i].level[i].span += x.level[i].span - 1
			update[i].level[i].forward = x.level[i].forward
		} else {
			update[i].level[i].span--
		}
	}
	if x.level[0].forward != nil {
		x.level[0].forward.backward = x.backward
	} else {
		sl.tail = x.backward
	}
	for sl.level > 1 && sl.header.level[sl.level-1].forward == nil {
		sl.level--
	}
	sl.length--
}

func (sl *zskiplist) delete(score float64, member string) bool {
	var update [ZSkiplistMaxLevel]*zskiplistNode
	x := sl.header
	for i := sl.level - 1; i >= 0; i-- {
		for x.level[i].forward != nil &&
			(x.level[i].forward.score < score ||
				(x.level[i].forward.score == score && x.level[i].forward.member < member)) {
			x = x.level[i].forward
		}
		update[i] = x
	}

	x = x.level[0].forward
	if x != nil && x.score == score && x.member == member {
		sl.deleteNode(x, &update)
		return true
	}
	return false
}

// getRank returns 1-based rank of member with given score.
func (sl *zskiplist) getRank(score float64, member string) uint64 {
	rank := uint64(0)
	x := sl.header
	for i := sl.level - 1; i >= 0; i-- {
		for x.level[i].forward != nil &&
			(x.level[i].forward.score < score ||
				(x.level[i].forward.score == score && x.level[i].forward.member <= member)) {
			rank += x.level[i].span
			x = x.level[i].forward
		}
		if x != nil && x.member == member {
			return rank
		}
	}
	return 0
}

// getElementByRank finds node by 1-based rank in O(log N).
func (sl *zskiplist) getElementByRank(rank uint64) *zskiplistNode {
	traversed := uint64(0)
	x := sl.header
	for i := sl.level - 1; i >= 0; i-- {
		for x.level[i].forward != nil && (traversed+x.level[i].span) <= rank {
			traversed += x.level[i].span
			x = x.level[i].forward
		}
		if traversed == rank {
			return x
		}
	}
	return nil
}

// ZSet is a combined Hash Map + Skiplist structure for Redis Sorted Sets.
type ZSet struct {
	mu   sync.Mutex
	dict map[string]float64
	sl   *zskiplist
}

// NewZSet initializes a new ZSet.
func NewZSet() *ZSet {
	return &ZSet{
		dict: make(map[string]float64),
		sl:   newZSkiplist(),
	}
}

// Len returns the number of elements in the sorted set.
func (zs *ZSet) Len() int64 {
	if zs == nil {
		return 0
	}
	zs.mu.Lock()
	defer zs.mu.Unlock()
	return int64(zs.sl.length)
}

// Add inserts or updates a member with the given score.
// Returns (added: true if new member, updated: true if score changed).
func (zs *ZSet) Add(member string, score float64) (added bool, updated bool) {
	zs.mu.Lock()
	defer zs.mu.Unlock()

	curScore, exists := zs.dict[member]
	if exists {
		if curScore != score {
			zs.sl.delete(curScore, member)
			zs.sl.insert(score, member)
			zs.dict[member] = score
			return false, true
		}
		return false, false
	}

	zs.dict[member] = score
	zs.sl.insert(score, member)
	return true, false
}

// Score retrieves the score of a member.
func (zs *ZSet) Score(member string) (float64, bool) {
	if zs == nil {
		return 0, false
	}
	zs.mu.Lock()
	defer zs.mu.Unlock()

	score, ok := zs.dict[member]
	return score, ok
}

// Remove deletes a member from the sorted set.
func (zs *ZSet) Remove(member string) bool {
	if zs == nil {
		return false
	}
	zs.mu.Lock()
	defer zs.mu.Unlock()

	score, ok := zs.dict[member]
	if !ok {
		return false
	}
	delete(zs.dict, member)
	zs.sl.delete(score, member)
	return true
}

// Rank returns the 0-based rank of member. If reverse is true, ordered by score descending.
func (zs *ZSet) Rank(member string, reverse bool) (int64, bool) {
	if zs == nil {
		return -1, false
	}
	zs.mu.Lock()
	defer zs.mu.Unlock()

	score, ok := zs.dict[member]
	if !ok {
		return -1, false
	}
	rank := zs.sl.getRank(score, member)
	if rank == 0 {
		return -1, false
	}
	if reverse {
		return int64(zs.sl.length - rank), true
	}
	return int64(rank - 1), true
}

// Range returns elements within index range [start, stop]. If reverse is true, order is descending.
func (zs *ZSet) Range(start, stop int64, reverse bool) []ZSetElement {
	if zs == nil {
		return nil
	}
	zs.mu.Lock()
	defer zs.mu.Unlock()

	length := int64(zs.sl.length)
	if length == 0 {
		return nil
	}

	if start < 0 {
		start = length + start
	}
	if stop < 0 {
		stop = length + stop
	}
	if start < 0 {
		start = 0
	}
	if start >= length || start > stop {
		return nil
	}
	if stop >= length {
		stop = length - 1
	}

	count := stop - start + 1
	result := make([]ZSetElement, 0, count)

	if !reverse {
		node := zs.sl.getElementByRank(uint64(start + 1))
		for node != nil && count > 0 {
			result = append(result, ZSetElement{
				Member: node.member,
				Score:  node.score,
			})
			node = node.level[0].forward
			count--
		}
	} else {
		node := zs.sl.getElementByRank(uint64(length - start))
		for node != nil && count > 0 {
			result = append(result, ZSetElement{
				Member: node.member,
				Score:  node.score,
			})
			node = node.backward
			count--
		}
	}
	return result
}
