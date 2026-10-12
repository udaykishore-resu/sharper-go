package amount

import (
	"strconv"
	"strings"
	"testing"
)

// Review check for Technique 67: the printed property, applied to
// the story's naive parser, fails on "1.-5" through the whole-part
// rule, even though the round trip agrees with itself.
func TestNaiveFailsWholePartRule(t *testing.T) {
	s := "1.-5"
	cents, err := parseNaive(s)
	if err != nil {
		t.Fatalf("parseNaive(%q) rejected it: %v", s, err)
	}
	whole, _, _ := strings.Cut(s, ".")
	w, _ := strconv.ParseInt(whole, 10, 64)
	if !(cents < 0 || cents/100 != w) {
		t.Fatalf("property missed %q -> %d", s, cents)
	}
}

// Boundary and invalid-UTF-8 inputs, also kept as seed corpus
// files under testdata/fuzz/FuzzParseAmount.
func TestParseAmountBoundaries(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"92233720368547757.99", 9223372036854775799, true},
		{"92233720368547758.00", 0, false},
		{"0.00", 0, true},
		{"007.5", 750, true},
		{"1.", 0, false},
		{"\xff1.00", 0, false},
		{"١.٠٠", 0, false},
	}
	for _, c := range cases {
		got, err := ParseAmount(c.in)
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("ParseAmount(%q) = %d, %v", c.in, got, err)
		}
	}
}
