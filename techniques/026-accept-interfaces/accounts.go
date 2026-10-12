// Package httpapi is Technique 26: the consumer declares the
// small interface it needs.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

// Account and ErrNotFound stand in for package ledger.
type Account struct {
	ID      string
	Balance int64
}

var ErrNotFound = errors.New("ledger: not found")

// in internal/httpapi
type AccountGetter interface {
	Account(ctx context.Context,
		id string) (Account, error)
}

func GetAccount(st AccountGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, err := st.Account(r.Context(), r.PathValue("id"))
		if errors.Is(err, ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		json.NewEncoder(w).Encode(a)
	}
}

// ValidationError and validateBad show the typed nil trap.
type ValidationError struct{ Field string }

func (e *ValidationError) Error() string { return e.Field + " is invalid" }

func validateBad(ok bool) error {
	var verr *ValidationError
	if !ok {
		verr = &ValidationError{Field: "amount"}
	}
	return verr // a typed nil when ok: never == nil
}

// validateGood is the fix: return the literal nil on success.
func validateGood(ok bool) error {
	if !ok {
		return &ValidationError{Field: "amount"}
	}
	return nil
}
