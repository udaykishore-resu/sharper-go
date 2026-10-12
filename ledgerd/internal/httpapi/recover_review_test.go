package httpapi

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"example.com/ledgerd/internal/obs"
)

// Technique 90 review: http.ErrAbortHandler passes through
// untouched: no log line, no count, and the panic continues.
func TestRecoverPassesErrAbortHandler(t *testing.T) {
	var buf bytes.Buffer
	before := testutil.ToFloat64(obs.Panics)
	h := Recover(slog.New(slog.NewJSONHandler(&buf, nil)))(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			panic(http.ErrAbortHandler)
		}))
	defer func() {
		if v := recover(); v != any(http.ErrAbortHandler) {
			t.Fatalf("recovered %v, want ErrAbortHandler", v)
		}
		if buf.Len() != 0 || testutil.ToFloat64(obs.Panics) != before {
			t.Fatalf("abort was logged or counted: %s", &buf)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
}

// A panic after WriteHeader, with no body yet, still aborts: the
// status is gone, so a clean end would pass for success.
func TestRecoverAfterHeadersAborts(t *testing.T) {
	before := testutil.ToFloat64(obs.Panics)
	srv := httptest.NewServer(Recover(slog.New(slog.DiscardHandler))(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "10")
			w.WriteHeader(http.StatusOK)
			http.NewResponseController(w).Flush()
			panic("after headers")
		})))
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err == nil {
		_, err = io.ReadAll(resp.Body)
		resp.Body.Close()
	}
	if err == nil {
		t.Fatal("aborted response read as success")
	}
	if testutil.ToFloat64(obs.Panics) != before+1 {
		t.Fatal("late panic not counted")
	}
}

// Recovery covers only the handler's goroutine: a panic in a
// goroutine the handler starts kills the process. Run in a child
// process so the crash is observable.
func TestRecoverMissesChildGoroutine(t *testing.T) {
	if os.Getenv("LEDGERD_CHILD_PANIC") == "1" {
		h := Recover(slog.New(slog.DiscardHandler))(
			http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				go func() { nilMap()["x"] = 1 }()
				time.Sleep(time.Second)
			}))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
		os.Exit(0) // not reached: the goroutine's panic exits first
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRecoverMissesChildGoroutine$")
	cmd.Env = append(os.Environ(), "LEDGERD_CHILD_PANIC=1")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "assignment to entry in nil map") {
		t.Fatalf("child survived: err=%v\n%s", err, out)
	}
}
