// Package report is Technique 14: slices and maps instead of
// hand-written loops.
package report

import (
	"cmp"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
)

// Transfer is a stand-in for ledger.Transfer.
type Transfer struct {
	ID, From, To string
	Amount       int64
}

// Write sorts big transfers first, drops zero amounts and prints
// per-account totals in key order.
func Write(w io.Writer, ts []Transfer,
	byAcct map[string]int64) []Transfer {
	slices.SortFunc(ts, func(a, b Transfer) int {
		return cmp.Or(
			cmp.Compare(b.Amount, a.Amount), // big first
			strings.Compare(a.ID, b.ID),     // then by ID
		)
	})

	ts = slices.DeleteFunc(ts, func(t Transfer) bool {
		return t.Amount == 0
	})

	for _, id := range slices.Sorted(maps.Keys(byAcct)) {
		fmt.Fprintf(w, "%s %d\n", id, byAcct[id])
	}
	return ts
}
