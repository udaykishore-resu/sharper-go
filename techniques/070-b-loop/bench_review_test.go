package bloop

import (
	"testing"

	amount "github.com/udaykishore-resu/sharper-go/techniques/067-native-fuzzing"
)

// Review checks for Technique 70, against the testing.B.Loop docs
// in Go 1.27.2 (src/testing/benchmark.go).
func TestBLoopRunsFunctionOnce(t *testing.T) {
	if testing.Short() {
		t.Skip("runs benchmarks")
	}
	calls := 0
	r := testing.Benchmark(func(b *testing.B) {
		calls++
		inputs := []string{"0.01", "12.34", "99999.99"}
		for b.Loop() {
			for _, s := range inputs {
				amount.ParseAmount(s)
			}
		}
	})
	if calls != 1 {
		t.Errorf("b.Loop benchmark ran %d times, want 1", calls)
	}
	// The calls inside the braces were measured, not deleted:
	// three parses cannot cost under a nanosecond.
	if ns := r.NsPerOp(); ns < 1 {
		t.Errorf("%d ns/op: the loop body was optimized away", ns)
	}
	t.Logf("%s", r)
}

func TestBNRunsFunctionManyTimes(t *testing.T) {
	if testing.Short() {
		t.Skip("runs benchmarks")
	}
	calls := 0
	testing.Benchmark(func(b *testing.B) {
		calls++
		for range b.N {
			amount.ParseAmount("12.34")
		}
	})
	if calls < 2 {
		t.Errorf("b.N benchmark ran %d times; expected a ramp-up", calls)
	}
}
