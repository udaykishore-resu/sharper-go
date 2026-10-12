package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStatusWriter(t *testing.T) {
	cases := []struct {
		name string
		h    http.HandlerFunc
		want int
	}{
		{"bare Write is an implicit 200", func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte("x"))
		}, 200},
		{"repeated WriteHeader keeps the first", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(201)
			w.WriteHeader(500) // net/http logs "superfluous" and ignores it
		}, 201},
		{"no write at all records nothing", func(http.ResponseWriter, *http.Request) {}, 0},
	}
	for _, c := range cases {
		sw := &statusWriter{ResponseWriter: httptest.NewRecorder()}
		c.h(sw, httptest.NewRequest("GET", "/", nil))
		if sw.status != c.want {
			t.Errorf("%s: status %d, want %d", c.name, sw.status, c.want)
		}
	}
}

// Through a real server: the wrapper hides http.Flusher, but a
// ResponseController follows Unwrap to Flush and to set deadlines.
func TestStatusWriterOptionalInterfaces(t *testing.T) {
	res := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w}
		_, isFlusher := any(sw).(http.Flusher)
		rc := http.NewResponseController(sw)
		sw.Write([]byte("chunk"))
		ferr := rc.Flush()
		derr := rc.SetWriteDeadline(time.Now().Add(time.Minute))
		var b strings.Builder
		if isFlusher {
			b.WriteString("flusher ")
		}
		if ferr != nil || derr != nil {
			b.WriteString("rc failed")
		}
		res <- b.String()
	}))
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := <-res; got != "" {
		t.Fatalf("got %q, want no Flusher and working ResponseController", got)
	}
}

// A panic inside the readers becomes a 500 that the access log
// still records with the route.
func TestPanicIsLogged(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/transfers", func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})
	h := Chain(mux, RequestID, AccessLog(log), Recover(log))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/transfers", nil))
	if rec.Code != 500 ||
		!strings.Contains(buf.String(), `"route":"POST /v1/transfers","status":500`) {
		t.Fatalf("code %d, log %s", rec.Code, &buf)
	}
}
