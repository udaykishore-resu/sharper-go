package retry

import (
	"strings"
	"testing"

	"github.com/udaykishore-resu/sharper-go/techniques/internal/gotool"
)

const loops = `package m

import "fmt"

func F() {
	for i := 0; i < 3; i++ { // qualifies
		fmt.Println(i)
	}
	q := []int{1}
	for i := 0; i < len(q); i++ { // body grows the bound
		q = append(q, i)
		if len(q) > 5 {
			break
		}
	}
	for i := 0; i < 5; i++ { // body writes the counter
		if i == 1 {
			i++
		}
		fmt.Println(i)
	}
}
`

// go fix -rangeint (Go 1.26+) rewrites only the loop that
// qualifies, and -diff leaves the file alone.
func TestGoFixRangeint(t *testing.T) {
	dir := gotool.Module(t, "1.26.0", map[string]string{"a.go": loops})
	out, err := gotool.Go(t, dir, "fix", "-rangeint", "-diff", "./...")
	if err == nil {
		t.Fatalf("-diff exits non-zero when there is a patch:\n%s", out)
	}
	if strings.Count(out, "+\tfor i := range") != 1 ||
		!strings.Contains(out, "+\tfor i := range 3 {") {
		t.Fatalf("want exactly the first loop rewritten:\n%s", out)
	}
	if gotool.Read(t, dir, "a.go") != loops {
		t.Fatal("-diff modified the file")
	}
}

// Below go 1.22 the fixer leaves loops alone: range over an int
// would not compile there.
func TestGoFixRangeintOldGoLine(t *testing.T) {
	dir := gotool.Module(t, "1.21", map[string]string{"a.go": loops})
	out, err := gotool.Go(t, dir, "fix", "-rangeint", "-diff", "./...")
	if err != nil || strings.Contains(out, "range 3") {
		t.Fatalf("go 1.21 module changed: %v\n%s", err, out)
	}
	src := "package m\n\nfunc G() {\n\tfor range 3 {\n\t}\n}\n"
	dir = gotool.Module(t, "1.21", map[string]string{"a.go": src})
	out, err = gotool.Go(t, dir, "build", "./...")
	if err == nil || !strings.Contains(out, "requires go1.22 or later") {
		t.Fatalf("want a go1.22 error: %v\n%s", err, out)
	}
}

// A zero or negative bound runs the body zero times.
func TestNonPositiveBound(t *testing.T) {
	for _, n := range []int{0, -3} {
		for range n {
			t.Fatalf("body ran for n = %d", n)
		}
	}
}
