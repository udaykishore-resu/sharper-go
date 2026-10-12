package weakcache

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"
)

// Replace one key many times while the GC runs: every old value's
// cleanup fires, and none of them deletes the current entry.
func TestRepeatedReplacement(t *testing.T) {
	var c Cache[string, statement]
	var cur *statement
	for i := range 200 {
		cur = &statement{sql: fmt.Sprint(i)}
		c.Put("q", cur)
		if i%20 == 0 {
			runtime.GC()
		}
	}
	for range 10 {
		runtime.GC()
		time.Sleep(2 * time.Millisecond)
		if got := c.Get("q"); got != cur {
			t.Fatalf("Get = %v, want the latest value", got)
		}
	}
	if c.Len() != 1 {
		t.Fatalf("map holds %d entries", c.Len())
	}
	runtime.KeepAlive(cur)
}

// The same pointer put twice gets two cleanups; both are harmless.
func TestSamePointerTwice(t *testing.T) {
	var c Cache[string, statement]
	s := &statement{sql: "x"}
	c.Put("q", s)
	c.Put("q", s)
	runtime.KeepAlive(s)
	s = nil
	for i := 0; c.Len() > 0; i++ {
		if i == 50 {
			t.Fatal("entry never cleaned up")
		}
		runtime.GC()
		time.Sleep(5 * time.Millisecond)
	}
}

// Concurrent Get and Put on shared keys, with cleanups running.
// Run with -race. Whatever a Get returns must be a value some Put
// stored under that key.
func TestConcurrentGetPut(t *testing.T) {
	var c Cache[int, statement]
	var wg sync.WaitGroup
	for w := range 8 {
		wg.Go(func() {
			keep := make([]*statement, 0, 100)
			for i := range 500 {
				k := i % 5
				if v := c.Get(k); v != nil && v.sql[0] != byte('0'+k) {
					t.Errorf("key %d holds %q", k, v.sql)
					return
				}
				s := &statement{sql: fmt.Sprintf("%d-%d-%d", k, w, i)}
				c.Put(k, s)
				if i%5 == 0 {
					keep = append(keep, s)
				}
				if i%100 == 0 {
					runtime.GC()
				}
			}
			runtime.KeepAlive(keep)
		})
	}
	wg.Wait()
	for i := 0; c.Len() > 0; i++ {
		if i == 100 {
			t.Fatalf("%d dead entries left", c.Len())
		}
		runtime.GC()
		time.Sleep(5 * time.Millisecond)
	}
}
