// Package store is Technique 55: pgx's native interface through
// pgxpool, with generic row helpers and typed errors.
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/udaykishore-resu/sharper-go/techniques-thirdparty/055-pgx/ledger"
)

type Store struct{ pool *pgxpool.Pool }

// Open sizes the pool from Technique 54's math and pings, so an
// unreachable database is a crash loop at startup.
func Open(ctx context.Context, dsn string, maxConns int32) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = maxConns
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Account(ctx context.Context,
	id string) (ledger.Account, error) {
	rows, _ := s.pool.Query(ctx,
		`SELECT id, balance FROM accounts
         WHERE id = $1`, id)
	a, err := pgx.CollectOneRow(rows,
		pgx.RowToStructByName[ledger.Account])
	if errors.Is(err, pgx.ErrNoRows) {
		return ledger.Account{}, ledger.ErrNotFound
	}
	return a, err
}

// Insert maps a reused transfer ID (SQLSTATE 23505) to a sentinel.
func (s *Store) Insert(ctx context.Context, id string, amount int64) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO transfers (id, amount) VALUES ($1, $2)`, id, amount)
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return ledger.ErrDuplicate
	}
	return err
}

// All shows CollectRows for lists.
func (s *Store) All(ctx context.Context) ([]ledger.Account, error) {
	rows, _ := s.pool.Query(ctx, `SELECT id, balance FROM accounts ORDER BY id`)
	return pgx.CollectRows(rows, pgx.RowToStructByName[ledger.Account])
}
