package obs

import (
	"context"
	"io"
	"log/slog"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

// Level can change while the program runs (Techniques 85, 91).
var Level = new(slog.LevelVar) // Info by default

// NewLogger builds the one JSON logger ledgerd uses
// (Technique 85). It redacts secret keys and adds trace IDs
// (Technique 86).
func NewLogger(w io.Writer, ver string) *slog.Logger {
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:       Level,
		ReplaceAttr: redact,
	})
	return slog.New(TraceHandler{h}).With(
		slog.String("service", "ledgerd"),
		slog.String("version", ver))
}

// Secret logs as [REDACTED] under any key, in any handler.
type Secret string

func (Secret) LogValue() slog.Value {
	return slog.StringValue("[REDACTED]")
}

var secretKeys = map[string]bool{
	"authorization": true, "password": true, "token": true,
	"api_key": true, "dsn": true, "database_url": true,
}

func redact(_ []string, a slog.Attr) slog.Attr {
	if secretKeys[strings.ToLower(a.Key)] {
		return slog.String(a.Key, "[REDACTED]")
	}
	return a
}

// TraceHandler adds trace_id and span_id from the context.
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

// WithAttrs and WithGroup keep the wrapper; without them the
// first logger.With drops the trace IDs.
func (h TraceHandler) WithAttrs(
	as []slog.Attr) slog.Handler {
	return TraceHandler{h.Handler.WithAttrs(as)}
}

func (h TraceHandler) WithGroup(name string) slog.Handler {
	return TraceHandler{h.Handler.WithGroup(name)}
}
