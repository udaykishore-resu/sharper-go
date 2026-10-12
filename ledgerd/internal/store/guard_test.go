package store

import (
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// Technique 57's "no Begin outside the store", enforced: no
// non-test file outside internal/store imports pgx, so nothing
// else can open a transaction.
func TestOnlyStoreImportsPgx(t *testing.T) {
	root := filepath.Join("..", "..")
	checked := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if rel == filepath.Join("internal", "store") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil,
			parser.ImportsOnly)
		if err != nil {
			return err
		}
		checked++
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if strings.HasPrefix(p, "github.com/jackc/pgx") {
				t.Errorf("%s imports %s; only internal/store may", rel, p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Fatal("walked no files")
	}
}

// RunTx retries exactly two SQLSTATEs, also when wrapped.
func TestRetryable(t *testing.T) {
	for _, c := range []struct {
		err  error
		want bool
	}{
		{&pgconn.PgError{Code: "40001"}, true},
		{&pgconn.PgError{Code: "40P01"}, true},
		{fmt.Errorf("x: %w", &pgconn.PgError{Code: "40P01"}), true},
		{&pgconn.PgError{Code: "23505"}, false},
		{&pgconn.PgError{Code: "57014"}, false},
		{errors.New("40001"), false},
		{nil, false},
	} {
		if got := retryable(c.err); got != c.want {
			t.Errorf("retryable(%v) = %v", c.err, got)
		}
	}
}
