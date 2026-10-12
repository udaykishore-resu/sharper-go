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
	pub = withTimeout(pub, 5*time.Second)
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

// withTimeout bounds each publish, so a slow broker cannot hold
// the claim transaction (and its row locks) open for long.
func withTimeout(pub Publisher, d time.Duration) Publisher {
	return func(ctx context.Context, topic string, p []byte) error {
		ctx, cancel := context.WithTimeout(ctx, d)
		defer cancel()
		return pub(ctx, topic, p)
	}
}

// relayOnce claims up to 100 rows and, inside the claim's
// transaction, marks each row sent right after the broker
// accepts it. A row is never marked unless its publish returned
// nil. On a publish failure it stops and commits: rows already
// sent stay marked, the failed row and the rest stay unsent for
// the next round, and the publish error is returned for the log.
// If marking or the commit fails, the whole batch rolls back and
// is sent again: delivery is at least once
// (TestRelayMarksOnlySentRows, TestRelayCrashBeforeCommit).
func (s *Store) relayOnce(ctx context.Context,
	pub Publisher) (int, error) {
	var (
		sent int
		perr error
	)
	err := s.inTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted},
		func(tx pgx.Tx) error {
			sent, perr = 0, nil
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
				perr = pub(ctx, e.Topic, e.Payload) // for the log
				if perr != nil {
					return nil // commit the rows already sent
				}
				_, err := tx.Exec(ctx, `UPDATE outbox
                    SET sent_at = now() WHERE id = $1`, e.ID)
				if err != nil {
					return err // roll back: all are resent
				}
				sent++
			}
			return nil
		})
	if err != nil {
		return 0, err
	}
	return sent, perr
}
