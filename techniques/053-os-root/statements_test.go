package statements

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Layout under a temp dir:
//
//	secret.csv                       outside the root
//	statements/acc_1/2026-09.csv     a real statement
//	statements/acc_1/evil.csv     -> ../../secret.csv  (relative, escapes)
//	statements/acc_1/latest.csv   -> 2026-09.csv       (relative, inside)
//	statements/acc_1/abs.csv      -> /abs/path/2026-09.csv (absolute, inside)
func setup(t *testing.T) (dir string, s *Statements, logs *bytes.Buffer) {
	t.Helper()
	base := t.TempDir()
	dir = filepath.Join(base, "statements")
	acc := filepath.Join(dir, "acc_1")
	must(t, os.MkdirAll(acc, 0o755))
	must(t, os.WriteFile(filepath.Join(acc, "2026-09.csv"), []byte("id,amount\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(base, "secret.csv"), []byte("password\n"), 0o644))
	must(t, os.Symlink("../../secret.csv", filepath.Join(acc, "evil.csv")))
	must(t, os.Symlink("2026-09.csv", filepath.Join(acc, "latest.csv")))
	must(t, os.Symlink(filepath.Join(acc, "2026-09.csv"), filepath.Join(acc, "abs.csv")))
	logs = &bytes.Buffer{}
	s, err := New(dir, slog.New(slog.NewTextHandler(logs, nil)))
	must(t, err)
	t.Cleanup(func() { s.root.Close() })
	return dir, s, logs
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func get(s *Statements, id, month string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/", nil)
	req.SetPathValue("id", id)
	req.SetPathValue("month", month)
	rec := httptest.NewRecorder()
	s.Get(rec, req)
	return rec
}

func TestRootRefusesEscapes(t *testing.T) {
	dir, s, _ := setup(t)
	for _, c := range []struct {
		id, month string
		want      int
	}{
		{"acc_1", "2026-09", 200},
		{"acc_1", "latest", 200}, // relative link that stays inside
		{"..", "secret", 403},    // parent traversal
		{"acc_1", "evil", 403},   // relative link out of the root
		{"acc_1", "abs", 403},    // absolute link, even to inside
		{"acc_1", "2026-10", 404},
	} {
		if got := get(s, c.id, c.month).Code; got != c.want {
			t.Errorf("%s/%s: %d, want %d", c.id, c.month, got, c.want)
		}
	}
	_, err := s.root.Open("../secret.csv")
	if err == nil || !strings.Contains(err.Error(), "path escapes from parent") {
		t.Fatalf("err = %v", err)
	}
	// Plain os.Open on filepath.Join happily follows both.
	if _, err := os.Open(filepath.Join(dir, "..", "secret.csv")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Open(filepath.Join(dir, "acc_1", "evil.csv")); err != nil {
		t.Fatal(err)
	}
}

// root.FS() keeps the protection behind http.FileServerFS;
// http.Dir follows the same link out.
func TestFileServerFS(t *testing.T) {
	dir, s, _ := setup(t)
	fetch := func(h http.Handler, path string) int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec.Code
	}
	rooted := http.FileServerFS(s.root.FS())
	if got := fetch(rooted, "/acc_1/2026-09.csv"); got != 200 {
		t.Fatalf("rooted legit file: %d", got)
	}
	if got := fetch(rooted, "/acc_1/evil.csv"); got == 200 {
		t.Fatal("FileServerFS(root.FS()) served a link out of the root")
	}
	if got := fetch(http.FileServer(http.Dir(dir)), "/acc_1/evil.csv"); got != 200 {
		t.Fatalf("http.Dir: %d (expected it to follow the link)", got)
	}
}

// A Root holds a handle, not a name: after a deploy swaps the
// directory, it keeps serving the old one, with no error.
func TestRootSurvivesSwap(t *testing.T) {
	dir, s, _ := setup(t)
	must(t, os.Rename(dir, dir+".old"))
	must(t, os.MkdirAll(filepath.Join(dir, "acc_1"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "acc_1", "2026-09.csv"), []byte("new\n"), 0o644))
	rec := get(s, "acc_1", "2026-09")
	if rec.Code != 200 || rec.Body.String() != "id,amount\n" {
		t.Fatalf("got %d %q, want the old file", rec.Code, rec.Body)
	}
}

// failWriter's body writes fail, like a client that hung up.
type failWriter struct{ *httptest.ResponseRecorder }

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

func (f failWriter) ReadFrom(r io.Reader) (int64, error) {
	return 0, errors.New("broken pipe")
}

// A failed copy can't become a 500 (the status is sent), so it
// is logged.
func TestCopyErrorLogged(t *testing.T) {
	_, s, logs := setup(t)
	req := httptest.NewRequest("GET", "/", nil)
	req.SetPathValue("id", "acc_1")
	req.SetPathValue("month", "2026-09")
	s.Get(failWriter{httptest.NewRecorder()}, req)
	if !strings.Contains(logs.String(), "statement copy") ||
		!strings.Contains(logs.String(), "broken pipe") {
		t.Fatalf("logs: %q", logs)
	}
}
