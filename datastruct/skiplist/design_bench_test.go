package skiplist

import (
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"testing"
)

// mapZSet models the sort-on-read design used by map-backed sorted sets: the
// scores live in a plain map and any rank query sorts the whole set.
type mapZSet struct {
	mu   sync.RWMutex
	dict map[string]float64
}

func newMapZSet() *mapZSet { return &mapZSet{dict: make(map[string]float64)} }

func (m *mapZSet) Add(member string, score float64) {
	m.mu.Lock()
	m.dict[member] = score
	m.mu.Unlock()
}

func (m *mapZSet) sorted() []ZSetElement {
	out := make([]ZSetElement, 0, len(m.dict))
	for k, v := range m.dict {
		out = append(out, ZSetElement{Member: k, Score: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score < out[j].Score
		}
		return out[i].Member < out[j].Member
	})
	return out
}

func (m *mapZSet) Rank(member string) int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i, e := range m.sorted() {
		if e.Member == member {
			return int64(i)
		}
	}
	return -1
}

func (m *mapZSet) Range(start, stop int64) []ZSetElement {
	m.mu.RLock()
	defer m.mu.RUnlock()
	all := m.sorted()
	if start >= int64(len(all)) {
		return nil
	}
	if stop >= int64(len(all)) {
		stop = int64(len(all)) - 1
	}
	return all[start : stop+1]
}

func sizes() []int { return []int{2000, 50000, 200000} }

func BenchmarkZRankSkiplist(b *testing.B) {
	for _, n := range sizes() {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			zs := NewZSet()
			rnd := rand.New(rand.NewSource(1))
			for i := 0; i < n; i++ {
				zs.Add(fmt.Sprintf("player_%d", i), float64(rnd.Intn(100000)))
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				zs.Rank(fmt.Sprintf("player_%d", i%n), false)
			}
		})
	}
}

func BenchmarkZRankMapSort(b *testing.B) {
	for _, n := range sizes() {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			m := newMapZSet()
			rnd := rand.New(rand.NewSource(1))
			for i := 0; i < n; i++ {
				m.Add(fmt.Sprintf("player_%d", i), float64(rnd.Intn(100000)))
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				m.Rank(fmt.Sprintf("player_%d", i%n))
			}
		})
	}
}

func BenchmarkZRangeTop10Skiplist(b *testing.B) {
	for _, n := range sizes() {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			zs := NewZSet()
			rnd := rand.New(rand.NewSource(1))
			for i := 0; i < n; i++ {
				zs.Add(fmt.Sprintf("player_%d", i), float64(rnd.Intn(100000)))
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				zs.Range(0, 10, false)
			}
		})
	}
}

func BenchmarkZRangeTop10MapSort(b *testing.B) {
	for _, n := range sizes() {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			m := newMapZSet()
			rnd := rand.New(rand.NewSource(1))
			for i := 0; i < n; i++ {
				m.Add(fmt.Sprintf("player_%d", i), float64(rnd.Intn(100000)))
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				m.Range(0, 10)
			}
		})
	}
}
