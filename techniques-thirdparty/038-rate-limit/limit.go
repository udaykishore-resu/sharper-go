// Package limit is Technique 38: a token bucket per tenant with
// an accurate Retry-After.
package limit

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"sync"

	"golang.org/x/time/rate"
)

type tenantKey struct{}

// tenant reads what the auth middleware stored. A missing tenant
// gets the shared, strict bucket "".
func tenant(ctx context.Context) string {
	t, _ := ctx.Value(tenantKey{}).(string)
	return t
}

// perClient is a mutex and a map of limiters, one per tenant.
type perClient struct {
	mu    sync.Mutex
	m     map[string]*rate.Limiter
	r     rate.Limit
	burst int
}

// newPerClient refuses settings that build a limiter which can
// never grant: a burst below 1 (the zero rate.Limiter{} has burst
// 0) or a rate that is not positive. Such a limiter's Delay is
// rate.InfDuration, so every request would get 429.
func newPerClient(r rate.Limit, burst int) (*perClient, error) {
	if burst < 1 || r <= 0 {
		return nil, fmt.Errorf("limit: rate %v burst %d never grants", r, burst)
	}
	return &perClient{m: map[string]*rate.Limiter{}, r: r, burst: burst}, nil
}

func (p *perClient) get(tenant string) *rate.Limiter {
	p.mu.Lock()
	defer p.mu.Unlock()
	l, ok := p.m[tenant]
	if !ok {
		l = rate.NewLimiter(p.r, p.burst)
		if tenant == "" {
			l = rate.NewLimiter(p.r/10, 1)
		}
		p.m[tenant] = l
	}
	return l
}

func limit(p *perClient,
	next http.Handler) http.Handler {
	return http.HandlerFunc(func(
		w http.ResponseWriter, r *http.Request) {
		res := p.get(tenant(r.Context())).Reserve()
		if d := res.Delay(); d > 0 {
			res.Cancel()
			secs := int(math.Ceil(d.Seconds()))
			w.Header().Set("Retry-After",
				strconv.Itoa(secs))
			http.Error(w, "slow down",
				http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
