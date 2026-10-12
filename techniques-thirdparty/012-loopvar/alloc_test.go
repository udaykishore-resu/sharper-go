package batch

import (
	"sync"
	"testing"
)

var sink []func() int

// A closure called in place keeps the per-pass variable on the
// stack: no allocation.
func TestInPlaceClosureDoesNotAllocate(t *testing.T) {
	if raceOn {
		t.Skip("allocation counts are not meaningful under -race")
	}
	sum := 0
	a := testing.AllocsPerRun(100, func() {
		for i := range 8 {
			f := func() int { return i }
			sum += f()
		}
	})
	if a != 0 {
		t.Fatalf("in-place closures: %v allocs per run", a)
	}
}

// A closure that outlives the pass (stored, or run by a goroutine)
// moves to the heap, once per pass.
func TestEscapingClosureAllocates(t *testing.T) {
	if raceOn {
		t.Skip("allocation counts are not meaningful under -race")
	}
	a := testing.AllocsPerRun(100, func() {
		sink = sink[:0]
		for i := range 8 {
			sink = append(sink, func() int { return i })
		}
	})
	if a < 8 {
		t.Fatalf("stored closures: %v allocs per run, want >= 8", a)
	}
	var wg sync.WaitGroup
	g := testing.AllocsPerRun(20, func() {
		for i := range 8 {
			wg.Go(func() { _ = i })
		}
		wg.Wait()
	})
	if g < 8 {
		t.Fatalf("goroutine closures: %v allocs per run", g)
	}
}
