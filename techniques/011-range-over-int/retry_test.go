package retry

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestPostWithRetry(t *testing.T) {
	calls := 0
	c := &Client{Post: func(context.Context, Transfer) error {
		calls++
		return errors.New("503")
	}}
	if err := postWithRetry(t.Context(), c, Transfer{"t1"}); err == nil || calls != 3 {
		t.Fatalf("err=%v calls=%d, want 3 attempts", err, calls)
	}
}

// The bound is read once: a queue that grows is left behind.
func TestBoundReadOnce(t *testing.T) {
	q := []int{1, 2}
	n := 0
	for i := range len(q) {
		if i == 0 {
			q = append(q, 3)
		}
		n++
	}
	if n != 2 || len(q) != 3 {
		t.Fatalf("ran %d times over a queue of %d", n, len(q))
	}
}

// Writing to the counter is thrown away. The old skip printed
// "0 2 3 4"; the same body under range prints "0 2 2 3 4".
func TestCounterWriteIgnored(t *testing.T) {
	var old, got string
	for i := 0; i < 5; i++ {
		if i == 1 {
			i++ // the old skip
		}
		old += fmt.Sprint(i, " ")
	}
	for i := range 5 {
		if i == 1 {
			i++ // lost at the next pass
		}
		got += fmt.Sprint(i, " ")
	}
	if old != "0 2 3 4 " || got != "0 2 2 3 4 " {
		t.Fatalf("three-clause %q, range %q", old, got)
	}
}
