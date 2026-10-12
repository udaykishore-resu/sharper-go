package debug

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec
}

// nonLoopback returns one of this host's non-loopback IPv4
// addresses, the kind a pod IP or node port would use.
func nonLoopback() (string, bool) {
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && !n.IP.IsLoopback() && n.IP.To4() != nil {
			return n.IP.String(), true
		}
	}
	return "", false
}

// serveDebug answers on loopback and refuses the host's other
// addresses, so nothing that reaches the pod IP reaches pprof.
// Shutdown is the caller's job (Technique 45); after Close the
// port is gone.
func TestServeDebugLoopbackOnly(t *testing.T) {
	if ln, err := net.Listen("tcp", "127.0.0.1:6060"); err != nil {
		t.Skip("port 6060 in use")
	} else {
		ln.Close()
	}
	srv := serveDebug()
	url := "http://127.0.0.1:6060/debug/pprof/"
	var resp *http.Response
	var err error
	for range 50 { // the server starts in a goroutine
		if resp, err = http.Get(url); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("loopback: %v %v", err, resp)
	}
	resp.Body.Close()
	if ip, ok := nonLoopback(); !ok {
		t.Log("no non-loopback address to probe")
	} else {
		addr := net.JoinHostPort(ip, "6060")
		if c, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
			c.Close()
			t.Fatalf("pprof reachable on %s", addr)
		}
	}
	srv.Close()
	if c, err := net.DialTimeout("tcp", "127.0.0.1:6060", time.Second); err == nil {
		c.Close()
		t.Fatal("still listening after Close")
	}
}

func TestPrivateMux(t *testing.T) {
	mux := debugMux()
	for _, p := range []string{"/debug/pprof/heap?debug=1", "/debug/pprof/goroutine?debug=1"} {
		if rec := get(mux, p); rec.Code != 200 {
			t.Errorf("%s: %d", p, rec.Code)
		}
	}
	// Index also lists the named profiles, Go 1.27's goroutineleak included.
	if body := get(mux, "/debug/pprof/").Body.String(); !strings.Contains(body, "allocs") {
		t.Errorf("index: %.200s", body)
	}
}

// Importing net/http/pprof at all, blank or not, runs its init,
// which registers on http.DefaultServeMux. So the public server
// must never serve the default mux: give it its own.
func TestDefaultMuxIsTainted(t *testing.T) {
	if rec := get(http.DefaultServeMux, "/debug/pprof/"); rec.Code != 200 {
		t.Fatalf("expected init to register on DefaultServeMux, got %d", rec.Code)
	}
	// A Server with a nil Handler serves DefaultServeMux too.
	ts := httptest.NewServer(nil)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/debug/pprof/")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("nil Handler did not serve pprof: %v %v", err, resp)
	}
	resp.Body.Close()
	public := http.NewServeMux()
	public.HandleFunc("GET /v1/accounts/{id}", func(http.ResponseWriter, *http.Request) {})
	if rec := get(public, "/debug/pprof/"); rec.Code != 404 {
		t.Fatalf("public mux serves pprof: %d", rec.Code)
	}
}
