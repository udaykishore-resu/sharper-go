package batch

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// module writes a one-file main module with the given go line.
func module(t *testing.T, goLine, src string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":  "module m\n\ngo " + goLine + "\n",
		"main.go": src,
	}
	for name, data := range files {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func goCmd(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go command not on PATH")
	}
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=", "GOPROXY=off",
		"GOWORK=off", "GOTOOLCHAIN=local")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

const closures = `package main

import "fmt"

func main() {
	var fs []func()
	for i := 0; i < 3; i++ {
		fs = append(fs, func() { fmt.Print(i, " ") })
	}
	for _, f := range fs {
		f()
	}
}
`

// The go line, not the toolchain, decides the semantics: the same
// file built by the same compiler prints 3 3 3 or 0 1 2.
func TestGoLineIsTheGate(t *testing.T) {
	for goLine, want := range map[string]string{
		"1.21": "3 3 3 ", "1.22": "0 1 2 ",
	} {
		out, err := goCmd(t, module(t, goLine, closures), "run", ".")
		if err != nil || out != want {
			t.Errorf("go %s: got %q (%v), want %q", goLine, out, err, want)
		}
	}
}

const copies = `package main

import "fmt"

func main() {
	done := make(chan bool)
	for _, x := range []int{1, 2} {
		x := x
		go func() { fmt.Println(x); done <- true }()
	}
	<-done
	<-done
}
`

// go fix -forvar removes the copy only where the go line makes it
// redundant; in a 1.21 module it leaves the copy, and vet flags
// the capture there instead.
func TestForvarRespectsGoLine(t *testing.T) {
	out, _ := goCmd(t, module(t, "1.22", copies),
		"fix", "-forvar", "-diff", ".")
	if !strings.Contains(out, "-\t\tx := x") {
		t.Fatalf("go 1.22: copy not removed:\n%s", out)
	}
	old := module(t, "1.21", copies)
	if out, _ := goCmd(t, old, "fix", "-forvar", "-diff", "."); out != "" {
		t.Fatalf("go 1.21: fix changed code:\n%s", out)
	}
	noCopy := strings.Replace(copies, "\t\tx := x\n", "", 1)
	out, err := goCmd(t, module(t, "1.21", noCopy), "vet", ".")
	if err == nil ||
		!strings.Contains(out, "loop variable x captured by func literal") {
		t.Fatalf("vet on go 1.21: %v\n%s", err, out)
	}
}
