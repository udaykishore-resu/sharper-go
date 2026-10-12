package httpapi

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// The test from Breaks when: no exported sentinel maps to 500.
func TestEverySentinelHasARow(t *testing.T) {
	for _, err := range []error{ledger.ErrNotFound, ledger.ErrFunds, ledger.ErrInvalid} {
		if classify(fmt.Errorf("post: %w", err)).status == 500 {
			t.Errorf("%v falls through to 500", err)
		}
	}
}

func TestOrderDecidesJoinedErrors(t *testing.T) {
	err := errors.Join(ledger.ErrInvalid, ledger.ErrNotFound)
	if got := classify(err); got.status != 404 {
		t.Fatalf("first matching row wins, got %+v", got)
	}
}

func TestContextRows(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	<-ctx.Done()
	if got := classify(fmt.Errorf("store: %w", ctx.Err())); got.status != 503 {
		t.Fatalf("deadline: %+v", got)
	}
	// Hack 21 translates driver errors with %v, which cuts the
	// chain: a timeout then becomes a 500.
	if got := classify(fmt.Errorf("store: %v", ctx.Err())); got.status != 500 {
		t.Fatalf("cut chain: %+v", got)
	}
}
