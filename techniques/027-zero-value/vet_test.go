package obs

import (
	"os/exec"
	"strings"
	"testing"
)

// Breaks when: a Counters passed by value copies its mutex. go vet's
// copylocks check must reject testdata/copied.
func TestVetRejectsCopy(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go vet")
	}
	// go test puts GOROOT/bin first on PATH.
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go command on PATH")
	}
	out, err := exec.Command(gobin, "vet", "./testdata/copied").CombinedOutput()
	if err == nil {
		t.Fatal("go vet accepted a Counters passed by value")
	}
	if !strings.Contains(string(out), "passes lock by value") {
		t.Fatalf("unexpected vet output:\n%s", out)
	}
}
