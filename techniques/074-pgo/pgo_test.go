// Package pgo checks Technique 74's commands against a stand-in
// ledgerd in testdata: profiles pulled over HTTP from two
// processes, merged with go tool pprof -proto, picked up as
// cmd/ledgerd/default.pgo, and visible in go version -m.
package pgo

import (
	"bufio"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out := filepath.Join(dst, rel)
		os.MkdirAll(filepath.Dir(out), 0o755)
		return os.WriteFile(out, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func sh(t *testing.T, dir, script string) string {
	t.Helper()
	cmd := exec.Command("sh", "-c", script)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=", "GOTOOLCHAIN=local")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", script, err, out)
	}
	return string(out)
}

// profile starts the binary, pulls a CPU profile from its pprof
// endpoint, and stops it: one "pod".
func profile(t *testing.T, exe, out string) {
	t.Helper()
	cmd := exec.Command(exe)
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	addr, _ := bufio.NewReader(stdout).ReadString('\n')
	url := "http://" + strings.TrimSpace(addr) +
		"/debug/pprof/profile?seconds=1"
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		t.Fatal(err)
	}
}

func TestPGOLoop(t *testing.T) {
	// A PGO build recompiles every dependency with the profile:
	// minutes on a small machine, so this runs only on request:
	//	PGO_E2E=1 go test -run TestPGOLoop -timeout 20m ./074-pgo
	if os.Getenv("PGO_E2E") == "" {
		t.Skip("set PGO_E2E=1 to build, profile and rebuild")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go command")
	}
	dir := t.TempDir()
	copyTree(t, "testdata", dir)
	os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/ledgerd\n\ngo 1.26.0\n"), 0o644)

	sh(t, dir, "go build -o ledgerd ./cmd/ledgerd")
	if strings.Contains(sh(t, dir, "go version -m ./ledgerd"), "-pgo=") {
		t.Fatal("-pgo line before any default.pgo")
	}
	for _, p := range []string{"ledgerd-a", "ledgerd-b"} {
		profile(t, filepath.Join(dir, "ledgerd"), filepath.Join(dir, p+".pprof"))
	}
	// The book's merge line, verbatim.
	sh(t, dir, "go tool pprof -proto ledgerd-*.pprof \\\n  > cmd/ledgerd/default.pgo")

	// No flag: -pgo=auto finds default.pgo in the main package.
	sh(t, dir, "go build -o ledgerd ./cmd/ledgerd")
	want := "-pgo=" + filepath.Join(dir, "cmd/ledgerd/default.pgo")
	if v := sh(t, dir, "go version -m ./ledgerd"); !strings.Contains(v, want) {
		t.Fatalf("go version -m lacks %s:\n%s", want, v)
	}

	// The go test trap: -pgo=auto applies to main packages only,
	// so a test of ./internal/... gets no profile, not even from a
	// default.pgo in the tested package's own directory. Test
	// binaries carry no -pgo build setting, so read the plan
	// (go test -n) for the compiler's -pgoprofile flag instead.
	sh(t, dir, "cp cmd/ledgerd/default.pgo internal/ledger/")
	if strings.Contains(sh(t, dir, "go test -n ./internal/... 2>&1"),
		"-pgoprofile=") {
		t.Fatal("go test -pgo=auto used a profile")
	}
	sh(t, dir, "rm internal/ledger/default.pgo")
	plan := sh(t, dir, "P=$PWD/cmd/ledgerd/default.pgo\n"+
		"go test -n -pgo=$P ./internal/... 2>&1")
	if !strings.Contains(plan, "-pgoprofile=") {
		t.Fatal("explicit -pgo did not reach the compiler")
	}
	// And the benchmark commands themselves run.
	sh(t, dir, "P=$PWD/cmd/ledgerd/default.pgo\n"+
		"go test -bench=. -benchtime=100x -count=1 -pgo=off ./internal/... > off.txt\n"+
		"go test -bench=. -benchtime=100x -count=1 -pgo=$P ./internal/... > pgo.txt")
}
