package sum

import (
	"strings"
	"testing"
)

func TestNamedTypes(t *testing.T) {
	b := Batch{500, -5, 0, 250}
	var total Cents = Sum(b) // Sum over []Cents returns Cents
	if total != 745 {
		t.Fatalf("Sum = %d", total)
	}
	var pos Batch = Positive(b) // still a Batch
	if len(pos) != 2 {
		t.Fatalf("Positive = %v", pos)
	}
}

func TestComparableAnyPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil || !strings.Contains(fmtAny(r), "unhashable type") {
			t.Fatalf("recover() = %v", r)
		}
	}()
	s := Set[any]{}
	s.Add([]int{1})
}

func fmtAny(v any) string {
	if e, ok := v.(error); ok {
		return e.Error()
	}
	return ""
}
