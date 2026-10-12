package obs

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.opentelemetry.io/otel/trace"
)

// Technique 86: trace IDs survive logger.With.
func TestTraceIDAfterWith(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogger(&buf, "test").With("route", "x")
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1},
		SpanID:  trace.SpanID{1},
	})
	ctx := trace.ContextWithSpanContext(
		context.Background(), sc)
	log.InfoContext(ctx, "posted")
	if !strings.Contains(buf.String(), `"trace_id"`) {
		t.Fatalf("trace_id lost after With: %s", &buf)
	}
}

func TestRedact(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogger(&buf, "test")
	log.Info("auth", "Authorization", "Bearer x", "dsn", Secret("postgres://u:p@h"))
	if strings.Contains(buf.String(), "Bearer x") ||
		strings.Contains(buf.String(), "u:p@h") {
		t.Fatalf("secret leaked: %s", &buf)
	}
}

// Technique 88: a real route, a 404 and a 405 give two route values.
func TestMetricsRouteLabels(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/accounts/{id}", func(http.ResponseWriter, *http.Request) {})
	h := m.Wrap(mux)
	for _, c := range []struct{ method, path string }{
		{"GET", "/v1/accounts/a1"},
		{"GET", "/wp-admin"},
		{"DELETE", "/v1/accounts/a1"},
	} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(c.method, c.path, nil))
	}
	if n := testutil.CollectAndCount(m.Duration); n != 2 {
		t.Fatalf("route series = %d, want 2", n)
	}
	want := `
		# HELP ledgerd_http_requests_total Requests by route pattern and status code.
		# TYPE ledgerd_http_requests_total counter
		ledgerd_http_requests_total{code="200",route="GET /v1/accounts/{id}"} 1
		ledgerd_http_requests_total{code="404",route="unmatched"} 1
		ledgerd_http_requests_total{code="405",route="unmatched"} 1
	`
	if err := testutil.CollectAndCompare(m.Requests, strings.NewReader(want)); err != nil {
		t.Fatal(err)
	}
}

func TestBuildInfo(t *testing.T) {
	b := BuildInfo()
	if b.Version == "" || !strings.HasPrefix(b.Go, "go1.") {
		t.Fatalf("BuildInfo() = %+v", b)
	}
	_ = slog.Default()
}
