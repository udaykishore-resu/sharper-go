package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serve(h http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestRoutes(t *testing.T) {
	mux := (&API{}).Routes()
	for _, c := range []struct{ method, path, body string }{
		{"GET", "/v1/accounts/acc_1", "account acc_1"},
		{"GET", "/v1/accounts/house", "house"}, // most specific wins
		{"GET", "/v1/accounts/acc_1/entries", "entries acc_1"},
		{"HEAD", "/v1/accounts/acc_1", "account acc_1"}, // GET also matches HEAD
	} {
		rec := serve(mux, c.method, c.path)
		if rec.Code != 200 || rec.Body.String() != c.body {
			t.Errorf("%s %s = %d %q", c.method, c.path, rec.Code, rec.Body)
		}
	}
	rec := serve(mux, "DELETE", "/v1/accounts/acc_1")
	if rec.Code != 405 || !strings.Contains(rec.Header().Get("Allow"), "GET") {
		t.Fatalf("wrong method: %d Allow=%q", rec.Code, rec.Header().Get("Allow"))
	}
}

// Two patterns where neither is more specific panic at startup.
func TestOverlapPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil || !strings.Contains(fmt.Sprint(r), "conflicts with") {
			t.Fatalf("recover() = %v", r)
		}
	}()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /a/{x}/c", func(http.ResponseWriter, *http.Request) {})
	mux.HandleFunc("GET /a/b/{y}", func(http.ResponseWriter, *http.Request) {})
}

// PathValue returns the decoded segment: ..%2Fetc arrives as ../etc.
func TestPathValueDecodes(t *testing.T) {
	rec := serve((&API{}).Routes(), "GET", "/v1/accounts/..%2Fetc")
	if rec.Body.String() != "account ../etc" {
		t.Fatalf("got %q", rec.Body)
	}
}

// Without the mux, fill path values with SetPathValue.
func TestHandlerWithoutMux(t *testing.T) {
	req := httptest.NewRequest("GET", "/whatever", nil)
	req.SetPathValue("id", "acc_7")
	rec := httptest.NewRecorder()
	(&API{}).getAccount(rec, req)
	if rec.Body.String() != "account acc_7" {
		t.Fatalf("got %q", rec.Body)
	}
}
