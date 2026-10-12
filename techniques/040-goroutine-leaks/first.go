// Package first is Technique 40: the leak, the fix, and a test
// helper that catches it.
package first

import "context"

type Transfer struct{ ID string }

type fetchFunc func(context.Context) (Transfer, error)

// FirstLeaky is the bug: senders block forever on an unbuffered
// channel once the first answer has been read.
func FirstLeaky(ctx context.Context, fetch []fetchFunc) Transfer {
	ch := make(chan Transfer)
	for _, f := range fetch {
		go func() {
			t, _ := f(ctx)
			ch <- t
		}()
	}
	return <-ch
}

// First buffers the channel to len(fetch), so no sender blocks.
// Fetchers must still honor ctx, or a slow replica holds its
// goroutine.
func First(ctx context.Context, fetch []fetchFunc) Transfer {
	ch := make(chan Transfer, len(fetch))
	for _, f := range fetch {
		go func() {
			t, _ := f(ctx)
			ch <- t
		}()
	}
	return <-ch
}
