package idem

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
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
