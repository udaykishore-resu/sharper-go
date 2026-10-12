package clamp

import (
	"math"
	"testing"
)

func TestPageSize(t *testing.T) {
	for in, want := range map[int]int{0: 1, 50: 50, 9999: 500, -7: 1, 500: 500, 501: 500} {
		if got := pageSize(in); got != want {
			t.Errorf("pageSize(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestBackoffOverflow(t *testing.T) {
	if d := backoff(37); d <= 0 || d > 5e9 {
		t.Fatalf("backoff(37) = %v", d)
	}
	if d := unclamped(37); d >= 0 {
		t.Fatalf("expected the shift to wrap negative, got %v", d)
	}
}

// clear empties a map with a NaN key, which a delete loop cannot.
func TestClearNaN(t *testing.T) {
	m := map[float64]int{math.NaN(): 1, 1: 2}
	for k := range m {
		delete(m, k)
	}
	if len(m) != 1 {
		t.Fatalf("delete loop left %d entries, want 1 (the NaN)", len(m))
	}
	clear(m)
	if len(m) != 0 {
		t.Fatal("clear left entries")
	}
	s := []int{1, 2}
	clear(s)
	if len(s) != 2 || s[0] != 0 {
		t.Fatalf("clear on a slice: %v", s)
	}
}
