package obs

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestTraceIDAfterWith(t *testing.T) {
	var buf bytes.Buffer
	h := TraceHandler{slog.NewJSONHandler(&buf, nil)}
	log := slog.New(h).With("route", "x")
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

// Delete WithAttrs and the wrapper is dropped by the first With.
type noWithAttrs struct{ slog.Handler }

func (h noWithAttrs) Handle(ctx context.Context, r slog.Record) error {
	return TraceHandler(h).Handle(ctx, r)
}

func TestWithoutWithAttrsLosesTraceID(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(noWithAttrs{slog.NewJSONHandler(&buf, nil)}).With("route", "x")
	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{1}})
	log.InfoContext(trace.ContextWithSpanContext(context.Background(), sc), "posted")
	if strings.Contains(buf.String(), `"trace_id"`) {
		t.Fatal("expected the embedded WithAttrs to unwrap the handler")
	}
}

// The Try it: a key is redacted, the same header inside a map is
// not, and logging that one field by name as a Secret is. A
// Secret inside a map is not safe either: the JSON handler
// marshals map values with encoding/json, which never calls
// LogValue.
func TestRedaction(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogger(jsonHandler(&buf))
	h := http.Header{"Authorization": {"Bearer y"}}
	log.Info("a", "Authorization", "Bearer x")
	log.Info("b", "headers", h)
	log.Info("c", "auth", Secret(h.Get("Authorization")))
	log.Info("d", "headers", map[string]Secret{"Authorization": "Bearer z"})
	out := buf.String()
	if strings.Contains(out, "Bearer x") || !strings.Contains(out, "Bearer y") {
		t.Fatalf("key list: %s", out)
	}
	if !strings.Contains(out, `"auth":"[REDACTED]"`) {
		t.Fatalf("Secret by name not redacted: %s", out)
	}
	if !strings.Contains(out, "Bearer z") {
		t.Fatal("expected a Secret inside a map to leak")
	}
}
