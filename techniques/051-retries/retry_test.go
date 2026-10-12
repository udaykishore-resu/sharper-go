package retry

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// The Try it: 1,000 calls to a server that always says 503 cost
// about 1,100 requests, not 3,000.
func TestBudgetCapsLoad(t *testing.T) {
	var seen atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		seen.Add(1)
		w.WriteHeader(503)
	}))
	defer srv.Close()
	p := Policy{Attempts: 3, Base: time.Microsecond, Max: time.Millisecond}
	b := NewBudget(10, 0.1)
	for range 1000 {
		resp, err := Do(t.Context(), srv.Client(), p, b, func() (*http.Request, error) {
			return http.NewRequest("GET", srv.URL, nil)
		})
		if err != nil {
			t.Fatal(err)
		}
		drainClose(resp)
	}
	if n := seen.Load(); n < 1100 || n > 1120 {
		t.Fatalf("server saw %d requests, want about 1,100", n)
	}
}

func TestBackoffBounds(t *testing.T) {
	p := Default
	for n := range 70 { // n past 63 overflows the shift
		if d := p.backoff(n); d < 0 || d > p.Max {
			t.Fatalf("backoff(%d) = %v", n, d)
		}
	}
}

func TestNoRetryOn4xx(t *testing.T) {
	var seen atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		seen.Add(1)
		w.WriteHeader(422)
	}))
	defer srv.Close()
	resp, _ := Do(t.Context(), srv.Client(), Default, NewBudget(10, 0.1),
		func() (*http.Request, error) { return http.NewRequest("POST", srv.URL, nil) })
	drainClose(resp)
	if seen.Load() != 1 {
		t.Fatalf("a 422 was retried %d times", seen.Load()-1)
	}
}
