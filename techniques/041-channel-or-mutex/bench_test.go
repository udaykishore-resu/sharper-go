package balances

import (
	"fmt"
	"sync"
	"testing"
)

var keys = func() []string {
	k := make([]string, 5)
	for i := range k {
		k[i] = fmt.Sprint("acc_", i)
	}
	return k
}()

// Both versions under the same contended workload: every
// goroutine adds to one of 5 keys. Run with -cpu=1,4,16 and
// compare with benchstat before you rewrite anything.
func BenchmarkMutexAdd(b *testing.B) {
	m := &balances{m: map[string]int64{}}
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			m.Add(keys[i%5], 1)
			i++
		}
	})
}

func BenchmarkChannelAdd(b *testing.B) {
	o := newOwned()
	defer o.Close()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			o.Add(keys[i%5], 1)
			i++
		}
	})
}

// The channel version must be correct too, or the comparison is
// meaningless.
func TestOwnedHammer(t *testing.T) {
	o := newOwned()
	defer o.Close()
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Go(func() {
			for range 100 {
				o.Add(keys[i%5], 1)
			}
		})
	}
	wg.Wait()
	for _, k := range keys {
		if got := o.Get(k); got != 1000 {
			t.Fatalf("%s = %d, want 1000", k, got)
		}
	}
}
