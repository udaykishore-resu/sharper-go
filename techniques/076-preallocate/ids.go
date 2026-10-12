// Package ledger is Technique 76: preallocate when you know the
// count, and lock the win in with AllocsPerRun.
package ledger

type Transfer struct {
	ID, From, To string
	Amount       int64
}

func IDs(ts []Transfer) []string {
	ids := make([]string, 0, len(ts))
	for _, t := range ts {
		ids = append(ids, t.ID)
	}
	return ids
}

// Touched sizes the map for the worst case: two accounts each.
func Touched(ts []Transfer) map[string]int {
	m := make(map[string]int, 2*len(ts))
	for _, t := range ts {
		m[t.From]++
		m[t.To]++
	}
	return m
}
