// Package hold keeps reserved funds for a TTL; a sweeper releases
// expired holds. It uses the real clock: synctest fakes it.
package hold

import (
	"context"
	"sync"
	"time"
)

type Holds struct {
	ttl time.Duration
	mu  sync.Mutex
	m   map[string]time.Time
}

func New(ttl time.Duration) *Holds {
	return &Holds{ttl: ttl, m: map[string]time.Time{}}
}

func (h *Holds) Place(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.m[id] = time.Now().Add(h.ttl)
}

func (h *Holds) Active(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, ok := h.m[id]
	return ok
}

// Sweep releases expired holds every interval until ctx ends.
func (h *Holds) Sweep(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			h.mu.Lock()
			for id, exp := range h.m {
				if !now.Before(exp) {
					delete(h.m, id)
				}
			}
			h.mu.Unlock()
		}
	}
}
