// Package replay is Technique 31: sync.WaitGroup.Go (Go 1.25).
package replay

import (
	"sync"
	"sync/atomic"
)

type Transfer struct{ ID string }

// replayAll starts one goroutine per transfer and waits. Go does
// the Add and the Done, so neither can be misplaced.
func replayAll(batch []Transfer, replay func(Transfer)) {
	var wg sync.WaitGroup
	for _, t := range batch {
		wg.Go(func() { replay(t) })
	}
	wg.Wait()
}

// counter is a replay target that records what it saw.
type counter struct{ n atomic.Int64 }

func (c *counter) replay(Transfer) { c.n.Add(1) }
