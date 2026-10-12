//go:build integration

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"example.com/ledgerd/internal/config"
	"example.com/ledgerd/internal/store"
)

// Review checks for Technique 101 (the capstone), run against the
// assembled service: the production middleware chain from serve,
// real Postgres, and in the last test the real binary built from
// this checkout. Needs LEDGERD_TEST_DATABASE_URL.

type capDB struct {
	t   *testing.T
	dsn string
	db  *pgxpool.Pool
}

func capSetup(t *testing.T) *capDB {
	t.Helper()
	dsn := os.Getenv("LEDGERD_TEST_DATABASE_URL")
	if dsn == "" {
		// Technique 7: a tagged run with no database fails.
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
	db, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	return &capDB{t, dsn, db}
}

// accounts creates a funded payer and an empty payee.
func (c *capDB) accounts() (from, to string) {
	from, to = "acc_"+rand.Text()[:10], "acc_"+rand.Text()[:10]
	if _, err := c.db.Exec(context.Background(), `INSERT INTO accounts
        (id, balance) VALUES ($1, 1000), ($2, 0)`, from, to); err != nil {
		c.t.Fatal(err)
	}
	return from, to
}

func (c *capDB) int(q string, args ...any) (n int64) {
	c.t.Helper()
	if err := c.db.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		c.t.Fatalf("%s: %v", q, err)
	}
	return n
}

// movedOnce checks balances, transfer rows and outbox rows for
// one scoped key: the money moved exactly once.
func (c *capDB) movedOnce(from, to, scoped string, amount int64) {
	c.t.Helper()
	fb := c.int(`SELECT balance FROM accounts WHERE id = $1`, from)
	tb := c.int(`SELECT balance FROM accounts WHERE id = $1`, to)
	n := c.int(`SELECT count(*) FROM transfers WHERE idem_key = $1`, scoped)
	ev := c.int(`SELECT count(*) FROM outbox o JOIN transfers t
        ON o.payload->>'id' = t.id WHERE t.idem_key = $1`, scoped)
	if fb != 1000-amount || tb != amount || n != 1 || ev != 1 {
		c.t.Fatalf("balances %d/%d, %d transfer rows, %d events; want %d/%d, 1, 1",
			fb, tb, n, ev, 1000-amount, amount)
	}
}

func (c *capDB) keyState(scoped string) string {
	var s string
	err := c.db.QueryRow(context.Background(),
		`SELECT state FROM idem WHERE key = $1`, scoped).Scan(&s)
	if err != nil {
		return "none"
	}
	return s
}

// lockRow holds the payer's row, so a transfer stays in flight.
func (c *capDB) lockRow(id string) func() {
	tx, err := c.db.Begin(context.Background())
	if err != nil {
		c.t.Fatal(err)
	}
	if _, err := tx.Exec(context.Background(),
		`SELECT 1 FROM accounts WHERE id = $1 FOR UPDATE`, id); err != nil {
		c.t.Fatal(err)
	}
	return func() { tx.Rollback(context.Background()) }
}

