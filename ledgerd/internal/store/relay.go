package store

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

// Publisher sends one outbox event. A sink from internal/sink
// provides it (Technique 99).
type Publisher func(ctx context.Context, topic string, payload []byte) error

// Relay publishes outbox rows until its context ends
// (Technique 58). Every pod can run one: FOR UPDATE SKIP LOCKED
// lets relays share the table with no leader election.
type Relay struct {
	done chan struct{}
	// OnRound runs after each round with the count it sent, so
	// the caller can publish a last-run timestamp (Technique 89).
	onRound func(sent int)
}

// Done is closed once the relay has stopped.
func (r *Relay) Done() <-chan struct{} { return r.done }

// StartRelay runs the relay in its own goroutine.
func (s *Store) StartRelay(ctx context.Context, pub Publisher,
	log *slog.Logger, onRound func(sent int)) *Relay {
	r := &Relay{done: make(chan struct{}), onRound: onRound}
	go func() {
		defer close(r.done)
		for ctx.Err() == nil {
			n, err := s.relayOnce(ctx, pub)
			if err != nil && ctx.Err() == nil {
				log.Warn("outbox relay", "err", err)
			}
			if r.onRound != nil {
				r.onRound(n)
			}
			if n == 0 {
				select {
				case <-ctx.Done():
				case <-time.After(500 * time.Millisecond):
				}
			}
		}
	}()
	return r
}

type event struct {
	ID      int64
	Topic   string
	Payload []byte
}

// relayOnce claims up to 100 rows, publishes each and marks it
// sent. On a publish failure it stops but still commits, so the
// rows it did send stay marked. Delivery is at least once.
func (s *Store) relayOnce(ctx context.Context, pub Publisher) (int, error) {
	sent := 0
	err := s.inTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted},
		func(tx pgx.Tx) error {
			sent = 0
			rows, _ := tx.Query(ctx, `SELECT id, topic, payload
                FROM outbox WHERE sent_at IS NULL
                ORDER BY id LIMIT 100
                FOR UPDATE SKIP LOCKED`)
			evs, err := pgx.CollectRows(rows,
				pgx.RowToStructByName[event])
			if err != nil {
				return err
			}
			for _, e := range evs {
				pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
				err := pub(pctx, e.Topic, e.Payload)
				cancel()
				if err != nil {
					return nil // commit what was sent
				}
				_, err = tx.Exec(ctx, `UPDATE outbox
                    SET sent_at = now() WHERE id = $1`, e.ID)
				if err != nil {
					return err
				}
				sent++
			}
			return nil
		})
	return sent, err
}
