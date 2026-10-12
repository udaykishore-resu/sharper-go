package obs

import (
	"sync"
	"testing"
)

func TestZeroValue(t *testing.T) {
	var c Counters
	if c.Get("x") != 0 {
		t.Fatal("read of nil map")
	}
	var s Server
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() { s.Counters.Add("transfers", 1) })
	}
	wg.Wait()
	if got := s.Counters.Get("transfers"); got != 50 {
		t.Fatalf("got %d, want 50", got)
	}
}

func TestNilPointerPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("a nil *Counters is not a zero struct")
		}
	}()
	var c *Counters
	c.Add("x", 1)
}
