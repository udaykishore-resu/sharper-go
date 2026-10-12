package raceci

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// With GORACE=halt_on_error=1 the first race stops the program
// with exit code 66, and the report shows both stacks.
func TestHaltOnErrorExits66(t *testing.T) {
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go command on PATH")
	}
	cmd := exec.Command(gobin, "run", "-race", "testdata/racy.go")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1",
		"GORACE=halt_on_error=1")
	out, err := cmd.CombinedOutput()
	if strings.Contains(string(out), "requires cgo") ||
		strings.Contains(string(out), "C compiler") {
		t.Skipf("no cgo toolchain here: %s", out)
	}
	if !strings.Contains(string(out), "WARNING: DATA RACE") {
		t.Fatalf("no race report:\n%s", out)
	}
	var ee *exec.ExitError
	// go run reports the child's failure as exit status 1 and
	// prints the child's own status.
	if !errors.As(err, &ee) || !strings.Contains(string(out), "exit status 66") {
		t.Fatalf("want exit status 66, got %v:\n%s", err, out)
	}
}
