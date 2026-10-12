// Package ledger holds the types and sentinels the store returns.
package ledger

import "errors"

type Account struct {
	ID      string
	Balance int64
}

var (
	ErrNotFound  = errors.New("ledger: not found")
	ErrDuplicate = errors.New("ledger: duplicate transfer id")
)
