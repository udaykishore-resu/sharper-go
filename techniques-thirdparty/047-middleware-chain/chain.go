// Package httpapi is Technique 47: middleware as plain functions,
// composed in one order. Layers that copy the request run outside
// the layers that read r.Pattern.
package httpapi

import (
	"cmp"
	"context"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type Middleware func(http.Handler) http.Handler

// Chain applies mws so the first one runs outermost.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// build is the book's chain; v is the auth verifier, m the
// metrics wrapper, log the access logger.
func build(routes http.Handler, v Verifier, m Wrapper,
	log *slog.Logger) http.Handler {
	h := Chain(routes,
		RequestID,                         // copies r
		Auth(v),                           // copies r
		Deadline(10*time.Second),          // copies r
		otelhttp.NewMiddleware("ledgerd"), // copies r
		AccessLog(log),                    // reads r.Pattern
		m.Wrap,                            // Hack 88
		Recover(log),                      // Hack 90
		LimitBody(1<<20),                  // Hack 48
	)
	return h
}

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
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

type ctxKey int

const (
	requestIDKey ctxKey = iota
	principalKey
)

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), requestIDKey, "req-1")
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type Verifier func(token string) (string, bool)

func Auth(v Verifier) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			who, ok := v(r.Header.Get("Authorization"))
			if !ok {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return // every early exit needs a return
			}
			ctx := context.WithValue(r.Context(), principalKey, who)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func Deadline(d time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func AccessLog(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sw := &statusWriter{ResponseWriter: w}
			next.ServeHTTP(sw, r)
			log.InfoContext(r.Context(), "request",
				"route", cmp.Or(r.Pattern, "unmatched"),
				"status", cmp.Or(sw.status, 200))
		})
	}
}

type Wrapper interface {
	Wrap(http.Handler) http.Handler
}

// Recover is Hack 90's layer. Before the first write it answers
// 500, which the access log and metrics outside it record. After
// it, headers are gone: it aborts with http.ErrAbortHandler so a
// half body can't pass as success. An ErrAbortHandler panic from
// the handler passes through untouched.
func Recover(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sw := &statusWriter{ResponseWriter: w}
			defer func() {
				v := recover()
				if v == nil {
					return
				}
				if v == http.ErrAbortHandler {
					panic(v)
				}
				log.ErrorContext(r.Context(), "panic",
					"route", r.Pattern, "panic", v)
				if sw.status != 0 {
					panic(http.ErrAbortHandler)
				}
				http.Error(sw, "internal error", 500)
			}()
			next.ServeHTTP(sw, r)
		})
	}
}

func LimitBody(n int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, n)
			next.ServeHTTP(w, r)
		})
	}
}
