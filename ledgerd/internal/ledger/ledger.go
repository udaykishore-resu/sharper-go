// Package ledger holds ledgerd's domain types and rules.
// It imports no other internal package (Appendix C.1).
package ledger

import "log/slog"

// Account holds a balance in cents.
type Account struct {
	ID      string
	Balance int64
}

// Transfer moves Amount cents between accounts.
// Key is the client's idempotency key.
type Transfer struct {
	ID, From, To string
	Amount       int64
	Key          string
}

// LogValue keeps the idempotency key out of logs (Technique 85).
func (t Transfer) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("id", t.ID),
		slog.Int64("amount", t.Amount))
}

// HouseAccount is the funding account. It may go negative.
const HouseAccount = "house"

// Saved is a response recorded under an idempotency key
// (Technique 52). The store returns it and the HTTP layer
// replays it, so it lives in the domain package.
type Saved struct {
	Status int
	Body   []byte
}

// ScopedKey scopes a client's idempotency key to that client,
// so two partners using counters never collide (Technique 52).
func ScopedKey(client, key string) string {
	return client + "/" + key
}
