// Package ledger is Technique 8: go generate through go tool, so
// every machine runs the stringer pinned in go.mod.
package ledger

//go:generate go tool stringer -type=Kind -linecomment

type Kind int

const (
	Debit  Kind = iota // debit
	Credit             // credit
)
