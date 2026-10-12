package registry

import (
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/udaykishore-resu/sharper-go/techniques/099-plugin-registry/sink"
)

// Review checks for Technique 99: a failing factory, concurrent
// lookups after init, and a build tag that drops a backend.

// A factory that rejects its config fails New with that error;
// nothing is half-built. (The registry is global, so this test
// registers nothing new.)
func TestFactoryErrorReachesCaller(t *testing.T) {
	s, err := sink.New("stdout", map[string]string{"ulr": "x"})
	if s != nil || err == nil || !strings.Contains(err.Error(), `unknown setting "ulr"`) {
		t.Fatalf("sink %v, err %v", s, err)
	}
}

// After init the map is only read, so concurrent New calls are
// safe (run with -race).
func TestConcurrentNew(t *testing.T) {
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			if _, err := sink.New("stdout", nil); err != nil {
				t.Error(err)
			}
			sink.New("kafka", nil)
		})
	}
	wg.Wait()
}

func TestBuildTagDropsBackend(t *testing.T) {
	deps := func(tags string) string {
		out, err := exec.Command("go", "list", "-tags", tags, "-deps",
			"./cmd/sinkdemo").CombinedOutput()
		if err != nil {
			t.Fatalf("go list: %v\n%s", err, out)
		}
		return string(out)
	}
	const stdout = "099-plugin-registry/sink/stdout\n"
	if !strings.Contains(deps(""), stdout) {
		t.Fatal("default build lacks the stdout sink")
	}
	if strings.Contains(deps("nostdout"), stdout) {
		t.Fatal("-tags nostdout still links the stdout sink")
	}
	// Without the backend, asking for it fails at startup with the
	// list of what was built in.
	cmd := exec.Command("go", "run", "-tags", "nostdout", "./cmd/sinkdemo")
	cmd.Env = append(os.Environ(), "SINK=stdout")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), `sink "stdout" not built in (have [])`) {
		t.Fatalf("want a not-built-in error, got %v\n%s", err, out)
	}
}
