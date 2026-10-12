// Package entry is Technique 19: interning with unique.
package entry

import (
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

func parse(line string) Entry {
	f := strings.Split(line, ",")
	amt, _ := strconv.ParseInt(f[2], 10, 64)
	return Entry{f[0], unique.Make(f[1]), amt}
}

// plain is the version without interning, for the benchmark.
type plain struct {
	Account, Currency string
	Amount            int64
}

func parsePlain(line string) plain {
	f := strings.Split(line, ",")
	amt, _ := strconv.ParseInt(f[2], 10, 64)
	return plain{f[0], f[1], amt}
}
