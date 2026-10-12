package govulncheck

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const modified = "2026-01-01T00:00:00Z"

// writeDB writes a minimal v1 vulnerability database.
func writeDB(t *testing.T) string {
	t.Helper()
	db := t.TempDir()
	entries := map[string][2]string{
		"GO-2099-0001": {"strconv", "QuoteToGraphic"},
		"GO-2099-0002": {"strings", "Replacer.Replace"},
	}
	var mods, vulns []map[string]any
	for id, sym := range entries {
		osv := map[string]any{
			"schema_version": "1.3.1", "id": id,
			"modified": modified, "published": modified,
			"details":           "Test entry.",
			"database_specific": map[string]any{"url": "https://pkg.go.dev/vuln/" + id},
			"affected": []any{map[string]any{
				"package": map[string]any{
					"name": "stdlib", "ecosystem": "Go"},
				"ranges": []any{map[string]any{
					"type": "SEMVER",
					"events": []any{
						map[string]any{"introduced": "0"},
						map[string]any{"fixed": "1.99.0"},
					}}},
				"ecosystem_specific": map[string]any{
					"imports": []any{map[string]any{
						"path": sym[0], "symbols": []string{sym[1]},
					}}},
			}},
		}
		writeJSON(t, filepath.Join(db, "ID", id+".json"), osv)
		mods = append(mods, map[string]any{"id": id,
			"modified": modified, "fixed": "1.99.0"})
		vulns = append(vulns, map[string]any{"id": id,
			"modified": modified})
	}
	writeJSON(t, filepath.Join(db, "index", "db.json"),
		map[string]any{"modified": modified})
	writeJSON(t, filepath.Join(db, "index", "modules.json"),
		[]any{map[string]any{"path": "stdlib", "vulns": mods}})
	writeJSON(t, filepath.Join(db, "index", "vulns.json"), vulns)
	return "file://" + filepath.ToSlash(db)
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	write(t, path, string(b))
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// program writes a one-module program with the given files.
func program(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"),
		"module example.com/p\n\ngo 1.26.0\n")
	for name, src := range files {
		write(t, filepath.Join(dir, name), src)
	}
	return dir
}

// buildGovulncheck builds the govulncheck that ledgerd pins.
func buildGovulncheck(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds govulncheck; skipped in -short mode")
	}
	bin := filepath.Join(t.TempDir(), "govulncheck")
	cmd := exec.Command("go", "build", "-o", bin,
		"golang.org/x/vuln/cmd/govulncheck")
	cmd.Dir = filepath.Join("..", "..", "ledgerd")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build govulncheck: %v\n%s", err, out)
	}
	return bin
}

// scan runs govulncheck and returns its exit code and output.
func scan(t *testing.T, bin, dir string,
	args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=", "GOWORK=off",
		"GOTOOLCHAIN=local")
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return 0, string(out)
	case errors.As(err, &ee):
		return ee.ExitCode(), string(out)
	}
	t.Fatalf("run govulncheck: %v\n%s", err, out)
	return -1, ""
}

func buildStripped(t *testing.T, dir string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "app")
	cmd := exec.Command("go", "build", "-trimpath",
		"-ldflags=-s -w", "-o", bin, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=", "CGO_ENABLED=0",
		"GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

const reachable = `package main

import "strconv"

func main() { println(strconv.QuoteToGraphic("x")) }
`

func TestGovulncheck(t *testing.T) {
	bin := buildGovulncheck(t)
	db := writeDB(t)

	t.Run("reachable finding fails the job", func(t *testing.T) {
		dir := program(t, map[string]string{"main.go": reachable})
		code, out := scan(t, bin, dir, "-db", db,
			"-show", "traces", "./...")
		if code != 3 || !strings.Contains(out, "GO-2099-0001") ||
			!strings.Contains(out, "strconv.QuoteToGraphic") {
			t.Fatalf("exit %d, want 3 with a trace:\n%s",
				code, out)
		}
	})

	t.Run("machine formats always exit 0", func(t *testing.T) {
		dir := program(t, map[string]string{"main.go": reachable})
		for _, f := range []string{"json", "sarif", "openvex"} {
			code, out := scan(t, bin, dir, "-db", db,
				"-format", f, "./...")
			if code != 0 || !strings.Contains(out, "GO-2099-0001") {
				t.Errorf("-format %s: exit %d, want 0 with the "+
					"finding in the output", f, code)
			}
		}
	})

	t.Run("imported but not called passes", func(t *testing.T) {
		dir := program(t, map[string]string{"main.go": `package main

import "strconv"

func main() { println(strconv.Itoa(1)) }
`})
		if code, out := scan(t, bin, dir, "-db", db,
			"./..."); code != 0 {
			t.Fatalf("exit %d, want 0:\n%s", code, out)
		}
	})

	t.Run("tagged files need -tags", func(t *testing.T) {
		dir := program(t, map[string]string{
			"main.go": "package main\n\nfunc main() { run() }\n",
			"plain.go": "//go:build !extra\n\npackage main\n\n" +
				"func run() {}\n",
			"extra.go": "//go:build extra\n\npackage main\n\n" +
				"import \"strconv\"\n\n" +
				"func run() { println(strconv.QuoteToGraphic(\"x\")) }\n",
		})
		if code, _ := scan(t, bin, dir, "-db", db,
			"./..."); code != 0 {
			t.Fatalf("untagged scan: exit %d, want 0", code)
		}
		if code, _ := scan(t, bin, dir, "-db", db,
			"-tags", "extra", "./..."); code != 3 {
			t.Fatalf("tagged scan: exit %d, want 3", code)
		}
	})

	// A call made only through reflection is invisible to source
	// mode. Binary mode sees the symbol in the stripped binary.
	t.Run("reflection hides a call", func(t *testing.T) {
		dir := program(t, map[string]string{"main.go": `package main

import (
	"reflect"
	"strings"
)

func main() {
	r := strings.NewReplacer("a", "b")
	m := reflect.ValueOf(r).MethodByName("Rep" + "lace")
	out := m.Call([]reflect.Value{reflect.ValueOf("abc")})
	println(out[0].String())
}
`})
		_, out := scan(t, bin, dir, "-db", db, "./...")
		if strings.Contains(out, "GO-2099-0002") {
			t.Fatalf("source mode saw the reflect call:\n%s", out)
		}
		app := buildStripped(t, dir)
		code, out := scan(t, bin, dir, "-db", db,
			"-mode", "binary", app)
		if code != 3 || !strings.Contains(out, "GO-2099-0002") {
			t.Fatalf("binary mode: exit %d, want 3 and "+
				"GO-2099-0002:\n%s", code, out)
		}
	})
}
