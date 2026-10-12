// Package batch is Technique 12: since Go 1.22, each loop pass
// has its own variables, so the t := t copy goes away.
package batch

import (
	"context"

	"golang.org/x/sync/errgroup"
)

type Transfer struct{ ID string }

func postAll(ctx context.Context, batch []Transfer,
	post func(context.Context, Transfer) error) error {
	g, ctx := errgroup.WithContext(ctx)
	for _, t := range batch {
		g.Go(func() error {
			return post(ctx, t)
		})
	}
	return g.Wait()
}
