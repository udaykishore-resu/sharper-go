package validate

import "testing"

type coldType struct {
	Name string `validate:"required"`
	Note string
}

func TestRequired(t *testing.T) {
	if err := Required(coldType{}); err == nil || err.Error() != "Name is required" {
		t.Fatalf("err = %v", err)
	}
	if err := Validate(coldType{Name: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := Validate(TransferReq{From: "a", To: "b"}); err == nil {
		t.Fatal("hot path missed the amount")
	}
}

// NumField panics on a pointer kind: call Elem first, or document
// that Required takes values.
func TestPointerPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("no panic for a pointer")
		}
	}()
	Required(&coldType{})
}

var req = coldType{Name: "x"}

func BenchmarkPlanCached(b *testing.B) {
	for b.Loop() {
		Required(req)
	}
}

func BenchmarkUncached(b *testing.B) {
	for b.Loop() {
		uncached(req)
	}
}

func BenchmarkCheckMethod(b *testing.B) {
	r := TransferReq{From: "a", To: "b", Amount: 1}
	for b.Loop() {
		Validate(r)
	}
}
