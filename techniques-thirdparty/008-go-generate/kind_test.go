package ledger

import "testing"

func TestKindString(t *testing.T) {
	if Debit.String() != "debit" || Credit.String() != "credit" {
		t.Fatalf("got %q %q", Debit, Credit)
	}
	if Kind(7).String() != "Kind(7)" {
		t.Fatalf("got %q", Kind(7))
	}
}
