// Package pool is Technique 32: a bounded worker pool that stops
// on cancel.
package pool

import (
	"context"
	"sync"
)

type Transfer struct {
	ID     string
	Amount int64
}

// Apply is the per-job function.
type Apply func(context.Context, Transfer) error

// Result carries an error per job.
type Result struct {
	ID  string
	Err error
}

func pool(ctx context.Context, n int,
	jobs <-chan Transfer, apply Apply,
) <-chan Result {
	out := make(chan Result)
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() { work(ctx, jobs, out, apply) })
	}
	go func() { wg.Wait(); close(out) }()
	return out
}

func work(ctx context.Context, jobs <-chan Transfer,
	out chan<- Result, apply Apply) {
	for {
		select {
		case <-ctx.Done():
			return
		case t, ok := <-jobs:
			if !ok {
				return
			}
			r := Result{t.ID, apply(ctx, t)}
			select {
			case out <- r:
			case <-ctx.Done():
				return
			}
		}
	}
}

// produce owns jobs: its first line closes it on every path, and
// each send selects on ctx.Done().
func produce(ctx context.Context, ts []Transfer) <-chan Transfer {
	jobs := make(chan Transfer)
	go func() {
		defer close(jobs)
		for _, t := range ts {
			select {
			case jobs <- t:
			case <-ctx.Done():
				return
			}
		}
	}()
	return jobs
}
