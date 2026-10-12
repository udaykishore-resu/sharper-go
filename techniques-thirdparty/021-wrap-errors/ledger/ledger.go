// Package ledger holds the sentinels the store translates to.
package ledger

import "errors"

type Account struct {
	ID      string
	Balance int64
}

var ErrNotFound = errors.New("ledger: not found")
