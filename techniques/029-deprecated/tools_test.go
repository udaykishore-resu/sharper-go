package ledger

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// callerModule copies ledger.go into a throwaway module and adds a
// caller in another package. inline adds a //go:fix inline line.
func callerModule(t *testing.T, inline bool) string {
	t.Helper()
	src, err := os.ReadFile("ledger.go")
	if err != nil {
		t.Fatal(err)
	}
	if inline {
		src = []byte(strings.Replace(string(src),
			"Transfer.Key.\nfunc Post",
			"Transfer.Key.\n//\n//go:fix inline\nfunc Post", 1))
	}
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":           "module example.com/dep\n\ngo 1.26.0\n",
		"ledger/ledger.go": string(src),
		"caller/caller.go": "package caller\n\nimport \"example.com/dep/ledger\"\n\n" +
			"func Pay() (string, error) {\n\treturn ledger.Post(\"a\", \"b\", 5)\n}\n",
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func goCmd(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	if testing.Short() {
		t.Skip("runs the go command")
	}
	gobin, err := exec.LookPath("go") // go test puts GOROOT/bin first
	if err != nil {
		t.Skip("no go command on PATH")
	}
	cmd := exec.Command(gobin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=", "GOWORK=off",
		"GOTOOLCHAIN=local", "GOPROXY=off")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Breaks when: go vet does not report uses of deprecated names.
func TestVetIgnoresDeprecated(t *testing.T) {
	out, err := goCmd(t, callerModule(t, false), "vet", "./...")
	if err != nil || strings.TrimSpace(out) != "" {
		t.Fatalf("go vet reported something: %v\n%s", err, out)
	}
}

// Go 1.26's go fix inlines calls to a //go:fix inline wrapper.
func TestGoFixInlines(t *testing.T) {
	out, _ := goCmd(t, callerModule(t, true), "fix", "-diff", "./...")
	want := "+\treturn ledger.PostTransfer(ledger.Transfer{" +
		"From: \"a\", To: \"b\", Amount: 5})"
	if !strings.Contains(out, want) {
		t.Fatalf("go fix -diff did not inline Post:\n%s", out)
	}
}
