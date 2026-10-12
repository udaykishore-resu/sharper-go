// Package retry is Technique 11: range over an integer.
package retry

import (
	"context"
	"time"
)

// Transfer is a stand-in for ledger.Transfer.
type Transfer struct{ ID string }

// Client posts transfers.
type Client struct {
	Post func(context.Context, Transfer) error
}

const maxAttempts = 3

func postWithRetry(ctx context.Context, c *Client,
	t Transfer) error {
	var err error
	for attempt := range maxAttempts {
		if err = c.Post(ctx, t); err == nil {
			return nil
		}
		sleep(ctx, backoff(attempt))
	}
	return err
}

func backoff(attempt int) time.Duration {
	return time.Millisecond << attempt
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
