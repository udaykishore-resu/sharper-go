package sum

import (
	"os"
	"strings"
	"testing"

	"github.com/udaykishore-resu/sharper-go/techniques/internal/gotool"
)

// The printed Integer, Sum and Positive compile at the card's
// minimum, go 1.21, and accept the named type Cents.
func TestCompilesAtGo121(t *testing.T) {
	src, err := os.ReadFile("sum.go")
	if err != nil {
		t.Fatal(err)
	}
	use := "package sum\n\nvar _ Cents = Sum(Positive(Batch{1, -2}))\n"
	dir := gotool.Module(t, "1.21", map[string]string{
		"sum.go": string(src), "use.go": use,
	})
	if out, err := gotool.Go(t, dir, "vet", "."); err != nil {
		t.Fatalf("go 1.21: %v\n%s", err, out)
	}
}

// Without the tilde, Cents is rejected with "possibly missing ~".
func TestNoTildeRejectsCents(t *testing.T) {
	src := `package m

type Integer interface{ int | int64 }

func Sum[T Integer](xs []T) (t T) {
	for _, x := range xs {
		t += x
	}
	return t
}

type Cents int64

var _ = Sum([]Cents{1})
`
	dir := gotool.Module(t, "1.26.0", map[string]string{"a.go": src})
	out, err := gotool.Go(t, dir, "build", ".")
	if err == nil || !strings.Contains(out, "Cents does not satisfy Integer") ||
		!strings.Contains(out, "possibly missing ~") {
		t.Fatalf("%v\n%s", err, out)
	}
}

const store = `package m

type Store struct{}

func (s *Store) Get[T any](id string) (T, error) {
	var z T
	return z, nil
}
`

// A generic method needs a go 1.27 line, and even then it does not
// satisfy an interface: interface methods take no type parameters.
func TestGenericMethodGates(t *testing.T) {
	gotool.NeedMinor(t, 27)
	dir := gotool.Module(t, "1.26.0", map[string]string{"a.go": store})
	out, err := gotool.Go(t, dir, "build", ".")
	if err == nil ||
		!strings.Contains(out, "generic method requires go1.27 or later") {
		t.Fatalf("go 1.26: %v\n%s", err, out)
	}
	dir = gotool.Module(t, "1.27.0", map[string]string{"a.go": store})
	if out, err := gotool.Go(t, dir, "build", "."); err != nil {
		t.Fatalf("go 1.27: %v\n%s", err, out)
	}
	iface := "package m\n\ntype Getter interface {\n" +
		"\tGet(id string) (int, error)\n}\n\n" +
		"var _ Getter = (*Store)(nil)\n"
	dir = gotool.Module(t, "1.27.0", map[string]string{
		"a.go": store, "b.go": iface,
	})
	out, err = gotool.Go(t, dir, "build", ".")
	if err == nil || !strings.Contains(out, "wrong type for method Get") {
		t.Fatalf("generic method satisfied an interface: %v\n%s", err, out)
	}
}
