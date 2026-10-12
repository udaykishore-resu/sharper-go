// Package outbox is Technique 63's subject: a relay that runs
// until its context is done and then closes Done.
package outbox

import (
	"context"
	"time"
)

// stuck is a test seam: the review test sets it to simulate a
// relay whose shutdown path is broken and never returns.
var stuck bool

type Relay struct {
	done  chan struct{}
	ticks int
}

// Start runs the relay loop in its own goroutine.
func Start(ctx context.Context) *Relay {
	r := &Relay{done: make(chan struct{})}
	go func() {
		defer close(r.done)
		t := time.NewTicker(10 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				if stuck {
					select {} // the bug: ignores cancellation
				}
				return
			case <-t.C:
				r.ticks++ // publish a batch here
			}
		}
	}()
	return r
}

func (r *Relay) Done() <-chan struct{} { return r.done }
