package ledger

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// repo makes a Git repo whose one go:generate directive writes
// gen.go, and commits the generated file.
func repo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/g\n\ngo 1.26.0\n",
		"g.go": "package g\n\n" +
			"//go:generate sh -c \"printf 'package g\\\\n' > gen.go\"\n",
		"gen.go": "package g\n",
	}
	for name, data := range files {
		err := os.WriteFile(filepath.Join(dir, name), []byte(data),
			0o644)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"}, {"add", "."},
		{"-c", "user.name=a", "-c", "user.email=a@example.com",
			"commit", "-q", "-m", "init"},
	} {
		if out, err := git(dir, args...); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func driftCheck(t *testing.T, dir string) error {
	t.Helper()
	script, err := filepath.Abs("drift-check.sh")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", script)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=", "GOWORK=off")
	return cmd.Run()
}

func TestDriftCheck(t *testing.T) {
	t.Run("clean", func(t *testing.T) {
		if err := driftCheck(t, repo(t)); err != nil {
			t.Fatalf("clean repo failed: %v", err)
		}
	})
	t.Run("hand edit to generated file", func(t *testing.T) {
		dir := repo(t)
		os.WriteFile(filepath.Join(dir, "gen.go"),
			[]byte("package g\n\n// edited\n"), 0o644)
		git(dir, "-c", "user.name=a", "-c",
			"user.email=a@example.com", "commit", "-qam", "edit")
		if err := driftCheck(t, dir); err == nil {
			t.Fatal("committed hand edit passed")
		}
	})
	t.Run("input changed, output not regenerated", func(t *testing.T) {
		dir := repo(t)
		src := "package g\n\n" +
			"//go:generate sh -c \"printf 'package g\\\\n" +
			"// v2\\\\n' > gen.go\"\n"
		os.WriteFile(filepath.Join(dir, "g.go"), []byte(src), 0o644)
		git(dir, "-c", "user.name=a", "-c",
			"user.email=a@example.com", "commit", "-qam", "bump")
		if err := driftCheck(t, dir); err == nil {
			t.Fatal("stale output passed")
		}
	})
	t.Run("new untracked output", func(t *testing.T) {
		dir := repo(t)
		src := "package g\n\n" +
			"//go:generate sh -c \"printf 'package g\\\\n' > new.go\"\n"
		os.WriteFile(filepath.Join(dir, "g.go"), []byte(src), 0o644)
		git(dir, "-c", "user.name=a", "-c",
			"user.email=a@example.com", "commit", "-qam", "new")
		// git diff --exit-code alone misses an untracked file.
		exec.Command("sh", "-c", "cd "+dir+
			" && go generate ./...").Run()
		if _, err := git(dir, "diff", "--exit-code"); err != nil {
			t.Fatal("expected git diff to miss the new file")
		}
		if err := driftCheck(t, dir); err == nil {
			t.Fatal("untracked generated file passed")
		}
	})
}
