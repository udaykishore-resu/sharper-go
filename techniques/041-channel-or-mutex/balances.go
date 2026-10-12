// Package balances is Technique 41: data that stays put and many
// goroutines touch gets a lock, not an owner goroutine.
package balances

import "sync"

type balances struct {
	mu sync.Mutex
	m  map[string]int64
}

func (b *balances) Add(id string, d int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.m[id] += d
}

func (b *balances) Get(id string) int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.m[id]
}
