// Package obs is Technique 86: redact secrets with a type and a
// key list, and add trace IDs with a wrapping handler.
package obs

import (
	"context"
	"log/slog"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

type Secret string

func (Secret) LogValue() slog.Value {
	return slog.StringValue("[REDACTED]")
}

var secretKeys = map[string]bool{
	"authorization": true, "password": true, "token": true,
}

func redact(_ []string, a slog.Attr) slog.Attr {
	if secretKeys[strings.ToLower(a.Key)] {
		return slog.String(a.Key, "[REDACTED]")
	}
	return a
}

type TraceHandler struct{ slog.Handler }

func (h TraceHandler) Handle(
	ctx context.Context, r slog.Record,
) error {
	sc := trace.SpanContextFromContext(ctx)
	if sc.IsValid() {
		r = r.Clone()
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()))
	}
	return h.Handler.Handle(ctx, r)
}

func (h TraceHandler) WithAttrs(
	as []slog.Attr) slog.Handler {
	return TraceHandler{h.Handler.WithAttrs(as)}
}

func (h TraceHandler) WithGroup(name string) slog.Handler {
	return TraceHandler{h.Handler.WithGroup(name)}
}

// NewLogger wires both hooks.
func NewLogger(h slog.Handler) *slog.Logger { return slog.New(TraceHandler{h}) }

func jsonHandler(w interface{ Write([]byte) (int, error) }) slog.Handler {
	return slog.NewJSONHandler(w, &slog.HandlerOptions{ReplaceAttr: redact})
}
