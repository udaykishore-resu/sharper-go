package export

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	accounts "github.com/udaykishore-resu/sharper-go/techniques/013-iterators"
)

// pager serves n accounts with zero balances and fails on call
// failAt, like the Postgres failover in the story.
type pager struct{ n, calls, failAt int }

func (p *pager) Page(_ context.Context, after string,
	n int) ([]accounts.Account, error) {
	p.calls++
	if p.calls == p.failAt {
		return nil, errors.New("failover")
	}
	start := 0
	if after != "" {
		fmt.Sscanf(after, "a%06d", &start)
		start++
	}
	var out []accounts.Account
	for i := start; i < p.n && len(out) < n; i++ {
		out = append(out, accounts.Account{ID: fmt.Sprintf("a%06d", i)})
	}
	return out, nil
}

func intp(v int) *int { return &v }

// The whole story: a page fails mid-export, the config says zero
// retries, and every balance is zero. The export reports the
// error, the short file has no trailer, and Upload refuses to
// send it at all.
func TestShortExportIsNeverUploaded(t *testing.T) {
	var buf bytes.Buffer
	p := &pager{n: 48_000, failAt: 59}
	err := Write(&buf, accounts.Accounts(t.Context(), p))
	if err == nil || !strings.Contains(err.Error(), "after 29000 accounts") {
		t.Fatalf("Write = %v", err)
	}
	sent := 0
	send := func([]byte) error { sent++; return nil }
	err = Upload(UploadConfig{Retries: intp(0)}, buf.Bytes(), send)
	if !errors.Is(err, ErrNoTrailer) || sent != 0 {
		t.Fatalf("Upload = %v, sent %d times", err, sent)
	}
}

func TestFullExportUploadsOnce(t *testing.T) {
	var buf bytes.Buffer
	p := &pager{n: 1234}
	if err := Write(&buf, accounts.Accounts(t.Context(), p)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"balance":0`) {
		t.Fatal("zero balances must be encoded")
	}
	calls := 0
	fail := func([]byte) error { calls++; return errors.New("timeout") }
	if Upload(UploadConfig{Retries: intp(0)}, buf.Bytes(), fail) == nil ||
		calls != 1 {
		t.Fatalf("retries: 0 made %d calls", calls)
	}
	calls = 0
	_ = Upload(UploadConfig{}, buf.Bytes(), fail)
	if calls != 4 {
		t.Fatalf("unset retries made %d calls, want 1+3", calls)
	}
}

func TestRetriesFromConfig(t *testing.T) {
	for raw, want := range map[string]int{
		`{"upload_retries":0}`: 1, `{}`: 4, `{"upload_retries":2}`: 3,
	} {
		var c UploadConfig
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			t.Fatal(err)
		}
		if got := c.Attempts(); got != want {
			t.Fatalf("%s: %d attempts, want %d", raw, got, want)
		}
	}
}

func TestVerifyRejectsTampering(t *testing.T) {
	for _, f := range []string{
		"", `{"id":"a","balance":1}` + "\n",
		`{"id":"a","balance":1}` + "\n" + `{"count":2}` + "\n",
	} {
		if Verify(strings.NewReader(f)) == nil {
			t.Fatalf("accepted %q", f)
		}
	}
}
