package limit

import (
	"errors"
	"fmt"
	"testing"
)

func TestBoth(t *testing.T) {
	err := fmt.Errorf("post t_88: %w",
		&LimitError{Account: "acc_19", Limit: 500, Requested: 900})
	if !errors.Is(err, ErrLimit) {
		t.Fatal("errors.Is(ErrLimit) = false after wrapping")
	}
	var le *LimitError
	if !errors.As(err, &le) || le.Limit != 500 {
		t.Fatalf("errors.As = %+v", le)
	}
	if le2, ok := errors.AsType[*LimitError](err); !ok || le2 != le {
		t.Fatal("AsType (Go 1.26) disagrees with As")
	}
}

// With methods on *LimitError, a LimitError target does not
// implement error: errors.As panics, and go vet's errorsas check
// rejects the call at compile time. (The target goes through an
// any here only to get past vet.)
func TestValueTargetPanics(t *testing.T) {
	err := fmt.Errorf("x: %w", &LimitError{})
	defer func() {
		if recover() == nil {
			t.Fatal("errors.As accepted a LimitError target")
		}
	}()
	var le LimitError
	var target any = &le
	errors.As(err, target)
}

// valueLimit has a value receiver, so a valueLimit target compiles,
// but it never matches the *valueLimit that was returned, and
// nothing tells you.
type valueLimit struct{}

func (valueLimit) Error() string { return "limit" }

func TestValueReceiverNeverMatchesPointer(t *testing.T) {
	err := fmt.Errorf("x: %w", &valueLimit{})
	var v valueLimit
	if errors.As(err, &v) {
		t.Fatal("a valueLimit target matched a *valueLimit")
	}
	if _, ok := errors.AsType[*LimitError](fmt.Errorf("x: %w", ErrFrozen)); ok {
		t.Fatal("a sentinel is not a *LimitError")
	}
}
