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
	"net/url"
	"strconv"
	"sync"
	"time"
)

type Policy struct {
	Attempts  int
	Base, Max time.Duration
}

// ledgerd's policy: three attempts, 50 ms base, 1 s cap.
var Default = Policy{Attempts: 3, Base: 50 * time.Millisecond, Max: time.Second}

var errPolicy = errors.New("retry: need Attempts >= 1 and 0 < Base <= Max")

// Validate states backoff's preconditions; Do calls it first, so
// rand.N never sees a zero or negative bound.
func (p Policy) Validate() error {
	if p.Attempts < 1 || p.Base <= 0 || p.Max < p.Base {
		return errPolicy
	}
	return nil
}

// backoff needs a valid Policy and n >= 0 (Do counts up from 0).
func (p Policy) backoff(n int) time.Duration {
	d := p.Max
	if p.Base <= p.Max>>n { // so Base<<n can't pass Max
		d = p.Base << n
	}
	return rand.N(d) // math/rand/v2, Go 1.22
}

// Budget lets retries add about ratio of extra load once it is
// empty: each request deposits ratio of a token, each retry
// spends a whole one, and the bucket holds at most max tokens.
type Budget struct {
	mu          sync.Mutex
	tokens, max float64
	ratio       float64
}

// NewBudget starts full, so a healthy dependency never notices.
// ledgerd uses NewBudget(10, 0.1).
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
// because a body can only be read once. Every response it does
// not return is drained and closed; the one it returns, success
// or the last retryable failure, is intact for the caller.
func Do(ctx context.Context, c *http.Client, p Policy, b *Budget,
	mk func() (*http.Request, error)) (*http.Response, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	b.OnRequest() // one deposit per logical call
	for n := 0; ; n++ {
		req, err := mk()
		if err != nil {
			return nil, err
		}
		resp, err := c.Do(req.WithContext(ctx))
		if !retryable(ctx, resp, err) || n+1 >= p.Attempts {
			return resp, err
		}
		wait := max(p.backoff(n), retryAfter(resp, time.Now()))
		if !fits(ctx, p, wait) || !b.TryRetry() {
			return resp, err
		}
		if resp != nil {
			drainClose(resp) // Hack 46
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
}

// fits: a wait past the policy's cap or the caller's deadline
// means give up now and hand back what we have.
func fits(ctx context.Context, p Policy, wait time.Duration) bool {
	dl, ok := ctx.Deadline()
	return wait <= p.Max && (!ok || time.Until(dl) > wait)
}

// retryable: network errors, 429, 502, 503 and 504. Every Do
// error is a *url.Error, which itself satisfies net.Error, so
// look inside it. Once the caller's context is done, stop.
func retryable(ctx context.Context, resp *http.Response, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		var ne net.Error
		return errors.As(err, &ne) || errors.Is(err, io.EOF) ||
			errors.Is(err, io.ErrUnexpectedEOF)
	}
	switch resp.StatusCode {
	case 429, 502, 503, 504:
		return true
	}
	return false
}

// retryAfter reads both forms of Retry-After: delay-seconds and
// an HTTP-date. Missing or malformed means zero.
func retryAfter(resp *http.Response, now time.Time) time.Duration {
	if resp == nil {
		return 0
	}
	v := resp.Header.Get("Retry-After")
	if v == "" {
		return 0
	}
	if s, err := strconv.Atoi(v); err == nil && s >= 0 {
		return time.Duration(min(s, 1<<20)) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil && t.After(now) {
		return t.Sub(now)
	}
	return 0
}

func drainClose(resp *http.Response) {
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
}
