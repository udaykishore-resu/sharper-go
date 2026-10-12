package server

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// start serves h with newServer's settings, shortened by tweak.
func start(t *testing.T, h http.Handler,
	tweak func(*http.Server)) (string, *http.Server) {
	t.Helper()
	srv := newServer(h)
	tweak(srv)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return ln.Addr().String(), srv
}

// ReadTimeout covers the body: a client that stalls mid-upload
// makes the handler's read fail.
func TestSlowBodyFailsRead(t *testing.T) {
	readErr := make(chan error, 1)
	addr, _ := start(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		readErr <- err
	}), func(s *http.Server) { s.ReadTimeout = 150 * time.Millisecond })
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	io.WriteString(conn, "POST / HTTP/1.1\r\nHost: x\r\n"+
		"Content-Length: 100\r\n\r\n0123456789") // then stall
	select {
	case err := <-readErr:
		if err == nil {
			t.Fatal("stalled body read succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ReadTimeout never fired")
	}
}

// WriteTimeout does not stop a slow handler: it runs to the end,
// and the client gets no status line at all.
func TestWriteTimeoutDoesNotStopHandler(t *testing.T) {
	finished := make(chan struct{})
	addr, _ := start(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.Write([]byte("late"))
		close(finished)
	}), func(s *http.Server) { s.WriteTimeout = 100 * time.Millisecond })
	resp, err := http.Get("http://" + addr)
	if err == nil {
		resp.Body.Close()
		t.Fatalf("client got %d after WriteTimeout", resp.StatusCode)
	}
	select {
	case <-finished: // the handler kept running
	case <-time.After(3 * time.Second):
		t.Fatal("handler never finished")
	}
}

// WriteTimeout also cuts a client that stops reading a big or
// streamed response: the handler's Write fails.
func TestSlowReaderFailsWrite(t *testing.T) {
	writeErr := make(chan error, 1)
	addr, _ := start(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunk := make([]byte, 64<<10)
		for {
			if _, err := w.Write(chunk); err != nil {
				writeErr <- err
				return
			}
		}
	}), func(s *http.Server) { s.WriteTimeout = 200 * time.Millisecond })
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	io.WriteString(conn, "GET / HTTP/1.1\r\nHost: x\r\n\r\n") // never read
	select {
	case <-writeErr:
	case <-time.After(5 * time.Second):
		t.Fatal("write to a stalled reader never failed")
	}
}

// IdleTimeout closes a keep-alive connection between requests.
func TestIdleConnectionClosed(t *testing.T) {
	addr, _ := start(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}), func(s *http.Server) { s.IdleTimeout = 150 * time.Millisecond })
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	io.WriteString(conn, "GET / HTTP/1.1\r\nHost: x\r\n\r\n")
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	start := time.Now()
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, err = br.ReadByte() // blocks until the server hangs up
	if err == nil || strings.Contains(err.Error(), "timeout") ||
		time.Since(start) > 2*time.Second {
		t.Fatalf("idle conn not closed: %v after %v", err, time.Since(start))
	}
}

// A stream that outlives WriteTimeout is cut mid-body, even while
// the client reads happily; export avoids it by extending its own
// deadline (TestExportExtendsDeadline).
func TestStreamCutByWriteTimeout(t *testing.T) {
	addr, _ := start(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rc := http.NewResponseController(w)
		for range 20 {
			if _, err := w.Write([]byte("row\n")); err != nil {
				return
			}
			rc.Flush()
			time.Sleep(25 * time.Millisecond)
		}
	}), func(s *http.Server) { s.WriteTimeout = 150 * time.Millisecond })
	resp, err := http.Get("http://" + addr)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err == nil || len(b) >= 20*len("row\n") {
		t.Fatalf("read %d bytes, err %v: stream was not cut", len(b), err)
	}
}
