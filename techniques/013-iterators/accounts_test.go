package accounts

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type fakePager struct {
	n, calls int
	failAt   int
}

func (f *fakePager) Page(_ context.Context, after string, n int) ([]Account, error) {
	f.calls++
	if f.failAt > 0 && f.calls == f.failAt {
		return nil, errors.New("failover")
	}
	start := 0
	if after != "" {
		fmt.Sscanf(after, "a%07d", &start)
		start++
	}
	var out []Account
	for i := start; i < f.n && len(out) < n; i++ {
		out = append(out, Account{ID: fmt.Sprintf("a%07d", i), Balance: 1})
	}
	return out, nil
}

// Break on the tenth account and the store makes one call.
func TestBreakStopsFetching(t *testing.T) {
	p := &fakePager{n: 2000}
	seen := 0
	for _, err := range Accounts(t.Context(), p) {
		if err != nil {
			t.Fatal(err)
		}
		if seen++; seen == 10 {
			break
		}
	}
	if p.calls != 1 {
		t.Fatalf("pages fetched = %d, want 1", p.calls)
	}
}

func TestTotal(t *testing.T) {
	if got, err := Total(t.Context(), &fakePager{n: 1234}); err != nil || got != 1234 {
		t.Fatalf("Total = %d, %v", got, err)
	}
	if _, err := Total(t.Context(), &fakePager{n: 1234, failAt: 2}); err == nil {
		t.Fatal("page error dropped")
	}
}

// The trap: one loop variable on a Seq2 drops the error.
func TestOneVariableDropsError(t *testing.T) {
	var total int64
	for a := range Accounts(t.Context(), &fakePager{n: 1234, failAt: 2}) {
		total += a.Balance
	}
	if total != 500 {
		t.Fatalf("total = %d; the error went unnoticed after page 1", total)
	}
}
