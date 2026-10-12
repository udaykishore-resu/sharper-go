//go:build integration

package store

import (
	"errors"
	"testing"

	pgtest "github.com/udaykishore-resu/sharper-go/techniques-thirdparty/068-testcontainers"

	"github.com/udaykishore-resu/sharper-go/techniques-thirdparty/055-pgx/ledger"
)

func TestStore(t *testing.T) {
	pool := pgtest.StartPG(t)
	ctx := t.Context()
	_, err := pool.Exec(ctx, `
        CREATE TABLE accounts (id text PRIMARY KEY, balance bigint NOT NULL);
        CREATE TABLE transfers (id text PRIMARY KEY, amount bigint NOT NULL);
        INSERT INTO accounts VALUES ('a1', 500), ('a2', 0);`)
	if err != nil {
		t.Fatal(err)
	}
	s := &Store{pool: pool}
	if a, err := s.Account(ctx, "a1"); err != nil || a.Balance != 500 {
		t.Fatalf("a1 = %+v, %v", a, err)
	}
	if _, err := s.Account(ctx, "zz"); !errors.Is(err, ledger.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if all, err := s.All(ctx); err != nil || len(all) != 2 {
		t.Fatalf("All = %v, %v", all, err)
	}
	if err := s.Insert(ctx, "t1", 5); err != nil {
		t.Fatal(err)
	}
	if err := s.Insert(ctx, "t1", 5); !errors.Is(err, ledger.ErrDuplicate) {
		t.Fatalf("reused id: %v", err)
	}
}
