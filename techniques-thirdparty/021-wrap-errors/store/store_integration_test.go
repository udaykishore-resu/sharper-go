//go:build integration

package store

import (
	"errors"
	"testing"

	"github.com/udaykishore-resu/sharper-go/techniques-thirdparty/021-wrap-errors/ledger"
	pgtest "github.com/udaykishore-resu/sharper-go/techniques-thirdparty/068-testcontainers"
)

func TestAccountNotFound(t *testing.T) {
	pool := pgtest.StartPG(t)
	_, err := pool.Exec(t.Context(),
		`CREATE TABLE accounts (id text PRIMARY KEY, balance bigint NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = New(pool).Account(t.Context(), "acc_19")
	if !errors.Is(err, ledger.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}
