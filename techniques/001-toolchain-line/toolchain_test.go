package toolchainline

import (
	"archive/zip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// goCmd runs the go command in dir with a clean, offline
// environment plus extra settings.
func goCmd(t *testing.T, dir string, env []string,
	args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GOFLAGS=-modcacherw", "GOPROXY=off", "GOSUMDB=off",
		"GOWORK=off", "GOTOOLCHAIN=local", "GODEBUG=")
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// module writes a tiny main module with the given go.mod.
func module(t *testing.T, gomod string) string {
	t.Helper()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"), gomod)
	write(t, filepath.Join(dir, "main.go"),
		"package main\n\nfunc main() {}\n")
	return dir
}

func localVersion() string {
	return strings.TrimPrefix(runtime.Version(), "go")
}

// The printed CI check passes when the tag matches, fails when it
// drifts, and fails when go.mod has no toolchain line at all.
func TestBuilderTagCheck(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not installed; the CI check needs it")
	}
	script, err := filepath.Abs("builder-tag-check.sh")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, toolchain, from string
		ok                    bool
	}{
		{"match", "toolchain go1.27.2\n",
			"FROM golang:1.27.2 AS build", true},
		{"match with platform", "toolchain go1.27.2\n",
			"FROM --platform=$BUILDPLATFORM golang:1.27.2 AS build",
			true},
		{"drift", "toolchain go1.27.2\n",
			"FROM golang:1.27.0 AS build", false},
		{"prefix is not a match", "toolchain go1.27.2\n",
			"FROM golang:1.27.20 AS build", false},
		{"no toolchain line", "",
			"FROM golang:1.27.2 AS build", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := module(t, "module example.com/m\n\n"+
				"go 1.26.0\n\n"+c.toolchain)
			write(t, filepath.Join(dir, "Dockerfile"), c.from+"\n")
			cmd := exec.Command("sh", script)
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()
			if (err == nil) != c.ok {
				t.Fatalf("ok=%v, want %v; output:\n%s",
					err == nil, c.ok, out)
			}
		})
	}
}

// The check passes on the companion ledgerd, whose Dockerfile
// copies the toolchain line.
func TestBuilderTagCheckLedgerd(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not installed; the CI check needs it")
	}
	script, _ := filepath.Abs("builder-tag-check.sh")
	cmd := exec.Command("sh", script)
	cmd.Dir = filepath.Join("..", "..", "ledgerd")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ledgerd drifted: %v\n%s", err, out)
	}
}

// In local mode (what the official image sets) a newer toolchain
// line is ignored and nothing is printed.
func TestLocalIgnoresNewerToolchainLine(t *testing.T) {
	dir := module(t, "module example.com/m\n\ngo 1.26.0\n\n"+
		"toolchain go1.99.0\n")
	out, err := goCmd(t, dir, nil, "build", "-o", os.DevNull, ".")
	if err != nil || out != "" {
		t.Fatalf("err=%v out=%q, want a silent build", err, out)
	}
}

// Only a go line above the local release fails in local mode.
func TestLocalRejectsNewerGoLine(t *testing.T) {
	dir := module(t, "module example.com/m\n\ngo 1.99.0\n")
	out, err := goCmd(t, dir, nil, "build", "-o", os.DevNull, ".")
	want := "go.mod requires go >= 1.99.0 (running go " +
		localVersion() + "; GOTOOLCHAIN=local)"
	if err == nil || !strings.Contains(out, want) {
		t.Fatalf("err=%v out=%q, want %q", err, out, want)
	}
}

// In auto mode a newer toolchain line means a download. With no
// proxy the build stops before compiling anything.
func TestAutoNeedsADownload(t *testing.T) {
	dir := module(t, "module example.com/m\n\ngo 1.26.0\n\n"+
		"toolchain go1.99.0\n")
	out, err := goCmd(t, dir,
		[]string{"GOTOOLCHAIN=auto", "GODEBUG=toolchaintrace=1"},
		"build", "-o", os.DevNull, ".")
	if err == nil {
		t.Fatal("build succeeded without the toolchain")
	}
	for _, want := range []string{
		"upgrading toolchain to go1.99.0",
		"toolchain not available",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

// go get keeps the graph consistent: asking for a go line older
// than a dependency needs downgrades that dependency, and says so.
func TestGoGetDowngradesDependency(t *testing.T) {
	proxy := t.TempDir()
	for _, v := range []struct{ ver, golang string }{
		{"v1.0.0", "1.20"}, {"v1.1.0", "1.22"},
	} {
		addModule(t, proxy, "example.com/dep", v.ver,
			"module example.com/dep\n\ngo "+v.golang+"\n",
			"package dep\n\nfunc F() {}\n")
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"),
		"module example.com/m\n\ngo 1.22\n")
	write(t, filepath.Join(dir, "m.go"), "package m\n\n"+
		"import \"example.com/dep\"\n\nvar _ = dep.F\n")
	env := []string{"GOPROXY=file://" + filepath.ToSlash(proxy),
		"GOMODCACHE=" + t.TempDir()}
	if out, err := goCmd(t, dir, env, "get",
		"example.com/dep@v1.1.0"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	out, err := goCmd(t, dir, env, "get", "go@1.21.0")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	want := "downgraded example.com/dep v1.1.0 => v1.0.0"
	if !strings.Contains(out, want) {
		t.Fatalf("output lacks %q:\n%s", want, out)
	}
	mod, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
	if !strings.Contains(string(mod), "go 1.21.0") ||
		!strings.Contains(string(mod), "example.com/dep v1.0.0") {
		t.Fatalf("go.mod:\n%s", mod)
	}
}

// addModule writes one module version into a file-based GOPROXY.
func addModule(t *testing.T, proxy, path, ver, gomod,
	src string) {
	t.Helper()
	base := filepath.Join(proxy, filepath.FromSlash(path), "@v")
	write(t, filepath.Join(base, ver+".mod"), gomod)
	write(t, filepath.Join(base, ver+".info"),
		`{"Version":"`+ver+`"}`)
	f, err := os.OpenFile(filepath.Join(base, "list"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(ver + "\n")
	f.Close()
	zf, err := os.Create(filepath.Join(base, ver+".zip"))
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)
	prefix := path + "@" + ver + "/"
	for name, data := range map[string]string{
		"go.mod": gomod, "x.go": src,
	} {
		w, err := zw.Create(prefix + name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(data))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zf.Close()
}

// The printed command writes both lines; GOTOOLCHAIN=local and
// GOPROXY=off prove it needs no download when the local Go is
// already the toolchain it names.
func TestGoGetWritesBothLines(t *testing.T) {
	dir := module(t, "module example.com/m\n\ngo 1.22\n")
	out, err := goCmd(t, dir, nil, "get", "go@1.26.0",
		"toolchain@go"+localVersion())
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	mod, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
	want := "go 1.26.0\n\ntoolchain go" + localVersion() + "\n"
	if !strings.Contains(string(mod), want) {
		t.Fatalf("go.mod:\n%s\nwant it to contain:\n%s", mod, want)
	}
	out, err = goCmd(t, dir, []string{"GODEBUG=toolchaintrace=1"},
		"version")
	if err != nil || !strings.Contains(out,
		"using local toolchain "+runtime.Version()) {
		t.Fatalf("toolchaintrace: %v\n%s", err, out)
	}
}
