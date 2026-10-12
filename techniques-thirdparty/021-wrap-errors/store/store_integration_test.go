//go:build integration

package store

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/udaykishore-resu/sharper-go/techniques-thirdparty/021-wrap-errors/ledger"
	pgtest "github.com/udaykishore-resu/sharper-go/techniques-thirdparty/068-testcontainers"
)

func setup(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := pgtest.StartPG(t)
	_, err := pool.Exec(t.Context(), `
		CREATE TABLE accounts (id text PRIMARY KEY, balance bigint NOT NULL);
		INSERT INTO accounts VALUES ('acc_1', 500)`)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}

func TestAccountFound(t *testing.T) {
	a, err := New(setup(t)).Account(t.Context(), "acc_1")
	if err != nil || a.ID != "acc_1" || a.Balance != 500 {
		t.Fatalf("got %+v, %v", a, err)
	}
}

func TestAccountNotFound(t *testing.T) {
	_, err := New(setup(t)).Account(t.Context(), "acc_19")
	if !errors.Is(err, ledger.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if err.Error() != "account acc_19: ledger: not found" {
		t.Fatalf("message = %q", err)
	}
}

// getAccount is the handler check from the text: match the
// sentinel, return 404; anything else is a 500.
func getAccount(s *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, err := s.Account(r.Context(), r.PathValue("id"))
		switch {
		case errors.Is(err, ledger.ErrNotFound):
			http.Error(w, "not found", http.StatusNotFound)
		case err != nil:
			http.Error(w, "internal error", 500)
		}
	}
}

func TestHandlerMapsNotFoundTo404(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /v1/accounts/{id}", getAccount(New(setup(t))))
	for path, want := range map[string]int{
		"/v1/accounts/acc_1":  200,
		"/v1/accounts/acc_19": 404,
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != want {
			t.Errorf("%s: %d, want %d", path, rec.Code, want)
		}
	}
}

// A driver error is cut with %v: callers can no longer match the
// *pgconn.PgError, but the text (with SQLSTATE) still reaches the
// 5xx log, so diagnosis survives the cut.
func TestDriverErrorIsCutButLogged(t *testing.T) {
	pool := setup(t)
	if _, err := pool.Exec(t.Context(), `DROP TABLE accounts`); err != nil {
		t.Fatal(err)
	}
	_, err := New(pool).Account(t.Context(), "acc_1")
	if err == nil || errors.Is(err, ledger.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if _, ok := errors.AsType[*pgconn.PgError](err); ok {
		t.Fatal("the v verb should cut the driver error out of the chain")
	}
	if !strings.Contains(err.Error(), "42P01") {
		t.Fatalf("message lost the SQLSTATE: %q", err)
	}
}

// Technique 24's Breaks when: pgx's own timeout error wraps
// context.DeadlineExceeded, and the %v cut in Account loses it.
func TestTimeoutChain(t *testing.T) {
	pool := setup(t)
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	_, err := pool.Exec(ctx, `SELECT pg_sleep(2)`)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("pgx timeout does not wrap DeadlineExceeded: %v", err)
	}
	ctx2, cancel2 := context.WithTimeout(t.Context(), time.Nanosecond)
	defer cancel2()
	<-ctx2.Done()
	_, err = New(pool).Account(ctx2, "acc_1")
	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected the cut chain, got %v", err)
	}
}
