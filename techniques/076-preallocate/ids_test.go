package ledger

import (
	"runtime"
	"slices"
	"strconv"
	"testing"
	"weak"
)

func TestIDsAllocs(t *testing.T) {
	if raceOn {
		t.Skip("allocation counts are not meaningful under -race")
	}
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
	if raceOn {
		t.Skip("allocation counts are not meaningful under -race")
	}
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

func touchedNoHint(ts []Transfer) map[string]int {
	m := map[string]int{}
	for _, t := range ts {
		m[t.From]++
		m[t.To]++
	}
	return m
}

// The map hint skips the rebuilds a growing map pays for.
func TestTouchedHintAllocsLess(t *testing.T) {
	if raceOn {
		t.Skip("allocation counts are not meaningful under -race")
	}
	ts := make([]Transfer, 500)
	for i := range ts {
		ts[i].From = strconv.Itoa(2 * i)
		ts[i].To = strconv.Itoa(2*i + 1)
	}
	hint := testing.AllocsPerRun(20, func() { _ = Touched(ts) })
	grow := testing.AllocsPerRun(20, func() { _ = touchedNoHint(ts) })
	if hint >= grow {
		t.Fatalf("hint %v allocs, no hint %v", hint, grow)
	}
	t.Logf("map allocs: hint %v, grow %v", hint, grow)
}

func TestSizeHintClamps(t *testing.T) {
	for in, want := range map[int]int{-5: 0, 0: 0, 10: 10, 1e9: 1000} {
		if got := sizeHint(in); got != want {
			t.Errorf("sizeHint(%d) = %d, want %d", in, got, want)
		}
		_ = make([]string, 0, sizeHint(in)) // never panics
	}
	defer func() {
		if recover() == nil {
			t.Fatal("make with min(-5, 1000) did not panic")
		}
	}()
	n := -5
	_ = make([]string, 0, min(n, 1000))
}

// slices.Clip only limits capacity: the small result still pins
// the whole array. slices.Clone copies and lets the array go.
func TestClipPinsCloneReleases(t *testing.T) {
	pinned := func(keep func([]string) []string) bool {
		arr := new([10000]string)
		wp := weak.Make(arr)
		res := keep(arr[:])
		arr = nil
		for range 5 {
			runtime.GC()
		}
		alive := wp.Value() != nil
		runtime.KeepAlive(res)
		return alive
	}
	if !pinned(func(s []string) []string { return slices.Clip(s[:3]) }) {
		t.Fatal("Clip released the big array")
	}
	if pinned(func(s []string) []string { return slices.Clone(s[:3]) }) {
		t.Fatal("Clone still pins the big array")
	}
}
