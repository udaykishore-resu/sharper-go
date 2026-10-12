//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	pgtest "github.com/udaykishore-resu/sharper-go/techniques-thirdparty/068-testcontainers"

	"github.com/udaykishore-resu/sharper-go/techniques-thirdparty/057-tx-helper/ledger"
)

const schema = `
CREATE TABLE accounts (id text PRIMARY KEY, balance bigint NOT NULL);
CREATE TABLE transfers (id text PRIMARY KEY, amount bigint NOT NULL);
CREATE TABLE idem (key text PRIMARY KEY,
    state text NOT NULL DEFAULT 'done', response bytea);
CREATE TABLE outbox (id bigserial PRIMARY KEY, topic text NOT NULL,
    payload jsonb NOT NULL, sent_at timestamptz);
INSERT INTO accounts VALUES ('a1', 1000), ('a2', 0);`

func setup(t *testing.T) *Store {
	t.Helper()
	pool := pgtest.StartPG(t)
	if _, err := pool.Exec(t.Context(), schema); err != nil {
		t.Fatal(err)
	}
	return &Store{pool: pool}
}

func balance(t *testing.T, s *Store, id string) (b int64) {
	t.Helper()
	if err := s.pool.QueryRow(context.Background(),
		`SELECT balance FROM accounts WHERE id = $1`, id).Scan(&b); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestTransferOnceWithEvent(t *testing.T) {
	s := setup(t)
	tr := ledger.Transfer{ID: "t1", From: "a1", To: "a2", Amount: 300, Key: "k1"}
	first, err := s.Transfer(t.Context(), tr)
	if err != nil {
		t.Fatal(err)
	}
	tr.ID = "t1-retry"
	second, err := s.Transfer(t.Context(), tr)
	if err != nil || string(second) != string(first) {
		t.Fatalf("replay %s, %v", second, err)
	}
	var events int
	s.pool.QueryRow(t.Context(), `SELECT count(*) FROM outbox`).Scan(&events)
	if balance(t, s, "a2") != 300 || events != 1 {
		t.Fatalf("a2=%d events=%d", balance(t, s, "a2"), events)
	}
}

// A middleware claim (Technique 52) flips to done in the same tx.
func TestClaimedKeyFlips(t *testing.T) {
	s := setup(t)
	s.pool.Exec(t.Context(), `INSERT INTO idem (key, state) VALUES ('k2', 'inflight')`)
	_, err := s.Transfer(t.Context(), ledger.Transfer{ID: "t2", From: "a1", To: "a2", Amount: 1, Key: "k2"})
	var state string
	s.pool.QueryRow(t.Context(), `SELECT state FROM idem WHERE key = 'k2'`).Scan(&state)
	if err != nil || state != "done" || balance(t, s, "a2") != 1 {
		t.Fatalf("err=%v state=%s", err, state)
	}
}

// A failed transfer leaves no key, no row and no event behind.
func TestRollbackLeavesNothing(t *testing.T) {
	s := setup(t)
	_, err := s.Transfer(t.Context(), ledger.Transfer{ID: "t3", From: "a2", To: "a1", Amount: 5, Key: "k3"})
	if !errors.Is(err, ledger.ErrFunds) {
		t.Fatalf("err = %v", err)
	}
	var n int
	s.pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM idem) + (SELECT count(*) FROM outbox)`).Scan(&n)
	if n != 0 {
		t.Fatalf("%d rows survived a rollback", n)
	}
}

// A panic inside fn rolls back and re-panics; no idle-in-transaction.
func TestPanicRollsBack(t *testing.T) {
	s := setup(t)
	func() {
		defer func() { recover() }()
		s.InTx(t.Context(), func(tx pgx.Tx) error {
			tx.Exec(t.Context(), `UPDATE accounts SET balance = 0 WHERE id = 'a1'`)
			panic("boom")
		})
	}()
	if balance(t, s, "a1") != 1000 || s.pool.Stat().AcquiredConns() != 0 {
		t.Fatal("transaction leaked")
	}
}

// Racing keys: serializable plus RunTx's retries never overdraw.
func TestConcurrent(t *testing.T) {
	s := setup(t)
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			s.Transfer(context.Background(), ledger.Transfer{ID: fmt.Sprint("c", i),
				From: "a1", To: "a2", Amount: 100, Key: fmt.Sprint("ck", i)})
		})
	}
	wg.Wait()
	if a1, a2 := balance(t, s, "a1"), balance(t, s, "a2"); a1 < 0 || a1+a2 != 1000 {
		t.Fatalf("a1=%d a2=%d", a1, a2)
	}
}
