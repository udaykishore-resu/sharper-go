package httpapi

import (
	"cmp"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Middleware takes a handler and returns a handler (Technique 47).
type Middleware func(http.Handler) http.Handler

// Chain applies mws so the first one runs outermost.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// statusWriter records the first status code and counts bytes.
// Unwrap lets http.ResponseController find the real writer.
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

type requestIDKey struct{}

// RequestID copies r: it adds an ID to the context and the
// response, reusing a sane X-Request-ID from the caller.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" || len(id) > 64 || strings.ContainsFunc(id, notToken) {
			id = rand.Text()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func notToken(c rune) bool {
	return c <= ' ' || c > '~'
}

// RequestIDFrom returns the request's ID, or "".
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

type principalKey struct{}

// Principal is the client the auth middleware identified.
func Principal(ctx context.Context) string {
	p, _ := ctx.Value(principalKey{}).(string)
	return p
}

// Auth copies r: it maps a bearer token to a client and stores
// it in the context. A rejection never reaches the access log,
// so Auth logs its own, with the request ID.
func Auth(keys map[string]string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			client := ""
			for k, c := range keys {
				if subtle.ConstantTimeCompare([]byte(k), []byte(tok)) == 1 {
					client = c
				}
			}
			if !ok || client == "" {
				slog.WarnContext(r.Context(), "auth rejected",
					"request_id", RequestIDFrom(r.Context()),
					"path", r.URL.Path)
				w.Header().Set("WWW-Authenticate", `Bearer realm="ledgerd"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), principalKey{}, client)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

var errBudget = errors.New("request budget exceeded")

// Deadline copies r: it bounds the work, which the server's
// WriteTimeout never does (Technique 44). The cause says which
// timer fired (Technique 35).
func Deadline(d time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeoutCause(r.Context(), d, errBudget)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AccessLog passes r through, so after the mux runs it reads
// r.Pattern from the same request.
func AccessLog(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sw := &statusWriter{ResponseWriter: w}
			start := time.Now()
			next.ServeHTTP(sw, r)
			log.LogAttrs(r.Context(), slog.LevelInfo, "request",
				slog.String("route", cmp.Or(r.Pattern, "unmatched")),
				slog.Int("status", cmp.Or(sw.status, http.StatusOK)),
				slog.Int("bytes", sw.bytes),
				slog.Int64("dur_ms", time.Since(start).Milliseconds()),
				slog.String("client", Principal(r.Context())),
				slog.String("request_id", RequestIDFrom(r.Context())))
		})
	}
}

// LimitBody caps every body (Technique 48). MaxBytesReader
// replaces r.Body on the same request, so nothing is copied.
func LimitBody(n int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, n)
			next.ServeHTTP(w, r)
		})
	}
}
