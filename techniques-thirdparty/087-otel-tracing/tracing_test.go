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
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
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

// Review: inbound, otelhttp continues the caller's trace from
// traceparent, and background work started with
// context.WithoutCancel stays in the same trace after the
// request ends.
func TestInboundAndBackgroundPropagation(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr)))
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	bg := make(chan trace.SpanContext, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/transfers", func(_ http.ResponseWriter, r *http.Request) {
		ctx := context.WithoutCancel(r.Context())
		go func() {
			_, span := otel.Tracer("relay").Start(ctx, "notify")
			defer span.End()
			bg <- span.SpanContext()
		}()
	})
	srv := httptest.NewServer(otelhttp.NewHandler(mux, "ledgerd"))
	defer srv.Close()
	const parent = "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	req, _ := http.NewRequest("POST", srv.URL+"/v1/transfers", nil)
	req.Header.Set("traceparent", parent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	child := <-bg
	if got := child.TraceID().String(); got != "0af7651916cd43dd8448eb211c80319c" {
		t.Fatalf("background span trace = %s", got)
	}
	var server sdktrace.ReadOnlySpan
	for end := time.Now().Add(5 * time.Second); server == nil; time.Sleep(time.Millisecond) {
		if time.Now().After(end) {
			t.Fatal("server span never ended")
		}
		for _, s := range sr.Ended() {
			if s.SpanKind() == trace.SpanKindServer {
				server = s
			}
		}
	}
	if server.Parent().SpanID().String() != "b7ad6b7169203331" ||
		server.Name() != "POST /v1/transfers" {
		t.Fatalf("server span %q parent %s", server.Name(), server.Parent().SpanID())
	}
}

// Review: a collector that hangs cannot hold shutdown past the
// timeout the caller gives it.
func TestShutdownBoundedWhenCollectorHangs(t *testing.T) {
	block := make(chan struct{})
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-block:
		case <-r.Context().Done():
		}
	}))
	defer collector.Close()
	defer close(block)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.URL)
	shutdown, err := SetupTracing(t.Context(), "v1")
	if err != nil {
		t.Fatal(err)
	}
	_, span := otel.Tracer("test").Start(context.Background(), "x")
	span.End()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	err = shutdown(ctx)
	if took := time.Since(start); took > 3*time.Second {
		t.Fatalf("shutdown took %v", took)
	}
	if err == nil {
		t.Fatal("want an error from a flush that never finished")
	}
}
