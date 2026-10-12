package ledger

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func goCmd(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(), "GOFLAGS=", "GOWORK=off")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// Habit one: the directive runs the stringer pinned in go.mod.
func TestDirectiveUsesPinnedTool(t *testing.T) {
	out := goCmd(t, "generate", "-n", ".")
	if !strings.Contains(out,
		"go tool stringer -type=Kind -linecomment") {
		t.Fatalf("go generate -n:\n%s", out)
	}
	if out := goCmd(t, "generate", "-n", "-skip=stringer",
		"."); strings.Contains(out, "stringer") {
		t.Fatalf("-skip=stringer still runs it:\n%s", out)
	}
	if !strings.Contains(goCmd(t, "tool"),
		"golang.org/x/tools/cmd/stringer") {
		t.Fatal("stringer is not in go.mod's tool block")
	}
}

// Habit two: the output carries the standard generated marker,
// which go/ast recognises.
func TestOutputIsMarked(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(),
		"kind_string.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	if !ast.IsGenerated(f) {
		t.Fatal("kind_string.go lacks the Code generated line")
	}
}

// Habit three: regenerate and compare, as the CI drift check
// does with git diff --exit-code. The file is restored after.
func TestNoDrift(t *testing.T) {
	const out = "kind_string.go"
	want, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.WriteFile(out, want, 0o644) })
	goCmd(t, "generate", ".")
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s drifted from its generator; "+
			"run go generate and commit", out)
	}
}
