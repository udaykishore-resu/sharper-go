package obs

import (
	"context"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// SetupTracing installs the global tracer provider and the W3C
// propagator (Technique 87). The exporter reads
// OTEL_EXPORTER_OTLP_ENDPOINT; sampling comes from
// OTEL_TRACES_SAMPLER, so ops owns it.
//
// With no endpoint configured, ledgerd still propagates
// traceparent but exports nothing, so a laptop run does not log
// export errors. The returned func flushes spans on shutdown.
func SetupTracing(ctx context.Context, ver string) (
	func(context.Context) error, error,
) {
	otel.SetTextMapPropagator(propagation.TraceContext{})
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" &&
		os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") == "" {
		return func(context.Context) error { return nil }, nil
	}
	exp, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}
	res := resource.NewWithAttributes(semconv.SchemaURL,
		semconv.ServiceName("ledgerd"),
		semconv.ServiceVersion(ver))
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res))
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}
