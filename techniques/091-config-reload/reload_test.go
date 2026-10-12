package config

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"testing/synctest"
	"time"
)

// The first signal.Notify starts process-wide goroutines. Start
// them outside any synctest bubble, or the bubble waits on them.
func init() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGUSR2)
	signal.Stop(c)
}

func newReloader(t *testing.T, body string) *Reloader {
	t.Helper()
	p := filepath.Join(t.TempDir(), "live.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return &Reloader{Path: p, Log: slog.New(slog.DiscardHandler), Level: new(slog.LevelVar)}
}

func write(t *testing.T, p, body string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Under testing/synctest the 10-second poll takes no real time.
func TestWatchReloadsAndRejects(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newReloader(t, `{"version":"v1","max_transfer_cents":500}`)
		if err := r.Reload(); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() { r.Watch(ctx, 10*time.Second); close(done) }()

		write(t, r.Path, `{"version":"v2","max_transfer_cents":0}`)
		synctest.Sleep(10 * time.Second)
		if r.cur.Load().Version != "v1" || r.fails.Load() != 1 {
			t.Errorf("bad file applied: %+v", r.cur.Load())
		}
		write(t, r.Path, `{"version":"v3","log_level":"DEBUG","max_transfer_cents":900}`)
		synctest.Sleep(10 * time.Second)
		if c := r.cur.Load(); c.Version != "v3" || r.Level.Level() != slog.LevelDebug {
			t.Errorf("not applied: %+v", c)
		}
		cancel()
		<-done
	})
}
