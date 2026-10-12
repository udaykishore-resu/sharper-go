package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
)

// Go is the helper the Breaks when list asks for: background work
// started by a handler recovers and logs its own panics, because
// Recover only covers the handler's goroutine. WithoutCancel keeps
// the request's values, such as the trace ID, and drops its
// cancellation, so the work outlives the response.
func Go(ctx context.Context, log *slog.Logger,
	fn func(context.Context)) {
	ctx = context.WithoutCancel(ctx)
	go func() {
		defer func() {
			if v := recover(); v != nil {
				log.ErrorContext(ctx, "panic in background",
					"panic", fmt.Sprint(v),
					"stack", string(debug.Stack()))
				panics.Add(1)
			}
		}()
		fn(ctx)
	}()
}
