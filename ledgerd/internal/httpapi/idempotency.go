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
	"time"

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

// ClaimBudget is how long one request may use its claim. It
// ends 5 s before the store's 30 s lease (store.Lease), so a
// takeover can only meet a stale holder at commit, where the
// row lock in store.Transfer serialises them.
const ClaimBudget = 25 * time.Second

// MaxSaved bounds the response copy kept for replay. A bigger
// response is not saved; the claim is released instead, which
// is safe because Release never touches a committed key.
const MaxSaved = 64 << 10

// recorder is statusWriter plus a bounded copy of the body. The
// embedded statusWriter turns a bare Write into a 200.
type recorder struct {
	statusWriter
	buf      bytes.Buffer
	overflow bool
}

func (r *recorder) Write(b []byte) (int, error) {
	if r.buf.Len()+len(b) > MaxSaved {
		r.overflow = true
	} else {
		r.buf.Write(b)
	}
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
		until := time.Now().Add(ClaimBudget) // before the claim
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
			_, _ = w.Write(saved.Body) //nolint:gosec // G705: replays our own stored JSON
		}
		if err != nil || saved != nil {
			return
		}
		rec := &recorder{statusWriter: statusWriter{ResponseWriter: w}}
		hctx, cancel := context.WithDeadline(r.Context(), until)
		defer cancel()
		next.ServeHTTP(rec, r.WithContext(hctx))
		status := cmp.Or(rec.status, http.StatusOK) // wrote nothing
		// The client may have hung up; it is the one that retries.
		ctx, cancel2 := context.WithDeadline(
			context.WithoutCancel(r.Context()), until)
		defer cancel2()
		// Save only a final answer. A 5xx, a response too big to
		// keep, or a handler whose context ended (client gone,
		// shutdown, budget) is not one: release, so the retry runs
		// again. Release never touches a key a transfer committed.
		if status >= 500 || rec.overflow || hctx.Err() != nil {
			if err := st.Release(ctx, client, key); err != nil {
				a.log.ErrorContext(ctx, "idempotency release", "err", err)
			}
			return
		}
		err = st.Finish(ctx, client, key,
			Saved{Status: status, Body: rec.buf.Bytes()})
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
