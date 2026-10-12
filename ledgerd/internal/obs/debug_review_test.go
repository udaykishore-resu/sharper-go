package obs

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Review check: the debug mux serves the profile endpoints the
// book uses, but not pprof's command-line dump.
func TestDebugMuxDropsCmdline(t *testing.T) {
	mux := DebugMux()
	get := func(path string) (int, string) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec.Code, rec.Body.String()
	}
	code, body := get("/debug/pprof/cmdline")
	if code == http.StatusOK || strings.Contains(body, os.Args[0]) {
		t.Fatalf("cmdline served: %d %q", code, body)
	}
	for _, p := range []string{"/debug/pprof/",
		"/debug/pprof/heap", "/debug/pprof/goroutine?debug=1",
		"/debug/pprof/symbol", "/debug/vars"} {
		if code, _ := get(p); code != http.StatusOK {
			t.Errorf("GET %s = %d", p, code)
		}
	}
}
