package ledgerctl

import (
	"io"
	"os"
	"strings"
	"testing"
)

// Review checks for Technique 97: missing and malformed flags,
// help output, subcommand errors and exit codes, and no secret in
// any of that output.

const secret = "s3cret-token-from-env"

// capture runs Main with os.Stderr redirected, returning the exit
// code and everything written to stderr.
func capture(t *testing.T, args ...string) (int, string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	code := Main(t.Context(), args)
	os.Stderr = old
	w.Close()
	b, _ := io.ReadAll(r)
	return code, string(b)
}

func TestCLIErrorsAndHelp(t *testing.T) {
	t.Setenv("LEDGERCTL_TOKEN", secret)
	cases := []struct {
		name string
		args []string
		code int
		want string
	}{
		{"no command", nil, 1, "replay   re-post transfers"},
		{"unknown command", []string{"rewind"}, 1, "missing or unknown command"},
		{"missing file", []string{"replay"}, 1, "want one file"},
		{"malformed value", []string{"replay", "-dry-run=maybe", "f"}, 1,
			`invalid boolean value "maybe"`},
		{"unknown flag", []string{"replay", "-force", "f"}, 1,
			"flag provided but not defined: -force"},
		{"flag after file", []string{"replay", "f", "-dry-run=false"}, 1,
			`got ["f" "-dry-run=false"]`},
		{"help", []string{"replay", "-h"}, 0, "-dry-run"},
		{"ok", []string{"replay", "-dry-run=false", "f"}, 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out := capture(t, tc.args...)
			if code != tc.code || !strings.Contains(out, tc.want) {
				t.Fatalf("exit %d, stderr:\n%s", code, out)
			}
			if strings.Contains(out, secret) {
				t.Fatalf("secret echoed:\n%s", out)
			}
		})
	}
	if lastReplay.token != secret {
		t.Fatalf("token from env not used: %q", lastReplay.token)
	}
}

// The flag beats the environment, and with neither the command
// fails before doing anything.
func TestTokenSources(t *testing.T) {
	t.Setenv("LEDGERCTL_TOKEN", secret)
	if code, _ := capture(t, "replay", "-token", "from-flag", "f"); code != 0 ||
		lastReplay.token != "from-flag" {
		t.Fatalf("exit %d, token %q", code, lastReplay.token)
	}
	t.Setenv("LEDGERCTL_TOKEN", "")
	if code, out := capture(t, "replay", "f"); code != 1 || !strings.Contains(out, "no token") {
		t.Fatalf("exit %d: %s", code, out)
	}
}
