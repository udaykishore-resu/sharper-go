package httpapi

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Review check for Recover: a panic before the first write is a
// logged 500; after the first write it re-panics
// http.ErrAbortHandler (the server cuts the connection) and
// never calls http.Error on a sent response; an ErrAbortHandler
// from the handler passes through.
func TestRecoverBeforeAndAfterWrite(t *testing.T) {
	cases := []struct {
		name     string
		h        http.HandlerFunc
		want500  bool
		wantLogs bool
	}{
		{"before write", func(http.ResponseWriter, *http.Request) {
			panic("boom")
		}, true, true},
		{"after header", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			panic("boom")
		}, false, true},
		{"after body", func(w http.ResponseWriter, _ *http.Request) {
			io.WriteString(w, "partial")
			panic("boom")
		}, false, true},
		{"abort passes", func(http.ResponseWriter, *http.Request) {
			panic(http.ErrAbortHandler)
		}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			log := slog.New(slog.NewJSONHandler(&buf, nil))
			h := Recover(log)(tc.h)
			rec := httptest.NewRecorder()
			got := func() (v any) {
				defer func() { v = recover() }()
				h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
				return nil
			}()
			if tc.want500 {
				if got != nil || rec.Code != 500 {
					t.Fatalf("panic %v, code %d", got, rec.Code)
				}
			} else if got != http.ErrAbortHandler {
				t.Fatalf("re-panic = %v, want ErrAbortHandler", got)
			}
			if !tc.want500 && strings.Contains(rec.Body.String(), "internal error") {
				t.Fatalf("http.Error after headers: %q", rec.Body)
			}
			if logged := strings.Contains(buf.String(), `"msg":"panic"`); logged != tc.wantLogs {
				t.Fatalf("logged = %v: %s", logged, &buf)
			}
		})
	}
}

// Through a real server, a late panic cuts the connection: the
// client sees an error, never a clean "partial" body.
func TestRecoverLatePanicCutsConnection(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	srv := httptest.NewServer(Recover(log)(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "100")
			io.WriteString(w, "partial")
			http.NewResponseController(w).Flush()
			panic("late")
		})))
	defer srv.Close()
	resp, err := srv.Client().Get(srv.URL)
	if err != nil {
		return // cut before the headers arrived: fine
	}
	defer resp.Body.Close()
	if _, err := io.ReadAll(resp.Body); err == nil {
		t.Fatal("late panic produced a complete response")
	}
}