func (c *capDB) waitState(scoped, want string) {
	c.t.Helper()
	for i := 0; c.keyState(scoped) != want; i++ {
		if i > 500 {
			c.t.Fatalf("key %s never reached %s (is %s)", scoped, want, c.keyState(scoped))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func capCfg() config.Config {
	return config.Config{
		Keys: map[string]string{
			"tok-0123456789": "acme", "tok-9876543210": "beta"},
		ReqTimeout: 10 * time.Second, Grace: 2 * time.Second, Sink: "log"}
}

type reply struct {
	code     int
	replayed string
	retry    string
	body     string
}

func post(base, token, key, body string) (reply, error) {
	req, _ := http.NewRequest("POST", base+"/v1/transfers", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Idempotency-Key", key)
	resp, err := client.Do(req)
	if err != nil {
		return reply{}, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return reply{resp.StatusCode, resp.Header.Get("Idempotent-Replayed"),
		resp.Header.Get("Retry-After"), string(b)}, nil
}

// Twenty requests with one key arrive together while the first
// one is held inside its transaction. One runs the transfer; the
// rest get 409 with Retry-After or a replay, never a second
// transfer. The same key from another client is its own transfer.
func TestCapstoneConcurrentSameKey(t *testing.T) {
	c := capSetup(t)
	from, to := c.accounts()
	drainDelay = 50 * time.Millisecond
	base, done := pod(t, c.dsn, capCfg())
	key := rand.Text()
	body := fmt.Sprintf(`{"from":%q,"to":%q,"amount":300}`, from, to)

	unlock := c.lockRow(from)
	const n = 20
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		replies []reply
	)
	for range n {
		wg.Go(func() {
			r, err := post(base, "tok-0123456789", key, body)
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			replies = append(replies, r)
			mu.Unlock()
		})
	}
	c.waitState("acme/"+key, "inflight")
	time.Sleep(300 * time.Millisecond) // let the others arrive
	unlock()
	wg.Wait()

	var fresh, conflict, replayed int
	var first string
	for _, r := range replies {
		switch {
		case r.code == 201 && r.replayed == "":
			fresh++
			first = r.body
		case r.code == 201 && r.replayed == "true":
			replayed++
		case r.code == 409 && r.retry != "":
			conflict++
		default:
			t.Errorf("unexpected reply %+v", r)
		}
	}
	t.Logf("%d fresh, %d replayed, %d in flight (409)", fresh, replayed, conflict)
	if fresh != 1 || fresh+replayed+conflict != n {
		t.Fatalf("want exactly one fresh 201 out of %d", n)
	}
	// A later retry replays the first response byte for byte.
	r, err := post(base, "tok-0123456789", key, body)
	if err != nil || r.code != 201 || r.replayed != "true" || r.body != first {
		t.Fatalf("retry %+v %v", r, err)
	}
	c.movedOnce(from, to, "acme/"+key, 300)

	// Scope is (client, key): beta's identical key is a new transfer.
	r, err = post(base, "tok-9876543210", key, body)
	if err != nil || r.code != 201 || r.replayed != "" {
		t.Fatalf("other client, same key: %+v %v", r, err)
	}
	if got := c.int(`SELECT balance FROM accounts WHERE id = $1`, to); got != 600 {
		t.Fatalf("payee %d after two clients, want 600", got)
	}
	syscall.Kill(os.Getpid(), syscall.SIGTERM)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// The response is lost after the commit: the client sends the
// POST, the transfer commits, and the client hangs up without
// reading. The retry replays the committed response and moves no
// money, even though the hang-up ended the handler's context
// (Release never touches a committed key).
func TestCapstoneLostResponseAfterCommit(t *testing.T) {
	c := capSetup(t)
	from, to := c.accounts()
	drainDelay = 50 * time.Millisecond
	base, done := pod(t, c.dsn, capCfg())
	key := rand.Text()
	scoped := "acme/" + key
	body := fmt.Sprintf(`{"from":%q,"to":%q,"amount":250}`, from, to)

	conn, err := net.Dial("tcp", strings.TrimPrefix(base, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(conn, "POST /v1/transfers HTTP/1.1\r\nHost: x\r\n"+
		"Authorization: Bearer tok-0123456789\r\nIdempotency-Key: %s\r\n"+
		"Content-Type: application/json\r\nContent-Length: %d\r\n\r\n%s",
		key, len(body), body)
	c.waitState(scoped, "done")
	conn.Close() // the response is never read

	r, err := post(base, "tok-0123456789", key, body)
	if err != nil || r.code != 201 || r.replayed != "true" {
		t.Fatalf("retry after lost response: %+v %v", r, err)
	}
	var saved []byte
	if err := c.db.QueryRow(t.Context(), `SELECT response FROM idem
        WHERE key = $1`, scoped).Scan(&saved); err != nil {
		t.Fatal(err)
	}
	if r.body != string(saved) {
		t.Fatalf("replayed %q, saved %q", r.body, saved)
	}
	c.movedOnce(from, to, scoped, 250)
	syscall.Kill(os.Getpid(), syscall.SIGTERM)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// The real binary, built from this checkout, is killed with
// SIGKILL in the middle of its drain while a transfer is held in
// flight, then restarted. The retry gets 409 + Retry-After while
// the dead pod's lease lasts, then takes the key over; money moves
// once, and the new pod's relay delivers the transfer's event.
// Uses ledgerd's fixed ports (:8080, :9090, 127.0.0.1:6060).
func TestCapstoneKillDuringShutdownThenRestart(t *testing.T) {
	c := capSetup(t)
	from, to := c.accounts()
	const tok = "tok-0123456789abcdef"
	bin := filepath.Join(t.TempDir(), "ledgerd")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	env := append(os.Environ(),
		"DATABASE_URL="+c.dsn,
		"API_KEYS=acme:"+tok,
		"SHUTDOWN_GRACE=2s",
		"OTEL_EXPORTER_OTLP_ENDPOINT=", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=")
	const base = "http://127.0.0.1:8080"
	type proc struct {
		*exec.Cmd
		logs   *bytes.Buffer
		exited chan error
	}
	start := func() proc {
		cmd := exec.Command(bin)
		cmd.Env = env
		var logs bytes.Buffer
		cmd.Stderr = &logs
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		p := proc{cmd, &logs, make(chan error, 1)}
		go func() { p.exited <- cmd.Wait() }()
		for i := 0; ; i++ {
			select {
			case err := <-p.exited:
				t.Fatalf("ledgerd exited: %v\n%s", err, &logs)
			default:
			}
			resp, err := client.Get(base + "/readyz")
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == 200 {
					return p
				}
			}
			if i > 500 {
				cmd.Process.Kill()
				t.Fatalf("ledgerd never became ready:\n%s", &logs)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}

	key := rand.Text()
	scoped := "acme/" + key
	body := fmt.Sprintf(`{"from":%q,"to":%q,"amount":400}`, from, to)
	old := start()
	unlock := c.lockRow(from)
	clientErr := make(chan error, 1)
	go func() {
		_, err := post(base, tok, key, body)
		clientErr <- err
	}()
	c.waitState(scoped, "inflight")
	old.Process.Signal(syscall.SIGTERM)
	for i := 0; ; i++ { // the drain has begun: /readyz is 503
		resp, err := client.Get(base + "/readyz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 503 {
				break
			}
		}
		if i > 200 {
			t.Fatalf("readiness never failed:\n%s", old.logs)
		}
		time.Sleep(10 * time.Millisecond)
	}
	old.Process.Kill() // SIGKILL mid-drain
	<-old.exited
	if err := <-clientErr; err == nil {
		t.Fatal("client got a response from a killed pod")
	}
	unlock()

	pod2 := start()
	logs2 := pod2.logs
	defer pod2.Process.Kill()
	r, err := post(base, tok, key, body)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("first retry on the new pod: %d (Retry-After %q)", r.code, r.retry)
	if r.code == http.StatusConflict {
		// The dead pod's claim: 409 + Retry-After until the 30 s
		// lease ends. Age it instead of waiting.
		if r.retry == "" || c.keyState(scoped) != "inflight" {
			t.Fatalf("409 %+v with key %s", r, c.keyState(scoped))
		}
		if _, err := c.db.Exec(t.Context(), `UPDATE idem SET locked_until =
            now() - interval '1 second' WHERE key = $1`, scoped); err != nil {
			t.Fatal(err)
		}
		r, err = post(base, tok, key, body)
	}
	if err != nil || r.code != 201 {
		t.Fatalf("retry on the new pod: %+v %v\n%s", r, err, logs2)
	}
	c.movedOnce(from, to, scoped, 400)

	// The new pod's relay delivers the event (at least once).
	for i := 0; ; i++ {
		sent := c.int(`SELECT count(*) FROM outbox o JOIN transfers t
            ON o.payload->>'id' = t.id
            WHERE t.idem_key = $1 AND o.sent_at IS NOT NULL`, scoped)
		if sent == 1 {
			break
		}
		if i > 300 {
			t.Fatalf("event never relayed:\n%s", logs2)
		}
		time.Sleep(20 * time.Millisecond)
	}
	pod2.Process.Signal(syscall.SIGTERM)
	select {
	case err := <-pod2.exited:
		if err != nil {
			t.Fatalf("clean exit expected: %v\n%s", err, logs2)
		}
	case <-time.After(drain + 5*time.Second):
		t.Fatal("new pod did not exit after SIGTERM")
	}
	sc := bufio.NewScanner(logs2)
	for sc.Scan() {
		if strings.Contains(sc.Text(), `"level":"ERROR"`) {
			t.Errorf("error log line: %s", sc.Text())
		}
	}
}
