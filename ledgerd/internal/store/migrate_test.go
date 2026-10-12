package store

import (
	"io/fs"
	"regexp"
	"testing"
)

// Technique 61: every embedded file has a zero-padded name, and
// the test fails if nothing is embedded at all.
func TestMigrationNames(t *testing.T) {
	names, err := fs.Glob(embedded, "migrations/*")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatal("no migrations embedded")
	}
	ok := regexp.MustCompile(`^migrations/\d{5}_[a-z0-9_]+\.sql$`)
	for _, n := range names {
		if !ok.MatchString(n) {
			t.Errorf("bad migration name %q", n)
		}
	}
}
