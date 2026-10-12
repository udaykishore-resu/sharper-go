// Package ledger is Technique 75's subject. Ask the compiler:
//
//	go build -gcflags=-m ./075-escape-analysis
package ledger

import "strconv"

type Transfer struct {
	ID, From, To string
	Amount       int64
}

// KeyFor puts a ':' between the parts. The amount never contains
// one, so the last ':' splits any key back into From and Amount:
// ("ab", 12) and ("ab1", 2) give different keys.
func KeyFor(t *Transfer) string {
	buf := make([]byte, 0, 64)
	buf = append(buf, t.From...)
	buf = append(buf, ':')
	buf = strconv.AppendInt(buf, t.Amount, 10)
	return string(buf)
}

func KeyLen(t *Transfer, n int) int {
	buf := make([]byte, 0, n)
	buf = append(buf, t.From...)
	return len(buf)
}
