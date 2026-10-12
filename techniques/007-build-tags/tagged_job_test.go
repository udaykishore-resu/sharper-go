package store

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func goCmd(t *testing.T, dir string, env []string,
	args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=", "GOPROXY=off",
		"GOWORK=off", "GOTOOLCHAIN=local",
		"LEDGERD_TEST_DATABASE_URL=")
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// The tagged job, run the way CI runs it. The tag alone proves
// nothing: without a database the job must fail, not skip.
func TestTaggedJob(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test in a subprocess")
	}
	run := []string{"test", "-count=1", "-tags=integration",
		"-run", "^TestPingIntegration$", "-v", "."}

	out, err := goCmd(t, ".", nil, run...)
	if err == nil || !strings.Contains(out, "--- FAIL") {
		t.Fatalf("no database: err=%v, want a failure:\n%s",
			err, out)
	}
	if strings.Contains(out, "SKIP") {
		t.Fatalf("no database: the test skipped:\n%s", out)
	}

	out, err = goCmd(t, ".", []string{
		"LEDGERD_TEST_DATABASE_URL=postgres://ci/ledgerd"}, run...)
	if err != nil || !strings.Contains(out,
		"--- PASS: TestPingIntegration") {
		t.Fatalf("with database: err=%v\n%s", err, out)
	}

	// Without the tag the test isn't in the binary at all.
	out, err = goCmd(t, ".", nil, "test", "-count=1", "-run",
		"^TestPingIntegration$", "-v", ".")
	if err != nil || strings.Contains(out, "TestPingIntegration") {
		t.Fatalf("untagged run saw the test: err=%v\n%s", err, out)
	}
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Plain vet skips tagged files; vet with the tag reads them. A
// misspelled tag is valid syntax that never matches.
func TestVetNeedsTheTag(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"),
		"module example.com/v\n\ngo 1.26.0\n")
	write(t, filepath.Join(dir, "v.go"), "package v\n")
	write(t, filepath.Join(dir, "v_integration_test.go"),
		"//go:build integration\n\npackage v\n\n"+
			"import (\n\t\"fmt\"\n\t\"testing\"\n)\n\n"+
			"func TestX(t *testing.T) { fmt.Printf(\"%d\", \"s\") }\n")
	if out, err := goCmd(t, dir, nil, "vet", "./..."); err != nil {
		t.Fatalf("plain vet failed; it should not see the file:\n%s",
			out)
	}
	if _, err := goCmd(t, dir, nil, "vet", "-tags=integration",
		"./..."); err == nil {
		t.Fatal("vet -tags=integration missed the Printf bug")
	}
	if _, err := goCmd(t, dir, nil, "vet", "-tags=integraton",
		"./..."); err != nil {
		t.Fatal("a misspelled tag should match nothing, quietly")
	}
}

// store_test_linux.go doesn't end in _test.go: it is an ordinary
// Linux-only source file in the package, not a test.
func TestFileNameTrap(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"),
		"module example.com/s\n\ngo 1.26.0\n")
	write(t, filepath.Join(dir, "store.go"), "package store\n")
	write(t, filepath.Join(dir, "store_test_linux.go"),
		"package store\n")
	write(t, filepath.Join(dir, "client_js.go"), "package store\n")
	out, err := goCmd(t, dir, []string{"GOOS=linux"}, "list", "-f",
		"{{.GoFiles}} {{.TestGoFiles}} {{.IgnoredGoFiles}}", ".")
	if err != nil {
		t.Fatal(err, out)
	}
	want := "[store.go store_test_linux.go] [] [client_js.go]"
	if strings.TrimSpace(out) != want {
		t.Fatalf("go list = %q, want %q", out, want)
	}
}
