package ledger

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestPostDelegates(t *testing.T) {
	posted = nil
	a, b := Post("a", "b", 5), Post("a", "b", 5)
	if a == b {
		t.Fatal("Post has no key, so a retry posts twice")
	}
	k1 := PostTransfer(Transfer{From: "a", To: "b", Amount: 5, Key: "k"})
	if PostTransfer(Transfer{From: "a", To: "b", Amount: 5, Key: "k"}) != k1 {
		t.Fatal("PostTransfer must honor the key")
	}
}

// The marker must be its own paragraph, starting "Deprecated: ",
// or staticcheck, gopls and pkg.go.dev ignore it.
func TestDeprecatedParagraph(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "ledger.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == "Post" {
			if !strings.Contains(fn.Doc.Text(), "\n\nDeprecated: ") {
				t.Fatalf("no standalone Deprecated: paragraph in %q", fn.Doc.Text())
			}
			return
		}
	}
	t.Fatal("Post not found")
}
