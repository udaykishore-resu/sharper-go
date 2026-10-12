package config

import (
	"strings"
	"testing"
)

// Review check for the idempotency-key scope: keys are stored as
// client + "/" + key, so a client name with a slash could collide
// with another client's keys ("a/b" + "c" == "a" + "b/c").
// Load refuses such a name.
func TestKeysRejectSlashInClient(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL": "postgres://x",
		"API_KEYS":     "a:tok-aaaaaaaaaaaaaaaa,a/b:tok-bbbbbbbbbbbbbbbb",
	}
	_, err := Load(func(k string) string { return env[k] })
	if err == nil || !strings.Contains(err.Error(), `client "a/b"`) {
		t.Fatalf("Load = %v, want the a/b client refused", err)
	}
	env["API_KEYS"] = "a:tok-aaaaaaaaaaaaaaaa,b:tok-bbbbbbbbbbbbbbbb"
	c, err := Load(func(k string) string { return env[k] })
	if err != nil || len(c.Keys) != 2 {
		t.Fatalf("Load = %+v, %v", c.Keys, err)
	}
}
