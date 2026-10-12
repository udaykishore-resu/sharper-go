package validate

import (
	"fmt"
	"sync"
	"testing"
)

type coldType struct {
	Name string `validate:"required"`
	Note string
}

type Base struct {
	Owner string `validate:"required"`
}

// embedded: the plan reads top-level tags only, so Base.Owner is
// not checked. Cached and uncached agree on that.
type embedded struct {
	Base
	ID string `validate:"required"`
}

type ptrChecker struct{ X int }

func (p *ptrChecker) Check() error { return fmt.Errorf("checked") }

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

func errString(f func(any) error, in any) (s string) {
	defer func() {
		if r := recover(); r != nil {
			s = fmt.Sprint("panic: ", r)
		}
	}()
	if err := f(in); err != nil {
		return err.Error()
	}
	return "ok"
}

// The plan cache changes speed, not answers: same result as the
// uncached walk for values, pointers, nil, non-structs and
// embedded structs. No input panics.
func TestCachedMatchesUncached(t *testing.T) {
	var nilCold *coldType
	inputs := []any{
		coldType{}, coldType{Name: "x"}, &coldType{}, &coldType{Name: "x"},
		nilCold, nil, 42, "str", []int{1},
		embedded{}, embedded{ID: "e1"},
		TransferReq{}, TransferReq{From: "a", To: "b", Amount: 1},
	}
	for _, in := range inputs {
		got, want := errString(Required, in), errString(uncached, in)
		if got != want {
			t.Errorf("%#v: cached %q, uncached %q", in, got, want)
		}
		if len(got) > 5 && got[:6] == "panic:" {
			t.Errorf("%#v: %s", in, got)
		}
	}
	if got := errString(Required, embedded{ID: "e1"}); got != "ok" {
		t.Errorf("embedded tags: %s", got)
	}
}

// The hand-written Check gives the same answers as the plan.
func TestCheckMatchesPlan(t *testing.T) {
	for _, r := range []TransferReq{
		{}, {From: "a"}, {From: "a", To: "b"}, {To: "b", Amount: 1},
		{From: "a", To: "b", Amount: -1}, {From: "a", To: "b", Amount: 1},
	} {
		if a, b := errString(Validate, r), errString(Required, r); a != b {
			t.Errorf("%+v: Check %q, plan %q", r, a, b)
		}
	}
}

// A pointer-receiver Check is invisible to a plain value, which
// then takes the reflect path.
func TestPointerReceiverCheck(t *testing.T) {
	if errString(Validate, &ptrChecker{}) != "checked" {
		t.Fatal("pointer did not use Check")
	}
	if errString(Validate, ptrChecker{}) != "ok" {
		t.Fatal("value should fall back to the plan")
	}
}

// Two goroutines may race to build a plan; both get the right
// answer. Run with -race.
func TestConcurrentPlans(t *testing.T) {
	type fresh struct {
		A string `validate:"required"`
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if err := Required(fresh{}); err == nil {
				t.Error("missed A")
			}
		})
	}
	wg.Wait()
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

// A representative request: four fields, three required.
func BenchmarkTransferReqPlan(b *testing.B) {
	r := TransferReq{From: "acc_1", To: "acc_2", Amount: 500}
	for b.Loop() {
		Required(r)
	}
}

func BenchmarkTransferReqUncached(b *testing.B) {
	r := TransferReq{From: "acc_1", To: "acc_2", Amount: 500}
	for b.Loop() {
		uncached(r)
	}
}

func BenchmarkCheckMethod(b *testing.B) {
	r := TransferReq{From: "acc_1", To: "acc_2", Amount: 500}
	for b.Loop() {
		Validate(r)
	}
}
