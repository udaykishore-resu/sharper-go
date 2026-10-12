package goprivate

import (
	"archive/zip"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func goCmd(t *testing.T, env []string,
	args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "GOFLAGS=-modcacherw",
		"GOMODCACHE="+t.TempDir(), "GOWORK=off",
		"GOTOOLCHAIN=local", "GOPRIVATE=", "GONOPROXY=",
		"GONOSUMDB=", "GOINSECURE=", "GIT_TERMINAL_PROMPT=0")
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// The printed script, sourced: GOPRIVATE sets the defaults for
// GONOPROXY and GONOSUMDB, and the go command accepts GOFLAGS.
func TestScript(t *testing.T) {
	script, err := filepath.Abs("goenv.sh")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", ". "+script+
		" && go env GOPRIVATE GONOPROXY GONOSUMDB GOFLAGS")
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "GONOPROXY=", "GONOSUMDB=")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err, string(out))
	}
	want := "git.corp.example.com/*\ngit.corp.example.com/*\n" +
		"git.corp.example.com/*\n-trimpath -buildvcs=true\n"
	if string(out) != want {
		t.Fatalf("go env:\n%s\nwant:\n%s", out, want)
	}
}

// GOFLAGS splits on spaces, so -ldflags=-s -w becomes -ldflags=-s
// plus a stray -w. Some go command knows -w (go env -w), so go
// build accepts it and ignores it: the build silently keeps DWARF.
func TestGOFLAGSSpace(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/m\n\ngo 1.26.0\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"),
		[]byte("package main\n\nfunc main() {}\n"), 0o644)
	bin := filepath.Join(dir, "m")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-ldflags=-s -w",
		"GOWORK=off", "GOTOOLCHAIN=local")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	out, err := exec.Command("go", "version", "-m", bin).Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "-ldflags=-s\n") {
		t.Fatalf("want -ldflags=-s only:\n%s", out)
	}
}

// proxy is an httptest module proxy that answers every request
// with one status code and records the paths it was asked for.
type proxy struct {
	*httptest.Server
	mu    sync.Mutex
	paths []string
}

func newProxy(t *testing.T, status int) *proxy {
	p := &proxy{}
	p.Server = httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			p.mu.Lock()
			p.paths = append(p.paths, r.URL.Path)
			p.mu.Unlock()
			w.WriteHeader(status)
		}))
	t.Cleanup(p.Close)
	return p
}

func (p *proxy) asked(s string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, path := range p.paths {
		if strings.Contains(path, s) {
			return true
		}
	}
	return false
}

// fileProxy serves one module version from disk.
func fileProxy(t *testing.T, path, ver string) string {
	t.Helper()
	dir := t.TempDir()
	base := filepath.Join(dir, filepath.FromSlash(path), "@v")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	gomod := "module " + path + "\n\ngo 1.26.0\n"
	files := map[string]string{
		ver + ".mod":  gomod,
		ver + ".info": `{"Version":"` + ver + `"}`,
		"list":        ver + "\n",
	}
	for name, data := range files {
		err := os.WriteFile(filepath.Join(base, name), []byte(data),
			0o644)
		if err != nil {
			t.Fatal(err)
		}
	}
	zf, err := os.Create(filepath.Join(base, ver+".zip"))
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)
	for name, data := range map[string]string{
		"go.mod": gomod, "m.go": "package m\n",
	} {
		w, _ := zw.Create(path + "@" + ver + "/" + name)
		w.Write([]byte(data))
	}
	zw.Close()
	zf.Close()
	return "file://" + filepath.ToSlash(dir)
}

// A comma falls through only on 404 or 410; a pipe falls through
// on any error.
func TestProxySeparators(t *testing.T) {
	const mod = "example.com/pub/m"
	files := fileProxy(t, mod, "v1.0.0")
	cases := []struct {
		status int
		sep    string
		ok     bool
	}{
		{http.StatusNotFound, ",", true},
		{http.StatusGone, ",", true},
		{http.StatusInternalServerError, ",", false},
		{http.StatusBadGateway, ",", false},
		{http.StatusInternalServerError, "|", true},
	}
	for _, c := range cases {
		first := newProxy(t, c.status)
		out, err := goCmd(t, []string{
			"GOPROXY=" + first.URL + c.sep + files,
			"GOSUMDB=off"},
			"mod", "download", mod+"@v1.0.0")
		if (err == nil) != c.ok {
			t.Errorf("%d with %q: ok=%v, want %v\n%s",
				c.status, c.sep, err == nil, c.ok, out)
		}
	}
}

// Without GOPRIVATE the private path goes to the proxy, the leak
// from the opening scene. With it, the proxy never hears of it.
func TestGOPRIVATEKeepsPathsHome(t *testing.T) {
	const mod = "example.com/private/moneykit"
	leak := newProxy(t, http.StatusNotFound)
	goCmd(t, []string{"GOPROXY=" + leak.URL, "GOSUMDB=off"},
		"mod", "download", mod+"@v0.3.0")
	if !leak.asked("private/moneykit") {
		t.Fatal("without GOPRIVATE the proxy was not asked")
	}

	quiet := newProxy(t, http.StatusNotFound)
	goCmd(t, []string{"GOPROXY=" + quiet.URL,
		"GOPRIVATE=example.com/private/*"},
		"mod", "download", mod+"@v0.3.0")
	if quiet.asked("private/moneykit") {
		t.Fatal("GOPRIVATE path reached the proxy")
	}
}

// GONOPROXY=none sends private modules through the internal
// proxy again, while GONOSUMDB still comes from GOPRIVATE: the
// download succeeds with the public checksum database unreachable.
func TestGONOPROXYNone(t *testing.T) {
	const mod = "example.com/private/moneykit"
	files := fileProxy(t, mod, "v0.3.0")
	out, err := goCmd(t, []string{"GOPROXY=" + files,
		"GOPRIVATE=example.com/private/*", "GONOPROXY=none",
		"GOSUMDB=sum.golang.org http://127.0.0.1:1"},
		"mod", "download", mod+"@v0.3.0")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	// Control: without GOPRIVATE the same download asks the
	// checksum database, which is unreachable here, and fails.
	if out, err := goCmd(t, []string{"GOPROXY=" + files,
		"GOSUMDB=sum.golang.org http://127.0.0.1:1"},
		"mod", "download", mod+"@v0.3.0"); err == nil {
		t.Fatalf("checksum database was not consulted:\n%s", out)
	}
}
