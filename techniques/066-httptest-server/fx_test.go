//go:build go1.27

package fx

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func newFX(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewTestServer(t, h)
	hc := srv.Client() // first call sets srv.URL
	return &Client{BaseURL: srv.URL, HTTP: hc}
}

func TestRateHangup(t *testing.T) {
	c := newFX(t, func(w http.ResponseWriter,
		_ *http.Request) {
		conn, _, _ := w.(http.Hijacker).Hijack()
		conn.Close()
	})
	_, err := c.Rate(t.Context(), "EUR", "USD")
	if err == nil {
		t.Fatal("hangup returned nil error")
	}
}

func TestRateBadBody(t *testing.T) {
	c := newFX(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"ppm": "lots"}`))
	})
	if _, err := c.Rate(t.Context(), "EUR", "USD"); err == nil {
		t.Fatal("bad body accepted")
	}
}

// 503 twice, then 200: exactly three tries, and the three seconds
// of backoff run in virtual time inside a synctest bubble.
func TestRateRetriesInBubble(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		c := newFX(t, func(w http.ResponseWriter, _ *http.Request) {
			if calls.Add(1) < 3 {
				w.WriteHeader(503)
				return
			}
			w.Write([]byte(`{"ppm": 1084000}`))
		})
		c.Backoff = []time.Duration{time.Second, 2 * time.Second}
		start := time.Now()
		ppm, err := c.Rate(t.Context(), "EUR", "USD")
		if err != nil || ppm != 1084000 || calls.Load() != 3 {
			t.Fatalf("ppm=%d err=%v calls=%d", ppm, err, calls.Load())
		}
		if waited := time.Since(start); waited != 3*time.Second {
			t.Fatalf("virtual wait %v, want 3s", waited)
		}
	})
}
