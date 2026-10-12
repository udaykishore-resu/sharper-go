package debug

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec
}

// serveDebug binds loopback only; if 6060 is taken it just logs.
func TestServeDebug(t *testing.T) {
	srv := serveDebug()
	if srv.Addr != "127.0.0.1:6060" {
		t.Fatalf("debug server on %s", srv.Addr)
	}
	srv.Close()
}

func TestPrivateMux(t *testing.T) {
	mux := debugMux()
	for _, p := range []string{"/debug/pprof/heap?debug=1", "/debug/pprof/goroutine?debug=1"} {
		if rec := get(mux, p); rec.Code != 200 {
			t.Errorf("%s: %d", p, rec.Code)
		}
	}
	// Index also lists the named profiles, Go 1.27's goroutineleak included.
	if body := get(mux, "/debug/pprof/").Body.String(); !strings.Contains(body, "allocs") {
		t.Errorf("index: %.200s", body)
	}
}

// Importing net/http/pprof at all, blank or not, runs its init,
// which registers on http.DefaultServeMux. So the public server
// must never serve the default mux: give it its own.
func TestDefaultMuxIsTainted(t *testing.T) {
	if rec := get(http.DefaultServeMux, "/debug/pprof/"); rec.Code != 200 {
		t.Fatalf("expected init to register on DefaultServeMux, got %d", rec.Code)
	}
	public := http.NewServeMux()
	public.HandleFunc("GET /v1/accounts/{id}", func(http.ResponseWriter, *http.Request) {})
	if rec := get(public, "/debug/pprof/"); rec.Code != 404 {
		t.Fatalf("public mux serves pprof: %d", rec.Code)
	}
}
