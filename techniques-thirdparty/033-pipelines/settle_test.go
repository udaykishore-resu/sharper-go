package settle

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"testing"
	"time"
)

func batch(n int) []Transfer {
	ts := make([]Transfer, n)
	for i := range ts {
		ts[i] = Transfer{ID: fmt.Sprint("t", i), Amount: 1}
	}
	return ts
}

// The Try it: the last stage fails on its fifth item. Every stage
// exits, and Wait returns the real error, not a cancellation. The
// bounds are generous: they fail a hang, not a slow runner.
func TestWriterFailureStopsAll(t *testing.T) {
	before := runtime.NumGoroutine()
	errConstraint := errors.New("constraint violation")
	n := 0
	err := finishes(t, func() error {
		return run(t.Context(), batch(10_000),
			func(context.Context, Transfer) error {
				if n++; n == 5 {
					return errConstraint
				}
				return nil
			})
	})
	if !errors.Is(err, errConstraint) {
		t.Fatalf("err = %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before {
		t.Fatalf("goroutines %d -> %d", before, n)
	}
}

func TestValidatorFailure(t *testing.T) {
	ts := batch(100)
	ts[50].Amount = 0
	err := run(t.Context(), ts, func(context.Context, Transfer) error { return nil })
	if err == nil || err.Error() != "bad amount: t50" {
		t.Fatalf("err = %v", err)
	}
}
