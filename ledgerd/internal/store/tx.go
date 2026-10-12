package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"example.com/ledgerd/internal/ledger"
	"example.com/ledgerd/internal/store/sqlcdb"
)

// InTx owns the whole transaction lifecycle, so no code path can
// return with one still open (Technique 57). Serializable is the
// right default for a ledger.
func (s *Store) InTx(ctx context.Context,
	fn func(pgx.Tx) error) (err error) {
	return s.inTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable}, fn)
}

func (s *Store) inTx(ctx context.Context, opts pgx.TxOptions,
	fn func(pgx.Tx) error) (err error) {
	tx, err := s.pool.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() {
		p := recover()
		if p != nil || err != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
		if p != nil {
			panic(p)
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RunTx calls InTx up to three times while Postgres asks for a
// retry: 40001 (serialization) or 40P01 (deadlock), with full
// jitter and a few seconds in total. fn must be pure database
// work, since it may run more than once.
func (s *Store) RunTx(ctx context.Context,
	fn func(pgx.Tx) error) error {
	ctx, cancel := context.WithTimeoutCause(ctx, 5*time.Second,
		errors.New("store: transaction budget 5s"))
	defer cancel()
	var err error
	for attempt := range 3 {
		if err = s.InTx(ctx, fn); !retryable(err) {
			return err
		}
		d := 10 * time.Millisecond << attempt
		select {
		case <-ctx.Done():
			return errors.Join(err, context.Cause(ctx))
		case <-time.After(rand.N(d)):
		}
	}
	return err
}

func retryable(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) &&
		slices.Contains([]string{"40001", "40P01"}, pe.Code)
}

// Transfer claims the idempotency key, moves the money, writes
// the outbox event and saves the response in one transaction
// (Techniques 52, 57 and 58). A key the middleware claimed is
// still 'inflight' and flips to 'done' here; a key that is
// already done returns no row, so its stored response replays.
func (s *Store) Transfer(ctx context.Context,
	tr ledger.Transfer) (resp []byte, err error) {
	err = s.RunTx(ctx, func(tx pgx.Tx) error {
		var ok bool
		err := tx.QueryRow(ctx, `INSERT INTO idem (key)
            VALUES ($1) ON CONFLICT (key) DO UPDATE
            SET state = 'done'
            WHERE idem.state = 'inflight'
            RETURNING true`, tr.Key).Scan(&ok)
		if errors.Is(err, pgx.ErrNoRows) { // seen it
			return tx.QueryRow(ctx, `SELECT response
                FROM idem WHERE key = $1`,
				tr.Key).Scan(&resp)
		}
		if err != nil {
			return err
		}
		resp, err = applyTransfer(ctx, tx, tr)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE idem
            SET response = $2 WHERE key = $1`, tr.Key, resp)
		return err
	})
	if err != nil {
		return nil, translate(ctx, fmt.Errorf("transfer %s", tr.ID), err)
	}
	return resp, nil
}

// Posted is the response body of a committed transfer, and the
// payload of its transfer.created event.
type Posted struct {
	ID       string    `json:"id"`
	From     string    `json:"from"`
	To       string    `json:"to"`
	Amount   int64     `json:"amount"`
	PostedAt time.Time `json:"posted_at,omitzero"`
}

type lockedAccount struct {
	ID      string
	Balance int64
	Frozen  bool
}

// applyTransfer debits, credits, inserts the transfer row and
// the outbox event, and returns the response JSON.
func applyTransfer(ctx context.Context, tx pgx.Tx,
	tr ledger.Transfer) ([]byte, error) {
	// Lock both rows in ID order, so two opposite transfers
	// can't deadlock each other.
	rows, _ := tx.Query(ctx, `SELECT id, balance, frozen
        FROM accounts WHERE id = ANY($1)
        ORDER BY id FOR UPDATE`, []string{tr.From, tr.To})
	accts, err := pgx.CollectRows(rows,
		pgx.RowToStructByName[lockedAccount])
	if err != nil {
		return nil, err
	}
	byID := map[string]lockedAccount{}
	for _, a := range accts {
		byID[a.ID] = a
	}
	for _, id := range []string{tr.From, tr.To} {
		a, ok := byID[id]
		switch {
		case !ok:
			return nil, fmt.Errorf("account %s: %w", id, ledger.ErrNotFound)
		case a.Frozen:
			return nil, fmt.Errorf("account %s: %w", id, ledger.ErrFrozen)
		}
	}
	if from := byID[tr.From]; from.ID != ledger.HouseAccount &&
		from.Balance < tr.Amount {
		return nil, fmt.Errorf("account %s: %w", tr.From, ledger.ErrFunds)
	}
	q := sqlcdb.New(tx)
	for _, d := range []sqlcdb.AddToBalanceParams{
		{Delta: -tr.Amount, ID: tr.From},
		{Delta: tr.Amount, ID: tr.To},
	} {
		// An UPDATE that touches zero rows is fine to Postgres
		// but an error to a ledger (Technique 56).
		n, err := q.AddToBalance(ctx, d)
		if err != nil {
			return nil, err
		}
		if n != 1 {
			return nil, fmt.Errorf("account %s: updated %d rows", d.ID, n)
		}
	}
	p := Posted{ID: tr.ID, From: tr.From, To: tr.To, Amount: tr.Amount}
	err = tx.QueryRow(ctx, `INSERT INTO transfers
        (id, from_acct, to_acct, amount, idem_key)
        VALUES ($1, $2, $3, $4, $5) RETURNING created_at`,
		tr.ID, tr.From, tr.To, tr.Amount, tr.Key).Scan(&p.PostedAt)
	if err != nil {
		return nil, err
	}
	p.PostedAt = p.PostedAt.UTC()
	body, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox (topic, payload)
        VALUES ('transfer.created', $1)`, body)
	return body, err
}
