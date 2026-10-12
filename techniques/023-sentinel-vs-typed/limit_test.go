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

// With methods on *LimitError, a non-pointer target never matches,
// and nothing tells you.
type valueOnly struct{}

func (valueOnly) Error() string { return "value" }

func TestWrongPointerLevel(t *testing.T) {
	err := fmt.Errorf("x: %w", &LimitError{})
	var wrong valueOnly
	if errors.As(err, &wrong) {
		t.Fatal("matched the wrong type")
	}
	if _, ok := errors.AsType[*LimitError](fmt.Errorf("x: %w", ErrFrozen)); ok {
		t.Fatal("a sentinel is not a *LimitError")
	}
}
