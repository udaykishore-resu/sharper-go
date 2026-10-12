package entry

import (
	"encoding/json"
	"testing"
)

func TestHandles(t *testing.T) {
	e := parse("acc_1,USD,500")
	if e.Currency != usd || e.Currency.Value() != "USD" {
		t.Fatalf("got %v", e.Currency.Value())
	}
	totals := map[Currency]int64{}
	totals[e.Currency] += e.Amount // a cheap map key
	if totals[usd] != 500 {
		t.Fatal("handle is not a usable key")
	}
}

// A handle has no exported fields: it marshals as {} with no error.
func TestHandleMarshalsEmpty(t *testing.T) {
	//lint:ignore SA9005 this is the Breaks when being demonstrated
	b, err := json.Marshal(usd)
	if err != nil || string(b) != "{}" {
		t.Fatalf("%s, %v", b, err)
	}
}

func TestZeroHandlePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("zero handle Value did not panic")
		}
	}()
	var e Entry // skipped parse
	_ = e.Currency.Value()
}

// Measure before you keep it: compare with -benchmem and benchstat.
func BenchmarkParseInterned(b *testing.B) {
	for b.Loop() {
		parse("acc_1,USD,500")
	}
}

func BenchmarkParsePlain(b *testing.B) {
	for b.Loop() {
		parsePlain("acc_1,USD,500")
	}
}
