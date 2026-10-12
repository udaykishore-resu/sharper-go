package ledger

import (
	"errors"
	"testing"
)

func TestValidate(t *testing.T) {
	ok := Transfer{From: "a", To: "b",
		Amount: 500, Key: "k1"}
	tests := []struct {
		name string
		edit func(*Transfer)
		want error
	}{
		{"valid", func(*Transfer) {}, nil},
		{"zero amount",
			func(t *Transfer) { t.Amount = 0 },
			ErrBadAmount},
		{"same account",
			func(t *Transfer) { t.To = "a" },
			ErrSameAccount},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr := ok
			tc.edit(&tr)
			err := tr.Validate()
			switch {
			case tc.want == nil && err != nil:
				t.Errorf("%+v: unexpected %v", tr, err)
			case !errors.Is(err, tc.want):
				t.Errorf("%+v.Validate() = %v, want %v",
					tr, err, tc.want)
			}
		})
	}
}
