package flight

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime/trace"
	"strings"
	"testing"
	"time"
)

func TestSnapshot(t *testing.T) {
	fr, err := StartRecorder()
	if err != nil {
		t.Fatal(err)
	}
	defer fr.Stop()
	// Only one flight recorder may be active per process.
	if _, err := StartRecorder(); err == nil {
		t.Fatal("a second recorder started")
	}
	for range 1000 { // some activity to record
		go func() {}()
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "slow.trace")
	snapshot(fr, path)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := waitIdle(ctx); err != nil {
		t.Fatal("snapshot never finished:", err)
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Size() == 0 {
		t.Fatalf("snapshot: %v size=%v", err, fi)
	}
}

// While one snapshot runs, another trigger does nothing: no file,
// no second WriteTo.
func TestSnapshotsDoNotOverlap(t *testing.T) {
	fr, err := StartRecorder()
	if err != nil {
		t.Fatal(err)
	}
	defer fr.Stop()
	busy.Store(true) // a snapshot is in flight
	path := filepath.Join(t.TempDir(), "second.trace")
	snapshot(fr, path)
	time.Sleep(50 * time.Millisecond)
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("overlapping snapshot wrote a file: %v", err)
	}
	// Shutdown waits with a deadline, so a stuck write can't hang it.
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if err := waitIdle(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waitIdle = %v, want deadline exceeded", err)
	}
	busy.Store(false)
}

type blockingWriter struct{ entered, release chan struct{} }

func (w blockingWriter) Write(p []byte) (int, error) {
	select {
	case w.entered <- struct{}{}:
	default:
	}
	<-w.release
	return len(p), nil
}

// Without the guard, a second WriteTo during the first fails.
func TestConcurrentWriteToFails(t *testing.T) {
	fr := trace.NewFlightRecorder(trace.FlightRecorderConfig{})
	if err := fr.Start(); err != nil {
		t.Fatal(err)
	}
	defer fr.Stop()
	w := blockingWriter{make(chan struct{}, 1), make(chan struct{})}
	done := make(chan error, 1)
	go func() { _, err := fr.WriteTo(w); done <- err }()
	<-w.entered
	_, err := fr.WriteTo(io.Discard)
	close(w.release)
	if err == nil || !strings.Contains(err.Error(), "already in progress") {
		t.Fatalf("second WriteTo: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
