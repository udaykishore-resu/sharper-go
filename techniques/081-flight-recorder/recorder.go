// Package flight is Technique 81: an always-on flight recorder,
// snapshotted only when something rare happens.
package flight

import (
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
