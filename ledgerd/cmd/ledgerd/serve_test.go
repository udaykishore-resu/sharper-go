package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"example.com/ledgerd/internal/httpapi"
)

func ready(h *httpapi.Health) int {
	rec := httptest.NewRecorder()
	h.Ready(rec, httptest.NewRequest("GET", "/readyz", nil))
	return rec.Code
}

// Technique 45: on cancel, readiness fails first, the servers keep
// serving through the drain, then everything shuts down.
func TestRunServersDrains(t *testing.T) {
	drainDelay = 200 * time.Millisecond
	h := httpapi.NewHealth(func(context.Context) error { return nil })
	srv := httpapi.NewServer("127.0.0.1:0", h.Mux(http.NotFoundHandler()))
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- runServers(ctx, time.Second, h, srv) }()
	deadline := time.Now().Add(2 * time.Second)
	for ready(h) != 200 {
		if time.Now().After(deadline) {
			t.Fatal("never became ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	time.Sleep(50 * time.Millisecond)
	if ready(h) != 503 {
		t.Fatal("readiness still passing during drain")
	}
	select {
	case err := <-done:
		t.Fatalf("returned before the drain delay: %v", err)
	default:
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// A taken port fails before anything is marked ready.
func TestRunServersPortTaken(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	h := httpapi.NewHealth(func(context.Context) error { return nil })
	err = runServers(t.Context(), time.Second, h,
		httpapi.NewServer(ln.Addr().String(), http.NotFoundHandler()))
	if err == nil || ready(h) == 200 {
		t.Fatalf("err=%v ready=%d", err, ready(h))
	}
}

func TestSubcommand(t *testing.T) {
	for in, want := range map[string]string{"": "serve", "serve": "serve", "migrate": "migrate"} {
		var args []string
		if in != "" {
			args = []string{in}
		}
		if got, err := subcommand(args); err != nil || got != want {
			t.Errorf("subcommand(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := subcommand([]string{"migrate", "now"}); err == nil {
		t.Error("extra args accepted")
	}
}
