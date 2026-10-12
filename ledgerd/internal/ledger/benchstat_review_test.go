package ledger

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Review check for Technique 71: the benchstat pinned in go.mod's
// tool block prints the columns and significance marks the book
// describes. Inputs are synthetic so the result is deterministic.
func TestBenchstatOutputShape(t *testing.T) {
	if testing.Short() {
		t.Skip("builds benchstat")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}
	dir := t.TempDir()
	write := func(name string, base float64) string {
		var b strings.Builder
		b.WriteString("goos: linux\ngoarch: amd64\npkg: x\n")
		for i := range 10 {
			ns := base * (1 + float64(i%5-2)/200) // ±1% wobble
			fmt.Fprintf(&b, "BenchmarkParse-8 1000000 %.1f ns/op\n", ns)
			fmt.Fprintf(&b, "BenchmarkFormat-8 1000000 %.1f ns/op\n", 50+float64(i%3))
		}
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	before, after := write("old.txt", 91.8), write("new.txt", 64.1)
	out, err := exec.Command("go", "tool", "benchstat", before, after).CombinedOutput()
	if err != nil {
		t.Fatalf("benchstat: %v\n%s", err, out)
	}
	s := string(out)
	t.Logf("\n%s", s)
	for _, want := range []string{"sec/op", "vs base", "± ", "n=10"} {
		if !strings.Contains(s, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	for _, line := range strings.Split(s, "\n") {
		switch {
		case strings.HasPrefix(line, "Parse-8"):
			if !strings.Contains(line, "%") || !strings.Contains(line, "p=0.000") {
				t.Errorf("Parse should be a significant change: %q", line)
			}
		case strings.HasPrefix(line, "Format-8"):
			if !strings.Contains(line, "~") {
				t.Errorf("Format should show ~ (no change): %q", line)
			}
		}
	}
}
