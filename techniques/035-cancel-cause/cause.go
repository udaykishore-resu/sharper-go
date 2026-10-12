// Package cause is Technique 35: contexts that say who canceled
// them and why.
package cause

import (
	"context"
	"errors"
	"os"
	"time"
)

type sigErr struct{ sig os.Signal }

func (e sigErr) Error() string {
	return "received " + e.sig.String()
}

// rootFrom is the pre-1.26 helper: the root dies with the
// signal's name. sigs is the channel passed to signal.Notify.
func rootFrom(sigs <-chan os.Signal) context.Context {
	root, cancel := context.WithCancelCause(
		context.Background())
	go func() { cancel(sigErr{<-sigs}) }()
	return root
}

var errBudget = errors.New("balance query budget 2s")

// balanceCtx gives a query a budget with its own cause.
func balanceCtx(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeoutCause(ctx, d, errBudget)
}
