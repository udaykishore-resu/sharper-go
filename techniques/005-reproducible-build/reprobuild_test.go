package reprobuild

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const mainGo = `package main

import "fmt"

// Version is a "dev" string that -X can override.
var Version = "dev"

func main() { fmt.Println(Version) }
`

func run(t *testing.T, dir string, env []string, name string,
	args ...string) string {
	t.Helper()
	out, err := try(dir, env, name, args...)
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
	return out
}

// modcache is a private module cache for the whole test run. The
// go command caches the version it computes for a commit there,
// so tests that share one would see each other's stamps.
var modcache string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "reprobuild-modcache")
	if err != nil {
		panic(err)
	}
	modcache = dir
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func try(dir string, env []string, name string,
	args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-modcacherw",
		"GOMODCACHE="+modcache, "GOPROXY=off",
		"GOWORK=off", "GOTOOLCHAIN=local",
		"GIT_AUTHOR_NAME=a", "GIT_AUTHOR_EMAIL=a@example.com",
		"GIT_COMMITTER_NAME=a", "GIT_COMMITTER_EMAIL=a@example.com",
		"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z",
		"GIT_COMMITTER_DATE=2026-01-01T00:00:00Z")
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// checkout writes the module into a fresh folder.
func checkout(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":              "module example.com/ledgerd\n\ngo 1.26.0\n",
		"cmd/ledgerd/main.go": mainGo,
		// Without this, the first build's output is an untracked
		// file and every later build is stamped +dirty.
		".gitignore": "/bin/\n",
	}
	for name, data := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// gitCheckout is checkout plus one commit with a fixed author and
// date, so two folders with the same msg hold the same commit.
func gitCheckout(t *testing.T, msg string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := checkout(t)
	run(t, dir, nil, "git", "init", "-q", "-b", "main")
	run(t, dir, nil, "git", "add", ".")
	run(t, dir, nil, "git", "commit", "-q", "-m", msg)
	return dir
}

