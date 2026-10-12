//go:build go1.27

package fx

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// closeCounter wraps a transport and counts response bodies
// opened and closed, so each path can prove it closed its body.
type closeCounter struct {
	rt             http.RoundTripper
	opened, closed atomic.Int32
}

func (c *closeCounter) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := c.rt.RoundTrip(r)
	if err != nil {
		return nil, err
	}
	c.opened.Add(1)
	resp.Body = &countedBody{ReadCloser: resp.Body, n: &c.closed}
	return resp, nil
}

type countedBody struct {
	io.ReadCloser
	n    *atomic.Int32
	done bool
}

func (b *countedBody) Close() error {
	if !b.done {
		b.done = true
		b.n.Add(1)
	}
	return b.ReadCloser.Close()
}

func newCounted(t *testing.T, h http.HandlerFunc) (*Client, *closeCounter) {
	t.Helper()
	c := newFX(t, h)
	cc := &closeCounter{rt: c.HTTP.Transport}
	c.HTTP = &http.Client{Transport: cc}
	return c, cc
}

// Review check for Technique 66: every failure path returns a
// useful error and closes every response body it opened.
func TestRateFailurePaths(t *testing.T) {
	cases := []struct {
		name    string
		h       http.HandlerFunc
		wantErr string
	}{
		{"500 is not retried", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(500)
		}, "500 Internal Server Error"},
		{"503 three times", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(503)
		}, "503 Service Unavailable"},
		{"malformed body", func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte(`{"ppm": 1`))
		}, "bad body"},
		{"wrong type", func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte(`{"ppm": "lots"}`))
		}, "bad body"},
		{"reset mid-body", func(w http.ResponseWriter, _ *http.Request) {
			conn, buf, _ := w.(http.Hijacker).Hijack()
			buf.WriteString("HTTP/1.1 200 OK\r\n" +
				"Content-Length: 100\r\n\r\n{\"ppm\":")
			buf.Flush()
			conn.Close()
		}, "bad body"},
		{"hangup before headers", func(w http.ResponseWriter, _ *http.Request) {
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
		}, "EOF"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, cc := newCounted(t, tc.h)
			_, err := c.Rate(t.Context(), "EUR", "USD")
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.wantErr)
			}
			if o, cl := cc.opened.Load(), cc.closed.Load(); o != cl {
				t.Fatalf("opened %d bodies, closed %d", o, cl)
			}
		})
	}
}

// Slow headers: the server never answers, the caller's deadline
// fires, and the error says so. Virtual time keeps it instant.
func TestRateSlowHeadersDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newFX(t, func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(time.Hour):
			}
		})
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		_, err := c.Rate(ctx, "EUR", "USD")
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want DeadlineExceeded", err)
		}
	})
}

// Cancellation during backoff stops the retry loop at once.
func TestRateCanceledDuringBackoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		c := newFX(t, func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			w.WriteHeader(503)
		})
		c.Backoff = []time.Duration{time.Minute, time.Minute}
		ctx, cancel := context.WithCancel(t.Context())
		time.AfterFunc(time.Second, cancel)
		_, err := c.Rate(ctx, "EUR", "USD")
		if !errors.Is(err, context.Canceled) || calls.Load() != 1 {
			t.Fatalf("err = %v after %d calls", err, calls.Load())
		}
	})
}
