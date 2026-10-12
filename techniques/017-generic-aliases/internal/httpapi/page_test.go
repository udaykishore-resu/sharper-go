package httpapi

import (
	"fmt"
	"testing"

	"github.com/udaykishore-resu/sharper-go/techniques/017-generic-aliases/internal/page"
)

func takesNew(p page.Page[string]) int { return len(p.Items) }

func TestAliasIsSameType(t *testing.T) {
	old := Page[string]{Items: []string{"a", "b"}}
	if takesNew(old) != 2 { // no conversion needed
		t.Fatal("alias did not pass as page.Page")
	}
	if got := fmt.Sprintf("%T", old); got != "page.Page[string]" {
		t.Fatalf("%%T = %q; logs show the real name", got)
	}
	s := Set[string]{}
	s["x"] = struct{}{}
	delete(s, "x") // every map operation works
	if len(s) != 0 {
		t.Fatal("Set is not a plain map")
	}
}
