package amount

import (
	"strconv"
	"strings"
	"testing"
)

func FuzzParseAmount(f *testing.F) {
	for _, s := range []string{"0", "12.34", "-3"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		cents, err := ParseAmount(s)
		if err != nil {
			return // rejecting input is fine
		}
		whole, _, _ := strings.Cut(s, ".")
		w, _ := strconv.ParseInt(whole, 10, 64)
		if cents < 0 || cents/100 != w {
			t.Fatalf("%q -> %d cents", s, cents)
		}
		back := FormatAmount(cents)
		again, err := ParseAmount(back)
		if err != nil || again != cents {
			t.Fatalf("%q -> %d -> %q -> %d, %v",
				s, cents, back, again, err)
		}
	})
}

// The story's bug: "1.-5" became a positive 95 cents, and a
// round trip alone agrees with itself.
func TestNaiveParserBug(t *testing.T) {
	got, err := parseNaive("1.-5")
	if err != nil || got != 95 {
		t.Fatalf("parseNaive(1.-5) = %d, %v", got, err)
	}
	// The round trip alone would have passed: 95 cents formats
	// and parses back to 95 cents. The whole-part rule catches it
	// (TestNaiveFailsWholePartRule).
	if back, err := ParseAmount(FormatAmount(got)); err != nil || back != got {
		t.Fatalf("round trip of %d: %d, %v", got, back, err)
	}
	if _, err := ParseAmount("1.-5"); err == nil {
		t.Fatal("ParseAmount accepted 1.-5")
	}
}
