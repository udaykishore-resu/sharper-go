package config

import (
	"cmp"
	"net/url"
	"testing"
	"time"
)

func TestEnvDuration(t *testing.T) {
	t.Setenv("LEDGERD_TIMEOUT", "")
	if d, err := envDuration("LEDGERD_TIMEOUT"); d != 0 || err != nil {
		t.Fatalf("unset: %v, %v", d, err)
	}
	t.Setenv("LEDGERD_TIMEOUT", "30s")
	d, err := envDuration("LEDGERD_TIMEOUT")
	if err != nil || cmp.Or(d, 2*time.Second, 5*time.Second) != 30*time.Second {
		t.Fatalf("env must beat file: %v, %v", d, err)
	}
	t.Setenv("LEDGERD_TIMEOUT", "5sec")
	if _, err := envDuration("LEDGERD_TIMEOUT"); err == nil {
		t.Fatal("a bad value must be an error, not a fallback")
	}
}

// Empty strings at every level fall through to the default;
// conflicting values resolve flag > env > file.
func TestEmptyAndConflicting(t *testing.T) {
	t.Setenv("LEDGERD_ADDR", "")
	if got := load("", Config{Addr: ""}).Addr; got != ":8080" {
		t.Fatalf("all empty: %s", got)
	}
	t.Setenv("LEDGERD_ADDR", ":7000")
	f := Config{Addr: ":6000"}
	for flag, want := range map[string]string{"": ":7000", ":9000": ":9000"} {
		if got := load(flag, f).Addr; got != want {
			t.Fatalf("flag %q: got %s, want %s", flag, got, want)
		}
	}
	t.Setenv("LEDGERD_ADDR", "")
	if got := load("", f).Addr; got != ":6000" {
		t.Fatalf("file must beat default: %s", got)
	}
}

// In a handler, cmp.Or picks a default sort order. It compares
// nothing; tie-breaking in a comparator is a different job
// (Technique 14's test covers that).
func TestDefaultSortOrder(t *testing.T) {
	for raw, want := range map[string]string{
		"": "id", "sort=amount": "amount", "sort=": "id",
	} {
		q, err := url.ParseQuery(raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := cmp.Or(q.Get("sort"), "id"); got != want {
			t.Fatalf("%q: got %s, want %s", raw, got, want)
		}
	}
}
