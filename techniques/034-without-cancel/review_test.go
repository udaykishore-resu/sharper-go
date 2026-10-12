package audit

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"testing/synctest"
)

// stop before the cancel unregisters the callback: it never runs.
func TestStopBeforeCancelPreventsCallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		var ran atomic.Bool
		stop := context.AfterFunc(ctx, func() { ran.Store(true) })
		if !stop() {
			t.Fatal("stop before cancel should report true")
		}
		cancel()
		synctest.Wait()
		if ran.Load() {
			t.Fatal("callback ran after stop")
		}
	})
}

// stop does not wait: once the callback has started, stop returns
// false at once and the caller must synchronize itself.
func TestStopAfterCallbackStartedDoesNotWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		release := make(chan struct{})
		var done atomic.Bool
		stop := context.AfterFunc(ctx, func() {
			<-release
			done.Store(true)
		})
		cancel()
		synctest.Wait() // callback is parked on release
		if stop() {
			t.Fatal("stop after the callback started reported true")
		}
		if done.Load() {
			t.Fatal("callback finished before release")
		}
		close(release)
		synctest.Wait()
		if !done.Load() {
			t.Fatal("callback never finished")
		}
	})
}

// For the standard library's contexts, a registered AfterFunc
// costs no goroutine until the context is canceled.
func TestAfterFuncStartsNoGoroutine(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	before := runtime.NumGoroutine()
	stops := make([]func() bool, 1000)
	for i := range stops {
		stops[i] = context.AfterFunc(ctx, func() {})
	}
	if n := runtime.NumGoroutine(); n > before+5 {
		t.Fatalf("goroutines %d -> %d for 1000 AfterFuncs", before, n)
	}
	for _, stop := range stops {
		stop()
	}
}
