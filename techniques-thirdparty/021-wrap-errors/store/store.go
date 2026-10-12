// Package store is Technique 21: wrap with %w what callers may
// match; cut driver errors with %v at the package edge.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/udaykishore-resu/sharper-go/techniques-thirdparty/021-wrap-errors/ledger"
)

type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) Account(ctx context.Context,
	id string) (ledger.Account, error) {
	a := ledger.Account{ID: id}
	err := s.pool.QueryRow(ctx,
		`SELECT balance FROM accounts WHERE id = $1`,
		id).Scan(&a.Balance)
	if errors.Is(err, pgx.ErrNoRows) {
		return ledger.Account{}, fmt.Errorf(
			"account %s: %w", id, ledger.ErrNotFound)
	}
	if err != nil {
		return ledger.Account{}, fmt.Errorf(
			"account %s: %v", id, err)
	}
	return a, nil
}
