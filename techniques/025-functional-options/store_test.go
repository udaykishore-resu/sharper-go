package store

import "testing"

func TestOptions(t *testing.T) {
	s, err := New("postgres://x")
	if err != nil || s.maxConn != 10 || s.timeout.Seconds() != 5 {
		t.Fatalf("defaults: %+v %v", s, err)
	}
	s, _ = New("postgres://x", WithMaxConns(40))
	if s.maxConn != 40 {
		t.Fatalf("WithMaxConns: %d", s.maxConn)
	}
	if _, err := New("postgres://x", WithMaxConns(0)); err == nil {
		t.Fatal("validation must run after the options")
	}
}
