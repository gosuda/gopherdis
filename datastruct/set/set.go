package set

import "sync"

// Set is a thread-safe string set.
type Set struct {
	mu sync.RWMutex
	m  map[string]struct{}
}

// New creates a new Set.
func New() *Set {
	return &Set{
		m: make(map[string]struct{}),
	}
}

// Add adds members to the set. Returns number of newly added elements.
func (s *Set) Add(members ...string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	added := 0
	for _, m := range members {
		if _, exists := s.m[m]; !exists {
			s.m[m] = struct{}{}
			added++
		}
	}
	return added
}

// Remove removes members from the set. Returns number of elements removed.
func (s *Set) Remove(members ...string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	removed := 0
	for _, m := range members {
		if _, exists := s.m[m]; exists {
			delete(s.m, m)
			removed++
		}
	}
	return removed
}

// Contains checks if member exists.
func (s *Set) Contains(member string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, exists := s.m[member]
	return exists
}

// Card returns the cardinality (count).
func (s *Set) Card() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.m)
}

// Members returns all members.
func (s *Set) Members() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]string, 0, len(s.m))
	for k := range s.m {
		res = append(res, k)
	}
	return res
}

// Pop removes and returns up to count random members.
func (s *Set) Pop(count int) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.m) == 0 {
		return nil
	}
	if count > len(s.m) {
		count = len(s.m)
	}
	res := make([]string, 0, count)
	for k := range s.m {
		res = append(res, k)
		delete(s.m, k)
		if len(res) >= count {
			break
		}
	}
	return res
}
