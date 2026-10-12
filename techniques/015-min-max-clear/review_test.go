package clamp

import (
	"math"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Zero, negative, the cap, overflow-adjacent and extreme inputs.
func TestBackoffInputs(t *testing.T) {
	cases := map[int]time.Duration{
		math.MinInt: 100 * time.Millisecond,
		-1:          100 * time.Millisecond,
		0:           100 * time.Millisecond,
		5:           3200 * time.Millisecond,
		6:           5 * time.Second, // 6.4s, capped
		7:           5 * time.Second,
		63:          5 * time.Second,
		math.MaxInt: 5 * time.Second,
	}
	for in, want := range cases {
		if got := backoff(in); got != want {
			t.Errorf("backoff(%d) = %v, want %v", in, got, want)
		}
	}
}

// Without the lower clamp, a negative attempt panics.
func TestNegativeShiftPanics(t *testing.T) {
	defer func() {
		r := recover()
		e, ok := r.(runtime.Error)
		if !ok || !strings.Contains(e.Error(), "negative shift") {
			t.Fatalf("recover() = %v", r)
		}
	}()
	unclamped(-1)
}

func TestPageSizeExtremes(t *testing.T) {
	for in, want := range map[int]int{
		math.MinInt: 1, math.MaxInt: 500, 1: 1, 499: 499,
	} {
		if got := pageSize(in); got != want {
			t.Errorf("pageSize(%d) = %d, want %d", in, got, want)
		}
	}
}

// Any NaN argument makes min and max return NaN, in any position.
func TestNaN(t *testing.T) {
	nan, one := math.NaN(), 1.0
	for _, v := range []float64{
		min(nan, one), min(one, nan), max(nan, one), max(one, nan),
		min(one, 2, nan), max(nan, 2, one),
	} {
		if !math.IsNaN(v) {
			t.Fatalf("got %v, want NaN", v)
		}
	}
}

func fill(m map[int]bool, n int) {
	for i := range n {
		m[i] = true
	}
}

// clear keeps the map's grown storage: refilling it allocates
// nothing, where a fresh map allocates as it grows.
func TestClearKeepsStorage(t *testing.T) {
	if raceOn {
		t.Skip("allocation counts are not meaningful under -race")
	}
	m := map[int]bool{}
	fill(m, 1000)
	reuse := testing.AllocsPerRun(10, func() { clear(m); fill(m, 1000) })
	fresh := testing.AllocsPerRun(10, func() {
		fill(map[int]bool{}, 1000)
	})
	if reuse != 0 || fresh == 0 {
		t.Fatalf("reuse %v allocs, fresh %v allocs", reuse, fresh)
	}
}

var spike map[int]bool

// The flip side: after a spike, clear keeps the heap the spike
// grew; replacing the map lets the GC take it back.
func TestClearKeepsSpikeMemory(t *testing.T) {
	heap := func() uint64 {
		var ms runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&ms)
		return ms.HeapAlloc
	}
	base := heap()
	spike = map[int]bool{}
	fill(spike, 500_000)
	full := heap() - base
	clear(spike)
	cleared := heap() - base
	spike = map[int]bool{}
	replaced := max(heap(), base) - base
	if cleared < full/2 || replaced > full/4 {
		t.Fatalf("full %d, cleared %d, replaced %d bytes",
			full, cleared, replaced)
	}
}
