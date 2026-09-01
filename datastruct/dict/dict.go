package dict

// Pair represents a contiguous field-value entry.
type Pair struct {
	Key   string
	Value []byte
}

// Dict is an optimized hybrid hash data structure.
// It stores small hashes in a flat contiguous []Pair for CPU cache locality and zero bucket overhead,
// and automatically promotes to map[string][]byte when field count exceeds 64.
type Dict struct {
	pairs []Pair
	table map[string][]byte
}

// New creates a new Dict instance pre-allocated for small hashes.
func New() *Dict {
	return &Dict{
		pairs: make([]Pair, 0, 8),
	}
}

// Set inserts or updates a key-value pair. Returns true if key was newly created.
func (d *Dict) Set(key string, val []byte) (isNew bool) {
	if d.table != nil {
		_, exists := d.table[key]
		d.table[key] = val
		return !exists
	}

	for i := range d.pairs {
		if d.pairs[i].Key == key {
			d.pairs[i].Value = val
			return false
		}
	}

	if len(d.pairs) >= 64 {
		// Promote to hash table
		d.table = make(map[string][]byte, len(d.pairs)*2)
		for _, p := range d.pairs {
			d.table[p.Key] = p.Value
		}
		d.pairs = nil
		d.table[key] = val
		return true
	}

	d.pairs = append(d.pairs, Pair{Key: key, Value: val})
	return true
}

// Get retrieves value by key.
func (d *Dict) Get(key string) ([]byte, bool) {
	if d.table != nil {
		v, ok := d.table[key]
		return v, ok
	}
	for i := range d.pairs {
		if d.pairs[i].Key == key {
			return d.pairs[i].Value, true
		}
	}
	return nil, false
}

// Del removes a key. Returns true if key existed.
func (d *Dict) Del(key string) bool {
	if d.table != nil {
		if _, ok := d.table[key]; ok {
			delete(d.table, key)
			return true
		}
		return false
	}
	for i := range d.pairs {
		if d.pairs[i].Key == key {
			d.pairs = append(d.pairs[:i], d.pairs[i+1:]...)
			return true
		}
	}
	return false
}

// Len returns the count of entries.
func (d *Dict) Len() int {
	if d.table != nil {
		return len(d.table)
	}
	return len(d.pairs)
}

// ForEach iterates all entries.
func (d *Dict) ForEach(fn func(key string, val []byte)) {
	if d.table != nil {
		for k, v := range d.table {
			fn(k, v)
		}
		return
	}
	for _, p := range d.pairs {
		fn(p.Key, p.Value)
	}
}

// Keys returns all field names.
func (d *Dict) Keys() []string {
	res := make([]string, 0, d.Len())
	d.ForEach(func(k string, _ []byte) {
		res = append(res, k)
	})
	return res
}

// Values returns all values.
func (d *Dict) Values() [][]byte {
	res := make([][]byte, 0, d.Len())
	d.ForEach(func(_ string, v []byte) {
		res = append(res, v)
	})
	return res
}
