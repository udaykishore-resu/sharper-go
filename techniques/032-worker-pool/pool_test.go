package pool

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func TestPoolReportsPerJob(t *testing.T) {
	ts := make([]Transfer, 100)
	for i := range ts {
		ts[i] = Transfer{ID: fmt.Sprint(i), Amount: int64(i % 7)}
	}
	apply := func(_ context.Context, t Transfer) error {
		if t.Amount == 0 {
			return errors.New("bad row")
		}
		return nil
	}
	var bad int
	for r := range pool(t.Context(), 4, produce(t.Context(), ts), apply) {
		if r.Err != nil {
			bad++
		}
	}
	if bad != 15 {
		t.Fatalf("bad rows = %d, want 15", bad)
	}
}

func TestBoundedConcurrency(t *testing.T) {
	var now, peak atomic.Int32
	apply := func(context.Context, Transfer) error {
		n := now.Add(1)
		for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
		}
		time.Sleep(time.Millisecond)
		now.Add(-1)
		return nil
	}
	for range pool(t.Context(), 4, produce(t.Context(), make([]Transfer, 200)), apply) {
	}
	if peak.Load() > 4 {
		t.Fatalf("peak concurrency %d > 4", peak.Load())
	}
}

// Cancel halfway: every goroutine exits, including the producer.
func TestCancelStopsEverything(t *testing.T) {
	before := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(t.Context())
	apply := func(ctx context.Context, _ Transfer) error {
		select {
		case <-time.After(10 * time.Millisecond):
		case <-ctx.Done():
		}
		return nil
	}
	out := pool(ctx, 16, produce(ctx, make([]Transfer, 1000)), apply)
	for range 100 {
		<-out
	}
	cancel()
	for range out { // drains until the closer runs
	}
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before {
		t.Fatalf("goroutines %d -> %d", before, n)
	}
}
