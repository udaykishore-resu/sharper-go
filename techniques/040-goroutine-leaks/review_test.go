package first

import (
	"bytes"
	"context"
	"net/http/httptest"
	httppprof "net/http/pprof"
	"runtime/pprof"
	"strings"
	"sync"
	"testing"
	"time"
)

// leakProfile runs the goroutineleak profile (a GC cycle that
// looks for goroutines blocked on unreachable channels or locks)
// and returns its text form.
func leakProfile(t *testing.T) string {
	t.Helper()
	p := pprof.Lookup("goroutineleak")
	if p == nil {
		t.Skip("no goroutineleak profile: Go 1.26 needs GOEXPERIMENT=goroutineleakprofile")
	}
	var buf bytes.Buffer
	if err := p.WriteTo(&buf, 1); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// Go 1.27: the leaked senders of FirstLeaky block on a channel
// nothing else can reach, and the profile reports them.
func TestLeakProfileFindsFirstLeaky(t *testing.T) {
	FirstLeaky(t.Context(), replicas())
	time.Sleep(50 * time.Millisecond) // let the losers park on send
	if out := leakProfile(t); !strings.Contains(out, "FirstLeaky") {
		t.Fatalf("profile missed FirstLeaky:\n%s", out)
	}
}

// held is a global that keeps every channel reachable.
var held struct {
	sync.Mutex
	chans []chan Transfer
}

func firstHeld(ctx context.Context, fetch []fetchFunc) Transfer {
	ch := make(chan Transfer)
	held.Lock()
	held.chans = append(held.chans, ch)
	held.Unlock()
	for _, f := range fetch {
		go func() {
			t, _ := f(ctx)
			ch <- t
		}()
	}
	return <-ch
}

// The same leak through a channel a global still references is
// not reported: the profile only sees unreachable primitives.
func TestLeakProfileMissesReachableChannel(t *testing.T) {
	firstHeld(t.Context(), replicas())
	time.Sleep(50 * time.Millisecond)
	if out := leakProfile(t); strings.Contains(out, "firstHeld") {
		t.Fatalf("profile reported a reachable channel:\n%s", out)
	}
}

// noLeaks is a heuristic. A goroutine that exits during the test
// can hide a leak, because only the count is compared: here one
// unrelated goroutine exits, one sender leaks, and the helper
// stays silent. The leak profile above is the stronger check.
func TestHelperCanBeMasked(t *testing.T) {
	rec := &recordTB{TB: t}
	quit, exited := make(chan struct{}), make(chan struct{})
	go func() { <-quit; close(exited) }() // counted in the baseline
	func() {
		noLeaks(rec)
		close(quit)
		<-exited
		FirstLeaky(t.Context(), replicas()[:2]) // one sender leaks
	}()
	rec.runCleanups()
	if rec.msg != "" {
		t.Fatalf("expected the count to mask the leak, got: %s", rec.msg)
	}
}

// net/http/pprof serves the profile by name.
func TestLeakProfileOverHTTP(t *testing.T) {
	if pprof.Lookup("goroutineleak") == nil {
		t.Skip("no goroutineleak profile in this Go")
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/debug/pprof/goroutineleak?debug=1", nil)
	httppprof.Index(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "goroutineleak profile") {
		t.Fatalf("code %d body %.80q", rec.Code, rec.Body.String())
	}
}
