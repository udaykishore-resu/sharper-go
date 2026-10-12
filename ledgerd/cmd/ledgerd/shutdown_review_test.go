//go:build integration

package main

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"example.com/ledgerd/internal/config"
	"example.com/ledgerd/internal/store"
)

// The Part V checklist test: SIGTERM arrives while a transfer is
// in flight, the old pod drains and exits, and the client
// retries with the same key against a new pod. Money moves
// exactly once, and the key is never left stuck in flight.
// Needs LEDGERD_TEST_DATABASE_URL (a migrated or empty database).

// pod starts serve on fresh ports under a real SIGTERM handler
// and returns its API base URL and its exit channel.
func pod(t *testing.T, dsn string, cfg config.Config) (string, chan error) {
	t.Helper()
	apiLn := make(chan string, 1)
	listen = func(network, addr string) (net.Listener, error) {
		ln, err := net.Listen(network, "127.0.0.1:0")
		if err == nil && addr == ":8080" {
			apiLn <- ln.Addr().String()
		}
		return ln, err
	}
	t.Cleanup(func() { listen = net.Listen })
	st, err := store.Open(t.Context(), dsn, store.WithMaxConns(4))
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	done := make(chan error, 1)
	go func() {
		defer stop()
		defer st.Close() // run's order: servers, relay, then pool
		done <- serve(ctx, cfg, slog.New(slog.DiscardHandler), st)
	}()
	base := "http://" + <-apiLn
	for i := 0; ; i++ {
		resp, err := client.Get(base + "/readyz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return base, done
			}
		}
		if i > 300 {
			t.Fatal("pod never became ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// client opens a fresh connection per request, so no half-dialed
// spare connection (which Shutdown waits up to 5 s for) is left.
var client = &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}

func postTransfer(base, key, body string) (int, string, error) {
	return postTransferHdr(base, key, body, "Idempotent-Replayed")
}

// postTransferHdr returns the status and one response header.
func postTransferHdr(base, key, body, hdr string) (int, string, error) {
	req, _ := http.NewRequest("POST", base+"/v1/transfers", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok-0123456789")
	req.Header.Set("Idempotency-Key", key)
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, resp.Header.Get(hdr), nil
}

func TestSIGTERMDuringTransferThenRetry(t *testing.T) {
	dsn := os.Getenv("LEDGERD_TEST_DATABASE_URL")
	if dsn == "" {
		// Technique 7: a tagged run with no database fails; a skip
		// would let CI report green without running anything.
		t.Fatal("integration tag set but LEDGERD_TEST_DATABASE_URL is empty")
	}
	ctx := t.Context()
	st, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Migrate(ctx, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
	st.Close()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	from, to := "acc_"+rand.Text()[:10], "acc_"+rand.Text()[:10]
	if _, err := db.Exec(ctx, `INSERT INTO accounts (id, balance)
        VALUES ($1, 1000), ($2, 0)`, from, to); err != nil {
		t.Fatal(err)
	}
	balance := func(id string) (b int64) {
		if err := db.QueryRow(ctx, `SELECT balance FROM accounts
            WHERE id = $1`, id).Scan(&b); err != nil {
			t.Fatal(err)
		}
		return b
	}
	keyState := func(scoped string) (s string) {
		err := db.QueryRow(ctx, `SELECT state FROM idem WHERE key = $1`,
			scoped).Scan(&s)
		if err != nil {
			return "none"
		}
		return s
	}

	drainDelay = 100 * time.Millisecond
	cfg := config.Config{Keys: map[string]string{"tok-0123456789": "acme"},
		ReqTimeout: 10 * time.Second, Grace: 300 * time.Millisecond, Sink: "log"}
	key := rand.Text()
	scoped := "acme/" + key
	body := `{"from":"` + from + `","to":"` + to + `","amount":500}`

	// Hold the payer's row so the transfer is mid-flight when the
	// signal lands: a slow primary, as in the Part V story.
	lock, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lock.Exec(ctx, `SELECT 1 FROM accounts WHERE id = $1
        FOR UPDATE`, from); err != nil {
		t.Fatal(err)
	}

	base, done := pod(t, dsn, cfg)
	clientErr := make(chan error, 1)
	go func() {
		_, _, err := postTransfer(base, key, body)
		clientErr <- err
	}()
	for i := 0; keyState(scoped) != "inflight"; i++ {
		if i > 300 {
			t.Fatal("transfer never claimed its key")
		}
		time.Sleep(10 * time.Millisecond)
	}
	start := time.Now()
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	// Shutdown's deadline passes with the transfer still blocked,
	// so serve cuts it with Close and reports the deadline.
	if err := <-done; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first pod exit: %v", err)
	}
	if d := time.Since(start); d > drainDelay+cfg.Grace+2*time.Second {
		t.Fatalf("shutdown took %v", d)
	}
	if err := <-clientErr; err == nil {
		t.Fatal("client got a response from a request cut by Close")
	}
	lock.Rollback(ctx) // the slow primary recovers

	// serve has returned and the pool is closed, as in run. The
	// cut request either committed (key done), failed on its
	// cancelled context and released the key, or lost the race
	// with the pool closing and left its claim in flight.
	time.Sleep(200 * time.Millisecond)
	state := keyState(scoped)
	t.Logf("key after the old pod exited: %s", state)

	base, done = pod(t, dsn, cfg) // the rollout's new pod
	code, retryAfter, err := postTransferHdr(base, key, body, "Retry-After")
	if err != nil {
		t.Fatal(err)
	}
	if code == http.StatusConflict {
		// A stuck claim: 409 with Retry-After until the lease
		// (store.Lease) runs out. Age it instead of waiting 30 s.
		if state != "inflight" || retryAfter == "" {
			t.Fatalf("409 with key %s, Retry-After %q", state, retryAfter)
		}
		if _, err := db.Exec(ctx, `UPDATE idem SET locked_until =
            now() - interval '1 second' WHERE key = $1`, scoped); err != nil {
			t.Fatal(err)
		}
		code, _, err = postTransferHdr(base, key, body, "")
	}
	if err != nil || code != http.StatusCreated {
		t.Fatalf("retry: %d %v (key was %s)", code, err, state)
	}
	code, replayed, err := postTransfer(base, key, body)
	if err != nil || code != http.StatusCreated || replayed != "true" {
		t.Fatalf("second retry: %d replayed=%q %v", code, replayed, err)
	}
	other := strings.Replace(body, "500", "501", 1)
	if code, _, _ := postTransfer(base, key, other); code != http.StatusUnprocessableEntity {
		t.Fatalf("same key, other body: %d", code)
	}
	if balance(from) != 500 || balance(to) != 500 {
		t.Fatalf("balances %d/%d: money moved %s", balance(from), balance(to),
			map[bool]string{true: "twice", false: "zero times"}[balance(to) > 500])
	}
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("idle pod exit: %v", err)
	}
}
