package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"example.com/ledgerd/internal/config"
	"example.com/ledgerd/internal/httpapi"
	"example.com/ledgerd/internal/obs"
	"example.com/ledgerd/internal/sink"
	"example.com/ledgerd/internal/store"
)

// drainDelay keeps serving after readiness fails, while the
// endpoints controller removes the pod (Technique 45). Drain plus
// SHUTDOWN_GRACE must stay under terminationGracePeriodSeconds.
var drainDelay = 5 * time.Second

func serve(ctx context.Context, cfg config.Config,
	log *slog.Logger, st *store.Store) error {
	reg := prometheus.NewRegistry()
	m := obs.NewMetrics(reg)
	m.WatchPool(st.AcquiredConns)
	var opts []httpapi.Option
	if cfg.LiveConfig != "" {
		rel := &config.Reloader{Path: cfg.LiveConfig, Log: log,
			Level: obs.Level, OnApply: func(c *config.Live) {
				obs.ConfigVersion.Set(c.Version)
			}}
		if err := rel.Reload(); err != nil {
			return fmt.Errorf("live config: %w", err)
		}
		go rel.Watch(ctx, 10*time.Second)
		opts = append(opts, httpapi.WithLimit(rel.MaxTransfer))
	}
	api := httpapi.Chain(httpapi.New(st, log, opts...).Routes(),
		httpapi.RequestID,
		httpapi.Auth(cfg.Keys),
		httpapi.Deadline(cfg.ReqTimeout),
		otelhttp.NewMiddleware("ledgerd"),
		httpapi.AccessLog(log),
		m.Wrap,
		httpapi.Recover(log),
		httpapi.LimitBody(1<<20),
	)
	h := httpapi.NewHealth(st.Ping)
	root := h.Mux(api) // probes skip auth
	snk, err := sink.New(cfg.Sink, nil)
	if err != nil {
		return err
	}
	rctx, stopRelay := context.WithCancelCause(context.WithoutCancel(ctx))
	relay := st.StartRelay(rctx, snk.Publish, log, func(int) {
		obs.OutboxLastRun.Set(time.Now().UTC().Format(time.RFC3339))
	})
	err = runServers(ctx, cfg.Grace, h,
		httpapi.NewServer(":8080", root),
		internal(":9090", promhttp.HandlerFor(
			reg, promhttp.HandlerOpts{})),
		internal("127.0.0.1:6060", obs.DebugMux()),
	)
	// Tear down in reverse: servers are drained, now the relay,
	// and run closes the pool last.
	stopRelay(errors.New("shutdown: servers drained"))
	<-relay.Done()
	return err
}

// internal builds the metrics and debug servers: the same
// timeouts, except a write timeout long enough for a 30-second
// CPU profile from net/http/pprof.
func internal(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      65 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
}

// runServers is Technique 45 grown to several servers. Every
// listener binds before anything is marked ready, so a taken
// port fails run at once. On a signal it fails /readyz, waits
// drainDelay, then shuts every server down under grace.
func runServers(ctx context.Context, grace time.Duration,
	h *httpapi.Health, srvs ...*http.Server) error {
	lns := make([]net.Listener, 0, len(srvs))
	for _, s := range srvs {
		ln, err := net.Listen("tcp", s.Addr)
		if err != nil {
			for _, l := range lns {
				l.Close()
			}
			return fmt.Errorf("listen %s: %w", s.Addr, err)
		}
		lns = append(lns, ln)
	}
	errc := make(chan error, len(srvs))
	for i, s := range srvs {
		go func() { errc <- s.Serve(lns[i]) }()
	}
	h.SetReady()
	slog.InfoContext(ctx, "serving", "addrs", addrs(lns))
	select {
	case err := <-errc:
		shutdown(context.WithoutCancel(ctx), grace, srvs)
		return err // failed to serve
	case <-ctx.Done():
	}
	slog.InfoContext(ctx, "shutting down", "cause", context.Cause(ctx))
	h.Drain() // /readyz fails
	time.Sleep(drainDelay)
	return shutdown(context.WithoutCancel(ctx), grace, srvs)
}

// shutdown drains every server at once under one deadline, then
// cuts the stragglers.
func shutdown(ctx context.Context, grace time.Duration,
	srvs []*http.Server) error {
	sctx, cancel := context.WithTimeout(ctx, grace)
	defer cancel()
	var (
		mu   sync.Mutex
		errs []error
		wg   sync.WaitGroup
	)
	for _, s := range srvs {
		wg.Go(func() {
			if err := s.Shutdown(sctx); err != nil {
				s.Close()
				mu.Lock()
				errs = append(errs, fmt.Errorf("shutdown %s: %w", s.Addr, err))
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	return errors.Join(errs...)
}

func addrs(lns []net.Listener) []string {
	out := make([]string, len(lns))
	for i, l := range lns {
		out[i] = l.Addr().String()
	}
	return out
}
