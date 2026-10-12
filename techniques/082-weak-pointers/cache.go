// Package weakcache is Technique 82: a cache that shares live
// objects but never keeps them alive.
package weakcache

import (
	"runtime"
	"sync"
	"weak"
)

type Cache[K comparable, V any] struct {
	mu sync.Mutex
	m  map[K]weak.Pointer[V]
}

func (c *Cache[K, V]) Get(k K) *V {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.m[k].Value()
}

func (c *Cache[K, V]) Put(k K, v *V) {
	wp := weak.Make(v)
	c.mu.Lock()
	if c.m == nil { // zero Cache is usable
		c.m = make(map[K]weak.Pointer[V])
	}
	c.m[k] = wp
	c.mu.Unlock()
	runtime.AddCleanup(v, func(k K) {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.m[k] == wp {
			delete(c.m, k)
		}
	}, k)
}

func (c *Cache[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.m)
}
