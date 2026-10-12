package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImport(t *testing.T) {
	files, _ := filepath.Glob("testdata/import/*.csv")
	if len(files) == 0 {
		t.Fatal("no fixtures found")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			dir := t.TempDir()
			src := os.DirFS(filepath.Dir(f))
			if err := os.CopyFS(dir, src); err != nil {
				t.Fatal(err)
			}
			t.Chdir(dir)
			runImport(t, filepath.Base(f))
		})
	}
}

// runImport checks the outcome each fixture's name promises.
func runImport(t *testing.T, name string) {
	t.Helper()
	n, err := Import(name)
	wantErr := strings.HasPrefix(name, "negative_")
	if (err != nil) != wantErr {
		t.Fatalf("Import(%s) = %d, %v", name, n, err)
	}
	if !wantErr && n == 0 {
		t.Fatalf("Import(%s) read no rows", name)
	}
}

// The fixtures themselves must never change.
func TestFixturesUntouched(t *testing.T) {
	if _, err := os.Stat("testdata/import/ok.csv.bak"); err == nil {
		t.Fatal("a test wrote into testdata")
	}
}
