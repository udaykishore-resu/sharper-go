// Package amount is Technique 67's subject: a money parser with a
// round-trip fuzz target.
package amount

import (
	"errors"
	"strconv"
	"strings"
)

var errSyntax = errors.New("amount: bad syntax")

// ParseAmount parses "12.34" into cents. Signs are rejected
// anywhere, so "1.-5" is an error, not 95 cents.
func ParseAmount(s string) (int64, error) {
	whole, frac, hasDot := strings.Cut(s, ".")
	if !digits(whole) || (hasDot && (len(frac) > 2 || !digits(frac))) {
		return 0, errSyntax
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || w > (1<<63-1)/100-1 {
		return 0, errSyntax
	}
	f, _ := strconv.ParseInt(frac+"00"[len(frac):], 10, 64)
	return w*100 + f, nil
}

func digits(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// FormatAmount renders cents as "12.34".
func FormatAmount(cents int64) string {
	s := strconv.FormatInt(cents/100, 10)
	return s + "." + strconv.FormatInt(100+cents%100, 10)[1:]
}

// parseNaive is the bug the fuzzer found in eleven seconds: it
// splits on the dot and parses "-5" as the fraction, so
// "1.-5" is 100 - 5 = 95 cents.
func parseNaive(s string) (int64, error) {
	whole, frac, _ := strings.Cut(s, ".")
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, err
	}
	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, err
	}
	return w*100 + f, nil
}
