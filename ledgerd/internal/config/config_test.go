package config

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
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
		synctest.Sleep(10 * time.Second)
		if c := r.Get(); c == nil || c.Version != "v2" {
			t.Errorf("after one tick: %+v", c)
		}
		cancel()
		<-done
	})
}
