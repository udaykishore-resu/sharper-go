package settle

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

// finishes runs f and fails the test if it has not returned
// within a generous deadline, instead of timing each stage.
func finishes(t *testing.T, f func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- f() }()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("pipeline did not stop within 10s")
		return nil
	}
}

// The consumer has enough after five items. errStop cancels the
// upstream stages, run returns nil, and the bubble proves every
// stage exited.
func TestEarlyStopIsNotAnError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		n := 0
		err := run(t.Context(), batch(10_000),
			func(context.Context, Transfer) error {
				if n++; n == 5 {
					return errStop
				}
				return nil
			})
		if err != nil {
			t.Fatalf("err = %v, want nil after an early stop", err)
		}
	})
}

// A real failure that comes first is not hidden by a later stop:
// the validator fails on t2 while the writer holds t1, and the
// writer then stops with errStop. run returns the validator's
// error.
func TestRealErrorBeatsLaterStop(t *testing.T) {
	ts := batch(100)
	ts[2].Amount = 0
	err := finishes(t, func() error {
		return run(t.Context(), ts,
			func(ctx context.Context, tr Transfer) error {
				if tr.ID == "t0" {
					return nil
				}
				<-ctx.Done() // hold t1 until the validator fails
				return errStop
			})
	})
	if err == nil || err.Error() != "bad amount: t2" {
		t.Fatalf("err = %v, want the validator's error", err)
	}
}

// Wrapping keeps the sentinel recognisable; a different error with
// the same text is not filtered.
func TestOnlyTheSentinelIsDropped(t *testing.T) {
	look := errors.New(errStop.Error())
	err := finishes(t, func() error {
		return run(t.Context(), batch(10),
			func(context.Context, Transfer) error { return look })
	})
	if !errors.Is(err, look) {
		t.Fatalf("err = %v, want the look-alike error", err)
	}
}
