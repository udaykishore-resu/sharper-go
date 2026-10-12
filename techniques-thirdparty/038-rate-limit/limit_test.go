package limit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/time/rate"
)

// The Try it: 2 per second with burst 4, hit 10 times.
func TestBurstThen429(t *testing.T) {
	p, err := newPerClient(2, 4)
	if err != nil {
		t.Fatal(err)
	}
	h := limit(p, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	ok, limited := 0, 0
	for range 10 {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/transfers", nil)
		req = req.WithContext(context.WithValue(req.Context(), tenantKey{}, "acme"))
		h.ServeHTTP(rec, req)
		switch rec.Code {
		case 200:
			ok++
		case 429:
			limited++
			if rec.Header().Get("Retry-After") != "1" {
				t.Errorf("Retry-After = %q", rec.Header().Get("Retry-After"))
			}
		}
	}
	if ok != 4 || limited != 6 {
		t.Fatalf("ok=%d limited=%d", ok, limited)
	}
}

// Cancel hands the token back; without it the client stays locked
// out longer than the rate says.
func TestCancelReturnsToken(t *testing.T) {
	l := rate.NewLimiter(1, 1)
	l.Allow()
	res := l.Reserve()
	res.Cancel()
	if got := l.Tokens(); got < -0.01 {
		t.Fatalf("tokens after cancel = %v", got)
	}
}

// The zero Limiter allows no events.
func TestZeroLimiterRejects(t *testing.T) {
	var l rate.Limiter
	if l.Allow() {
		t.Fatal("zero limiter allowed an event")
	}
}
