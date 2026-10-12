// Package ledger is Technique 29: a Deprecated: paragraph tools
// understand.
package ledger

import "fmt"

type Transfer struct {
	ID, From, To string
	Amount       int64
	Key          string
}

var posted []Transfer

// PostTransfer records t and returns its ID. It honors t.Key.
func PostTransfer(t Transfer) string {
	for _, p := range posted {
		if t.Key != "" && p.Key == t.Key {
			return p.ID
		}
	}
	t.ID = fmt.Sprintf("t_%d", len(posted)+1)
	posted = append(posted, t)
	return t.ID
}

// Post records a transfer and returns its ID.
//
// Deprecated: Post ignores the idempotency key.
// Use [PostTransfer], which honors Transfer.Key.
func Post(from, to string, amount int64) string {
	return PostTransfer(Transfer{
		From: from, To: to, Amount: amount,
	})
}
