package pool

import (
	"context"
	"iter"
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// Each test runs in a synctest bubble: synctest.Test fails if any
// goroutine the pool started is still blocked when the test ends,
// so every case below also proves nothing leaked.

// counted yields n transfers and counts how many were pulled.
func counted(n int, pulled *atomic.Int32) iter.Seq[Transfer] {
	return func(yield func(Transfer) bool) {
		for range n {
			pulled.Add(1)
			if !yield(Transfer{ID: "t"}) {
				return
			}
		}
	}
}

// blockUntilCancel is an apply that honors ctx and never finishes
// on its own.
func blockUntilCancel(ctx context.Context, _ Transfer) error {
	<-ctx.Done()
	return ctx.Err()
}

// Workers busy, producer blocked on send: depth bounds what it
// pulled, and cancel frees both.
func TestCancelWhileProducerBlocked(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		var pulled atomic.Int32
		jobs := produce(ctx, counted(1000, &pulled), 2)
		out := pool(ctx, 4, jobs, blockUntilCancel)
		synctest.Wait() // everyone is durably blocked
		// 4 in workers + 2 queued + 1 held by the blocked send
		if got := pulled.Load(); got > 4+2+1 {
			t.Fatalf("producer pulled %d rows: no backpressure", got)
		}
		cancel()
		for range out {
		}
	})
}

// Consumer not reading: workers block sending to out, cancel
// frees them, and the closer closes out.
func TestCancelWhileWorkerBlockedOnOut(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		ok := func(context.Context, Transfer) error { return nil }
		out := pool(ctx, 4,
			produce(ctx, slices.Values(make([]Transfer, 100)), 0), ok)
		synctest.Wait() // workers parked on out <- r
		cancel()
		synctest.Wait()
		n := 0
		for range out { // out is closed: nothing is left blocked
			n++
		}
		if n > 0 {
			t.Logf("%d results raced the cancel", n)
		}
	})
}

// The consumer takes three results, cancels and walks away
// without draining out. Nothing stays blocked.
func TestConsumerExitsEarly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		ok := func(context.Context, Transfer) error { return nil }
		out := pool(ctx, 4,
			produce(ctx, slices.Values(make([]Transfer, 100)), 0), ok)
		for range 3 {
			<-out
		}
		cancel()
	})
}

// apply ignores ctx and takes 50 ms. out must close only after
// every started apply has returned, i.e. after every worker.
func TestCloserWaitsForWorkers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		var started, finished atomic.Int32
		slow := func(context.Context, Transfer) error {
			started.Add(1)
			time.Sleep(50 * time.Millisecond)
			finished.Add(1)
			return nil
		}
		out := pool(ctx, 4,
			produce(ctx, slices.Values(make([]Transfer, 100)), 0), slow)
		<-out
		cancel()
		for range out {
		}
		if s, f := started.Load(), finished.Load(); s != f {
			t.Fatalf("out closed with %d of %d applies running", s-f, s)
		}
	})
}
