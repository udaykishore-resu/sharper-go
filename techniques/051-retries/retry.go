// Package retry is Technique 51: backoff with full jitter and a
// retry budget per dependency.
package retry

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"sync"
	"time"
)

type Policy struct {
	Attempts  int
	Base, Max time.Duration
}

// ledgerd's policy: three attempts, 50 ms base, 1 s cap.
var Default = Policy{Attempts: 3, Base: 50 * time.Millisecond, Max: time.Second}

func (p Policy) backoff(n int) time.Duration {
	d := p.Base << n
	if d <= 0 || d > p.Max {
		d = p.Max // also guards shift overflow
	}
	return rand.N(d) // math/rand/v2, Go 1.22
}

// Budget lets retries add about Ratio of extra load: each request
// deposits Ratio of a token, each retry spends a whole one.
type Budget struct {
	mu          sync.Mutex
	tokens, max float64
	ratio       float64
}

// NewBudget starts full, so a healthy dependency never notices.
func NewBudget(max, ratio float64) *Budget {
	return &Budget{tokens: max, max: max, ratio: ratio}
}

func (b *Budget) OnRequest() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.tokens = min(b.tokens+b.ratio, b.max)
}

func (b *Budget) TryRetry() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Do runs one logical call. mk builds a fresh request each time,
// because a body can only be read once.
func Do(ctx context.Context, c *http.Client, p Policy, b *Budget,
	mk func() (*http.Request, error)) (*http.Response, error) {
	b.OnRequest() // one deposit per logical call
	for n := 0; ; n++ {
		req, err := mk()
		if err != nil {
			return nil, err
		}
		resp, err := c.Do(req.WithContext(ctx))
		last := n+1 >= p.Attempts
		if !retryable(resp, err) || last || !b.TryRetry() {
			return resp, err
		}
		if resp != nil {
			drainClose(resp) // Hack 46
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(p.backoff(n)):
		}
	}
}

// retryable: network errors, 429, 502, 503 and 504.
func retryable(resp *http.Response, err error) bool {
	if err != nil {
		var ne net.Error
		return errors.As(err, &ne) || errors.Is(err, io.ErrUnexpectedEOF)
	}
	switch resp.StatusCode {
	case 429, 502, 503, 504:
		return true
	}
	return false
}

func drainClose(resp *http.Response) {
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
}
