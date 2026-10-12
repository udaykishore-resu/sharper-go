package httpapi

import (
	"go/build"
	"strings"
	"testing"
)

// Technique 101 says the handler package never imports the store;
// it declares the small interface it needs (Technique 26). This
// pins it for the non-test files.
func TestHandlersDoNotImportStore(t *testing.T) {
	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range pkg.Imports {
		if strings.HasSuffix(imp, "/internal/store") ||
			strings.HasPrefix(imp, "github.com/jackc/pgx") {
			t.Errorf("httpapi imports %s", imp)
		}
	}
}
