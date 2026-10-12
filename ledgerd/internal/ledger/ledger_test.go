package ledger

import (
	"errors"
	"testing"
)

// TestValidate is Technique 62's table test.
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
		{"empty to",
			func(t *Transfer) { t.To = "" },
			ErrInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr := ok
			tc.edit(&tr)
			err := tr.Validate()
			if !errors.Is(err, tc.want) {
				t.Errorf("%+v.Validate() = %v, want %v",
					tr, err, tc.want)
			}
		})
	}
}

// Technique 22: one call reports both rules, each matchable.
func TestValidateJoinsRules(t *testing.T) {
	err := Transfer{From: "a", To: "a", Amount: -1}.Validate()
	for _, want := range []error{ErrSameAccount, ErrBadAmount, ErrInvalid} {
		if !errors.Is(err, want) {
			t.Errorf("errors.Is(%v, %v) = false", err, want)
		}
	}
}

// Technique 23: the typed error reports the sentinel and keeps its data.
func TestLimitError(t *testing.T) {
	err := Transfer{From: "a", To: "b", Amount: 900}.CheckLimit(500)
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("errors.Is(ErrLimit) = false for %v", err)
	}
	le, ok := errors.AsType[*LimitError](err)
	if !ok || le.Limit != 500 || le.Requested != 900 {
		t.Fatalf("AsType = %+v, %v", le, ok)
	}
}

func TestParseAmount(t *testing.T) {
	tests := []struct {
		in   string
		want int64
		bad  bool
	}{
		{"0", 0, false},
		{"12.34", 1234, false},
		{"12.3", 1230, false},
		{"99999.99", 9999999, false},
		{"1.-5", 0, true},
		{"-3", 0, true},
		{"1.234", 0, true},
		{"", 0, true},
		{".5", 0, true},
		{"5.", 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseAmount(tc.in)
			if (err != nil) != tc.bad || got != tc.want {
				t.Errorf("ParseAmount(%q) = %d, %v", tc.in, got, err)
			}
		})
	}
}

// FuzzParseAmount is Technique 67's round-trip target.
func FuzzParseAmount(f *testing.F) {
	for _, s := range []string{"0", "12.34", "-3"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		cents, err := ParseAmount(s)
		if err != nil {
			return // rejecting input is fine
		}
		if cents < 0 {
			t.Fatalf("%q -> negative %d", s, cents)
		}
		back := FormatAmount(cents)
		again, err := ParseAmount(back)
		if err != nil || again != cents {
			t.Fatalf("%q -> %d -> %q -> %d, %v",
				s, cents, back, again, err)
		}
	})
}

// BenchmarkParseAmount is Technique 70's b.Loop benchmark.
func BenchmarkParseAmount(b *testing.B) {
	inputs := []string{"0.01", "12.34", "99999.99"}
	for b.Loop() {
		for _, s := range inputs {
			ParseAmount(s)
		}
	}
}
