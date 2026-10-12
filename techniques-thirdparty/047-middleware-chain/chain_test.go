package httpapi

import (
	"bytes"
	"cmp"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type routeRecorder struct{ route string }

func (m *routeRecorder) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		m.route = cmp.Or(r.Pattern, "unmatched")
	})
}

func setup(t *testing.T) (http.Handler, *bytes.Buffer, *routeRecorder, *tracetest.SpanRecorder) {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr)))
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/accounts/{id}", func(http.ResponseWriter, *http.Request) {})
	var buf bytes.Buffer
	m := &routeRecorder{}
	v := func(tok string) (string, bool) { return "acme", tok == "Bearer ok" }
	return build(mux, v, m, slog.New(slog.NewJSONHandler(&buf, nil))), &buf, m, sr
}

// Readers inside the copiers see the pattern the mux set, and so
// does otelhttp, which names its span from it.
func TestReadersSeePattern(t *testing.T) {
	h, buf, m, sr := setup(t)
	req := httptest.NewRequest("GET", "/v1/accounts/acc_1", nil)
	req.Header.Set("Authorization", "Bearer ok")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if !strings.Contains(buf.String(), `"route":"GET /v1/accounts/{id}"`) {
		t.Fatalf("access log: %s", buf)
	}
	if m.route != "GET /v1/accounts/{id}" {
		t.Fatalf("metrics route = %q", m.route)
	}
	if spans := sr.Ended(); len(spans) != 1 || spans[0].Name() != "GET /v1/accounts/{id}" {
		t.Fatalf("spans = %v", spans)
	}
}

// A layer that copies r between the mux and the readers hides the
// pattern: this is the "route is empty" breakage.
func TestCopyInsideReadersHidesPattern(t *testing.T) {
	var buf bytes.Buffer
	mux := http.NewServeMux()
	mux.HandleFunc("GET /x/{id}", func(http.ResponseWriter, *http.Request) {})
	h := Chain(mux, AccessLog(slog.New(slog.NewJSONHandler(&buf, nil))), RequestID)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x/1", nil))
	if !strings.Contains(buf.String(), `"route":"unmatched"`) {
		t.Fatalf("expected the pattern to be lost: %s", &buf)
	}
}

// Auth rejects before the access log runs.
func TestAuthRejectsBeforeLog(t *testing.T) {
	h, buf, _, _ := setup(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/accounts/acc_1", nil))
	if rec.Code != 401 || buf.Len() != 0 {
		t.Fatalf("code %d, log %q", rec.Code, buf)
	}
}
