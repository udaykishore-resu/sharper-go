// Package validate is Technique 84: pay for reflection once per
// type, not once per request.
package validate

import (
	"fmt"
	"reflect"
	"sync"
)

var plans sync.Map // reflect.Type -> []int

func planFor(t reflect.Type) []int {
	if p, ok := plans.Load(t); ok {
		return p.([]int)
	}
	var p []int
	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).Tag.Get("validate") == "required" {
			p = append(p, i)
		}
	}
	plans.Store(t, p)
	return p
}

func Required(s any) error {
	v := reflect.Indirect(reflect.ValueOf(s))
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("validate: %T is not a struct", s)
	}
	for _, i := range planFor(v.Type()) {
		if v.Field(i).IsZero() {
			name := v.Type().Field(i).Name
			return fmt.Errorf("%s is required", name)
		}
	}
	return nil
}

type checker interface{ Check() error }

// Validate tries the hand-written method first; cold types keep
// working through the plan cache.
func Validate(s any) error {
	if c, ok := s.(checker); ok {
		return c.Check()
	}
	return Required(s)
}

type TransferReq struct {
	From   string `validate:"required"`
	To     string `validate:"required"`
	Amount int64  `validate:"required"`
	Memo   string
}

func (r TransferReq) Check() error {
	switch {
	case r.From == "":
		return required("From")
	case r.To == "":
		return required("To")
	case r.Amount == 0:
		return required("Amount")
	}
	return nil
}

// uncached redoes the analysis every call, for the benchmark.
func uncached(s any) error {
	v := reflect.Indirect(reflect.ValueOf(s))
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("validate: %T is not a struct", s)
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).Tag.Get("validate") == "required" && v.Field(i).IsZero() {
			return fmt.Errorf("%s is required", t.Field(i).Name)
		}
	}
	return nil
}

func required(field string) error { return fmt.Errorf("%s is required", field) }
