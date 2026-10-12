package shutdown

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"syscall"
	"testing"
	"time"
)

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

func status(url string) int {
	resp, err := http.Get(url)
	if err != nil {
		return 0
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestDrainThenShutdown(t *testing.T) {
	addr, drain = freeAddr(t), 300*time.Millisecond
	health.draining.Store(false)
	url := "http://" + addr + "/readyz"
	done := make(chan error, 1)
	go func() { done <- run() }()
	for i := 0; status(url) != 200; i++ {
		if i > 200 {
			t.Fatal("server never came up")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if got := status(url); got != 503 {
		t.Fatalf("during drain /readyz = %d, want 503 and still serving", got)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if status(url) != 0 {
		t.Fatal("server still up after Shutdown")
	}
}

// A taken port returns at once instead of waiting for a signal.
func TestBindFailureReturns(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	addr = ln.Addr().String()
	done := make(chan error, 1)
	go func() { done <- run() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("want a bind error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run waited for a signal on a taken port")
	}
}

// Shutdown alone waits; when its deadline passes it returns an
// error and leaves the straggler running. run then calls Close,
// which cuts the client's connection. The handler itself is not
// stopped: only its own context checks can do that.
func TestShutdownDeadlineThenClose(t *testing.T) {
	addr, drain, grace = freeAddr(t), 50*time.Millisecond, 200*time.Millisecond
	health.draining.Store(false)
	defer func() { grace = 20 * time.Second }()
	release := make(chan struct{})
	entered := make(chan struct{})
	slow = func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release // ignores r.Context()
	}
	defer func() { slow = func(http.ResponseWriter, *http.Request) {} }()
	defer close(release)
	done := make(chan error, 1)
	go func() { done <- run() }()
	for i := 0; status("http://"+addr+"/readyz") != 200; i++ {
		if i > 200 {
			t.Fatal("server never came up")
		}
		time.Sleep(10 * time.Millisecond)
	}
	clientErr := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://" + addr + "/slow")
		if err == nil {
			resp.Body.Close()
		}
		clientErr <- err
	}()
	<-entered
	start := time.Now()
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("run = %v, want DeadlineExceeded", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("shutdown took %v", d)
	}
	select {
	case err := <-clientErr:
		if err == nil {
			t.Fatal("straggler got a response after Close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not cut the straggler's connection")
	}
}
