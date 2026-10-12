package httpapi

import (
	"strings"
	"testing"

	"github.com/udaykishore-resu/sharper-go/techniques/internal/gotool"
)

const pageSrc = `package m

type Page[T any] struct{ Items []T }

type Old[T any] = Page[T]
`

// A generic alias needs a go line of 1.23 or later (with this
// 1.24+ toolchain); vet in a go 1.21 module says so.
func TestAliasGoLine(t *testing.T) {
	dir := gotool.Module(t, "1.21", map[string]string{"a.go": pageSrc})
	out, err := gotool.Go(t, dir, "vet", ".")
	if err == nil || !strings.Contains(out,
		"generic type alias requires go1.23 or later") {
		t.Fatalf("go 1.21: %v\n%s", err, out)
	}
	dir = gotool.Module(t, "1.23", map[string]string{"a.go": pageSrc})
	if out, err := gotool.Go(t, dir, "vet", "."); err != nil {
		t.Fatalf("go 1.23: %v\n%s", err, out)
	}
}

// An alias cannot gain methods, generic or not. A defined type
// can, but it is a new type: a Page no longer passes as one.
func TestAliasVersusDefinedType(t *testing.T) {
	gotool.NeedMinor(t, 27)
	method := "\nfunc (p Old[T]) Len() int { return len(p.Items) }\n"
	dir := gotool.Module(t, "1.27.0",
		map[string]string{"a.go": pageSrc + method})
	out, err := gotool.Go(t, dir, "build", ".")
	if err == nil || !strings.Contains(out,
		"cannot define new methods on generic alias type") {
		t.Fatalf("method on alias: %v\n%s", err, out)
	}
	defined := `package m

type Page[T any] struct{ Items []T }

type New[T any] Page[T]

func (p New[T]) Len() int { return len(p.Items) }

func use(n New[int]) int { return n.Len() }

var _ = use(New[int](Page[int]{})) // conversion compiles
`
	dir = gotool.Module(t, "1.27.0", map[string]string{"a.go": defined})
	if out, err := gotool.Go(t, dir, "build", "."); err != nil {
		t.Fatalf("defined type: %v\n%s", err, out)
	}
	noConv := strings.Replace(defined,
		"use(New[int](Page[int]{}))", "use(Page[int]{})", 1)
	dir = gotool.Module(t, "1.27.0", map[string]string{"a.go": noConv})
	out, err = gotool.Go(t, dir, "build", ".")
	if err == nil || !strings.Contains(out, "cannot use") {
		t.Fatalf("defined type passed without conversion: %v\n%s",
			err, out)
	}
}
