package ledger

import (
	"errors"
	"go/ast"
	"go/doc/comment"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestPostDelegates(t *testing.T) {
	posted = nil
	a, errA := Post("a", "b", 5)
	b, errB := Post("a", "b", 5)
	if errA != nil || errB != nil || a == b {
		t.Fatalf("Post has no key, so a retry posts twice: %q %q %v %v", a, b, errA, errB)
	}
	k1, err := PostTransfer(Transfer{From: "a", To: "b", Amount: 5, Key: "k"})
	k2, err2 := PostTransfer(Transfer{From: "a", To: "b", Amount: 5, Key: "k"})
	if err != nil || err2 != nil || k1 != k2 {
		t.Fatal("PostTransfer must honor the key")
	}
}

// The old API keeps the new one's error semantics: every failure
// PostTransfer reports reaches Post's callers unchanged.
func TestPostKeepsErrors(t *testing.T) {
	posted = nil
	id, err := Post("a", "b", 0)
	if !errors.Is(err, ErrBadAmount) || id != "" {
		t.Fatalf("Post lost PostTransfer's error: %q, %v", id, err)
	}
	if len(posted) != 0 {
		t.Fatal("a failed Post must not record anything")
	}
}

func postDoc(t *testing.T) string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "ledger.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == "Post" {
			return fn.Doc.Text()
		}
	}
	t.Fatal("Post not found")
	return ""
}

// The marker must be its own paragraph, starting "Deprecated: ",
// or staticcheck, gopls and pkg.go.dev ignore it.
func TestDeprecatedParagraph(t *testing.T) {
	if !strings.Contains(postDoc(t), "\n\nDeprecated: ") {
		t.Fatalf("no standalone Deprecated: paragraph in %q", postDoc(t))
	}
}

// [PostTransfer] parses as a doc link, which go doc and pkg.go.dev
// render as a link to the replacement.
func TestDocLink(t *testing.T) {
	var p comment.Parser
	p.LookupSym = func(recv, name string) bool { return recv == "" && name == "PostTransfer" }
	d := p.Parse(postDoc(t))
	for _, b := range d.Content {
		para, ok := b.(*comment.Paragraph)
		if !ok {
			continue
		}
		for _, txt := range para.Text {
			if l, ok := txt.(*comment.DocLink); ok && l.Name == "PostTransfer" {
				return
			}
		}
	}
	t.Fatal("[PostTransfer] is not a doc link")
}
