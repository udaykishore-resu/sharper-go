package batch

import (
	"context"
	"slices"
	"sync"
	"testing"
)

func TestEachGoroutineSeesItsOwnT(t *testing.T) {
	var mu sync.Mutex
	var got []string
	err := postAll(t.Context(), []Transfer{{"t1"}, {"t2"}, {"t3"}},
		func(_ context.Context, tr Transfer) error {
			mu.Lock()
			defer mu.Unlock()
			got = append(got, tr.ID)
			return nil
		})
	slices.Sort(got)
	if err != nil || !slices.Equal(got, []string{"t1", "t2", "t3"}) {
		t.Fatalf("posted %v, %v", got, err)
	}
}

func TestDistinctPointers(t *testing.T) {
	var ps []*int
	for i := range 3 {
		ps = append(ps, &i)
	}
	if ps[0] == ps[1] || *ps[2] != 2 {
		t.Fatal("loop variable shared across passes")
	}
}
