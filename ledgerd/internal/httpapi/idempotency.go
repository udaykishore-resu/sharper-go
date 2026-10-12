package httpapi

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"example.com/ledgerd/internal/ledger"
)

// Saved is a recorded response. The store returns it, so the
// type lives in package ledger.
type Saved = ledger.Saved

var (
	ErrInFlight = ledger.ErrInFlight
	ErrMismatch = ledger.ErrMismatch
)

// IdemStore has the three verbs of Technique 52.
type IdemStore interface {
	Begin(ctx context.Context, client, key string,
		hash [32]byte) (*Saved, error)
	Finish(ctx context.Context, client, key string, s Saved) error
	Release(ctx context.Context, client, key string) error
}

// recorder is statusWriter plus a copy of the body.
type recorder struct {
	statusWriter
	buf bytes.Buffer
}

func (r *recorder) Write(b []byte) (int, error) {
	r.buf.Write(b)
	return r.statusWriter.Write(b)
}

// idem replays a finished key, refuses one in flight, and never
// saves a success itself: store.Transfer marks the key done
// inside the transfer's own transaction (Technique 57).
func (a *API) idem(next http.Handler) http.Handler {
	st := a.st
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		if key == "" || len(key) > 200 {
			http.Error(w, "Idempotency-Key header required", http.StatusBadRequest)
			return
		}
		client := Principal(r.Context())
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		saved, err := st.Begin(r.Context(), client, key,
			sha256.Sum256(body))
		switch {
		case errors.Is(err, ErrInFlight):
			w.Header().Set("Retry-After", "1")
			http.Error(w, "in progress", http.StatusConflict)
		case errors.Is(err, ErrMismatch):
			http.Error(w, "key reused", http.StatusUnprocessableEntity)
		case err != nil:
			a.log.ErrorContext(r.Context(), "idempotency begin", "err", err)
			http.Error(w, "store down", http.StatusServiceUnavailable)
		case saved != nil:
			w.Header().Set("Content-Type", contentType(saved.Body))
			w.Header().Set("Idempotent-Replayed", "true")
			w.WriteHeader(saved.Status)
			w.Write(saved.Body)
		}
		if err != nil || saved != nil {
			return
		}
		rec := &recorder{statusWriter: statusWriter{ResponseWriter: w}}
		next.ServeHTTP(rec, r)
		ctx := context.WithoutCancel(r.Context())
		if cmp.Or(rec.status, 200) >= 500 {
			if err := st.Release(ctx, client, key); err != nil {
				a.log.ErrorContext(ctx, "idempotency release", "err", err)
			}
			return
		}
		err = st.Finish(ctx, client, key,
			Saved{Status: cmp.Or(rec.status, 200), Body: rec.buf.Bytes()})
		if err != nil {
			a.log.ErrorContext(ctx, "idempotency finish", "err", err)
		}
	})
}

func contentType(body []byte) string {
	if json.Valid(body) {
		return "application/json"
	}
	return "text/plain; charset=utf-8"
}
