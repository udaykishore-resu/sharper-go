package importer

import (
	"bytes"
	"crypto/sha256"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func hashTree(t *testing.T, root string) map[string][32]byte {
	t.Helper()
	sums := map[string][32]byte{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		sums[p] = sha256.Sum256(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return sums
}

// Review check for Technique 69: running the whole fixture suite
// leaves every committed file in testdata byte-for-byte unchanged
// and adds nothing (no ok.csv.bak).
func TestImportNeverMutatesFixtures(t *testing.T) {
	before := hashTree(t, "testdata")
	t.Run("suite", TestImport)
	after := hashTree(t, "testdata")
	if len(after) != len(before) {
		t.Fatalf("testdata had %d files, now %d", len(before), len(after))
	}
	for p, h := range before {
		if after[p] != h {
			t.Errorf("%s changed", p)
		}
	}
}

// The CRLF fixture really holds CRLF bytes in this checkout.
func TestCRLFFixtureIsCRLF(t *testing.T) {
	b, err := os.ReadFile("testdata/import/crlf_lines.csv")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(b, []byte("\r\n")) != bytes.Count(b, []byte("\n")) {
		t.Fatalf("crlf_lines.csv lost its CRLF endings: %q", b)
	}
}

// t.Chdir after t.Parallel panics: the working directory belongs
// to the whole process.
func TestChdirRefusedInParallel(t *testing.T) {
	t.Run("p", func(t *testing.T) {
		t.Parallel()
		defer func() {
			if recover() == nil {
				t.Error("t.Chdir in a parallel test did not panic")
			}
		}()
		t.Chdir(t.TempDir())
	})
}

// The -text attribute keeps exact bytes through a checkout that
// converts line endings (core.autocrlf=true, as on Windows), in
// both directions: CRLF stays CRLF and LF stays LF.
func TestGitattributesKeepBytes(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	src := t.TempDir()
	if err := os.CopyFS(filepath.Join(src, "testdata"),
		os.DirFS("testdata")); err != nil {
		t.Fatal(err)
	}
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{
			"-c", "user.name=t", "-c", "user.email=t@example.com",
			"-c", "init.defaultBranch=main"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git(src, "init", "-q")
	git(src, "add", ".")
	git(src, "commit", "-q", "-m", "fixtures")
	dst := filepath.Join(t.TempDir(), "clone")
	git(filepath.Dir(dst), "-c", "core.autocrlf=true",
		"clone", "-q", src, dst)
	for _, f := range []string{"crlf_lines.csv", "ok.csv"} {
		rel := filepath.Join("testdata", "import", f)
		want, _ := os.ReadFile(rel)
		got, err := os.ReadFile(filepath.Join(dst, rel))
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("%s changed in an autocrlf checkout: %q -> %q",
				f, want, got)
		}
	}
}
