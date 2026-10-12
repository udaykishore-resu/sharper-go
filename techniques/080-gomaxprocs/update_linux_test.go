package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// This box has fewer than 4 CPUs and cgroup v1, so the cgroup
// path itself can't be exercised (the runtime's own cgroup tests
// skip here too). The update loop and its off switches can: they
// treat a CPU-affinity change like a quota change.

const probe = `package main

import (
	"fmt"
	"os"
	"runtime"
	"time"
)

func main() {
	if os.Getenv("PROBE_SET") != "" {
		runtime.GOMAXPROCS(2) // code you can't change
	}
	if os.Getenv("PROBE_RESET") != "" {
		runtime.SetDefaultGOMAXPROCS() // hand it back
	}
	for range 60 {
		fmt.Println(runtime.GOMAXPROCS(0))
		time.Sleep(100 * time.Millisecond)
	}
}
`

// build writes probe into a module with the given go line and
// //go:debug lines and builds it.
func build(t *testing.T, goLine string, debug ...string) string {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go command")
	}
	dir := t.TempDir()
	src := probe
	for _, d := range debug {
		src = "//go:debug " + d + "\n" + src
	}
	write := func(name, s string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module probe\n\ngo "+goLine+"\n")
	write("main.go", src)
	exe := filepath.Join(dir, "probe")
	cmd := exec.Command(goBin, "build", "-o", exe, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=", "GOTOOLCHAIN=local")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return exe
}

// run starts exe on CPUs 0 and 1, pins it to CPU 0 after the first
// reading, and returns the first and last GOMAXPROCS it printed
// within 3s. Starting on exactly two CPUs makes the first reading
// the same on a 2-CPU laptop and a 4-CPU CI runner.
func run(t *testing.T, exe string, env ...string) (first, last string) {
	t.Helper()
	cmd := exec.Command("taskset", "-c", "0,1", exe)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	sc := bufio.NewScanner(out)
	sc.Scan()
	first = sc.Text()
	pid := fmt.Sprint(cmd.Process.Pid)
	if b, err := exec.Command("taskset", "-a", "-p", "-c", "0", pid).CombinedOutput(); err != nil {
		t.Skipf("taskset: %v %s", err, b)
	}
	for end := time.Now().Add(3 * time.Second); time.Now().Before(end) && sc.Scan(); {
		last = sc.Text()
	}
	return first, last
}

func TestUpdateFollowsAffinity(t *testing.T) {
	if testing.Short() || runtime.NumCPU() < 2 {
		t.Skip("needs 2+ CPUs and builds binaries")
	}
	if _, err := exec.LookPath("taskset"); err != nil {
		t.Skip("no taskset")
	}
	want2 := "2" // run starts every probe on CPUs 0 and 1
	modern := build(t, "1.25")
	old := build(t, "1.24")
	optIn := build(t, "1.24", "containermaxprocs=1", "updatemaxprocs=1")
	startOnly := build(t, "1.24", "containermaxprocs=1")
	cases := []struct {
		name string
		exe  string
		env  []string
		last string
	}{
		{"go 1.25 line updates", modern, nil, "1"},
		{"GOMAXPROCS env pins it", modern, []string{"GOMAXPROCS=2"}, "2"},
		{"a GOMAXPROCS call pins it", modern, []string{"PROBE_SET=1"}, "2"},
		{"SetDefaultGOMAXPROCS hands it back", modern, []string{"PROBE_SET=1", "PROBE_RESET=1"}, "1"},
		{"updatemaxprocs=0 stops it", modern, []string{"GODEBUG=updatemaxprocs=0"}, want2},
		{"go 1.24 line keeps the old default", old, nil, want2},
		{"go 1.24 plus both lines opts in", optIn, nil, "1"},
		{"containermaxprocs alone: startup only", startOnly, nil, want2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			first, last := run(t, c.exe, c.env...)
			if first != want2 {
				t.Fatalf("started at %s, want %s", first, want2)
			}
			if last != c.last {
				t.Fatalf("after pinning to one CPU: %s, want %s", last, c.last)
			}
		})
	}
}
