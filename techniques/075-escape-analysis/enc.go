// Package ledger is Technique 75's subject. Ask the compiler:
//
//	go build -gcflags=-m ./075-escape-analysis
package ledger

import "strconv"

type Transfer struct {
	ID, From, To string
	Amount       int64
}

func KeyFor(t *Transfer) string {
	buf := make([]byte, 0, 64)
	buf = append(buf, t.From...)
	buf = strconv.AppendInt(buf, t.Amount, 10)
	return string(buf)
}

func KeyLen(t *Transfer, n int) int {
	buf := make([]byte, 0, n)
	buf = append(buf, t.From...)
	return len(buf)
}
