package gowork

import (
	"archive/zip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func goCmd(t *testing.T, dir string, env []string,
	args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=", "GOPROXY=off",
		"GOSUMDB=off", "GOTOOLCHAIN=local", "GOWORK=")
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

// layout writes src/ledgerd and src/moneykit. ledgerd calls a
// function that exists only in the local moneykit checkout.
func layout(t *testing.T) (src, ledgerd string) {
	t.Helper()
	src = t.TempDir()
	ledgerd = filepath.Join(src, "ledgerd")
	write(t, filepath.Join(ledgerd, "go.mod"),
		"module example.com/ledgerd\n\ngo 1.26.0\n\n"+
			"require example.com/moneykit v0.3.0\n")
	write(t, filepath.Join(ledgerd, "fee.go"),
		"package ledgerd\n\nimport \"example.com/moneykit\"\n\n"+
			"func Fee(c int64) int64 { return moneykit.Round(c) }\n")
	write(t, filepath.Join(ledgerd, "fee_test.go"),
		"package ledgerd\n\nimport \"testing\"\n\n"+
			"func TestFee(t *testing.T) {\n"+
			"\tif Fee(5) != 10 {\n\t\tt.Fatal(Fee(5))\n\t}\n}\n")
	mk := filepath.Join(src, "moneykit")
	write(t, filepath.Join(mk, "go.mod"),
		"module example.com/moneykit\n\ngo 1.26.0\n")
	write(t, filepath.Join(mk, "round.go"),
		"package moneykit\n\n"+
			"// Round is the unreleased change.\n"+
			"func Round(c int64) int64 { return (c + 9) / 10 * 10 }\n")
	write(t, filepath.Join(mk, "round_test.go"),
		"package moneykit\n\nimport \"testing\"\n\n"+
			"func TestRound(t *testing.T) {\n"+
			"\tif Round(1) != 10 {\n\t\tt.Fatal(Round(1))\n\t}\n}\n")
	return src, ledgerd
}

func TestWorkspace(t *testing.T) {
	src, ledgerd := layout(t)
	gomod, _ := os.ReadFile(filepath.Join(ledgerd, "go.mod"))

	// Without a workspace, v0.3.0 can't be found: it doesn't exist.
	if out, err := goCmd(t, ledgerd, nil, "build", "./..."); err == nil {
		t.Fatalf("built without moneykit:\n%s", out)
	}

	// go work init resolves its paths from the current folder,
	// which is why it runs in src/, the parent of both checkouts.
	if out, err := goCmd(t, src, nil, "work", "init",
		"./ledgerd", "./moneykit"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}

	// The go command finds src/go.work by walking up from ledgerd/.
	out, err := goCmd(t, ledgerd, nil, "env", "GOWORK")
	if err != nil || strings.TrimSpace(out) !=
		filepath.Join(src, "go.work") {
		t.Fatalf("GOWORK = %q, %v", out, err)
	}
	if out, err := goCmd(t, ledgerd, nil, "test", "./..."); err != nil {
		t.Fatalf("workspace test failed: %v\n%s", err, out)
	}

	// The work pattern (Go 1.25) tests every workspace module.
	out, err = goCmd(t, ledgerd, nil, "test", "work")
	if err != nil {
		t.Fatalf("go test work: %v\n%s", err, out)
	}
	for _, m := range []string{"example.com/ledgerd",
		"example.com/moneykit"} {
		if !strings.Contains(out, m) {
			t.Errorf("go test work skipped %s:\n%s", m, out)
		}
	}

	// Workspace mode refuses -mod=mod.
	out, err = goCmd(t, ledgerd, []string{"GOFLAGS=-mod=mod"},
		"build", "./...")
	const want = "-mod may only be set to readonly or vendor " +
		"when in workspace mode"
	if err == nil || !strings.Contains(out, want) {
		t.Fatalf("err=%v out=%q, want %q", err, out, want)
	}

	// GOWORK=off is the build everyone else gets: it fails until
	// moneykit is tagged and published.
	if out, err := goCmd(t, ledgerd, []string{"GOWORK=off"},
		"build", "./..."); err == nil {
		t.Fatalf("GOWORK=off built against the local folder:\n%s",
			out)
	}

	// Nothing in either go.mod changed.
	after, _ := os.ReadFile(filepath.Join(ledgerd, "go.mod"))
	if string(after) != string(gomod) {
		t.Fatalf("go.mod changed:\n%s", after)
	}
}

// The release order from the book: tag moneykit, go get it from
// inside the workspace, then test once with the workspace off.
// go get is the first command that fetches anything.
func TestReleaseFlow(t *testing.T) {
	src, ledgerd := layout(t)
	proxy := t.TempDir()
	round, _ := os.ReadFile(filepath.Join(src, "moneykit",
		"round.go"))
	addModule(t, proxy, "example.com/moneykit", "v0.3.0",
		"package moneykit\n")
	addModule(t, proxy, "example.com/moneykit", "v0.4.0",
		string(round))
	env := []string{"GOPROXY=file://" + filepath.ToSlash(proxy),
		"GOMODCACHE=" + t.TempDir(), "GOFLAGS=-modcacherw"}
	if out, err := goCmd(t, src, env, "work", "init",
		"./ledgerd", "./moneykit"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if out, err := goCmd(t, ledgerd, env, "get",
		"example.com/moneykit@v0.4.0"); err != nil {
		t.Fatalf("go get in workspace: %v\n%s", err, out)
	}
	off := append(env, "GOWORK=off")
	if out, err := goCmd(t, ledgerd, off, "test",
		"./..."); err != nil {
		t.Fatalf("GOWORK=off test: %v\n%s", err, out)
	}
}

// addModule writes one module version into a file-based GOPROXY.
func addModule(t *testing.T, proxy, path, ver, src string) {
	t.Helper()
	gomod := "module " + path + "\n\ngo 1.26.0\n"
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
