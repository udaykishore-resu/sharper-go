// Package audit is Technique 34: context.WithoutCancel for work
// that outlives a request, and context.AfterFunc for cleanup.
package audit

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"time"
)

type Transfer struct{ ID string }

type auditor interface {
	Record(ctx context.Context, t Transfer) error
}

// afterCommit keeps the request's values but not its
// cancellation, and puts a budget back right away. wg lets
// shutdown wait for detached work. It is best effort: a crash or
// the 5 s budget still loses the row. Rows that must survive go
// in the transaction or an outbox (Technique 58).
func afterCommit(ctx context.Context,
	wg *sync.WaitGroup, a auditor, t Transfer) {
	bg := context.WithoutCancel(ctx)
	wg.Go(func() {
		ctx, cancel := context.WithTimeout(
			bg, 5*time.Second)
		defer cancel()
		if err := a.Record(ctx, t); err != nil {
			slog.ErrorContext(ctx, "audit failed",
				"transfer", t.ID, "err", err)
		}
	})
}

// readUntilCancel wakes a blocking read that takes no context.
func readUntilCancel(ctx context.Context, conn net.Conn) error {
	stop := context.AfterFunc(ctx, func() {
		_ = conn.Close()
	})
	defer stop()
	buf := make([]byte, 1)
	_, err := conn.Read(buf)
	return err
}
