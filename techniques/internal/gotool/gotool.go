// Package gotool lets technique tests run the go command against
// a throwaway module, so claims about go lines, compiler errors,
// vet messages and go fix rewrites are checked, not assumed.
package gotool

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// Module writes go.mod (with the given go line) and files into a
// fresh temp directory and returns it.
func Module(t testing.TB, goLine string,
	files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	mod := "module m\n\ngo " + goLine + "\n"
	write(t, filepath.Join(dir, "go.mod"), mod)
	for name, src := range files {
		write(t, filepath.Join(dir, name), src)
	}
	return dir
}

// Go runs the go command in dir, offline and with the local
// toolchain, and returns its combined output and error. It skips
// the test when no go command is on PATH.
func Go(t testing.TB, dir string, args ...string) (string, error) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go command not on PATH")
	}
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GOFLAGS=", "GOPROXY=off", "GOWORK=off",
		"GOTOOLCHAIN=local")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Read returns the content of a file in dir.
func Read(t testing.TB, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func write(t testing.TB, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// NeedMinor skips t when the running toolchain is older than
// Go 1.minor. Tests that probe a newer release's features use it,
// so the module's own tests still run on the go line's minimum.
func NeedMinor(t testing.TB, minor int) {
	t.Helper()
	if have, ok := RunningMinor(); ok && have < minor {
		t.Skipf("needs Go 1.%d, running %s", minor, runtime.Version())
	}
}

// RunningMinor reports the minor version of the running
// toolchain, such as 27 for go1.27.2.
func RunningMinor() (int, bool) {
	v := strings.TrimPrefix(runtime.Version(), "go1.")
	if i := strings.IndexAny(v, ".rc beta"); i >= 0 {
		v = v[:i]
	}
	n, err := strconv.Atoi(v)
	return n, err == nil
}
