// Package validate is Technique 22: errors.Join for validation
// and for deferred Close.
package validate

import (
	"errors"
	"fmt"
	"io"
)

type Transfer struct {
	ID, From, To string
	Amount       int64
	Key          string
}

var ErrInvalid = errors.New("invalid transfer")

func invalid(msg string) error {
	return fmt.Errorf("%w: %s", ErrInvalid, msg)
}

var (
	ErrBadAmount   = invalid("amount must be > 0")
	ErrSameAccount = invalid("from equals to")
	errNoAccount   = invalid("from or to is empty")
)

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

// writeAll keeps the Close error with a named result.
func writeAll(w io.WriteCloser, b []byte) (err error) {
	defer func() {
		err = errors.Join(err, w.Close())
	}()
	_, err = w.Write(b)
	return err
}
