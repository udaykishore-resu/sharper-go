// Package obs is Technique 27: a zero value that works.
package obs

import "sync"

// Counters counts named events. The zero value is
// ready to use; a nil *Counters panics.
type Counters struct {
	mu sync.Mutex
	m  map[string]int64
}

func (c *Counters) Add(name string, n int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = make(map[string]int64)
	}
	c.m[name] += n
}

// Get reads a nil map safely; only writes need the check.
func (c *Counters) Get(name string) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.m[name]
}

// Server holds Counters by value with no setup.
type Server struct {
	Counters Counters
}
