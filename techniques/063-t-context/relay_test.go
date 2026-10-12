package outbox

import (
	"testing"
	"time"
)

func startRelay(t *testing.T) *Relay {
	t.Helper()
	r := Start(t.Context())
	t.Cleanup(func() {
		select {
		case <-r.Done():
		case <-time.After(2 * time.Second):
			t.Error("relay did not stop")
		}
	})
	return r
}

// No defer, no cancel func, no teardown: t.Context() is canceled
// just before the cleanup runs, and the cleanup confirms the stop.
func TestRelayStopsWithTest(t *testing.T) {
	r := startRelay(t)
	select {
	case <-r.Done():
		t.Fatal("relay stopped while the test was running")
	case <-time.After(30 * time.Millisecond):
	}
}

// The order the design depends on: cancel first, cleanups after.
func TestContextCanceledBeforeCleanup(t *testing.T) {
	ctx := t.Context()
	t.Cleanup(func() {
		if ctx.Err() == nil {
			t.Error("t.Context() still live in Cleanup")
		}
	})
}
