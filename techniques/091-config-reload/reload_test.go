package config

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
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
		time.Sleep(10 * time.Second)
		synctest.Wait()
		if r.cur.Load().Version != "v1" || r.fails.Load() != 1 {
			t.Errorf("bad file applied: %+v", r.cur.Load())
		}
		write(t, r.Path, `{"version":"v3","log_level":"DEBUG","max_transfer_cents":900}`)
		time.Sleep(10 * time.Second)
		synctest.Wait()
		if c := r.cur.Load(); c.Version != "v3" || r.Level.Level() != slog.LevelDebug {
			t.Errorf("not applied: %+v", c)
		}
		cancel()
		<-done
	})
}

// Review: malformed, trailing or invalid files are rejected
// whole, and the old snapshot stays live.
func TestRejectsBadFilesWhole(t *testing.T) {
	for _, body := range []string{
		`{"version":"v2",`,
		`{"version":"v2","max_transfer_cents":9} junk`,
		`{"version":"v2","max_transfer_cents":0}`,
		`{"version":"v2","log_level":"LOUD","max_transfer_cents":9}`,
		``,
	} {
		r := newReloader(t, `{"version":"v1","max_transfer_cents":500}`)
		if err := r.Reload(); err != nil {
			t.Fatal(err)
		}
		write(t, r.Path, body)
		if r.Reload() == nil || r.cur.Load().Version != "v1" {
			t.Fatalf("accepted %q", body)
		}
	}
}

// An unchanged file is one compare: the snapshot is not swapped.
func TestIdenticalFileIsNoop(t *testing.T) {
	r := newReloader(t, `{"version":"v1","max_transfer_cents":500}`)
	r.Reload()
	first := r.cur.Load()
	for range 3 {
		if err := r.Reload(); err != nil || r.cur.Load() != first {
			t.Fatal("unchanged file swapped the snapshot")
		}
	}
}

// SIGHUP reloads long before the next tick.
func TestSIGHUPReloads(t *testing.T) {
	guard := make(chan os.Signal, 1) // a stray SIGHUP must not exit
	signal.Notify(guard, syscall.SIGHUP)
	defer signal.Stop(guard)
	r := newReloader(t, `{"version":"v1","max_transfer_cents":500}`)
	r.Reload()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { r.Watch(ctx, time.Hour); close(done) }()
	defer func() { cancel(); <-done }()
	write(t, r.Path, `{"version":"v2","max_transfer_cents":700}`)
	for end := time.Now().Add(5 * time.Second); r.cur.Load().Version != "v2"; {
		if time.Now().After(end) {
			t.Fatal("no reload on SIGHUP")
		}
		syscall.Kill(os.Getpid(), syscall.SIGHUP)
		time.Sleep(10 * time.Millisecond)
	}
}

// Readers load once per request and never see a torn config.
// Run with -race.
func TestReadersDuringReload(t *testing.T) {
	r := newReloader(t, `{"version":"v1","max_transfer_cents":1}`)
	r.Reload()
	stop := make(chan struct{})
	var wg sync.WaitGroup
	var torn atomic.Bool
	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				c := r.cur.Load()
				if c.Version != fmt.Sprintf("v%d", c.MaxTransfer) {
					torn.Store(true)
				}
			}
		})
	}
	for i := 2; i < 100; i++ {
		write(t, r.Path, fmt.Sprintf(
			`{"version":"v%d","max_transfer_cents":%d}`, i, i))
		r.Reload()
	}
	close(stop)
	wg.Wait()
	if torn.Load() {
		t.Fatal("a reader saw a torn config")
	}
}
