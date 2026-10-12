// Package client is Technique 46: one shared http.Client per
// downstream, with a tuned Transport.
package client

import (
	"io"
	"net/http"
	"time"
)

func newRiskClient() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = 200
	t.MaxIdleConnsPerHost = 64
	t.MaxConnsPerHost = 128
	t.IdleConnTimeout = 60 * time.Second
	t.ResponseHeaderTimeout = 3 * time.Second
	return &http.Client{Transport: t,
		Timeout: 5 * time.Second}
}

// drainClose finishes the body so the connection goes back to
// the pool. The limit stops a huge error page from stalling you.
func drainClose(resp *http.Response) {
	io.Copy(io.Discard,
		io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
}
