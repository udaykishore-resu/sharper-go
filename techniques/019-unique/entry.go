// Package entry is Technique 19: interning with unique.
package entry

import (
	"fmt"
	"strconv"
	"strings"
	"unique"
)

// Currency is an interned ISO 4217 code.
type Currency = unique.Handle[string]

type Entry struct {
	Account  string
	Currency Currency
	Amount   int64
}

var usd = unique.Make("USD")

func parse(line string) (Entry, error) {
	f := strings.Split(line, ",")
	if len(f) != 3 {
		return Entry{}, fmt.Errorf("bad row: %q", line)
	}
	amt, err := strconv.ParseInt(f[2], 10, 64)
	if err != nil {
		return Entry{}, fmt.Errorf("amount: %w", err)
	}
	return Entry{f[0], unique.Make(f[1]), amt}, nil
}

// plain is the version without interning, for the benchmark.
type plain struct {
	Account, Currency string
	Amount            int64
}

func parsePlain(line string) (plain, error) {
	f := strings.Split(line, ",")
	if len(f) != 3 {
		return plain{}, fmt.Errorf("bad row: %q", line)
	}
	amt, err := strconv.ParseInt(f[2], 10, 64)
	if err != nil {
		return plain{}, fmt.Errorf("amount: %w", err)
	}
	return plain{f[0], f[1], amt}, nil
}
