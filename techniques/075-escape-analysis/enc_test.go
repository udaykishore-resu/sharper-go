package ledger

import "testing"

// Guard escapes with AllocsPerRun, not by grepping -m output.
// The n-sized buffer gets a small stack buffer and falls back to
// the heap at run time when n is too big.
func TestKeyLenAllocs(t *testing.T) {
	tr := &Transfer{From: "acc_1"}
	for n, want := range map[int]float64{32: 0, 33: 1} {
		got := testing.AllocsPerRun(100, func() { KeyLen(tr, n) })
		if got != want {
			t.Errorf("KeyLen(n=%d): %v allocs, want %v", n, got, want)
		}
	}
}

func TestKeyForAllocsOnce(t *testing.T) {
	tr := &Transfer{From: "acc_1", Amount: 12345}
	if got := testing.AllocsPerRun(100, func() { KeyFor(tr) }); got != 1 {
		t.Fatalf("KeyFor: %v allocs, want 1 (the string)", got)
	}
}
