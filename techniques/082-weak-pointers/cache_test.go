package weakcache

import (
	"runtime"
	"testing"
	"time"
)

type statement struct {
	sql string
	_   [64]int // a real object, not a tiny one
}

func TestEntryVanishesWithLastUser(t *testing.T) {
	var c Cache[string, statement] // zero Cache never panics
	if c.Get("missing") != nil {
		t.Fatal("missing key in a nil map")
	}
	s := &statement{sql: "SELECT 1"}
	c.Put("q1", s)
	if c.Get("q1") != s {
		t.Fatal("live value not shared")
	}
	runtime.KeepAlive(s)
	s = nil
	// GC timing is not a contract: retry in a bounded loop.
	for i := 0; c.Len() > 0; i++ {
		if i == 50 {
			t.Fatal("cleanup never removed the dead entry")
		}
		runtime.GC()
		time.Sleep(5 * time.Millisecond)
	}
	if c.Get("q1") != nil {
		t.Fatal("dead value returned")
	}
}

// An old cleanup must leave a newer value under the same key alone.
func TestNewerValueSurvives(t *testing.T) {
	var c Cache[string, statement]
	old := &statement{sql: "old"}
	c.Put("q", old)
	cur := &statement{sql: "new"}
	c.Put("q", cur)
	old = nil
	for range 5 {
		runtime.GC()
		time.Sleep(5 * time.Millisecond)
	}
	if c.Get("q") != cur {
		t.Fatal("old cleanup deleted the newer entry")
	}
	runtime.KeepAlive(cur)
}
