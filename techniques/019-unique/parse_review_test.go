package entry

import (
	"errors"
	"strconv"
	"testing"
)

// Short, long and malformed rows are errors, not panics or zero
// amounts.
func TestParseRejectsBadRows(t *testing.T) {
	for _, line := range []string{
		"", "acc_1", "acc_1,USD", "acc_1,USD,500,x",
		"acc_1,USD,", "acc_1,USD,5.00", "acc_1,USD,99999999999999999999",
	} {
		if e, err := parse(line); err == nil {
			t.Errorf("parse(%q) = %+v, want an error", line, e)
		}
	}
	_, err := parse("acc_1,USD,12x")
	if !errors.Is(err, strconv.ErrSyntax) {
		t.Fatalf("amount error not wrapped: %v", err)
	}
}

// Handles compare equal exactly when the values do.
func TestHandleEquality(t *testing.T) {
	a, err := parse("acc_1,EUR,1")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := parse("acc_2,EUR,2")
	c, _ := parse("acc_3,USD,3")
	if a.Currency != b.Currency || a.Currency == c.Currency {
		t.Fatal("handle equality does not follow value equality")
	}
}
