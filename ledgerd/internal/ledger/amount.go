package ledger

import (
	"errors"
	"strconv"
	"strings"
)

var errAmount = invalid("bad amount syntax")

// ParseAmount parses a non-negative decimal amount such as
// "12.34" into cents. It accepts at most two fraction digits
// and rejects signs anywhere, so "1.-5" is an error
// (Technique 67).
func ParseAmount(s string) (int64, error) {
	whole, frac, hasDot := strings.Cut(s, ".")
	if whole == "" || !digits(whole) {
		return 0, errAmount
	}
	if hasDot && (frac == "" || len(frac) > 2 || !digits(frac)) {
		return 0, errAmount
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || w > (1<<63-1)/100-1 {
		return 0, errors.Join(errAmount, err)
	}
	var f int64
	if hasDot {
		f, _ = strconv.ParseInt(frac, 10, 64)
		if len(frac) == 1 {
			f *= 10
		}
	}
	return w*100 + f, nil
}

func digits(s string) bool {
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// FormatAmount renders cents as "12.34".
func FormatAmount(cents int64) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	b := strconv.AppendInt(nil, cents/100, 10)
	b = append(b, '.', byte('0'+cents%100/10), byte('0'+cents%10))
	if neg {
		return "-" + string(b)
	}
	return string(b)
}
