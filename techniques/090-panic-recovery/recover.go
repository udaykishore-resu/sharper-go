// Package httpapi is Technique 90: panic-recovery middleware that
// logs the stack as one JSON field.
package httpapi

import (
	"expvar"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
)

type Middleware func(http.Handler) http.Handler

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(c int) {
	if w.status == 0 {
		w.status = c
	}
	w.ResponseWriter.WriteHeader(c)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the real writer.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// panics stands in for the Prometheus counter of Technique 88.
var panics = expvar.NewInt("panics")

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
	panics.Add(1)
	if sw.status == 0 {
		http.Error(sw, "internal error",
			http.StatusInternalServerError)
		return
	}
	panic(http.ErrAbortHandler)
}
