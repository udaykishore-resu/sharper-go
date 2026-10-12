// Package accounts is Technique 13: a paged store read exposed
// as an iter.Seq2 of values and errors.
package accounts

import (
	"context"
	"iter"
)

// Account is a stand-in for ledger.Account.
type Account struct {
	ID      string
	Balance int64
}

// Pager is anything with a Page method. The contract Accounts
// relies on: Page returns up to n accounts with ID > after, in ID
// order, and a page shorter than n means there are no more.
type Pager interface {
	Page(ctx context.Context, after string, n int) ([]Account, error)
}

// Accounts reads 500 rows at a time and stops fetching as soon
// as the caller's loop breaks.
func Accounts(ctx context.Context, p Pager) iter.Seq2[Account, error] {
	return func(yield func(Account, error) bool) {
		after := ""
		for {
			page, err := p.Page(ctx, after, 500)
			if err != nil {
				yield(Account{}, err)
				return
			}
			for _, a := range page {
				if !yield(a, nil) {
					return
				}
			}
			if len(page) < 500 {
				return
			}
			after = page[len(page)-1].ID
		}
	}
}

// Total sums every balance, ranging with two variables so a page
// error is never dropped.
func Total(ctx context.Context, p Pager) (int64, error) {
	var total int64
	for a, err := range Accounts(ctx, p) {
		if err != nil {
			return 0, err
		}
		total += a.Balance
	}
	return total, nil
}
