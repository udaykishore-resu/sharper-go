package outbox

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Review check for Technique 63: when the relay ignores
// cancellation, the cleanup's timeout fails the test by name.
// The stuck goroutine is reported, not reclaimed: it lives until
// the test binary exits. The failing test runs in a child
// process so this package's own run stays green.
func TestStuckRelayFailsTest(t *testing.T) {
	if os.Getenv("OUTBOX_STUCK_CHILD") == "1" {
		stuck = true
		startRelay(t)
		return
	}
	cmd := exec.Command(os.Args[0],
		"-test.run=^TestStuckRelayFailsTest$", "-test.v")
	cmd.Env = append(os.Environ(), "OUTBOX_STUCK_CHILD=1")
	start := time.Now()
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("child passed with a stuck relay:\n%s", out)
	}
	if !strings.Contains(string(out), "relay did not stop") {
		t.Fatalf("child failed without naming the leak:\n%s", out)
	}
	if d := time.Since(start); d > 30*time.Second {
		t.Fatalf("child took %v; the 2s timeout should bound it", d)
	}
}

// A healthy relay leaves no goroutine behind once cleanup ran.
func TestRelayGoroutineGone(t *testing.T) {
	before := runtime.NumGoroutine()
	t.Run("inner", func(t *testing.T) { startRelay(t) })
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > before {
		if time.Now().After(deadline) {
			t.Fatalf("goroutines: before %d, after %d",
				before, runtime.NumGoroutine())
		}
		time.Sleep(5 * time.Millisecond)
	}
}
