// Package report is Technique 83: the same fields in a better
// order take less memory.
package report

type Posting struct {
	Posted   bool
	Amount   int64
	Reversed bool
	Seq      int32
	Currency [3]byte
	Account  string
}

type PostingPacked struct {
	Account  string
	Amount   int64
	Seq      int32
	Currency [3]byte
	Posted   bool
	Reversed bool
}
