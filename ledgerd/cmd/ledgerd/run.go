package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"example.com/ledgerd/internal/config"
	"example.com/ledgerd/internal/obs"
	"example.com/ledgerd/internal/store"
)

// flushBudget bounds the trace flush after the servers drain.
// It is the last term of the shutdown budget that
// k8s/deployment.yaml must cover (TestGraceBudgetFitsManifest).
const flushBudget = 5 * time.Second

// run is the startup list, in order: signals, config, logger
// and tracer, store, then the servers. Defers run bottom to top:
// servers drain, then the pool closes, then spans flush.
func run() error {
	ctx, stop := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cmd, err := subcommand(os.Args[1:])
	if err != nil {
		return err
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log := obs.NewLogger(os.Stderr, obs.Version)
	obs.Level.Set(cfg.LogLevel)
	slog.SetDefault(log)
	flush, err := obs.SetupTracing(ctx, obs.Version)
	if err != nil {
		return fmt.Errorf("tracing: %w", err)
	}
	defer func() {
		// The signal context is already cancelled here, so the
		// flush gets its own budget on a fresh context.
		fctx, cancel := context.WithTimeout(
			context.WithoutCancel(ctx), flushBudget)
		defer cancel()
		if err := flush(fctx); err != nil {
			log.Warn("trace flush", "err", err)
		}
	}()
	st, err := store.Open(ctx, cfg.DatabaseURL,
		store.WithMaxConns(cfg.MaxConns))
	if err != nil {
		return err
	}
	defer st.Close()
	if cmd == "migrate" {
		return st.Migrate(ctx, log)
	}
	log.Info("starting", "build", obs.BuildInfo())
	return serve(ctx, cfg, log, st)
}

func subcommand(args []string) (string, error) {
	switch {
	case len(args) == 0:
		return "serve", nil
	case len(args) == 1 && (args[0] == "serve" || args[0] == "migrate"):
		return args[0], nil
	}
	return "", fmt.Errorf("usage: ledgerd [serve|migrate], got %q", args)
}
