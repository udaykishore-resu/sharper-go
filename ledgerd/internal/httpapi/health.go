package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"example.com/ledgerd/internal/obs"
)

// Health answers the kubelet and people (Technique 49).
// Liveness depends on nothing outside the process; readiness has
// three reasons to say no.
type Health struct {
	ready, draining atomic.Bool
	ping            func(context.Context) error
	version         func() []byte
}

// NewHealth takes the store's ping. It starts not ready.
func NewHealth(ping func(context.Context) error) *Health {
	return &Health{ping: ping,
		version: sync.OnceValue(func() []byte {
			b, _ := json.Marshal(obs.BuildInfo())
			return append(b, '\n')
		})}
}

// SetReady flips once startup work is done.
func (h *Health) SetReady() { h.ready.Store(true) }

// Drain fails readiness for the shutdown drain (Technique 45).
func (h *Health) Drain() { h.draining.Store(true) }

// Live never checks the database: a database blip must not
// restart every pod at once.
func (h *Health) Live(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte("ok\n"))
}

func (h *Health) Ready(w http.ResponseWriter,
	r *http.Request) {
	if h.draining.Load() || !h.ready.Load() {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(),
		500*time.Millisecond)
	defer cancel()
	if err := h.ping(ctx); err != nil {
		http.Error(w, "db: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte("ready\n"))
}

// Version serves the build info, built once (Technique 6).
func (h *Health) Version(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(h.version())
}

// Mux serves the probes and /version itself and sends
// everything else to api, so the kubelet never meets auth.
func (h *Health) Mux(api http.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.Live)
	mux.HandleFunc("GET /readyz", h.Ready)
	mux.HandleFunc("GET /version", h.Version)
	mux.Handle("/", api)
	return mux
}
