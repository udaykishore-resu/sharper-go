package config

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"testing/synctest"
	"time"
)

// The first signal.Notify in a process starts runtime goroutines
// that live forever. Start them here, outside any synctest
// bubble, or the bubble waits on them and the test deadlocks.
func init() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGUSR2)
	signal.Stop(c)
}

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	c, err := Load(env(map[string]string{
		"DATABASE_URL": "postgres://x",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.ReqTimeout != 10*time.Second || c.Grace != 20*time.Second ||
		c.MaxConns != 12 || c.LogLevel != slog.LevelInfo || c.Sink != "log" {
		t.Fatalf("defaults: %+v", c)
	}
}

// A bad deploy shows every typo in one crash (Technique 22).
func TestLoadReportsEveryProblem(t *testing.T) {
	_, err := Load(env(map[string]string{
		"LOG_LEVEL":       "loud",
		"REQUEST_TIMEOUT": "5sec",
	}))
	msg := err.Error()
	for _, want := range []string{"DATABASE_URL", "LOG_LEVEL", "REQUEST_TIMEOUT"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not mention %s", msg, want)
		}
	}
}

func TestParseKeys(t *testing.T) {
	keys, err := parseKeys("acme:0123456789abcdef, globex:fedcba9876543210")
	if err != nil || keys["0123456789abcdef"] != "acme" ||
		keys["fedcba9876543210"] != "globex" {
		t.Fatalf("keys = %v, %v", keys, err)
	}
	if _, err := parseKeys("acme:short"); err == nil {
		t.Fatal("short token accepted")
	}
}

func newReloader(t *testing.T, body string) *Reloader {
	t.Helper()
	p := filepath.Join(t.TempDir(), "live.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return &Reloader{Path: p, Log: slog.New(slog.DiscardHandler),
		Level: new(slog.LevelVar)}
}

func TestReloadRejectsBadFileWhole(t *testing.T) {
	r := newReloader(t, `{"version":"v1","log_level":"DEBUG","max_transfer_cents":500}`)
	if err := r.Reload(); err != nil {
		t.Fatal(err)
	}
	if r.MaxTransfer() != 500 || r.Level.Level() != slog.LevelDebug {
		t.Fatalf("applied: %+v level %v", r.Get(), r.Level.Level())
	}
	err := os.WriteFile(r.Path,
		[]byte(`{"version":"v2","max_transfer_cents":0}`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Reload(); err == nil {
		t.Fatal("zero limit accepted")
	}
	if r.Get().Version != "v1" || r.Fails() != 1 {
		t.Fatalf("old config not kept: %+v fails=%d", r.Get(), r.Fails())
	}
}

// Technique 91: test the poll loop in a synctest bubble, no real sleeps.
func TestWatchPolls(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newReloader(t, `{"version":"v1","max_transfer_cents":500}`)
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() { r.Watch(ctx, 10*time.Second); close(done) }()
		err := os.WriteFile(r.Path,
			[]byte(`{"version":"v2","max_transfer_cents":700}`), 0o600)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Second)
		synctest.Wait()
		if c := r.Get(); c == nil || c.Version != "v2" {
			t.Errorf("after one tick: %+v", c)
		}
		cancel()
		<-done
	})
}

// Technique 91 review: every one of these files is rejected
// whole, and the old snapshot stays live.
func TestReloadRejectsBadFiles(t *testing.T) {
	good := `{"version":"v1","max_transfer_cents":500}`
	for name, body := range map[string]string{
		"malformed":     `{"version":"v2",`,
		"trailing data": `{"version":"v2","max_transfer_cents":9}{}`,
		"unknown field": `{"version":"v2","max_transfer":9}`,
		"no version":    `{"max_transfer_cents":9}`,
		"no limit":      `{"version":"v2"}`,
		"negative":      `{"version":"v2","max_transfer_cents":-1}`,
		"bad level":     `{"version":"v2","log_level":"LOUD","max_transfer_cents":9}`,
		"null":          `null`,
		"empty":         ``,
	} {
		t.Run(name, func(t *testing.T) {
			r := newReloader(t, good)
			if err := r.Reload(); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(r.Path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := r.Reload(); err == nil {
				t.Fatalf("accepted %q", body)
			}
			if c := r.Get(); c.Version != "v1" || c.MaxTransfer != 500 || r.Fails() != 1 {
				t.Fatalf("old config not kept: %+v fails=%d", c, r.Fails())
			}
		})
	}
}

// An unchanged file costs one compare: no swap, no OnApply, no
// log line, no failure.
func TestReloadIdenticalFileIsNoop(t *testing.T) {
	r := newReloader(t, `{"version":"v1","max_transfer_cents":500}`)
	applied := 0
	r.OnApply = func(*Live) { applied++ }
	for range 3 {
		if err := r.Reload(); err != nil {
			t.Fatal(err)
		}
	}
	first := r.Get()
	if err := r.Reload(); err != nil || r.Get() != first {
		t.Fatal("unchanged file swapped the snapshot")
	}
	if applied != 1 || r.Fails() != 0 {
		t.Fatalf("applied %d times, fails %d", applied, r.Fails())
	}
}

// SIGHUP reloads at once, long before the next poll tick.
func TestReloadOnSIGHUP(t *testing.T) {
	// Keep a SIGHUP that lands before Watch registers from
	// killing the test binary (Go's default is to exit).
	guard := make(chan os.Signal, 1)
	signal.Notify(guard, syscall.SIGHUP)
	defer signal.Stop(guard)

	r := newReloader(t, `{"version":"v1","max_transfer_cents":500}`)
	if err := r.Reload(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { r.Watch(ctx, time.Hour); close(done) }()
	defer func() { cancel(); <-done }()
	err := os.WriteFile(r.Path,
		[]byte(`{"version":"v2","max_transfer_cents":700}`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for r.Get().Version != "v2" {
		if time.Now().After(deadline) {
			t.Fatal("SIGHUP did not trigger a reload")
		}
		// Resend until Watch has registered its own channel.
		syscall.Kill(os.Getpid(), syscall.SIGHUP)
		time.Sleep(10 * time.Millisecond)
	}
}

// Readers never see a torn or half-applied snapshot while good
// and bad files alternate. Run with -race.
func TestConcurrentReadersDuringReload(t *testing.T) {
	r := newReloader(t, `{"version":"v100","max_transfer_cents":100}`)
	if err := r.Reload(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	errc := make(chan error, 8)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for ctx.Err() == nil {
				c := r.Get()
				if c == nil || c.Version != fmt.Sprintf("v%d", c.MaxTransfer) {
					errc <- fmt.Errorf("torn snapshot: %+v", c)
					return
				}
			}
		})
	}
	for i := range 200 {
		body := fmt.Sprintf(`{"version":"v%d","max_transfer_cents":%d}`, i+1, i+1)
		if i%3 == 0 {
			body = `{"version":"bad","max_transfer_cents":0}`
		}
		if err := os.WriteFile(r.Path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		r.Reload() // bad files fail; readers keep the old one
	}
	cancel()
	wg.Wait()
	close(errc)
	for err := range errc {
		t.Fatal(err)
	}
	if r.Fails() != 67 {
		t.Fatalf("fails = %d, want 67", r.Fails())
	}
}
