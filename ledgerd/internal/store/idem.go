package store

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"example.com/ledgerd/internal/ledger"
)

// Lease is how long a claim lives before another request may
// take it over. It outlasts the server's WriteTimeout (20 s) and
// the middleware's httpapi.ClaimBudget (25 s), which ends the
// holder's own use of the claim first (Technique 52).
const Lease = 30 * time.Second

// Begin claims (client, key) and commits at once. It returns:
// nil, nil when this request now owns the key; the saved
// response for a finished key; ErrInFlight while another request
// holds it; ErrMismatch when the body hash differs. An expired
// claim is taken over, so a process that died between Begin and
// commit does not block the key forever.
func (s *Store) Begin(ctx context.Context, client, key string,
	hash [32]byte) (*ledger.Saved, error) {
	k := ledger.ScopedKey(client, key)
	var saved *ledger.Saved
	err := s.RunTx(ctx, func(tx pgx.Tx) error {
		saved = nil
		tag, err := tx.Exec(ctx, `INSERT INTO idem
            (key, state, req_hash, locked_until)
            VALUES ($1, 'inflight', $2,
                    now() + $3 * interval '1 second')
            ON CONFLICT DO NOTHING`,
			k, hash[:], int(Lease/time.Second))
		if err != nil || tag.RowsAffected() == 1 {
			return err // error, or the key is ours
		}
		var (
			state   string
			h, body []byte
			status  int
			expired bool
		)
		err = tx.QueryRow(ctx, `SELECT state, req_hash, status,
            coalesce(response, ''::bytea),
            coalesce(locked_until < now(), false)
            FROM idem WHERE key = $1 FOR UPDATE`, k).
			Scan(&state, &h, &status, &body, &expired)
		if err != nil {
			return err
		}
		if h != nil && !bytes.Equal(h, hash[:]) {
			return ledger.ErrMismatch
		}
		if state == "done" {
			saved = &ledger.Saved{Status: status, Body: body}
			return nil
		}
		if !expired {
			return ledger.ErrInFlight
		}
		_, err = tx.Exec(ctx, `UPDATE idem SET locked_until =
            now() + $2 * interval '1 second' WHERE key = $1`,
			k, int(Lease/time.Second))
		return err
	})
	if err != nil {
		return nil, translate(ctx, fmt.Errorf("idempotency %s", key), err)
	}
	return saved, nil
}

// Finish saves a response for replay, but only while the row is
// still in flight: a committed transfer already flipped it.
func (s *Store) Finish(ctx context.Context, client, key string,
	saved ledger.Saved) error {
	_, err := s.pool.Exec(ctx, `UPDATE idem
        SET state = 'done', status = $2, response = $3,
            locked_until = NULL
        WHERE key = $1 AND state = 'inflight'`,
		ledger.ScopedKey(client, key), saved.Status, saved.Body)
	return err
}

// Release frees a claim after a 5xx (or a response too big to
// save), so the client may retry. It never touches a done row,
// so a committed transfer keeps its key whatever happened to the
// response.
func (s *Store) Release(ctx context.Context, client, key string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM idem
        WHERE key = $1 AND state = 'inflight'`,
		ledger.ScopedKey(client, key))
	return err
}
