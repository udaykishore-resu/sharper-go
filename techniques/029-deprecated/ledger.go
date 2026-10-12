// Package ledger is Technique 29: a Deprecated: paragraph tools
// understand.
package ledger

import (
	"errors"
	"fmt"
)

type Transfer struct {
	ID, From, To string
	Amount       int64
	Key          string
}

var ErrBadAmount = errors.New("ledger: amount must be > 0")

var posted []Transfer

// PostTransfer records t and returns its ID. It honors t.Key:
// a second call with the same non-empty key returns the first ID.
func PostTransfer(t Transfer) (string, error) {
	if t.Amount <= 0 {
		return "", ErrBadAmount
	}
	for _, p := range posted {
		if t.Key != "" && p.Key == t.Key {
			return p.ID, nil
		}
	}
	t.ID = fmt.Sprintf("t_%d", len(posted)+1)
	posted = append(posted, t)
	return t.ID, nil
}

// Post records a transfer and returns its ID.
//
// Deprecated: Post ignores the idempotency key.
// Use [PostTransfer], which honors Transfer.Key.
func Post(from, to string, amount int64) (string, error) {
	return PostTransfer(Transfer{
		From: from, To: to, Amount: amount,
	})
}
