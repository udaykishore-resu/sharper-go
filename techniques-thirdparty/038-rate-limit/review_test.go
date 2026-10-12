package limit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"golang.org/x/time/rate"
)

func hit(h http.Handler, tenant string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/transfers", nil)
	req = req.WithContext(context.WithValue(req.Context(), tenantKey{}, tenant))
	h.ServeHTTP(rec, req)
	return rec
}

func handler(t *testing.T, r rate.Limit, burst int) http.Handler {
	t.Helper()
	p, err := newPerClient(r, burst)
	if err != nil {
		t.Fatal(err)
	}
	return limit(p, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
}

// Retry-After rounds the reservation's delay up to whole seconds.
// One token every 5 s: the second call must wait about 5 s.
func TestRetryAfterMatchesRate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := handler(t, rate.Every(5*time.Second), 1)
		if c := hit(h, "acme").Code; c != 200 {
			t.Fatalf("first call = %d", c)
		}
		time.Sleep(time.Second)
		rec := hit(h, "acme")
		if rec.Code != 429 || rec.Header().Get("Retry-After") != "4" {
			t.Fatalf("code %d Retry-After %q, want 429 and 4",
				rec.Code, rec.Header().Get("Retry-After"))
		}
	})
}

// Rejected calls hand their token back, so a client that was
// refused six times is admitted again after one refill interval,
// not after six.
func TestRejectedCallsDoNotExtendLockout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := handler(t, 2, 4)
		for range 10 {
			hit(h, "acme")
		}
		time.Sleep(500 * time.Millisecond) // one token at 2/s
		if c := hit(h, "acme").Code; c != 200 {
			t.Fatalf("after one interval = %d, want 200", c)
		}
		if c := hit(h, "acme").Code; c != 429 {
			t.Fatalf("second call = %d, want 429", c)
		}
	})
}

// Burst is per tenant: one tenant's spike leaves another's bucket
// full, and a missing tenant shares one strict bucket.
func TestBucketsArePerTenant(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := handler(t, 2, 4)
		for range 4 {
			hit(h, "acme")
		}
		for i := range 4 {
			if c := hit(h, "globex").Code; c != 200 {
				t.Fatalf("globex call %d = %d", i, c)
			}
		}
		if c := hit(h, "").Code; c != 200 {
			t.Fatalf("anonymous first call = %d", c)
		}
		if c := hit(h, "").Code; c != 429 {
			t.Fatalf("anonymous second call = %d, want 429", c)
		}
	})
}

// A limiter that can never grant is refused at construction.
func TestNeverGrantingLimiterRefused(t *testing.T) {
	for _, c := range []struct {
		r rate.Limit
		b int
	}{{2, 0}, {0, 4}, {-1, 4}} {
		if _, err := newPerClient(c.r, c.b); err == nil {
			t.Errorf("newPerClient(%v, %d) accepted", c.r, c.b)
		}
	}
	// Why: the zero Limiter's reservation is not OK and its Delay
	// is InfDuration, which would become an absurd Retry-After.
	var zero rate.Limiter
	if res := zero.Reserve(); res.OK() || res.Delay() != rate.InfDuration {
		t.Fatalf("zero limiter: OK=%v Delay=%v", res.OK(), res.Delay())
	}
}
