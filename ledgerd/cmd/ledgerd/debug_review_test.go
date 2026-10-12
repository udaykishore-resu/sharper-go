package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"example.com/ledgerd/internal/httpapi"
	"example.com/ledgerd/internal/obs"
)

func freeLoopback(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

// Technique 73 review: the pprof server is loopback-only, serves
// on its own mux, and goes down with the public server.
func TestDebugServerLoopbackAndShutdown(t *testing.T) {
	drainDelay = 50 * time.Millisecond
	h := httpapi.NewHealth(func(context.Context) error { return nil })
	pub, dbg := freeLoopback(t), freeLoopback(t)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- runServers(ctx, time.Second, h,
			httpapi.NewServer(pub, h.Mux(http.NotFoundHandler())),
			internal(dbg, obs.DebugMux()))
	}()
	get := func(addr string) (int, error) {
		resp, err := http.Get("http://" + addr + "/debug/pprof/")
		if err != nil {
			return 0, err
		}
		resp.Body.Close()
		return resp.StatusCode, nil
	}
	var code int
	var err error
	for range 100 {
		if code, err = get(dbg); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if code != 200 {
		t.Fatalf("debug server: %d %v", code, err)
	}
	if code, _ := get(pub); code == 200 {
		t.Fatal("public server serves pprof")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if c, err := net.DialTimeout("tcp", dbg, time.Second); err == nil {
		c.Close()
		t.Fatal("debug server outlived the public one")
	}
}

// The probes-and-API root mux never falls through to pprof.
func TestRootMuxHasNoPprof(t *testing.T) {
	h := httpapi.NewHealth(func(context.Context) error { return nil })
	rec := httptest.NewRecorder()
	h.Mux(http.NotFoundHandler()).ServeHTTP(rec,
		httptest.NewRequest("GET", "/debug/pprof/heap", nil))
	if rec.Code == 200 {
		t.Fatal("root mux served a heap profile")
	}
}

// Neither the Service nor the container ports expose 6060.
func TestManifestsDoNotExposeDebugPort(t *testing.T) {
	for _, f := range []string{"../../k8s/service.yaml", "../../k8s/deployment.yaml"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			code, _, _ := strings.Cut(line, "#")
			if strings.Contains(code, "6060") {
				t.Errorf("%s exposes 6060: %s", f, line)
			}
		}
	}
}
