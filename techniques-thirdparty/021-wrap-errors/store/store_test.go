package store

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/udaykishore-resu/sharper-go/techniques-thirdparty/021-wrap-errors/ledger"
)

func TestWrapVersusCut(t *testing.T) {
	kept := fmt.Errorf("post transfer t_88: %w",
		fmt.Errorf("account acc_19: %w", ledger.ErrNotFound))
	if !errors.Is(kept, ledger.ErrNotFound) {
		t.Fatal("the chain built with the w verb is broken")
	}
	if kept.Error() != "post transfer t_88: account acc_19: ledger: not found" {
		t.Fatalf("chain reads %q", kept)
	}
	cut := fmt.Errorf("account acc_19: %v", pgx.ErrNoRows)
	if errors.Is(cut, pgx.ErrNoRows) {
		t.Fatal("the v verb kept the chain")
	}
}

// Passing a nil error to %w gives a non-nil error.
func TestNilWrap(t *testing.T) {
	var err error
	got := fmt.Errorf("account: %w", err)
	if got == nil || got.Error() != "account: %!w(<nil>)" {
		t.Fatalf("got %v", got)
	}
}
