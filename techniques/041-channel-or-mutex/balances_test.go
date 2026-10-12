package balances

import (
	"fmt"
	"sync"
	"testing"
)

// Hammer it from 50 goroutines on 5 keys under -race (Technique 42).
func TestHammer(t *testing.T) {
	b := &balances{m: map[string]int64{}}
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Go(func() {
			for range 100 {
				b.Add(fmt.Sprint("acc_", i%5), 1)
			}
		})
	}
	wg.Wait()
	for k := range 5 {
		if got := b.Get(fmt.Sprint("acc_", k)); got != 1000 {
			t.Fatalf("acc_%d = %d, want 1000", k, got)
		}
	}
}
