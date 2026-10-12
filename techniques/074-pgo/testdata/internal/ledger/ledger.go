package ledger

import "strconv"

// Store has one real implementation: the case PGO devirtualizes.
type Store interface{ Balance(id string) int64 }

type MemStore struct{}

func (MemStore) Balance(id string) int64 {
	var h int64
	for i := 0; i < len(id); i++ {
		h = h*31 + int64(id[i])
	}
	return h
}

func Work(s Store) (sum int64) {
	for i := range 1000 {
		sum += s.Balance("acc_" + strconv.Itoa(i))
	}
	return sum
}
