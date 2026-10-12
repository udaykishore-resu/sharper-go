// Package store is Technique 57: one helper owns every
// transaction, and store.Transfer claims the idempotency key,
// moves the money and saves the response in one of them.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/udaykishore-resu/sharper-go/techniques-thirdparty/057-tx-helper/ledger"
)

type Store struct{ pool *pgxpool.Pool }

func (s *Store) InTx(ctx context.Context,
	fn func(pgx.Tx) error) (err error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.Serializable,
	})
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

// RunTx retries 40001 and 40P01 up to three times, with jitter,
// inside a 5-second budget. Every other error returns at once.
func (s *Store) RunTx(ctx context.Context,
	fn func(pgx.Tx) error) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	const attempts = 3
	var err error
	for attempt := range attempts {
		err = s.InTx(ctx, fn)
		var pe *pgconn.PgError
		if !errors.As(err, &pe) ||
			(pe.Code != "40001" && pe.Code != "40P01") ||
			attempt == attempts-1 {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(rand.N(10 * time.Millisecond << attempt)):
		}
	}
	return err
}

// Transfer runs transferTx inside RunTx.
func (s *Store) Transfer(ctx context.Context,
	tr ledger.Transfer) ([]byte, error) {
	var resp []byte
	err := s.RunTx(ctx, func(tx pgx.Tx) error {
		var err error
		resp, err = transferTx(ctx, tx, tr)
		return err
	})
	return resp, err
}

// transferTx is the listing in Technique 57: claim or flip the
// key, move the money, save the response.
func transferTx(ctx context.Context, tx pgx.Tx,
	tr ledger.Transfer) ([]byte, error) {
	var ok bool
	err := tx.QueryRow(ctx, `INSERT INTO idem
        (key, state) VALUES ($1, 'done')
        ON CONFLICT (key) DO UPDATE SET state = 'done'
        WHERE idem.state = 'inflight'
        RETURNING true`, tr.Key).Scan(&ok)
	if errors.Is(err, pgx.ErrNoRows) { // already done
		return replay(ctx, tx, tr.Key)
	}
	if err != nil {
		return nil, err
	}
	resp, err := applyTransfer(ctx, tx, tr)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `UPDATE idem SET response = $2
        WHERE key = $1`, tr.Key, resp)
	return resp, err
}

// replay returns a committed transfer's saved response. A key
// that finished with an error status is not replayed as a
// success: the caller gets ErrBusy and retries through the
// middleware, which replays the saved error.
func replay(ctx context.Context, tx pgx.Tx,
	key string) ([]byte, error) {
	var (
		status int
		body   []byte
	)
	err := tx.QueryRow(ctx, `SELECT status, response
        FROM idem WHERE key = $1`, key).Scan(&status, &body)
	if err != nil {
		return nil, err
	}
	if status != 201 || body == nil {
		return nil, ledger.ErrBusy
	}
	return body, nil
}

// applyTransfer debits, credits, inserts the transfer row and its
// outbox event (Technique 58), and returns the response JSON.
func applyTransfer(ctx context.Context, tx pgx.Tx,
	tr ledger.Transfer) ([]byte, error) {
	tag, err := tx.Exec(ctx, `UPDATE accounts
        SET balance = balance - $2
        WHERE id = $1 AND balance >= $2`, tr.From, tr.Amount)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != 1 {
		return nil, ledger.ErrFunds
	}
	if _, err := tx.Exec(ctx, `UPDATE accounts
        SET balance = balance + $2 WHERE id = $1`, tr.To, tr.Amount); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO transfers (id, amount)
        VALUES ($1, $2)`, tr.ID, tr.Amount); err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string]any{"id": tr.ID, "amount": tr.Amount})
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox (topic, payload)
        VALUES ('transfer.created', $1)`, body)
	return body, err
}
