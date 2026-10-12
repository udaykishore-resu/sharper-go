// Package versiontags backs the book's promise about the Go
// version on each technique card. For every standard-library
// technique, it copies the non-test code into a scratch module
// whose go line is the card's tag and runs go vet. The compiler
// rejects language features newer than the go line, and vet's
// stdversion check rejects newer standard-library calls, so a tag
// that is too low fails here. ("any" means every supported
// release, so those directories are checked at go 1.26.)
//
// It checks the code, not behavior: a tag chosen for a runtime
// change (GOMAXPROCS in 1.25, PGO in 1.21) passes at any go line,
// and versions older than go 1.21 are not checked by stdversion.
package versiontags

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/udaykishore-resu/sharper-go/techniques/internal/gotool"
)

// tags mirrors the Go version on each card (Appendix B).
var tags = map[string]string{
	"001": "1.21", "003": "1.18", "004": "any", "005": "any",
	"006": "1.24", "007": "any", "009": "any", "011": "1.22",
	"013": "1.23", "014": "1.23", "015": "1.21", "016": "1.21",
	"017": "1.24", "018": "1.22", "019": "1.23", "020": "1.24",
	"022": "1.20", "023": "1.13", "024": "1.13", "025": "any",
	"026": "any", "027": "any", "028": "any", "029": "any",
	"031": "1.25", "032": "1.25", "034": "1.25", "035": "1.21",
	"036": "1.21", "039": "1.19", "040": "any", "041": "any",
	"042": "any", "043": "1.22", "044": "any", "045": "1.16",
	"046": "any", "048": "1.19", "049": "1.19", "051": "1.22",
	"052": "1.22", "053": "1.24", "054": "any", "059": "any",
	"060": "1.27", "062": "any", "063": "1.24", "064": "1.25",
	"065": "any", "066": "1.27", "067": "any", "069": "1.24",
	"070": "1.24", "072": "1.20", "073": "any", "074": "1.21",
	"075": "any", "076": "any", "077": "any", "078": "any",
	"079": "1.19", "080": "1.25", "081": "1.25", "082": "1.24",
	"083": "any", "084": "any", "085": "1.21", "089": "any",
	"090": "1.23", "091": "1.21", "097": "any", "098": "1.22",
	"099": "1.23", "100": "1.24",
}

func TestCodeBuildsAtCardVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("vets every technique in a scratch module")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Fatal("go not on PATH")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dirs, _ := filepath.Glob(filepath.Join(root, "[0-9][0-9][0-9]-*"))
	seen := map[string]bool{}
	for _, dir := range dirs {
		name := filepath.Base(dir)
		tag, ok := tags[name[:3]]
		if !ok {
			t.Errorf("%s: no card version in the table", name)
			continue
		}
		seen[name[:3]] = true
		if tag == "any" {
			tag = "1.26"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if m, err := strconv.Atoi(strings.TrimPrefix(tag, "1.")); err == nil {
				gotool.NeedMinor(t, m)
			}
			out, err := vetAt(t, root, dir, tag)
			if err != nil {
				t.Errorf("go %s: %v\n%s", tag, err, out)
			}
		})
	}
	for n := range tags {
		if !seen[n] {
			t.Errorf("table lists %s, but no directory has it", n)
		}
	}
}

// vetAt copies dir's non-test files (and its internal helpers) to
// a scratch module with the given go line and vets them.
func vetAt(t *testing.T, root, dir, goLine string) (string, error) {
	w := t.TempDir()
	base := filepath.Base(dir)
	copyTree(t, dir, filepath.Join(w, base))
	copyTree(t, filepath.Join(root, "internal"), filepath.Join(w, "internal"))
	mod := "module github.com/udaykishore-resu/sharper-go/techniques\n\ngo " +
		goLine + "\n"
	if err := os.WriteFile(filepath.Join(w, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "vet", "./"+base+"/...")
	cmd.Dir = w
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOFLAGS=-mod=mod")
	out, err := cmd.CombinedOutput()
	if strings.Contains(string(out), "matched no packages") {
		return "", nil // tests only, nothing to check
	}
	return string(out), err
}

func copyTree(t *testing.T, from, to string) {
	err := filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		dst := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		if strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}
