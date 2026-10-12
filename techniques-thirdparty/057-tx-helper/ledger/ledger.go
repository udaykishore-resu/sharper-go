// Package ledger holds the types the store works with.
package ledger

import "errors"

type Transfer struct {
	ID, From, To string
	Amount       int64
	Key          string
}

var ErrFunds = errors.New("ledger: insufficient funds")
