package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPanicBecomesOne500(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /boom", func(http.ResponseWriter, *http.Request) {
		nilMap()["x"] = 1 // write to a nil map
	})
	before := panics.Value()
	rec := httptest.NewRecorder()
	Recover(log)(mux).ServeHTTP(rec, httptest.NewRequest("GET", "/boom", nil))
	if rec.Code != 500 || panics.Value() != before+1 {
		t.Fatalf("code %d, panics %d", rec.Code, panics.Value())
	}
	if strings.Count(buf.String(), "\n") != 1 {
		t.Fatalf("want one JSON line, got %q", buf.String())
	}
	var line map[string]any
	json.Unmarshal(buf.Bytes(), &line)
	if !strings.Contains(line["stack"].(string), "TestPanicBecomesOne500") {
		t.Fatal("stack lacks the panicking frame")
	}
}

// After headers went out, the helper aborts: the client sees a
// broken transfer, not a clean half body.
func TestPanicAfterWriteAborts(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	srv := httptest.NewServer(Recover(log)(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte("partial"))
			http.NewResponseController(w).Flush()
			panic("late")
		})))
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		return // connection cut before headers were read: also fine
	}
	defer resp.Body.Close()
	if _, err := io.ReadAll(resp.Body); err == nil {
		t.Fatal("half body read as success")
	}
}

func nilMap() map[string]int { return nil }
