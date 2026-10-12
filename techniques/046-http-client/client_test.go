package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"sync/atomic"
	"testing"
)

func TestConnectionsAreReused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(make([]byte, 4096))
	}))
	defer srv.Close()
	c := newRiskClient()
	var reused atomic.Int32
	trace := &httptrace.ClientTrace{GotConn: func(i httptrace.GotConnInfo) {
		if i.Reused {
			reused.Add(1)
		}
	}}
	for range 50 {
		ctx := httptrace.WithClientTrace(context.Background(), trace)
		req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL, nil)
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		drainClose(resp)
	}
	if reused.Load() < 49 {
		t.Fatalf("reused %d of 50 connections", reused.Load())
	}
}

// drainClose reads at most 64 KiB. A smaller body is finished,
// so its connection is reused. A body far past the limit is not
// worth reading: Close drops that connection. (Go 1.27.2's
// transport also tries its own short drain on Close, up to
// 256 KiB in 50 ms; see maxPostCloseReadBytes in transport.go.)
func TestDrainLimit(t *testing.T) {
	for _, c := range []struct {
		size  int
		reuse bool
	}{{60 << 10, true}, {1 << 20, false}} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Write(make([]byte, c.size))
		}))
		cl := newRiskClient()
		var reused atomic.Int32
		trace := &httptrace.ClientTrace{GotConn: func(i httptrace.GotConnInfo) {
			if i.Reused {
				reused.Add(1)
			}
		}}
		for range 2 {
			ctx := httptrace.WithClientTrace(context.Background(), trace)
			req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL, nil)
			resp, err := cl.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			drainClose(resp)
		}
		if got := reused.Load() == 1; got != c.reuse {
			t.Errorf("%d-byte body: reused=%v, want %v", c.size, got, c.reuse)
		}
		srv.Close()
	}
}
