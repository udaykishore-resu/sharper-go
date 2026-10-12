package tlsca

import (
	"sync"
	"testing"
)

func TestLoadCAOnce(t *testing.T) {
	var wg sync.WaitGroup
	pools := make(chan any, 10)
	for range 10 {
		wg.Go(func() {
			p, err := loadCA()
			if err != nil {
				t.Error(err)
			}
			pools <- p
		})
	}
	wg.Wait()
	close(pools)
	first := <-pools
	for p := range pools {
		if p != first {
			t.Fatal("readCA ran more than once")
		}
	}
}

// If f panics, the returned func panics with the same value on
// every call, not only the first.
func TestPanicRepeats(t *testing.T) {
	calls := 0
	tmpl := sync.OnceValue(func() int { calls++; panic("bad template") })
	for range 3 {
		func() {
			defer func() {
				if r := recover(); r != "bad template" {
					t.Fatalf("recover = %v", r)
				}
			}()
			tmpl()
		}()
	}
	if calls != 1 {
		t.Fatalf("f ran %d times, want 1", calls)
	}
}
