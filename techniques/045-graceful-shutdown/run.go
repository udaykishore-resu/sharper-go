// Package shutdown is Technique 45: graceful shutdown with
// signal.NotifyContext.
package shutdown

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
)

type Health struct{ draining atomic.Bool }

func (h *Health) Ready(w http.ResponseWriter, _ *http.Request) {
	if h.draining.Load() {
		http.Error(w, "not ready", 503)
		return
	}
	w.Write([]byte("ready\n"))
}

type API struct{}

func (API) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /readyz", health.Ready)
	mux.HandleFunc("GET /slow", slow)
	return mux
}

var (
	health Health
	api    API
	// The book uses literals; tests shorten these.
	addr  = ":8080"
	drain = 5 * time.Second
	grace = 20 * time.Second
	// slow stands in for a handler that ignores cancellation.
	slow = func(http.ResponseWriter, *http.Request) {}
)

func newServer(h http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: h,
		ReadHeaderTimeout: 5 * time.Second}
}

func run() error {
	ctx, stop := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv := newServer(api.Routes())
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	select {
	case err := <-errc:
		return err // failed to bind, etc.
	case <-ctx.Done():
	}
	stop()                      // a second Ctrl-C now kills the process
	health.draining.Store(true) // /readyz fails
	time.Sleep(drain)
	sctx, cancel := context.WithTimeout(
		context.Background(), grace)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		srv.Close() // deadline passed: cut the stragglers
		return err
	}
	return nil
}