// build runs the flags from build.sh and returns the hash.
func build(t *testing.T, dir string, trimpath bool,
	ldflags string) string {
	t.Helper()
	args := []string{"build"}
	if trimpath {
		args = append(args, "-trimpath")
	}
	args = append(args, "-ldflags="+ldflags,
		"-o", "bin/ledgerd", "./cmd/ledgerd")
	run(t, dir, []string{"CGO_ENABLED=0"}, "go", args...)
	b, err := os.ReadFile(filepath.Join(dir, "bin", "ledgerd"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestTrimpathMakesFolderIrrelevant(t *testing.T) {
	a, b := checkout(t), checkout(t)
	if build(t, a, true, "-s -w") != build(t, b, true, "-s -w") {
		t.Fatal("-trimpath builds in two folders differ")
	}
	if build(t, a, false, "-s -w") == build(t, b, false, "-s -w") {
		t.Fatal("builds without -trimpath matched; " +
			"the folder should leak into the binary")
	}
}

// The same commit in two Git checkouts stamps the same revision,
// so VCS stamping keeps the build reproducible.
func TestSameCommitSameBytes(t *testing.T) {
	a, b := gitCheckout(t, "same"), gitCheckout(t, "same")
	if build(t, a, true, "-s -w") != build(t, b, true, "-s -w") {
		t.Fatal("same commit, two folders: hashes differ")
	}
	out := run(t, a, nil, "go", "version", "-m", "bin/ledgerd")
	for _, want := range []string{"vcs.revision=",
		"vcs.modified=false", "-trimpath=true"} {
		if !strings.Contains(out, want) {
			t.Errorf("go version -m lacks %q:\n%s", want, out)
		}
	}
}

// Since Go 1.24 the tag becomes the main module's version, and a
// dirty tree adds +dirty, which changes the bytes.
func TestTagStampAndDirty(t *testing.T) {
	dir := gitCheckout(t, "tagged")
	run(t, dir, nil, "git", "tag", "v0.4.0")
	clean := build(t, dir, true, "-s -w")
	out := run(t, dir, nil, "go", "version", "-m", "bin/ledgerd")
	if !strings.Contains(out, "example.com/ledgerd\tv0.4.0\t") &&
		!strings.Contains(out, "example.com/ledgerd\tv0.4.0\n") {
		t.Fatalf("no v0.4.0 stamp:\n%s", out)
	}
	f := filepath.Join(dir, "cmd", "ledgerd", "main.go")
	if err := os.WriteFile(f, []byte(mainGo+"\n// edit\n"),
		0o644); err != nil {
		t.Fatal(err)
	}
	dirty := build(t, dir, true, "-s -w")
	out = run(t, dir, nil, "go", "version", "-m", "bin/ledgerd")
	if !strings.Contains(out, "v0.4.0+dirty") {
		t.Fatalf("no +dirty stamp:\n%s", out)
	}
	if clean == dirty {
		t.Fatal("dirty build has the clean build's bytes")
	}
}

// The stamp for a commit is cached in GOMODCACHE. Build a commit,
// tag it afterwards, and the next build on that machine still
// says v0.0.0-...; a fresh cache sees the tag.
func TestTagAfterBuildKeepsPseudoVersion(t *testing.T) {
	dir := gitCheckout(t, "tagged later")
	cache := t.TempDir()
	env := []string{"GOMODCACHE=" + cache}
	stamp := func() string {
		run(t, dir, env, "go", "build", "-o", "bin/ledgerd",
			"./cmd/ledgerd")
		out := run(t, dir, env, "go", "version", "-m",
			"bin/ledgerd")
		for _, line := range strings.Split(out, "\n") {
			f := strings.Fields(line)
			if len(f) >= 3 && f[0] == "mod" {
				return f[2]
			}
		}
		t.Fatalf("no mod line:\n%s", out)
		return ""
	}
	before := stamp()
	if !strings.HasPrefix(before, "v0.0.0-") {
		t.Fatalf("untagged stamp = %q", before)
	}
	run(t, dir, nil, "git", "tag", "v0.4.0")
	if got := stamp(); got != before {
		t.Fatalf("after tagging, stamp = %q; the cached %q "+
			"was expected", got, before)
	}
	env[0] = "GOMODCACHE=" + t.TempDir()
	if got := stamp(); got != "v0.4.0" {
		t.Fatalf("fresh cache: stamp = %q, want v0.4.0", got)
	}
}

func TestXFlag(t *testing.T) {
	dir := checkout(t)
	bin := filepath.Join(dir, "bin", "ledgerd")

	build(t, dir, true, "-X main.Version=1.2.3")
	if got := run(t, dir, nil, bin); got != "1.2.3\n" {
		t.Fatalf("-X main.Version: got %q", got)
	}
	// A typo links fine and leaves the default in place.
	build(t, dir, true, "-X main.Versoin=1.2.3")
	if got := run(t, dir, nil, bin); got != "dev\n" {
		t.Fatalf("-X typo: got %q, want dev", got)
	}
}

// -X writes link-time data. An initializer that runs at start-up
// overwrites it; a non-string variable fails the link.
func TestXFlagLimits(t *testing.T) {
	dir := checkout(t)
	main := filepath.Join(dir, "cmd", "ledgerd", "main.go")
	src := `package main

import (
	"fmt"
	"os"
)

var Version = os.Getenv("LEDGERD_NO_SUCH_VAR")

var Build int

func main() { fmt.Println("v=" + Version) }
`
	if err := os.WriteFile(main, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	build(t, dir, true, "-X main.Version=1.2.3")
	bin := filepath.Join(dir, "bin", "ledgerd")
	if got := run(t, dir, nil, bin); got != "v=\n" {
		t.Fatalf("run-time initializer: got %q, want v=", got)
	}
	out, err := try(dir, []string{"CGO_ENABLED=0"}, "go", "build",
		"-ldflags=-X main.Build=7", "-o", bin, "./cmd/ledgerd")
	if err == nil || !strings.Contains(out,
		"cannot set with -X: not a var of type string") {
		t.Fatalf("non-string -X: err=%v\n%s", err, out)
	}
}

// -s -w drops the symbol table and DWARF, not the runtime's own
// function table, so a panic still names the function and line.
func TestStrippedPanicTrace(t *testing.T) {
	dir := checkout(t)
	main := filepath.Join(dir, "cmd", "ledgerd", "main.go")
	src := "package main\n\nfunc main() { panic(\"boom\") }\n"
	if err := os.WriteFile(main, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	build(t, dir, true, "-s -w")
	out, err := try(dir, nil, filepath.Join(dir, "bin", "ledgerd"))
	if err == nil {
		t.Fatal("binary did not panic")
	}
	for _, want := range []string{"panic: boom", "main.main()",
		"example.com/ledgerd/cmd/ledgerd/main.go:3"} {
		if !strings.Contains(out, want) {
			t.Errorf("trace lacks %q:\n%s", want, out)
		}
	}
}

// The printed script, run as-is in two folders.
func TestBuildScript(t *testing.T) {
	script, err := filepath.Abs("build.sh")
	if err != nil {
		t.Fatal(err)
	}
	var sums []string
	for range 2 {
		dir := checkout(t)
		out := run(t, dir, nil, "sh", script)
		for _, want := range []string{"-trimpath=true",
			"CGO_ENABLED=0"} {
			if !strings.Contains(out, want) {
				t.Errorf("go version -m lacks %q:\n%s", want, out)
			}
		}
		// With -trimpath the go command leaves -ldflags out of
		// the build info, so the stamp's -X value isn't listed.
		if strings.Contains(out, "-ldflags") {
			t.Errorf("-ldflags recorded despite -trimpath:\n%s",
				out)
		}
		b, err := os.ReadFile(filepath.Join(dir, "bin", "ledgerd"))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(b)
		sums = append(sums, hex.EncodeToString(sum[:]))
	}
	if sums[0] != sums[1] {
		t.Fatal("build.sh in two folders: hashes differ")
	}
}
