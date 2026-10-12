// Package settle is Technique 33: a pipeline where one failure
// stops every stage and nothing leaks.
package settle

import (
	"context"
	"errors"

	"golang.org/x/sync/errgroup"
)

type Transfer struct {
	ID     string
	Amount int64
}

func read(ctx context.Context, ts []Transfer,
	out chan<- Transfer) error {
	defer close(out)
	for _, t := range ts {
		select {
		case out <- t:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func validate(ctx context.Context,
	in <-chan Transfer,
	out chan<- Transfer,
) error {
	defer close(out)
	for t := range in {
		if t.Amount <= 0 {
			return errors.New("bad amount: " + t.ID)
		}
		select {
		case out <- t:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// errStop is how the last stage says "enough" without failing:
// errgroup cancels the other stages as for any error, and run
// drops it after Wait. Wait keeps the first error, so a real
// failure that happened before the stop is still returned.
var errStop = errors.New("settle: consumer stopped early")

// run wires one group, one unbuffered channel per edge and one
// g.Go per stage. Each edge has exactly one sender.
func run(ctx context.Context, ts []Transfer,
	write func(context.Context, Transfer) error) error {
	g, ctx := errgroup.WithContext(ctx)
	raw := make(chan Transfer)
	valid := make(chan Transfer)
	g.Go(func() error { return read(ctx, ts, raw) })
	g.Go(func() error { return validate(ctx, raw, valid) })
	g.Go(func() error {
		for t := range valid {
			if err := write(ctx, t); err != nil {
				return err
			}
		}
		return nil
	})
	if err := g.Wait(); !errors.Is(err, errStop) {
		return err
	}
	return nil
}
