// Package transfer is Technique 20: the omitzero JSON option.
package transfer

import "time"

type Money struct {
	Cents    int64
	Currency string
}

func (m Money) IsZero() bool { return m.Cents == 0 }

type TransferOut struct {
	ID       string    `json:"id"`
	Amount   int64     `json:"amount"`
	Fee      Money     `json:"fee,omitzero"`
	PostedAt time.Time `json:"posted_at,omitzero"`
	Tags     []string  `json:"tags,omitempty,omitzero"`
}

// Old is the version that showed "posted 2,025 years ago".
type Old struct {
	PostedAt time.Time `json:"posted_at,omitempty"`
}

// OnlyZero drops nil but keeps an empty slice.
type OnlyZero struct {
	Tags []string `json:"tags,omitzero"`
}

// PtrFee shows pointers: nil is omitted, and so is a pointer to
// a zero Money.
type PtrFee struct {
	Fee *Money `json:"fee,omitzero"`
}
