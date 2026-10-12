package ledger

import (
	"errors"
	"fmt"
)

// Sentinels callers match with errors.Is (Techniques 21 and 23).
var (
	ErrNotFound  = errors.New("ledger: not found")
	ErrFunds     = errors.New("ledger: insufficient funds")
	ErrFrozen    = errors.New("ledger: account frozen")
	ErrDuplicate = errors.New("ledger: duplicate transfer id")
	// ErrBusy means the transaction lost every serialization
	// retry; the client should back off and retry (503).
	ErrBusy = errors.New("ledger: busy, retry")
)

// Idempotency outcomes (Technique 52).
var (
	ErrInFlight = errors.New("ledger: request in flight")
	ErrMismatch = errors.New("ledger: key reused with other body")
)

// ErrInvalid is the family every validation rule wraps
// (Technique 22).
var ErrInvalid = errors.New("invalid transfer")

func invalid(msg string) error {
	return fmt.Errorf("%w: %s", ErrInvalid, msg)
}

var (
	ErrBadAmount   = invalid("amount must be > 0")
	ErrSameAccount = invalid("from equals to")
	errNoAccount   = invalid("from or to is empty")
)

// ErrLimit is reported by every *LimitError (Technique 23).
var ErrLimit = errors.New("ledger: limit exceeded")

// LimitError carries the numbers a handler needs for its body.
type LimitError struct {
	Account   string
	Limit     int64
	Requested int64
}

func (e *LimitError) Error() string {
	return fmt.Sprintf(
		"account %s: limit %d, requested %d",
		e.Account, e.Limit, e.Requested)
}

// Is reports the sentinel, so errors.Is(err, ErrLimit) holds.
func (e *LimitError) Is(target error) bool {
	return target == ErrLimit
}

// Validate collects every rule a transfer breaks (Technique 22).
func (t Transfer) Validate() error {
	var errs []error
	if t.From == "" || t.To == "" {
		errs = append(errs, errNoAccount)
	} else if t.From == t.To {
		errs = append(errs, ErrSameAccount)
	}
	if t.Amount <= 0 {
		errs = append(errs, ErrBadAmount)
	}
	return errors.Join(errs...)
}

// CheckLimit returns a *LimitError when t exceeds max.
// A max of zero or less means no limit.
func (t Transfer) CheckLimit(max int64) error {
	if max > 0 && t.Amount > max {
		return &LimitError{
			Account: t.From, Limit: max, Requested: t.Amount,
		}
	}
	return nil
}
