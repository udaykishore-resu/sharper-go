package ledgerctl

import (
	"errors"
	"flag"
	"testing"
)

func TestDispatch(t *testing.T) {
	if err := dispatch(t.Context(), []string{"replay", "-dry-run=false", "f.ndjson"}); err != nil {
		t.Fatal(err)
	}
	if lastReplay.dryRun || lastReplay.file != "f.ndjson" {
		t.Fatalf("parsed %+v", lastReplay)
	}
	if err := dispatch(t.Context(), []string{"bogus"}); err == nil {
		t.Fatal("unknown command accepted")
	}
}

// A flag after a positional argument is not parsed; reject the
// leftovers instead of leaving dry-run on silently.
func TestFlagAfterPositional(t *testing.T) {
	err := dispatch(t.Context(), []string{"replay", "f.ndjson", "-dry-run=false"})
	if err == nil {
		t.Fatal("leftover flag accepted")
	}
}

// -h returns flag.ErrHelp; main should exit 0 on it.
func TestHelp(t *testing.T) {
	err := dispatch(t.Context(), []string{"verify", "-h"})
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("err = %v", err)
	}
}
