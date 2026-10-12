// Package flight is Technique 81: an always-on flight recorder,
// snapshotted only when something rare happens.
package flight

import (
	"context"
	"log/slog"
	"os"
	"runtime/trace"
	"sync/atomic"
	"time"
)

func StartRecorder() (*trace.FlightRecorder, error) {
	fr := trace.NewFlightRecorder(
		trace.FlightRecorderConfig{
			MinAge:   10 * time.Second,
			MaxBytes: 32 << 20,
		})
	return fr, fr.Start()
}

var busy atomic.Bool

func snapshot(fr *trace.FlightRecorder, path string) {
	if !busy.CompareAndSwap(false, true) {
		return // a write is already running
	}
	go func() {
		defer busy.Store(false)
		f, err := os.Create(path)
		if err != nil {
			return
		}
		defer f.Close()
		if _, err := fr.WriteTo(f); err != nil {
			slog.Error("snapshot", "err", err)
		}
	}()
}

// waitIdle is for shutdown (Technique 45): let a running snapshot
// finish, but never wait past ctx's deadline.
func waitIdle(ctx context.Context) error {
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for busy.Load() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
	return nil
}
