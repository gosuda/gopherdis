// Package intset implements Redis' intset encoding: a sorted array of integers
// used for sets whose members are all integers.
//
// Membership is a binary search over one contiguous []int64 rather than a hash
// table, which is both smaller and cache friendlier at the sizes this encoding
// is used for.
package intset

import (
	"sort"
	"strconv"
)

// IntSet is a sorted, duplicate-free set of int64 values.
type IntSet struct {
	vals []int64
}

// New returns an empty intset.
func New() *IntSet { return &IntSet{} }

// Parse reports whether s is the canonical decimal form of an int64, which is
// the condition for a member to be storable in this encoding. Values such as
// "012" or "+1" parse as numbers but do not render back identically, and Redis
// keeps a set containing them out of the intset encoding.
func Parse(s string) (int64, bool) {
	if len(s) == 0 || len(s) > 20 {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	if strconv.FormatInt(n, 10) != s {
		return 0, false
	}
	return n, true
}

// Len returns the number of members.
func (is *IntSet) Len() int {
	if is == nil {
		return 0
	}
	return len(is.vals)
}

func (is *IntSet) search(v int64) (int, bool) {
	i := sort.Search(len(is.vals), func(i int) bool { return is.vals[i] >= v })
	return i, i < len(is.vals) && is.vals[i] == v
}

// Add inserts v, reporting whether it was newly added.
func (is *IntSet) Add(v int64) bool {
	i, found := is.search(v)
	if found {
		return false
	}
	is.vals = append(is.vals, 0)
	copy(is.vals[i+1:], is.vals[i:])
	is.vals[i] = v
	return true
}

// Remove deletes v, reporting whether it was present.
func (is *IntSet) Remove(v int64) bool {
	i, found := is.search(v)
	if !found {
		return false
	}
	is.vals = append(is.vals[:i], is.vals[i+1:]...)
	return true
}

// Contains reports membership.
func (is *IntSet) Contains(v int64) bool {
	if is == nil {
		return false
	}
	_, found := is.search(v)
	return found
}

// Members returns the members as decimal strings, in sorted numeric order.
func (is *IntSet) Members() []string {
	if is == nil {
		return nil
	}
	out := make([]string, 0, len(is.vals))
	for _, v := range is.vals {
		out = append(out, strconv.FormatInt(v, 10))
	}
	return out
}

// Values returns the raw sorted members.
func (is *IntSet) Values() []int64 {
	if is == nil {
		return nil
	}
	return is.vals
}
