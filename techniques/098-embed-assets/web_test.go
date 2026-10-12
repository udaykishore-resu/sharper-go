package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Render every template with sample data: parsing alone won't
// catch a field-name typo.
func TestRenderAll(t *testing.T) {
	sample := map[string]any{
		"account.html": struct {
			ID      string
			Balance int64
		}{"<script>x</script>", 1250},
	}
	names, _ := fs.Glob(files, "templates/*.html")
	for _, n := range names {
		name := strings.TrimPrefix(n, "templates/")
		rec := httptest.NewRecorder()
		Render(rec, name, sample[name])
		if rec.Code != 200 {
			t.Errorf("%s: %d", name, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "<script>x") {
			t.Errorf("%s: value not escaped", name)
		}
	}
}

func TestStatic(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static", Static()))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/static/admin.css", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "font-family") {
		t.Fatalf("css: %d", rec.Code)
	}
	// Embedded files have a zero modtime: no Last-Modified goes out.
	if rec.Header().Get("Last-Modified") != "" {
		t.Fatal("unexpected Last-Modified")
	}
}

func TestBadFieldFailsAtExecute(t *testing.T) {
	rec := httptest.NewRecorder()
	Render(rec, "account.html", struct{ Name string }{"x"})
	if rec.Code != 500 {
		t.Fatalf("typo rendered with %d", rec.Code)
	}
}
