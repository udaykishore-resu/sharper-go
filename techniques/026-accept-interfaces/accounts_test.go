package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeAccounts map[string]Account

func (f fakeAccounts) Account(_ context.Context,
	id string) (Account, error) {
	a, ok := f[id]
	if !ok {
		return a, ErrNotFound
	}
	return a, nil
}

func TestNotFoundFromFake(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /v1/accounts/{id}", GetAccount(fakeAccounts{"a1": {ID: "a1"}}))
	for path, want := range map[string]int{"/v1/accounts/a1": 200, "/v1/accounts/zz": 404} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != want {
			t.Errorf("%s: %d, want %d", path, rec.Code, want)
		}
	}
}

func TestTypedNil(t *testing.T) {
	if err := validateBad(true); err == nil {
		t.Fatal("expected the typed-nil trap to give a non-nil error")
	}
}
