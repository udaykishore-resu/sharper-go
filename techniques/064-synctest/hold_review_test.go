package synctestdemo

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/udaykishore-resu/sharper-go/techniques/064-synctest/hold"
)

// Review check for Technique 64: inside the bubble the clock moves
// exactly as far as the sleeps say, and the real wall time is
// whatever the CPU work costs, not fifteen minutes. We assert a
// loose real-time bound (2s), not a speed: the payoff is that the
// test never waits on the real clock.
func TestHoldVirtualClock(t *testing.T) {
	realStart := time.Now()
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		h := hold.New(15 * time.Minute)
		go h.Sweep(ctx, time.Minute)
		h.Place("tr_1")
		time.Sleep(14 * time.Minute)
		synctest.Wait()
		if !h.Active("tr_1") {
			t.Fatal("expired too early")
		}
		time.Sleep(time.Minute)
		synctest.Wait()
		if h.Active("tr_1") {
			t.Fatal("still active after 15m")
		}
		if got := time.Since(start); got != 15*time.Minute {
			t.Fatalf("virtual elapsed %v, want exactly 15m", got)
		}
	})
	if d := time.Since(realStart); d > 2*time.Second {
		t.Fatalf("real elapsed %v; the bubble should not wait", d)
	}
}

// The "goroutine never exits" breaks-when: forgetting cancel()
// leaves the sweeper blocked, and synctest.Test fails with a panic
// that lists it. Run in a child process so this package stays green.
func TestLeakedSweeperPanics(t *testing.T) {
	if os.Getenv("SYNCTEST_LEAK_CHILD") == "1" {
		synctest.Test(t, func(t *testing.T) {
			ctx := context.WithoutCancel(t.Context())
			h := hold.New(time.Minute)
			go h.Sweep(ctx, time.Minute) // never canceled
		})
		return
	}
	cmd := exec.Command(os.Args[0],
		"-test.run=^TestLeakedSweeperPanics$")
	cmd.Env = append(os.Environ(), "SYNCTEST_LEAK_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("child passed with a leaked goroutine:\n%s", out)
	}
	for _, want := range []string{
		"blocked goroutines remain", "hold.(*Holds).Sweep",
	} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("child output lacks %q:\n%s", want, out)
		}
	}
}
