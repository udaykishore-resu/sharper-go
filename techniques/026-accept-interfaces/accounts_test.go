package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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
	err := validateBad(true)
	if err == nil {
		t.Fatal("expected the typed-nil trap to give a non-nil error")
	}
	// fmt (and slog's text handler, which formats with fmt) prints
	// the nil pointer inside the interface as <nil>.
	if got := fmt.Sprintf("%v", err); got != "<nil>" {
		t.Fatalf("logged as %q", got)
	}
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Error("transfer", "err", err)
	if !strings.Contains(buf.String(), "err=<nil>") {
		t.Fatalf("slog line: %q", buf.String())
	}
	if validateGood(true) != nil || validateGood(false) == nil {
		t.Fatal("the literal-nil version must be nil only on success")
	}
}
