package replay

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/udaykishore-resu/sharper-go/techniques/internal/gotool"
)

// Since Go 1.26, WaitGroup.Go skips Done when f panics, so Wait
// never returns and main cannot exit 0 mid-crash. The child
// process below panics inside wg.Go; it must die with the panic
// and never print "wait returned".
func TestPanicSkipsDone(t *testing.T) {
	if os.Getenv("WG_PANIC_CHILD") == "1" {
		var wg sync.WaitGroup
		wg.Go(func() { panic("boom") })
		wg.Wait()
		println("wait returned")
		os.Exit(0)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestPanicSkipsDone$")
	cmd.Env = append(os.Environ(), "WG_PANIC_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("child exited 0:\n%s", out)
	}
	if strings.Contains(string(out), "wait returned") {
		t.Fatalf("Wait returned after a panic:\n%s", out)
	}
	if !strings.Contains(string(out), "panic: boom") {
		t.Fatalf("no panic in output:\n%s", out)
	}
}

const buggy = `package x

import "sync"

func Run(fs []func()) {
	var wg sync.WaitGroup
	for _, f := range fs {
		go func() {
			wg.Add(1)
			defer wg.Done()
			f()
		}()
	}
	wg.Wait()
}

func Plain(fs []func()) {
	var wg sync.WaitGroup
	for _, f := range fs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f()
		}()
	}
	wg.Wait()
}

func New() {
	var wg sync.WaitGroup
	wg.Go(func() {})
	wg.Wait()
}
`

// goTool runs the go command in a throwaway module whose go line
// is goLine. Skips when no go command is on PATH.
func goTool(t *testing.T, goLine string, args ...string) string {
	t.Helper()
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go command on PATH")
	}
	dir := t.TempDir()
	mod := "module x\n\ngo " + goLine + "\n"
	for name, body := range map[string]string{
		"go.mod": mod, "x.go": buggy} {
		if err := os.WriteFile(filepath.Join(dir, name),
			[]byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(gobin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local",
		"GOFLAGS=-mod=mod", "GOPROXY=off")
	out, _ := cmd.CombinedOutput()
	return string(out)
}

// go vet's waitgroup analyzer (Go 1.25) reports Add inside the
// goroutine; with an old go line, stdversion reports wg.Go even
// though it compiles.
func TestVetReports(t *testing.T) {
	out := goTool(t, "1.24", "vet", "./...")
	for _, want := range []string{
		"WaitGroup.Add called from inside new goroutine",
		"requires go1.25 or later (module is go1.24)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("vet output lacks %q:\n%s", want, out)
		}
	}
	if out := goTool(t, "1.24", "build", "./..."); out != "" {
		t.Errorf("wg.Go should compile with go 1.24: %s", out)
	}
}

// go fix -waitgroupgo (Go 1.27's name) rewrites only the plain
// Add(1), go, defer Done() shape.
func TestFixRewritesPlainShape(t *testing.T) {
	gotool.NeedMinor(t, 27)
	out := goTool(t, "1.26", "fix", "-diff", "-waitgroupgo", "./...")
	if !strings.Contains(out, "+\t\twg.Go(func() {") {
		t.Fatalf("no wg.Go rewrite:\n%s", out)
	}
	if strings.Count(out, "+\t\twg.Go(") != 1 {
		t.Fatalf("want exactly one rewrite:\n%s", out)
	}
}
