package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/ledgerd/internal/store"
)

// idemOnly wraps h in the idempotency middleware over a fake store.
func idemOnly(f *fakeStore, h http.HandlerFunc) http.Handler {
	a := New(f, slog.New(slog.DiscardHandler))
	return a.idem(h)
}

func post(h http.Handler, key, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/v1/transfers", strings.NewReader(body))
	req.Header.Set("Idempotency-Key", key)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// The middleware reads the body to hash it and puts the same
// bytes back, for empty and non-empty bodies.
func TestIdemRestoresBody(t *testing.T) {
	for _, body := range []string{"", `{"from":"a1","to":"a2","amount":5}`} {
		var got string
		h := idemOnly(newFake(), func(w http.ResponseWriter, r *http.Request) {
			b, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			got = string(b)
		})
		post(h, "k", body)
		if got != body {
			t.Errorf("handler got %q, want %q", got, body)
		}
	}
}

// A handler that writes a body without WriteHeader, or nothing at
// all, is saved and replayed as a 200.
func TestIdemImplicit200(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"bare write": func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) },
		"no write":   func(http.ResponseWriter, *http.Request) {},
	} {
		f := newFake()
		mw := idemOnly(f, h)
		post(mw, "k", "{}")
		replay := post(mw, "k", "{}")
		if replay.Code != 200 || replay.Header().Get("Idempotent-Replayed") != "true" {
			t.Errorf("%s: replay %d %v", name, replay.Code, replay.Header())
		}
	}
}

// A response too big to keep is not saved; the claim is released
// instead, so the retry runs the handler again.
func TestIdemOversizeResponseReleases(t *testing.T) {
	runs := 0
	f := newFake()
	mw := idemOnly(f, func(w http.ResponseWriter, _ *http.Request) {
		runs++
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write(make([]byte, MaxSaved+1))
	})
	first := post(mw, "k", "{}")
	if first.Body.Len() != MaxSaved+1 {
		t.Fatalf("client got %d bytes", first.Body.Len())
	}
	post(mw, "k", "{}")
	if runs != 2 {
		t.Fatalf("handler ran %d times, want 2 (not replayed)", runs)
	}
}

// The handler's context ends at the claim budget, which ends
// before the store's lease; WriteTimeout ends earlier still.
func TestIdemClaimBudgetInsideLease(t *testing.T) {
	if ClaimBudget >= store.Lease ||
		NewServer(":0", nil).WriteTimeout >= ClaimBudget {
		t.Fatalf("WriteTimeout %v, budget %v, lease %v",
			NewServer(":0", nil).WriteTimeout, ClaimBudget, store.Lease)
	}
	var left time.Duration
	mw := idemOnly(newFake(), func(_ http.ResponseWriter, r *http.Request) {
		dl, ok := r.Context().Deadline()
		if ok {
			left = time.Until(dl)
		}
	})
	post(mw, "k", "{}")
	if left <= 0 || left > ClaimBudget {
		t.Fatalf("handler deadline in %v, want within %v", left, ClaimBudget)
	}
}
