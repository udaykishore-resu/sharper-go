package ledger

import "testing"

func TestIDsAllocs(t *testing.T) {
	ts := make([]Transfer, 500)
	n := testing.AllocsPerRun(100, func() {
		_ = IDs(ts)
	})
	if n > 1 {
		t.Fatalf("IDs: %v allocs, want 1", n)
	}
}

func idsGrow(ts []Transfer) []string {
	var ids []string
	for _, t := range ts {
		ids = append(ids, t.ID)
	}
	return ids
}

func TestGrowingCostsMore(t *testing.T) {
	ts := make([]Transfer, 500)
	if n := testing.AllocsPerRun(100, func() { _ = idsGrow(ts) }); n < 5 {
		t.Fatalf("growing from zero took only %v allocs", n)
	}
}

func BenchmarkIDs(b *testing.B) {
	ts := make([]Transfer, 5000)
	for b.Loop() {
		IDs(ts)
	}
}
