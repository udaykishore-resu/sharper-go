// Package pool is Technique 32: a bounded worker pool that stops
// on cancel.
package pool

import (
	"context"
	"iter"
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

// produce owns jobs. depth is the whole queue: 0 means each
// row waits for a free worker, which is the backpressure.
func produce(ctx context.Context,
	rows iter.Seq[Transfer],
	depth int) <-chan Transfer {
	jobs := make(chan Transfer, depth)
	go func() {
		defer close(jobs)
		for t := range rows {
			select {
			case jobs <- t:
			case <-ctx.Done():
				return
			}
		}
	}()
	return jobs
}
