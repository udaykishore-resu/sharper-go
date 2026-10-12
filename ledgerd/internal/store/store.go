// Package store is ledgerd's Postgres layer: pgx v5 through
// pgxpool (Technique 55), one transaction helper (Technique 57),
// idempotency keys (Technique 52), the outbox (Technique 58) and
// embedded migrations (Technique 61). No other package calls
// Begin.
package store

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"example.com/ledgerd/internal/ledger"
)

// Store owns the pool. Build it with Open.
type Store struct {
	pool     *pgxpool.Pool
	schemaOK atomic.Bool // CheckSchema has passed once
}

// Option configures Open (Technique 25).
type Option func(*pgxpool.Config)

// WithMaxConns sizes the pool. The default of 12 comes from
// Technique 54's math: Postgres max_connections minus admin
// headroom, divided by the highest replica count the autoscaler
// allows. 200 - 30 = 170 over 12 replicas is 14, so 12 is safe.
func WithMaxConns(n int32) Option {
	return func(c *pgxpool.Config) { c.MaxConns = n }
}

// Open parses the DSN, sizes the pool and pings, so an
// unreachable database is a crash loop at startup, not a failed
// request later (Technique 55).
func Open(ctx context.Context, dsn string, opts ...Option) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		// Not %w: the parse error can quote the DSN.
		return nil, errors.New("store: bad DATABASE_URL")
	}
	cfg.MaxConns = 12
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.MaxConns < 1 {
		return nil, errors.New("store: MaxConns < 1")
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		//nolint:errorlint // keep the driver out of the chain (Technique 21)
		return nil, fmt.Errorf("store: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		//nolint:errorlint // keep the driver out of the chain (Technique 21)
		return nil, fmt.Errorf("store: ping: %v", err)
	}
	return &Store{pool: pool}, nil
}

// Close closes the pool. Call it after the HTTP servers drain.
func (s *Store) Close() { s.pool.Close() }

// Ping backs /readyz. Until CheckSchema has passed once, it also
// fails while the database is behind this binary's newest
// embedded migration (Technique 61); after that it is a plain
// ping, since a schema only moves forward.
func (s *Store) Ping(ctx context.Context) error {
	if err := s.pool.Ping(ctx); err != nil {
		return err
	}
	if s.schemaOK.Load() {
		return nil
	}
	if err := s.CheckSchema(ctx); err != nil {
		return err
	}
	s.schemaOK.Store(true)
	return nil
}

// AcquiredConns feeds the pool gauge (Technique 88).
func (s *Store) AcquiredConns() int32 { return s.pool.Stat().AcquiredConns() }

// Account reads one account (Technique 55).
func (s *Store) Account(ctx context.Context,
	id string) (ledger.Account, error) {
	rows, _ := s.pool.Query(ctx,
		`SELECT id, balance FROM accounts
         WHERE id = $1`, id)
	a, err := pgx.CollectOneRow(rows,
		pgx.RowToStructByName[ledger.Account])
	if errors.Is(err, pgx.ErrNoRows) {
		return ledger.Account{}, fmt.Errorf(
			"account %s: %w", id, ledger.ErrNotFound)
	}
	if err != nil {
		return ledger.Account{}, translate(ctx, fmt.Errorf("account %s", id), err)
	}
	return a, nil
}

// Entries lists an account's latest transfers, newest first.
func (s *Store) Entries(ctx context.Context, id string,
	limit int) ([]ledger.Transfer, error) {
	rows, _ := s.pool.Query(ctx, `
        SELECT id, from_acct, to_acct, amount
        FROM transfers
        WHERE from_acct = $1 OR to_acct = $1
        ORDER BY created_at DESC, id
        LIMIT $2`, id, limit)
	ts, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (ledger.Transfer, error) {
		var t ledger.Transfer
		err := r.Scan(&t.ID, &t.From, &t.To, &t.Amount)
		return t, err
	})
	if err != nil {
		return nil, translate(ctx, fmt.Errorf("entries %s", id), err)
	}
	return ts, nil
}

// translate keeps context errors matchable and cuts the driver
// out of the chain (Techniques 21 and 24). The store checks
// ctx.Err() first, so a timeout maps to 503, not 500.
func translate(ctx context.Context, op error, err error) error {
	if cerr := ctx.Err(); cerr != nil {
		return fmt.Errorf("%s: %w", op.Error(), cerr)
	}
	for _, sentinel := range domainErrors {
		if errors.Is(err, sentinel) {
			return fmt.Errorf("%s: %w", op.Error(), err)
		}
	}
	if retryable(err) {
		return fmt.Errorf("%s: %w", op.Error(), ledger.ErrBusy)
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" &&
		pe.ConstraintName == "transfers_pkey" {
		return fmt.Errorf("%s: %w", op.Error(), ledger.ErrDuplicate)
	}
	// %v, not %w: the driver error is logged, never matched.
	return fmt.Errorf("%s: %s", op.Error(), err.Error())
}

var domainErrors = []error{
	ledger.ErrNotFound, ledger.ErrFunds, ledger.ErrFrozen,
	ledger.ErrDuplicate, ledger.ErrInvalid, ledger.ErrLimit,
	ledger.ErrInFlight, ledger.ErrMismatch, ledger.ErrBusy,
}
