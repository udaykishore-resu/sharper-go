// Package idem is Technique 52: idempotency middleware for POST.
// The store here is in memory; ledgerd's lives in Postgres and
// marks the key done inside the transfer's own transaction.
package idem

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"net/http"
	"sync"
)

var (
	ErrInFlight = errors.New("request in flight")
	ErrMismatch = errors.New("key reused with other body")
)

type Saved struct {
	Status int
	Body   []byte
}

type row struct {
	hash  [32]byte
	done  bool
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
	switch {
	case !ok:
		s.rows[k] = &row{hash: hash}
		return nil, nil // we own it
	case r.hash != hash:
		return nil, ErrMismatch
	case !r.done:
		return nil, ErrInFlight
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

type recorder struct {
	statusWriter
	buf bytes.Buffer
}

func (r *recorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = 200
	}
	r.buf.Write(b)
	return r.ResponseWriter.Write(b)
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
		next.ServeHTTP(rec, r)
		ctx := context.WithoutCancel(r.Context())
		if rec.status >= 500 {
			st.Release(ctx, client, key)
			return
		}
		st.Finish(ctx, client, key,
			Saved{rec.status, rec.buf.Bytes()})
	})
}
