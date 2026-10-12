package ledger

import (
	"context"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"testing"
)

// Guard escapes with AllocsPerRun, not by grepping -m output.
// Since Go 1.25 a variable-sized make gets a 32-byte stack
// buffer (cmd/compile's VariableMakeThreshold) and falls back to
// the heap at run time when n is bigger. The threshold is in
// bytes, so it does not depend on the architecture.
func TestKeyLenAllocs(t *testing.T) {
	if raceOn {
		t.Skip("allocation counts are not meaningful under -race")
	}
	tr := &Transfer{From: "acc_1"}
	for n, want := range map[int]float64{8: 0, 32: 0, 33: 1, 64: 1} {
		got := testing.AllocsPerRun(100, func() { KeyLen(tr, n) })
		if got != want {
			t.Errorf("KeyLen(n=%d): %v allocs, want %v", n, got, want)
		}
	}
}

// The constant 64-byte scratch buffer stays on the stack; the only
// allocation is the string that leaves the function.
func TestKeyForAllocsOnce(t *testing.T) {
	if raceOn {
		t.Skip("allocation counts are not meaningful under -race")
	}
	tr := &Transfer{From: "acc_1", Amount: 12345}
	if got := testing.AllocsPerRun(100, func() { KeyFor(tr) }); got != 1 {
		t.Fatalf("KeyFor: %v allocs, want 1 (the string)", got)
	}
}

// Without the separator, ("ab", 12) and ("ab1", 2) both gave "ab12".
func TestKeyForNoCollisions(t *testing.T) {
	pairs := []Transfer{
		{From: "ab", Amount: 12}, {From: "ab1", Amount: 2},
		{From: "a:b", Amount: 1}, {From: "a", Amount: -1},
		{From: "", Amount: 0}, {From: "0", Amount: 0},
		{From: "x:1", Amount: 2}, {From: "x", Amount: 12},
	}
	seen := map[string]Transfer{}
	for _, p := range pairs {
		k := KeyFor(&p)
		if prev, dup := seen[k]; dup {
			t.Fatalf("%+v and %+v share key %q", prev, p, k)
		}
		seen[k] = p
		// Decodable: the last ':' splits it.
		i := strings.LastIndexByte(k, ':')
		amt, err := strconv.ParseInt(k[i+1:], 10, 64)
		if err != nil || k[:i] != p.From || amt != p.Amount {
			t.Fatalf("key %q does not split back to %+v", k, p)
		}
	}
}

func FuzzKeyForInjective(f *testing.F) {
	f.Add("ab", int64(12), "ab1", int64(2))
	f.Fuzz(func(t *testing.T, f1 string, a1 int64, f2 string, a2 int64) {
		k1 := KeyFor(&Transfer{From: f1, Amount: a1})
		k2 := KeyFor(&Transfer{From: f2, Amount: a2})
		if k1 == k2 && (f1 != f2 || a1 != a2) {
			t.Fatalf("collision: %q", k1)
		}
	})
}

var n = int64(123456) // a variable, so the compiler can't fold it

// "Prefer slog.Int64 to loose any arguments" holds only with
// LogAttrs: an Attr passed through Info's ...any is boxed too.
func TestLogAttrsAvoidsBoxing(t *testing.T) {
	if raceOn {
		t.Skip("allocation counts are not meaningful under -race")
	}
	lg := slog.New(slog.NewJSONHandler(io.Discard, nil))
	ctx := context.Background()
	loose := testing.AllocsPerRun(100, func() { lg.Info("t", "n", n) })
	attrs := testing.AllocsPerRun(100, func() {
		lg.LogAttrs(ctx, slog.LevelInfo, "t", slog.Int64("n", n))
	})
	if attrs >= loose {
		t.Fatalf("LogAttrs %v allocs, loose args %v", attrs, loose)
	}
}
