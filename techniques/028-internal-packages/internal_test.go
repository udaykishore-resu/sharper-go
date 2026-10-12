package internalpkg

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// write lays out files under dir; keys are slash paths.
func write(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// build runs go build ./... in dir and returns its output.
func build(t *testing.T, dir string, env ...string) (string, error) {
	t.Helper()
	gobin, err := exec.LookPath("go") // go test puts GOROOT/bin first
	if err != nil {
		t.Skip("no go command on PATH")
	}
	cmd := exec.Command(gobin, "build", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=", "GOPROXY=off",
		"GOTOOLCHAIN=local", "GOWORK=off")
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

const notAllowed = "use of internal package " +
	"example.com/ledgerd/internal/ledger not allowed"

// ledgerd is the printed tree, plus a nested internal/ directory.
func ledgerd(t *testing.T, root string) {
	write(t, filepath.Join(root, "ledgerd"), map[string]string{
		"go.mod":                                 "module example.com/ledgerd\n\ngo 1.26.0\n",
		"cmd/ledgerd/main.go":                    "package main\n\nimport _ \"example.com/ledgerd/internal/store\"\n\nfunc main() {}\n",
		"internal/ledger/ledger.go":              "package ledger\n\nfunc Apply() {}\n",
		"internal/store/store.go":                "package store\n\nimport _ \"example.com/ledgerd/internal/store/internal/pgsql\"\n",
		"internal/store/internal/pgsql/pgsql.go": "package pgsql\n",
		"internal/httpapi/httpapi.go":            "package httpapi\n\nimport _ \"example.com/ledgerd/internal/ledger\"\n",
	})
}

func TestOwnTreeBuilds(t *testing.T) {
	root := t.TempDir()
	ledgerd(t, root)
	if out, err := build(t, filepath.Join(root, "ledgerd")); err != nil {
		t.Fatalf("own tree: %v\n%s", err, out)
	}
}

func TestNestedInternalHidesFromSiblings(t *testing.T) {
	root := t.TempDir()
	ledgerd(t, root)
	write(t, filepath.Join(root, "ledgerd"), map[string]string{
		"internal/httpapi/pg.go": "package httpapi\n\nimport _ \"example.com/ledgerd/internal/store/internal/pgsql\"\n",
	})
	out, err := build(t, filepath.Join(root, "ledgerd"))
	if err == nil || !strings.Contains(out, "use of internal package "+
		"example.com/ledgerd/internal/store/internal/pgsql not allowed") {
		t.Fatalf("httpapi reached store's internal: %v\n%s", err, out)
	}
}

// A nested module under example.com/ledgerd (a tools module) may
// import internal packages: the check compares import paths.
func TestNestedModuleUnderPathMayImport(t *testing.T) {
	root := t.TempDir()
	ledgerd(t, root)
	write(t, filepath.Join(root, "ledgerd", "tools"), map[string]string{
		"go.mod": "module example.com/ledgerd/tools\n\ngo 1.26.0\n\n" +
			"require example.com/ledgerd v0.0.0\n\nreplace example.com/ledgerd => ../\n",
		"m.go": "package tools\n\nimport \"example.com/ledgerd/internal/ledger\"\n\nvar _ = ledger.Apply\n",
	})
	if out, err := build(t, filepath.Join(root, "ledgerd", "tools")); err != nil {
		t.Fatalf("nested module under the path: %v\n%s", err, out)
	}
}

// An unrelated path stays outside, with replace or with go.work.
func TestUnrelatedPathIsRefused(t *testing.T) {
	root := t.TempDir()
	ledgerd(t, root)
	src := "package e2e\n\nimport \"example.com/ledgerd/internal/ledger\"\n\nvar _ = ledger.Apply\n"
	write(t, filepath.Join(root, "e2e"), map[string]string{
		"go.mod": "module example.com/e2e\n\ngo 1.26.0\n\n" +
			"require example.com/ledgerd v0.0.0\n\nreplace example.com/ledgerd => ../ledgerd\n",
		"m.go": src,
	})
	out, err := build(t, filepath.Join(root, "e2e"))
	if err == nil || !strings.Contains(out, "m.go:3:8: "+notAllowed) {
		t.Fatalf("replace opened the wall: %v\n%s", err, out)
	}

	write(t, filepath.Join(root, "e2ework"), map[string]string{
		"go.mod": "module example.com/e2ework\n\ngo 1.26.0\n",
		"m.go":   strings.Replace(src, "package e2e", "package e2ework", 1),
	})
	write(t, root, map[string]string{
		"go.work": "go 1.26.0\n\nuse (\n\t./ledgerd\n\t./e2ework\n)\n",
	})
	out, err = build(t, filepath.Join(root, "e2ework"),
		"GOWORK="+filepath.Join(root, "go.work"))
	if err == nil || !strings.Contains(out, notAllowed) {
		t.Fatalf("go.work opened the wall: %v\n%s", err, out)
	}
}

// cmd/ledgerd is outside internal/, but a main package can't be
// imported at all, so the tree exposes no Go API.
func TestMainIsNotImportable(t *testing.T) {
	root := t.TempDir()
	ledgerd(t, root)
	write(t, filepath.Join(root, "ledgerd"), map[string]string{
		"internal/httpapi/main.go": "package httpapi\n\nimport _ \"example.com/ledgerd/cmd/ledgerd\"\n",
	})
	out, err := build(t, filepath.Join(root, "ledgerd"))
	if err == nil || !strings.Contains(out, "is a program, not an importable package") {
		t.Fatalf("imported a main package: %v\n%s", err, out)
	}
}
