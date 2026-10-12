package transfer

import (
	"encoding/json"
	"testing"
)

func enc(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestOmitzero(t *testing.T) {
	pending := TransferOut{ID: "t1", Amount: 500,
		Fee: Money{Currency: "USD"}, Tags: []string{}}
	if got := enc(t, pending); got != `{"id":"t1","amount":500}` {
		t.Fatalf("got %s", got)
	}
	if got := enc(t, Old{}); got != `{"posted_at":"0001-01-01T00:00:00Z"}` {
		t.Fatalf("omitempty never omits a struct: %s", got)
	}
	if got := enc(t, OnlyZero{Tags: []string{}}); got != `{"tags":[]}` {
		t.Fatalf("empty slice: %s", got)
	}
	if got := enc(t, OnlyZero{}); got != `{}` {
		t.Fatalf("nil slice: %s", got)
	}
	if got := enc(t, PtrFee{}); got != `{}` {
		t.Fatalf("nil pointer: %s", got)
	}
	if got := enc(t, PtrFee{Fee: &Money{Currency: "USD"}}); got != `{}` {
		t.Fatalf("pointer to zero Money: %s", got)
	}
}
