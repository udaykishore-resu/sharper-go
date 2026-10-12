package report

import (
	"slices"
	"strings"
	"testing"
)

func TestWrite(t *testing.T) {
	var b strings.Builder
	ts := []Transfer{{ID: "t2", Amount: 5}, {ID: "t1", Amount: 0},
		{ID: "t3", Amount: 9}, {ID: "t0", Amount: 5}}
	got := Write(&b, ts, map[string]int64{"b": 2, "a": 1})
	ids := make([]string, 0, len(got))
	for _, t := range got {
		ids = append(ids, t.ID)
	}
	if !slices.Equal(ids, []string{"t3", "t0", "t2"}) || b.String() != "a 1\nb 2\n" {
		t.Fatalf("ids=%v out=%q", ids, b.String())
	}
}

// Insert follows append's rules and writes into a shared array.
func TestInsertWritesSharedArray(t *testing.T) {
	a := []int{1, 2, 3}
	s := a[:2]
	s = slices.Insert(s, 0, 9)
	if !slices.Equal(a, []int{9, 1, 2}) || len(s) != 3 {
		t.Fatalf("a = %v", a)
	}
	a = []int{1, 2, 3}
	_ = slices.Insert(slices.Clip(a[:2]), 0, 9)
	if !slices.Equal(a, []int{1, 2, 3}) {
		t.Fatalf("Clip did not make Insert reallocate: %v", a)
	}
}

// Clip only caps capacity; the clipped slice still shares a's
// elements. Clone copies them, so writes stay local.
func TestClipSharesCloneIsolates(t *testing.T) {
	a := []int{1, 2, 3}
	c := slices.Clip(a[:2])
	c[0] = 7
	if a[0] != 7 {
		t.Fatalf("Clip copied: a = %v", a)
	}
	a = []int{1, 2, 3}
	k := slices.Clone(a[:2])
	k[0] = 7
	k = slices.Insert(k, 0, 9)
	if !slices.Equal(a, []int{1, 2, 3}) || !slices.Equal(k, []int{9, 7, 2}) {
		t.Fatalf("a = %v, k = %v", a, k)
	}
}

// Since Go 1.22, Delete zeroes the tail the old header still sees.
func TestDeleteZeroesTail(t *testing.T) {
	s := []int{1, 2, 3}
	_ = slices.Delete(s, 0, 1)
	if !slices.Equal(s, []int{2, 3, 0}) {
		t.Fatalf("old header sees %v", s)
	}
}
