package ledger

import (
	"errors"
	"testing"
)

// Review check for Technique 62: the switch in TestValidate keeps
// "expected success" apart from "expected this sentinel". Each
// case below is one cell of that truth table.
func TestValidateAssertionTable(t *testing.T) {
	check := func(want, err error) (failed bool) {
		switch {
		case want == nil && err != nil:
			return true
		case !errors.Is(err, want):
			return true
		}
		return false
	}
	cases := []struct {
		name      string
		want, err error
		fail      bool
	}{
		{"success expected, got success", nil, nil, false},
		{"success expected, got error", nil, ErrBadAmount, true},
		{"error expected, got nil", ErrBadAmount, nil, true},
		{"error expected, got it wrapped",
			ErrBadAmount, errors.Join(ErrSameAccount, ErrBadAmount), false},
		{"error expected, got another", ErrBadAmount, ErrSameAccount, true},
	}
	for _, c := range cases {
		if got := check(c.want, c.err); got != c.fail {
			t.Errorf("%s: failed=%v, want %v", c.name, got, c.fail)
		}
	}
}
