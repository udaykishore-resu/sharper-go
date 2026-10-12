package first

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
)

func noLeaks(t testing.TB) {
	t.Helper()
	before := runtime.NumGoroutine()
	t.Cleanup(func() {
		end := time.Now().Add(time.Second)
		for time.Now().Before(end) {
			if runtime.NumGoroutine() <= before {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		buf := make([]byte, 1<<16)
		n := runtime.Stack(buf, true)
		t.Errorf("leak: %d -> %d\n%s", before,
			runtime.NumGoroutine(), buf[:n])
	})
}

func replicas() []fetchFunc {
	f := func(ctx context.Context) (Transfer, error) {
		select {
		case <-time.After(time.Millisecond):
		case <-ctx.Done():
		}
		return Transfer{"t1"}, nil
	}
	return []fetchFunc{f, f, f}
}

func TestFirstNoLeak(t *testing.T) {
	noLeaks(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if First(ctx, replicas()).ID != "t1" {
		t.Fatal("wrong answer")
	}
}

// The helper catches the leaky version: we run it against a fake
// TB and check that it reported the parked senders.
func TestHelperCatchesLeak(t *testing.T) {
	rec := &recordTB{TB: t}
	func() {
		noLeaks(rec)
		FirstLeaky(t.Context(), replicas())
	}()
	rec.runCleanups()
	if !strings.Contains(rec.msg, "chan send") {
		t.Fatalf("helper missed the leak: %q", rec.msg)
	}
}

type recordTB struct {
	testing.TB
	msg      string
	cleanups []func()
}

func (r *recordTB) Helper()                      {}
func (r *recordTB) Cleanup(f func())             { r.cleanups = append(r.cleanups, f) }
func (r *recordTB) Errorf(f string, args ...any) { r.msg = fmt.Sprintf(f, args...) }
func (r *recordTB) runCleanups() {
	for i := len(r.cleanups) - 1; i >= 0; i-- {
		r.cleanups[i]()
	}
}
