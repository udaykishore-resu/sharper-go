package idem

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func send(h http.Handler, key, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/v1/transfers", strings.NewReader(body))
	req.Header.Set("Idempotency-Key", key)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRetryMovesMoneyOnce(t *testing.T) {
	var moved atomic.Int32
	fail := atomic.Bool{}
	h := Middleware(NewStore(), func(*http.Request) string { return "acme" },
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if fail.Load() {
				http.Error(w, "db down", 500)
				return
			}
			moved.Add(1)
			w.WriteHeader(201)
			w.Write([]byte(`{"id":"t1"}`))
		}))
	first := send(h, "k1", `{"amount":500}`)
	second := send(h, "k1", `{"amount":500}`)
	if first.Code != 201 || second.Code != 201 ||
		second.Header().Get("Idempotent-Replayed") != "true" || moved.Load() != 1 {
		t.Fatalf("first %d, second %d %v, moved %d",
			first.Code, second.Code, second.Header(), moved.Load())
	}
	if got := send(h, "k1", `{"amount":9}`); got.Code != 422 {
		t.Fatalf("same key, other body: %d", got.Code)
	}
	fail.Store(true)
	if got := send(h, "k2", `{"amount":5}`); got.Code != 500 {
		t.Fatalf("want 500, got %d", got.Code)
	}
	fail.Store(false)
	if got := send(h, "k2", `{"amount":5}`); got.Code != 201 {
		t.Fatalf("a 5xx must release the key, got %d", got.Code)
	}
}

// The handler gets the same bytes the middleware hashed, for an
// empty body too.
func TestBodyRestored(t *testing.T) {
	for _, body := range []string{"", `{"amount":500}`} {
		var got string
		h := Middleware(NewStore(), func(*http.Request) string { return "acme" },
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				got = string(b)
			}))
		send(h, "k", body)
		if got != body {
			t.Errorf("handler got %q, want %q", got, body)
		}
	}
}

// A handler that never calls WriteHeader is saved as a 200, not 0.
func TestImplicit200Saved(t *testing.T) {
	for name, fn := range map[string]http.HandlerFunc{
		"bare write": func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) },
		"no write":   func(http.ResponseWriter, *http.Request) {},
	} {
		h := Middleware(NewStore(), func(*http.Request) string { return "acme" }, fn)
		send(h, "k", "{}")
		if got := send(h, "k", "{}"); got.Code != 200 ||
			got.Header().Get("Idempotent-Replayed") != "true" {
			t.Errorf("%s: replay %d", name, got.Code)
		}
	}
}

// Not final, so not saved: a response too big to keep, and a
// handler whose context ended (the client hung up).
func TestNotFinalIsReleased(t *testing.T) {
	var runs int
	big := Middleware(NewStore(), func(*http.Request) string { return "acme" },
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			runs++
			w.WriteHeader(422)
			w.Write(make([]byte, maxSaved+1))
		}))
	send(big, "k", "{}")
	send(big, "k", "{}")
	if runs != 2 {
		t.Fatalf("oversize response replayed: %d runs", runs)
	}
	runs = 0
	gone := Middleware(NewStore(), func(*http.Request) string { return "acme" },
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			runs++
			http.Error(w, "client closed", 499)
		}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest("POST", "/", strings.NewReader("{}")).WithContext(ctx)
	req.Header.Set("Idempotency-Key", "k")
	gone.ServeHTTP(httptest.NewRecorder(), req)
	send(gone, "k", "{}")
	if runs != 2 {
		t.Fatalf("a cancelled request's 499 was replayed: %d runs", runs)
	}
}

// A claim whose holder died is refused until its lease ends, then
// taken over.
func TestLeaseTakeover(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		st := NewStore()
		h := sha256.Sum256([]byte("{}"))
		st.Begin(t.Context(), "acme", "k", h) // ...and dies
		if _, err := st.Begin(t.Context(), "acme", "k", h); !errors.Is(err, ErrInFlight) {
			t.Fatalf("before the lease ends: %v", err)
		}
		time.Sleep(Lease + time.Second)
		if s, err := st.Begin(t.Context(), "acme", "k", h); s != nil || err != nil {
			t.Fatalf("takeover: %v %v", s, err)
		}
	})
}
