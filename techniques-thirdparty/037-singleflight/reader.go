// Package reader is Technique 37: singleflight collapses
// simultaneous reads of one key into one backend call.
package reader

import (
	"context"
	"time"

	"golang.org/x/sync/singleflight"
)

type Account struct {
	ID      string
	Balance int64
}

type Reader struct {
	sf singleflight.Group
	db func(context.Context, string) (Account, error)
	// Shared counts callers that got another caller's result;
	// export it next to Technique 88's metrics.
	Shared func()
}

func (r *Reader) Account(ctx context.Context,
	id string) (Account, error) {
	ch := r.sf.DoChan(id, func() (any, error) {
		c, cancel := context.WithTimeout(
			context.WithoutCancel(ctx),
			2*time.Second)
		defer cancel()
		return r.db(c, id)
	})
	select {
	case res := <-ch:
		if res.Shared && r.Shared != nil {
			r.Shared()
		}
		if res.Err != nil {
			return Account{}, res.Err
		}
		return res.Val.(Account), nil
	case <-ctx.Done():
		return Account{}, ctx.Err()
	}
}
