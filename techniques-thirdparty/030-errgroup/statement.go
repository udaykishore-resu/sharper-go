// Package statement is Technique 30: errgroup with a context
// and SetLimit for a fan-out that must all finish.
package statement

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"
)

type Account struct {
	ID      string
	Balance int64
}

type store interface {
	Account(ctx context.Context, id string) (Account, error)
}

func balances(ctx context.Context, st store,
	ids []string) ([]Account, error) {
	out := make([]Account, len(ids))
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(8)
	for i, id := range ids {
		g.Go(func() error {
			a, err := st.Account(ctx, id)
			if err != nil {
				return fmt.Errorf("account %s: %w", id, err)
			}
			out[i] = a
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return out, nil
}
