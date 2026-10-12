package stream

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func noop(Transfer) error { return nil }

func TestRejectsTrailingData(t *testing.T) {
	for _, in := range []string{`[{"amount":1}] x`, `[] {}`, `[]]`} {
		if _, err := EachTransfer(strings.NewReader(in), noop); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
	if n, err := EachTransfer(strings.NewReader("[{\"amount\":1}]\n "), noop); err != nil || n != 1 {
		t.Fatalf("trailing whitespace: n=%d err=%v", n, err)
	}
}

func TestRejectsNonArray(t *testing.T) {
	for _, in := range []string{`{"amount":1}`, `7`, ``} {
		if n, err := EachTransfer(strings.NewReader(in), noop); err == nil || n != 0 {
			t.Errorf("%q: n=%d err=%v", in, n, err)
		}
	}
}

// Malformed JSON near the end of a large stream: n counts the
// good items before it, and the error says where it broke.
func TestMalformedNearEnd(t *testing.T) {
	body, _ := io.ReadAll(gen(20_000))
	// Drop the closing bracket and append a truncated item.
	body = append(body[:len(body)-1], `,{"amount":`...)
	n, err := EachTransfer(strings.NewReader(string(body)), noop)
	if err == nil || n != 20_000 ||
		!strings.HasPrefix(err.Error(), "item 20000:") {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

// Cancellation halfway: fn reports ctx.Err() and decoding stops
// at that item.
func TestCancelHalfway(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	seen := 0
	n, err := EachTransfer(gen(10_000), func(Transfer) error {
		if seen++; seen == 5_000 {
			cancel()
		}
		return ctx.Err()
	})
	if !errors.Is(err, context.Canceled) || n != 4_999 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

// A failed decode cannot leave a partial import reported as
// complete: two batches committed, the third is dropped, and
// Import returns the committed count with the error.
func TestPartialImportReportsFailure(t *testing.T) {
	body, _ := io.ReadAll(gen(250))
	bad := strings.Replace(string(body), `"id":"t_230"`, `"id":t_230`, 1)
	var committed []Transfer
	done, err := Import(strings.NewReader(bad), 100, func(b []Transfer) error {
		committed = append(committed, b...)
		return nil
	})
	if err == nil || done != 200 || len(committed) != 200 {
		t.Fatalf("done=%d committed=%d err=%v", done, len(committed), err)
	}
	done, err = Import(gen(250), 100, func(b []Transfer) error { return nil })
	if err != nil || done != 250 {
		t.Fatalf("clean import: done=%d err=%v", done, err)
	}
}
