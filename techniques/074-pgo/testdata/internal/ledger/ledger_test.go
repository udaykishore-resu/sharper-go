package ledger

import "testing"

func BenchmarkWork(b *testing.B) {
	var s Store = MemStore{}
	for b.Loop() {
		Work(s)
	}
}
