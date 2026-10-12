package golden

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Review check for Technique 65. The golden helper runs in a child
// test binary whose working directory is a scratch copy of
// testdata, so the committed goldens are never touched.
func runGolden(t *testing.T, dir string, ci bool,
	args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], append([]string{
		"-test.run=^TestGetAccount$", "-test.count=1"}, args...)...)
	cmd.Dir = dir
	env := []string{}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "CI=") {
			env = append(env, kv)
		}
	}
	if ci {
		env = append(env, "CI=true")
	}
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func scratchCopy(t *testing.T) (dir, golden string) {
	t.Helper()
	dir = t.TempDir()
	if err := os.CopyFS(filepath.Join(dir, "testdata"),
		os.DirFS("testdata")); err != nil {
		t.Fatal(err)
	}
	golden = filepath.Join(dir, "testdata", "TestGetAccount", "acc_1.golden")
	if err := os.WriteFile(golden, []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, golden
}

func TestGoldenModes(t *testing.T) {
	committed, err := os.ReadFile("testdata/TestGetAccount/acc_1.golden")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("stale golden fails a normal run", func(t *testing.T) {
		dir, _ := scratchCopy(t)
		out, err := runGolden(t, dir, false)
		if err == nil || !strings.Contains(out, "acc_1.golden") {
			t.Fatalf("stale golden passed:\n%s", out)
		}
	})

	t.Run("-update rewrites it locally", func(t *testing.T) {
		dir, golden := scratchCopy(t)
		if out, err := runGolden(t, dir, false, "-update"); err != nil {
			t.Fatalf("-update failed: %v\n%s", err, out)
		}
		got, _ := os.ReadFile(golden)
		if !bytes.Equal(got, committed) {
			t.Fatalf("-update wrote %q, want %q", got, committed)
		}
	})

	t.Run("-update is refused when CI is set", func(t *testing.T) {
		dir, golden := scratchCopy(t)
		out, err := runGolden(t, dir, true, "-update")
		if err == nil || !strings.Contains(out, "not allowed in CI") {
			t.Fatalf("-update ran in CI:\n%s", out)
		}
		if got, _ := os.ReadFile(golden); string(got) != "stale\n" {
			t.Fatalf("golden rewritten in CI: %q", got)
		}
	})
}
