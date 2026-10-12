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
