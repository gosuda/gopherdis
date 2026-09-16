// Package encoding holds the thresholds that decide which physical encoding a
// collection uses, mirroring Redis' *-max-listpack-entries settings.
package encoding

import "sync/atomic"

// Defaults match Redis 8's shipped configuration.
const (
	DefaultHashMaxListpackEntries = 128
	DefaultHashMaxListpackValue   = 64
	DefaultSetMaxIntsetEntries    = 512
	DefaultSetMaxListpackEntries  = 128
	DefaultSetMaxListpackValue    = 64
	DefaultZsetMaxListpackEntries = 128
	DefaultZsetMaxListpackValue   = 64
	DefaultListMaxListpackSize    = 128
)

var (
	hashMaxListpackEntries int64 = DefaultHashMaxListpackEntries
	hashMaxListpackValue   int64 = DefaultHashMaxListpackValue
	setMaxIntsetEntries    int64 = DefaultSetMaxIntsetEntries
	setMaxListpackEntries  int64 = DefaultSetMaxListpackEntries
	setMaxListpackValue    int64 = DefaultSetMaxListpackValue
	zsetMaxListpackEntries int64 = DefaultZsetMaxListpackEntries
	zsetMaxListpackValue   int64 = DefaultZsetMaxListpackValue
	listMaxListpackSize    int64 = DefaultListMaxListpackSize
)

func HashMaxListpackEntries() int { return int(atomic.LoadInt64(&hashMaxListpackEntries)) }
func HashMaxListpackValue() int   { return int(atomic.LoadInt64(&hashMaxListpackValue)) }
func SetMaxIntsetEntries() int    { return int(atomic.LoadInt64(&setMaxIntsetEntries)) }
func SetMaxListpackEntries() int  { return int(atomic.LoadInt64(&setMaxListpackEntries)) }
func SetMaxListpackValue() int    { return int(atomic.LoadInt64(&setMaxListpackValue)) }
func ZsetMaxListpackEntries() int { return int(atomic.LoadInt64(&zsetMaxListpackEntries)) }
func ZsetMaxListpackValue() int   { return int(atomic.LoadInt64(&zsetMaxListpackValue)) }
func ListMaxListpackSize() int    { return int(atomic.LoadInt64(&listMaxListpackSize)) }

// Set applies a configuration parameter, reporting whether the name is one of
// the encoding thresholds. Unknown names are left to the caller.
func Set(name string, v int64) bool {
	target, ok := byName(name)
	if !ok {
		return false
	}
	atomic.StoreInt64(target, v)
	return true
}

// Get reads a threshold by configuration name.
func Get(name string) (int64, bool) {
	target, ok := byName(name)
	if !ok {
		return 0, false
	}
	return atomic.LoadInt64(target), true
}

func byName(name string) (*int64, bool) {
	switch name {
	case "hash-max-listpack-entries", "hash-max-ziplist-entries":
		return &hashMaxListpackEntries, true
	case "hash-max-listpack-value", "hash-max-ziplist-value":
		return &hashMaxListpackValue, true
	case "set-max-intset-entries":
		return &setMaxIntsetEntries, true
	case "set-max-listpack-entries":
		return &setMaxListpackEntries, true
	case "set-max-listpack-value":
		return &setMaxListpackValue, true
	case "zset-max-listpack-entries", "zset-max-ziplist-entries":
		return &zsetMaxListpackEntries, true
	case "zset-max-listpack-value", "zset-max-ziplist-value":
		return &zsetMaxListpackValue, true
	case "list-max-listpack-size", "list-max-ziplist-size":
		return &listMaxListpackSize, true
	default:
		return nil, false
	}
}

// Names lists every configurable threshold, for CONFIG GET globbing.
func Names() []string {
	return []string{
		"hash-max-listpack-entries", "hash-max-listpack-value",
		"set-max-intset-entries", "set-max-listpack-entries", "set-max-listpack-value",
		"zset-max-listpack-entries", "zset-max-listpack-value",
		"list-max-listpack-size",
	}
}
