// Package idem is Technique 52: idempotency middleware for POST.
// The store here is in memory; ledgerd's lives in Postgres and
// marks the key done inside the transfer's own transaction.
package idem

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"
)

var (
	ErrInFlight = errors.New("request in flight")
	ErrMismatch = errors.New("key reused with other body")
)

// Lease is how long a claim lives before a retry may take it
// over; claimBudget ends the holder's own use of it 5 s earlier.
const (
	Lease       = 30 * time.Second
	claimBudget = 25 * time.Second
	maxSaved    = 64 << 10 // largest response kept for replay
)

type Saved struct {
	Status int
	Body   []byte
}

type row struct {
	hash  [32]byte
	done  bool
	until time.Time // lease end while in flight
	saved Saved
}

// Store has the three verbs: Begin, Finish and Release.
type Store struct {
	mu   sync.Mutex
	rows map[[2]string]*row
}

func NewStore() *Store { return &Store{rows: map[[2]string]*row{}} }

func (s *Store) Begin(_ context.Context, client, key string,
	hash [32]byte) (*Saved, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := [2]string{client, key} // never the bare key
	r, ok := s.rows[k]
	now := time.Now()
	switch {
	case !ok:
		s.rows[k] = &row{hash: hash, until: now.Add(Lease)}
		return nil, nil // we own it
	case r.hash != hash:
		return nil, ErrMismatch
	case !r.done && now.Before(r.until):
		return nil, ErrInFlight
	case !r.done: // the holder died: take the claim over
		r.until = now.Add(Lease)
		return nil, nil
	}
	return &r.saved, nil
}

// Finish and Release touch a row only while it is in flight.
func (s *Store) Finish(_ context.Context, client, key string, sv Saved) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r := s.rows[[2]string{client, key}]; r != nil && !r.done {
		r.done, r.saved = true, sv
	}
}

func (s *Store) Release(_ context.Context, client, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := [2]string{client, key}
	if r := s.rows[k]; r != nil && !r.done {
		delete(s.rows, k)
	}
}

// statusWriter is Technique 47's: the first status wins, and a
// bare Write counts as 200.
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

// recorder keeps a copy of the body, up to maxSaved bytes.
type recorder struct {
	statusWriter
	buf      bytes.Buffer
	overflow bool
}

func (r *recorder) Write(b []byte) (int, error) {
	if r.buf.Len()+len(b) > maxSaved {
		r.overflow = true
	} else {
		r.buf.Write(b)
	}
	return r.statusWriter.Write(b)
}

// Middleware wraps POST /v1/transfers only. client names the
// caller, from the auth middleware in ledgerd.
func Middleware(st *Store, client func(*http.Request) string,
	next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			http.Error(w, "missing key", 400)
			return
		}
		client := client(r)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "bad body", 400)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		until := time.Now().Add(claimBudget)
		saved, err := st.Begin(r.Context(), client, key,
			sha256.Sum256(body))
		switch {
		case errors.Is(err, ErrInFlight):
			w.Header().Set("Retry-After", "1")
			http.Error(w, "in progress", 409)
		case errors.Is(err, ErrMismatch):
			http.Error(w, "key reused", 422)
		case err != nil:
			http.Error(w, "store down", 503)
		case saved != nil:
			w.Header().Set("Idempotent-Replayed", "true")
			w.WriteHeader(saved.Status)
			w.Write(saved.Body)
		}
		if err != nil || saved != nil {
			return
		}
		rec := &recorder{statusWriter: statusWriter{ResponseWriter: w}}
		hctx, cancel := context.WithDeadline(r.Context(), until)
		defer cancel()
		next.ServeHTTP(rec, r.WithContext(hctx))
		status := cmp.Or(rec.status, 200) // wrote nothing
		ctx, stop := context.WithDeadline(
			context.WithoutCancel(r.Context()), until)
		defer stop()
		if status >= 500 || rec.overflow || hctx.Err() != nil {
			st.Release(ctx, client, key)
			return
		}
		st.Finish(ctx, client, key,
			Saved{status, rec.buf.Bytes()})
	})
}
