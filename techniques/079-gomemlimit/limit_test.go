package memlimit

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"
)

func writeMax(t *testing.T, v string) string {
	p := filepath.Join(t.TempDir(), "memory.max")
	if err := os.WriteFile(p, []byte(v), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFromCgroup(t *testing.T) {
	if got, ok := FromCgroup(writeMax(t, "536870912\n"), 0.9); !ok || got != 483183820 {
		t.Fatalf("512 MiB limit: %d %v", got, ok)
	}
	for _, v := range []string{"max\n", "", "junk"} {
		if _, ok := FromCgroup(writeMax(t, v), 0.9); ok {
			t.Errorf("%q gave a limit", v)
		}
	}
	if _, ok := FromCgroup("/nonexistent/memory.max", 0.9); ok {
		t.Error("missing file gave a limit")
	}
}

func TestApplyUsesCgroup(t *testing.T) {
	if _, set := os.LookupEnv("GOMEMLIMIT"); set {
		t.Skip("GOMEMLIMIT set in the environment")
	}
	defer debug.SetMemoryLimit(debug.SetMemoryLimit(-1))
	if got := Apply(writeMax(t, "536870912")); got != 483183820 {
		t.Fatalf("limit %d", got)
	}
}

// The env var wins over memory.max, and "460MiB" means 460<<20
// bytes (powers of two, like Kubernetes's Mi). Runs in a child
// process so the runtime reads GOMEMLIMIT at startup.
func TestEnvWins(t *testing.T) {
	if os.Getenv("MEMLIMIT_CHILD") == "1" {
		lim := Apply(os.Getenv("MEMLIMIT_FILE"))
		os.Stdout.WriteString("LIMIT=" + strconv.FormatInt(lim, 10) + "\n")
		return
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestEnvWins$")
	cmd.Env = append(os.Environ(), "MEMLIMIT_CHILD=1",
		"GOMEMLIMIT=460MiB",
		"MEMLIMIT_FILE="+writeMax(t, "536870912"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	want := "LIMIT=" + strconv.Itoa(460<<20) + "\n"
	if !strings.Contains(string(out), want) {
		t.Fatalf("child: %s, want %s", out, want)
	}
}
