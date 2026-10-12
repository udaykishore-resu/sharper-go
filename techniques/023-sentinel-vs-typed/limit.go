// Package limit is Technique 23: a typed error that also reports
// a sentinel through Is.
package limit

import (
	"errors"
	"fmt"
)

var ErrFrozen = errors.New("ledger: account frozen")

var ErrLimit = errors.New("ledger: limit exceeded")

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

func (e *LimitError) Is(target error) bool {
	return target == ErrLimit
}
