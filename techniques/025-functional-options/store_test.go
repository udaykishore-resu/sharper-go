package store

import (
	"testing"
	"time"
)

func TestOptions(t *testing.T) {
	s, err := New("postgres://x")
	if err != nil || s.maxConn != 10 || s.timeout != 5*time.Second {
		t.Fatalf("defaults: %+v %v", s, err)
	}
	s, _ = New("postgres://x", WithMaxConns(40))
	if s.maxConn != 40 || s.timeout != 5*time.Second {
		t.Fatalf("WithMaxConns: %+v", s)
	}
}

// Options run in the order given; the same option twice means the
// last one wins. Options that set different fields commute.
func TestPrecedence(t *testing.T) {
	s, err := New("postgres://x", WithMaxConns(20), WithMaxConns(40))
	if err != nil || s.maxConn != 40 {
		t.Fatalf("last WithMaxConns should win: %+v %v", s, err)
	}
	a, _ := New("postgres://x", WithMaxConns(20), WithTimeout(time.Second))
	b, _ := New("postgres://x", WithTimeout(time.Second), WithMaxConns(20))
	if *a != *b {
		t.Fatalf("field-only options should commute: %+v vs %+v", a, b)
	}
	// A later valid value rescues an earlier invalid one, because
	// validation runs once, after every option.
	if _, err := New("postgres://x", WithMaxConns(0), WithMaxConns(5)); err != nil {
		t.Fatalf("validation ran before the last option: %v", err)
	}
}

func TestValidationAfterOptions(t *testing.T) {
	for name, opts := range map[string][]Option{
		"zero pool":     {WithMaxConns(0)},
		"negative pool": {WithMaxConns(-1)},
		"zero timeout":  {WithTimeout(0)},
		"last bad wins": {WithMaxConns(5), WithMaxConns(0)},
	} {
		if s, err := New("postgres://x", opts...); err == nil {
			t.Errorf("%s: want an error, got %+v", name, s)
		}
	}
}
