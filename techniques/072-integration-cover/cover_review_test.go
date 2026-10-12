package intcover

import (
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Review checks for Technique 72. They copy this directory into a
// scratch module with the same import path, so cover.sh's bin/ and
// cov/ never land in the source tree.
func scratch(t *testing.T) string {
	t.Helper()
	if os.Getenv("INTCOVER_CHILD") == "1" {
		t.Skip("inside cover.sh's own go test run")
	}
	if testing.Short() {
		t.Skip("builds and runs binaries")
	}
	for _, tool := range []string{"go", "bash", "curl"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not installed", tool)
		}
	}
	root := t.TempDir()
	mod := "module github.com/udaykishore-resu/sharper-go/techniques\n\ngo 1.26.0\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "072-integration-cover")
	if err := os.CopyFS(dir, os.DirFS(".")); err != nil {
		t.Fatal(err)
	}
	return dir
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

func env(extra ...string) []string {
	return append(os.Environ(), append([]string{"INTCOVER_CHILD=1"}, extra...)...)
}

// The whole printed sequence: instrumented build, real-binary smoke
// run, SIGTERM, unit run in the raw format, merge, percent. The
// merged profile covers both the handler path (smoke only) and
// Validate's error path (unit only).
func TestCoverScript(t *testing.T) {
	dir := scratch(t)
	cmd := exec.Command("bash", "cover.sh")
	cmd.Dir = dir
	cmd.Env = env("SVC_ADDR=" + freeAddr(t))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("cover.sh: %v\n%s", err, out)
	}
	percent := false // covdata percent: "<pkg>\t\tcoverage: N%"
	for _, l := range strings.Split(string(out), "\n") {
		f := strings.Fields(l)
		if len(f) >= 2 && strings.HasSuffix(f[0], "/072-integration-cover") &&
			f[1] == "coverage:" {
			percent = true
		}
	}
	if !percent {
		t.Fatalf("no percent line for the package:\n%s", out)
	}
	funcs := func(in string) string {
		c := exec.Command("go", "tool", "covdata", "func", "-i="+in)
		c.Dir = dir
		b, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("covdata func: %v\n%s", err, b)
		}
		return string(b)
	}
	covered := func(report, fn string) bool {
		for _, l := range strings.Split(report, "\n") {
			f := strings.Fields(l)
			if len(f) == 3 && f[1] == fn {
				return f[2] != "0.0%"
			}
		}
		t.Fatalf("%s missing from report:\n%s", fn, report)
		return false
	}
	intOnly, all := funcs("cov/int"), funcs("cov/all")
	if !covered(intOnly, "getAccount") || covered(intOnly, "Validate") {
		t.Errorf("smoke run should cover getAccount, not Validate:\n%s", intOnly)
	}
	if !covered(all, "getAccount") || !covered(all, "Validate") {
		t.Errorf("merged run should cover both:\n%s", all)
	}
}

// runBinary starts the -cover binary, waits until it serves, sends
// sig and returns its exit error and stderr.
func runBinary(t *testing.T, bin string, sig syscall.Signal,
	extra ...string) (error, string) {
	t.Helper()
	addr := freeAddr(t)
	cmd := exec.Command(bin)
	cmd.Env = append(append([]string{}, extra...), "SVC_ADDR="+addr,
		"PATH="+os.Getenv("PATH"))
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, err := http.Get("http://" + addr + "/healthz")
		if err == nil {
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			cmd.Process.Kill()
			t.Fatalf("binary never served: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	cmd.Process.Signal(sig)
	err := cmd.Wait()
	return err, stderr.String()
}

func counters(t *testing.T, dir string) int {
	t.Helper()
	m, _ := filepath.Glob(filepath.Join(dir, "covcounters.*"))
	return len(m)
}

// The breaks-when cases: counters appear only when main returns.
func TestCoverFlushNeedsCleanExit(t *testing.T) {
	dir := scratch(t)
	bin := filepath.Join(dir, "bin", "svc")
	build := exec.Command("go", "build", "-cover",
		"-coverpkg=./...", "-o", bin, "./cmd/svc")
	build.Dir = dir
	build.Env = env()
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	t.Run("SIGTERM handled: counters written", func(t *testing.T) {
		cov := t.TempDir()
		if err, _ := runBinary(t, bin, syscall.SIGTERM, "GOCOVERDIR="+cov); err != nil {
			t.Fatalf("exit: %v", err)
		}
		if counters(t, cov) == 0 {
			t.Fatal("no counter file after a clean return from main")
		}
	})
	t.Run("SIGTERM not handled: nothing written", func(t *testing.T) {
		cov := t.TempDir()
		err, _ := runBinary(t, bin, syscall.SIGTERM,
			"GOCOVERDIR="+cov, "SVC_NO_SIGNALS=1")
		if err == nil {
			t.Fatal("process exited cleanly without a handler")
		}
		if n := counters(t, cov); n != 0 {
			t.Fatalf("%d counter files after a signal death", n)
		}
	})
	t.Run("SIGKILL: nothing written", func(t *testing.T) {
		cov := t.TempDir()
		runBinary(t, bin, syscall.SIGKILL, "GOCOVERDIR="+cov)
		if n := counters(t, cov); n != 0 {
			t.Fatalf("%d counter files after SIGKILL", n)
		}
	})
	t.Run("GOCOVERDIR unset: warning only", func(t *testing.T) {
		err, stderr := runBinary(t, bin, syscall.SIGTERM)
		if err != nil {
			t.Fatalf("exit: %v", err)
		}
		if !strings.Contains(stderr, "GOCOVERDIR not set") {
			t.Fatalf("stderr = %q", stderr)
		}
	})
}
