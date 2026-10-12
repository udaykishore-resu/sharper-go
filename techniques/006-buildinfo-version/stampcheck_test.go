package buildinfo

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func try(dir string, env []string, name string,
	args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=", "GOPROXY=off",
		"GOWORK=off", "GOTOOLCHAIN=local",
		"GIT_AUTHOR_NAME=a", "GIT_AUTHOR_EMAIL=a@example.com",
		"GIT_COMMITTER_NAME=a", "GIT_COMMITTER_EMAIL=a@example.com")
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func run(t *testing.T, dir string, name string,
	args ...string) string {
	t.Helper()
	out, err := try(dir, nil, name, args...)
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
	return out
}

// service writes a tiny main module, optionally as a Git repo with
// one commit and bin/ ignored.
func service(t *testing.T, git bool) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":     "module example.com/ledgerd\n\ngo 1.26.0\n",
		"main.go":    "package main\n\nfunc main() {}\n",
		".gitignore": "/bin/\n",
	}
	for name, data := range files {
		err := os.WriteFile(filepath.Join(dir, name), []byte(data),
			0o644)
		if err != nil {
			t.Fatal(err)
		}
	}
	if git {
		if _, err := exec.LookPath("git"); err != nil {
			t.Skip("git not installed")
		}
		run(t, dir, "git", "init", "-q", "-b", "main")
		run(t, dir, "git", "add", ".")
		run(t, dir, "git", "commit", "-q", "-m", "init")
	}
	return dir
}

func stampCheck(t *testing.T, dir string) (string, error) {
	t.Helper()
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not installed; the CI check needs it")
	}
	script, err := filepath.Abs("stampcheck.sh")
	if err != nil {
		t.Fatal(err)
	}
	return try(dir, nil, "sh", script)
}

func TestStampCheck(t *testing.T) {
	cases := []struct {
		name  string
		git   bool
		flags []string
		dirty bool
		ok    bool
	}{
		{"clean commit", true, nil, false, true},
		{"dirty tree", true, nil, true, false},
		{"stamping off", true, []string{"-buildvcs=false"},
			false, false},
		// -buildvcs=true does not fail outside a repository:
		// there is nothing to ask. Only the check catches it.
		{"no repository", false, []string{"-buildvcs=true"},
			false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := service(t, c.git)
			if c.dirty {
				err := os.WriteFile(filepath.Join(dir, "x.txt"),
					[]byte("x"), 0o644)
				if err != nil {
					t.Fatal(err)
				}
			}
			args := append([]string{"build"}, c.flags...)
			args = append(args, "-o", "bin/ledgerd", ".")
			run(t, dir, "go", args...)
			out, err := stampCheck(t, dir)
			if (err == nil) != c.ok {
				t.Fatalf("check ok=%v, want %v:\n%s",
					err == nil, c.ok, out)
			}
		})
	}
}

// Git refuses a checkout owned by another user ("dubious
// ownership"), and go build reports exit status 128. Adding the
// path to safe.directory fixes it without -buildvcs=false.
func TestDubiousOwnership(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root to hand the checkout to another user")
	}
	dir := service(t, true)
	parent := filepath.Dir(dir)
	os.Chmod(parent, 0o755)
	run(t, dir, "chown", "-R", "65534:65534", dir)
	gitcfg := filepath.Join(t.TempDir(), "gitconfig")
	os.WriteFile(gitcfg, nil, 0o644)
	env := []string{"GIT_CONFIG_GLOBAL=" + gitcfg,
		"GIT_CONFIG_NOSYSTEM=1"}
	out, err := try(dir, env, "go", "build", "-o",
		os.DevNull, ".")
	if err == nil || !strings.Contains(out,
		"error obtaining VCS status: exit status 128") {
		t.Fatalf("err=%v, want exit status 128:\n%s", err, out)
	}
	if _, err := try(dir, env, "git", "config", "--global",
		"--add", "safe.directory", dir); err != nil {
		t.Fatal(err)
	}
	if out, err := try(dir, env, "go", "build", "-o",
		os.DevNull, "."); err != nil {
		t.Fatalf("after safe.directory: %v\n%s", err, out)
	}
}

// The endpoint exposes four approved fields and nothing else from
// the build info: no dependency list, no build settings.
func TestBuildJSONFields(t *testing.T) {
	b, err := json.Marshal(BuildInfo())
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	want := []string{"dirty", "go", "revision", "version"}
	if !slices.Equal(keys, want) {
		t.Fatalf("fields = %v, want %v", keys, want)
	}
}
