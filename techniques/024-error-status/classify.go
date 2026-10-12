// Package httpapi is Technique 24: one table maps every error to
// a status and a stable code.
package httpapi

import (
	"context"
	"errors"
)

// ledger stands in for internal/ledger's sentinels.
var ledger = struct {
	ErrNotFound, ErrFunds, ErrInvalid error
}{
	errors.New("ledger: not found"),
	errors.New("ledger: insufficient funds"),
	errors.New("invalid transfer"),
}

type apiErr struct {
	status int
	code   string
}

var table = []struct {
	target error
	apiErr
}{
	{ledger.ErrNotFound, apiErr{404, "not_found"}},
	{ledger.ErrFunds, apiErr{409, "insufficient_funds"}},
	{ledger.ErrInvalid, apiErr{422, "invalid_transfer"}},
	{context.DeadlineExceeded,
		apiErr{503, "timeout"}},
	{context.Canceled, apiErr{499, "client_closed"}},
}

func classify(err error) apiErr {
	for _, row := range table {
		if errors.Is(err, row.target) {
			return row.apiErr
		}
	}
	return apiErr{500, "internal"}
}
