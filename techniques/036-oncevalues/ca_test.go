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
