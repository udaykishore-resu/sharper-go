package obs

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
)

// A fake collector on OTEL_EXPORTER_OTLP_ENDPOINT receives the
// spans, and the propagator sends traceparent across a hop.
func TestSpansReachCollector(t *testing.T) {
	var batches atomic.Int32
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/traces" {
			io.Copy(io.Discard, r.Body)
			batches.Add(1)
		}
	}))
	defer collector.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.URL)

	shutdown, err := SetupTracing(t.Context(), "v1.4.2")
	if err != nil {
		t.Fatal(err)
	}
	var gotParent atomic.Value
	downstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotParent.Store(r.Header.Get("traceparent"))
	}))
	defer downstream.Close()
	client := &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}

	ctx, span := otel.Tracer("test").Start(context.Background(), "POST /v1/transfers")
	req, _ := http.NewRequestWithContext(ctx, "GET", downstream.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	span.End()

	// Shutdown flushes, on a fresh context with its own timeout.
	sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := shutdown(sctx); err != nil {
		t.Fatal(err)
	}
	if batches.Load() == 0 {
		t.Fatal("no spans reached the collector")
	}
	if p, _ := gotParent.Load().(string); len(p) != 55 {
		t.Fatalf("traceparent = %q", p)
	}
}
