package statements

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setup(t *testing.T) (dir string, s *Statements) {
	t.Helper()
	base := t.TempDir()
	dir = filepath.Join(base, "statements")
	must(t, os.MkdirAll(filepath.Join(dir, "acc_1"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "acc_1", "2026-09.csv"), []byte("id,amount\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(base, "secret.csv"), []byte("password\n"), 0o644))
	must(t, os.Symlink("../../secret.csv", filepath.Join(dir, "acc_1", "evil.csv")))
	s, err := New(dir)
	must(t, err)
	return dir, s
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
	dir, s := setup(t)
	if rec := get(s, "acc_1", "2026-09"); rec.Code != 200 {
		t.Fatalf("legit file: %d", rec.Code)
	}
	if rec := get(s, "..", "secret"); rec.Code != 403 {
		t.Fatalf("../ traversal: %d", rec.Code)
	}
	if rec := get(s, "acc_1", "evil"); rec.Code != 403 {
		t.Fatalf("planted symlink: %d", rec.Code)
	}
	_, err := s.root.Open("../secret.csv")
	if err == nil || !strings.Contains(err.Error(), "path escapes from parent") {
		t.Fatalf("err = %v", err)
	}
	// Plain os.Open on filepath.Join happily follows both.
	if _, err := os.Open(filepath.Join(dir, "..", "secret.csv")); err != nil {
		t.Fatal(err)
	}
}
