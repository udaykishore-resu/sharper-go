package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"example.com/ledgerd/internal/obs"
)

// panics is the counter from Technique 88's registry.
var panics = obs.Panics

// Recover turns a panic into one JSON log line and one count on
// obs.Panics (Technique 90). Before the first write it answers
// 500, which the access log and metrics outside it record. After
// the first write it aborts with http.ErrAbortHandler, so only
// the panic counter sees that request. An ErrAbortHandler panic
// from the handler passes through untouched.
func Recover(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			sw := &statusWriter{ResponseWriter: w}
			defer func() {
				v := recover()
				if v == nil {
					return
				}
				if v == http.ErrAbortHandler {
					panic(v)
				}
				handlePanic(log, sw, r, v)
			}()
			next.ServeHTTP(sw, r)
		}
		return http.HandlerFunc(fn)
	}
}

func handlePanic(log *slog.Logger, sw *statusWriter,
	r *http.Request, v any) {
	log.ErrorContext(r.Context(), "panic",
		"route", r.Pattern,
		"panic", fmt.Sprint(v),
		"stack", string(debug.Stack()))
	panics.Inc()
	if sw.status == 0 {
		http.Error(sw, "internal error",
			http.StatusInternalServerError)
		return
	}
	// Headers are gone; cut the connection so a half body
	// can't pass as success.
	panic(http.ErrAbortHandler)
}
