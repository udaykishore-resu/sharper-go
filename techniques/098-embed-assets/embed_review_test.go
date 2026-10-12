package web

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// Review checks for Technique 98: missing files fail the build,
// content types, template errors, escaping of untrusted data, and
// the dot-file rule for directory patterns.

func TestContentTypes(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static", Static()))
	for path, want := range map[string]string{
		"/static/admin.css": "text/css; charset=utf-8",
		"/static/nope.css":  "text/plain; charset=utf-8", // 404 body
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if got := rec.Header().Get("Content-Type"); got != want {
			t.Errorf("%s: Content-Type %q, want %q", path, got, want)
		}
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/static/nope.css", nil))
	if rec.Code != 404 {
		t.Errorf("missing asset: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	Render(rec, "account.html", struct {
		ID      string
		Balance int64
	}{"a1", 5})
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("page Content-Type %q", ct)
	}
}

// Untrusted values are escaped for the context they land in, and
// a value that looks like template code stays text: data is never
// parsed as template source.
func TestUntrustedDataStaysData(t *testing.T) {
	fsys := fstest.MapFS{"p.html": {Data: []byte(
		`<a href="{{.URL}}" title="{{.Name}}">{{.Name}}</a>` +
			`<script>var n = {{.Name}};</script>`)}}
	tpl := template.Must(template.ParseFS(fsys, "p.html"))
	var b strings.Builder
	err := tpl.Execute(&b, map[string]string{
		"URL":  "javascript:alert(1)",
		"Name": `"><script>x</script>{{.URL}}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, bad := range []string{"javascript:", "<script>x", `"><`} {
		if strings.Contains(out, bad) {
			t.Errorf("unescaped %q in %s", bad, out)
		}
	}
	if !strings.Contains(out, "#ZgotmplZ") || !strings.Contains(out, "{{.URL}}") {
		t.Errorf("want filtered URL and literal braces: %s", out)
	}
}

// A template syntax error is a ParseFS error, which template.Must
// turns into a panic at package init: startup, not first request.
func TestSyntaxErrorFailsParse(t *testing.T) {
	fsys := fstest.MapFS{"bad.html": {Data: []byte(`{{.ID`)}}
	if _, err := template.ParseFS(fsys, "*.html"); err == nil {
		t.Fatal("bad template parsed")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("Must did not panic")
		}
	}()
	template.Must(template.ParseFS(fsys, "*.html"))
}

// buildScratch builds a one-file program with the given embed
// line and files, then runs it; it returns build or run output.
func buildScratch(t *testing.T, pattern string, files map[string]string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(body), 0o644)
	}
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module scratch\n\ngo 1.22\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed `+pattern+`
var files embed.FS

func main() {
	fs.WalkDir(files, ".", func(p string, d fs.DirEntry, err error) error {
		if !d.IsDir() {
			fmt.Println(p)
		}
		return nil
	})
}
`), 0o644)
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestEmbedPatterns(t *testing.T) {
	if testing.Short() {
		t.Skip("builds scratch programs")
	}
	files := map[string]string{
		"static/app.css":                  "body{}",
		"static/.well-known/security.txt": "Contact: x",
		"static/_drafts/old.css":          "old",
	}
	out, err := buildScratch(t, "static", files)
	if err != nil || !strings.Contains(out, "static/app.css") ||
		strings.Contains(out, "security.txt") || strings.Contains(out, "_drafts") {
		t.Fatalf("plain pattern: %v\n%s", err, out)
	}
	out, err = buildScratch(t, "all:static", files)
	if err != nil || !strings.Contains(out, "static/.well-known/security.txt") {
		t.Fatalf("all: pattern: %v\n%s", err, out)
	}
	// A renamed or missing file fails the build, before any deploy.
	out, err = buildScratch(t, "static/admin.css", files)
	if err == nil || !strings.Contains(out, "no matching files found") {
		t.Fatalf("missing file built: %v\n%s", err, out)
	}
}
