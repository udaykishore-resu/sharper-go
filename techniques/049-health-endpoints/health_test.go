package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func code(f http.HandlerFunc) int {
	rec := httptest.NewRecorder()
	f(rec, httptest.NewRequest("GET", "/", nil))
	return rec.Code
}

func TestReadiness(t *testing.T) {
	dbDown := false
	h := &Health{ping: func(ctx context.Context) error {
		if dbDown {
			return errors.New("connection refused")
		}
		return nil
	}}
	if code(h.Ready) != 503 {
		t.Fatal("ready before startup finished")
	}
	h.ready.Store(true)
	if code(h.Ready) != 200 {
		t.Fatal("not ready after startup")
	}
	dbDown = true
	if code(h.Ready) != 503 || code(h.Live) != 200 {
		t.Fatal("a database outage must fail readiness only")
	}
	dbDown = false
	h.draining.Store(true)
	if code(h.Ready) != 503 {
		t.Fatal("draining must fail readiness")
	}
}

// A probe that hangs is worse than one that fails: the ping is
// bounded at 500 ms.
func TestPingIsBounded(t *testing.T) {
	h := &Health{ping: func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}}
	h.ready.Store(true)
	start := time.Now()
	if code(h.Ready) != 503 || time.Since(start) > 2*time.Second {
		t.Fatal("probe hung")
	}
}
