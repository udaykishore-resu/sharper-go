package server

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A slowloris client that never finishes its headers is cut off
// by ReadHeaderTimeout. The test shortens it to keep CI fast.
func TestSlowHeadersCutOff(t *testing.T) {
	srv := newServer(http.NotFoundHandler())
	srv.ReadHeaderTimeout = 100 * time.Millisecond
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(ln)
	defer srv.Close()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\n")) // never finishes
	start := time.Now()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 512)
	for {
		if _, err := conn.Read(buf); err != nil {
			break
		}
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("server held the connection for %v", d)
	}
}

func TestTimeoutsSet(t *testing.T) {
	s := newServer(nil)
	if s.ReadHeaderTimeout == 0 || s.ReadTimeout == 0 ||
		s.WriteTimeout == 0 || s.IdleTimeout == 0 {
		t.Fatalf("a timeout is zero: %+v", s)
	}
}

func TestExportExtendsDeadline(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(export))
	srv.Config.WriteTimeout = time.Second
	srv.Start()
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("resp=%v err=%v", resp, err)
	}
	resp.Body.Close()
}
