// Package health is Technique 49: liveness depends on nothing
// outside the process; readiness has three reasons to say no.
package health

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"
)

type Health struct {
	ready, draining atomic.Bool
	ping            func(context.Context) error
}

func (h *Health) Live(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("ok\n"))
}

func (h *Health) Ready(w http.ResponseWriter,
	r *http.Request) {
	if h.draining.Load() || !h.ready.Load() {
		http.Error(w, "not ready", 503)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(),
		500*time.Millisecond)
	defer cancel()
	if err := h.ping(ctx); err != nil {
		http.Error(w, "db: "+err.Error(), 503)
		return
	}
	w.Write([]byte("ready\n"))
}
