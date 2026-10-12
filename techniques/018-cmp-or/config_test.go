package config

import (
	"errors"
	"testing"
	"time"
)

func TestPrecedence(t *testing.T) {
	t.Setenv("LEDGERD_ADDR", ":7000")
	file := Config{Addr: ":6000", Timeout: 2 * time.Second}
	if got := load("", file).Addr; got != ":7000" {
		t.Fatalf("env must beat file, got %s", got)
	}
	if got := load(":9000", file).Addr; got != ":9000" {
		t.Fatalf("flag must win, got %s", got)
	}
	t.Setenv("LEDGERD_ADDR", "")
	c := load("", Config{})
	if c.Addr != ":8080" || c.Timeout != 5*time.Second || c.Page != 100 {
		t.Fatalf("defaults: %+v", c)
	}
}

func TestZeroIsNotUnset(t *testing.T) {
	if got := load("", Config{Retries: 0}).Retries; got != 3 {
		t.Fatalf("Retries = %d", got)
	}
	// "retries: 0" in the file became 3. Use a pointer for fields
	// where zero means something.
}

func TestFirstError(t *testing.T) {
	ran := 0
	a := func() error { ran++; return errors.New("a") }
	b := func() error { ran++; return errors.New("b") }
	if err := closeBoth(a, b); err.Error() != "a" || ran != 2 {
		t.Fatalf("err=%v ran=%d", err, ran)
	}
}
