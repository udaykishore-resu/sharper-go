//go:build integration

package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"example.com/ledgerd/internal/ledger"
	"example.com/ledgerd/internal/store/sqlcdb"
)

// freshStore opens a Store on a new, empty database on the test
// server, so relay and migration tests see no other test's rows.
func freshStore(t *testing.T, migrate bool) *Store {
	t.Helper()
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := "r_" + strings.ToLower(rand.Text()[:12])
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	cfg := admin.Config()
	url := fmt.Sprintf("postgres://%s@%s:%d/%s?sslmode=disable",
		cfg.User, cfg.Host, cfg.Port, name)
	st, err := Open(ctx, url, WithMaxConns(8))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		st.Close()
		admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)")
		admin.Close(ctx)
	})
	if migrate {
		if err := st.Migrate(ctx, slog.New(slog.DiscardHandler)); err != nil {
			t.Fatal(err)
		}
	}
	return st
}

func count(t *testing.T, st *Store, sql string, args ...any) (n int) {
	t.Helper()
	if err := st.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// killSelf terminates the backend running tx, from a second
// connection, so the next statement, rollback or commit on tx
// fails the way a dropped connection does.
func killSelf(t *testing.T, st *Store, tx pgx.Tx) {
	var pid int
	if err := tx.QueryRow(context.Background(), `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if _, err := st.pool.Exec(context.Background(), `SELECT pg_terminate_backend($1)`, pid); err != nil {
		t.Fatal(err)
	}
}

// Technique 57: when the rollback itself fails, InTx still
// returns fn's error, the write is gone and the pool gets its
// slot back (pgx discards the broken connection).
func TestInTxRollbackFailure(t *testing.T) {
	st := open(t)
	id := accounts(t, st, 50)[0]
	boom := errors.New("boom")
	err := st.InTx(t.Context(), func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `UPDATE accounts SET balance = 0 WHERE id = $1`, id); err != nil {
			return err
		}
		killSelf(t, st, tx)
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if balance(t, st, id) != 50 || st.AcquiredConns() != 0 {
		t.Fatalf("leaked: balance %d, acquired %d", balance(t, st, id), st.AcquiredConns())
	}
}

// The same with a panic: the original value is re-panicked even
// though the rollback failed.
func TestInTxPanicWithFailedRollback(t *testing.T) {
	st := open(t)
	id := accounts(t, st, 50)[0]
	var got any
	func() {
		defer func() { got = recover() }()
		st.InTx(t.Context(), func(tx pgx.Tx) error {
			tx.Exec(t.Context(), `UPDATE accounts SET balance = 0 WHERE id = $1`, id)
			killSelf(t, st, tx)
			panic("original")
		})
	}()
	if got != "original" || balance(t, st, id) != 50 || st.AcquiredConns() != 0 {
		t.Fatalf("recovered %v, balance %d, acquired %d", got, balance(t, st, id), st.AcquiredConns())
	}
}

// A failed commit reaches the caller through the named result.
func TestInTxCommitFailure(t *testing.T) {
	st := open(t)
	id := accounts(t, st, 50)[0]
	err := st.InTx(t.Context(), func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `UPDATE accounts SET balance = 0 WHERE id = $1`, id); err != nil {
			return err
		}
		killSelf(t, st, tx)
		return nil // the commit will fail
	})
	if err == nil {
		t.Fatal("commit on a dead connection reported success")
	}
	if balance(t, st, id) != 50 || st.AcquiredConns() != 0 {
		t.Fatalf("balance %d, acquired %d", balance(t, st, id), st.AcquiredConns())
	}
}

// RunTx retries only 40001 and 40P01, three attempts at most.
func TestRunTxRetryPolicy(t *testing.T) {
	st := open(t)
	for _, c := range []struct {
		code     string
		attempts int32
	}{{"40001", 3}, {"40P01", 3}, {"23505", 1}, {"", 1}} {
		var n atomic.Int32
		err := st.RunTx(t.Context(), func(pgx.Tx) error {
			n.Add(1)
			if c.code == "" {
				return errors.New("plain")
			}
			return &pgconn.PgError{Code: c.code}
		})
		if err == nil || n.Load() != c.attempts {
			t.Errorf("code %q: %d attempts, err %v", c.code, n.Load(), err)
		}
	}
	var n atomic.Int32
	err := st.RunTx(t.Context(), func(pgx.Tx) error {
		if n.Add(1) == 1 {
			return &pgconn.PgError{Code: "40001"}
		}
		return nil
	})
	if err != nil || n.Load() != 2 {
		t.Fatalf("retry then success: %d attempts, %v", n.Load(), err)
	}
}

// race runs fn in two goroutines released together.
func race(fn func(i int)) {
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Go(func() { <-start; fn(i) })
	}
	close(start)
	wg.Wait()
}

// moved checks that exactly one transfer, and one event, exist
// for key.
func moved(t *testing.T, st *Store, key string) {
	t.Helper()
	if n := count(t, st, `SELECT count(*) FROM transfers WHERE idem_key = $1`, key); n != 1 {
		t.Fatalf("%d transfer rows for one key", n)
	}
	n := count(t, st, `SELECT count(*) FROM outbox o JOIN transfers tr
        ON o.payload->>'id' = tr.id WHERE tr.idem_key = $1`, key)
	if n != 1 {
		t.Fatalf("%d outbox events for one key", n)
	}
}

// Two transfers with the same key and different IDs, at once,
// with no middleware: one moves money, both return its answer.
func TestSameKeyRace(t *testing.T) {
	st := open(t)
	for range 10 {
		ids := accounts(t, st, 100, 0)
		key := ledger.ScopedKey("acme", rand.Text())
		var resp [2][]byte
		var errs [2]error
		race(func(i int) {
			resp[i], errs[i] = st.Transfer(context.Background(), ledger.Transfer{
				ID: "t_" + rand.Text(), From: ids[0], To: ids[1], Amount: 30, Key: key})
		})
		if errs[0] != nil || errs[1] != nil || string(resp[0]) != string(resp[1]) {
			t.Fatalf("errs %v %v; bodies %s / %s", errs[0], errs[1], resp[0], resp[1])
		}
		if balance(t, st, ids[1]) != 30 {
			t.Fatalf("destination got %d, want 30", balance(t, st, ids[1]))
		}
		moved(t, st, key)
	}
}

// The mechanism behind the races above, made deterministic: A
// holds the key's row lock; B blocks on it, gets 40001 when A
// commits, and its next attempt replays A's response.
func TestRaceLoserGets40001ThenReplays(t *testing.T) {
	for _, claimed := range []bool{false, true} {
		t.Run(fmt.Sprint("claimed=", claimed), func(t *testing.T) {
			loserReplays(t, claimed)
		})
	}
}

func loserReplays(t *testing.T, claimed bool) {
	st := open(t)
	ctx := t.Context()
	ids := accounts(t, st, 100, 0)
	raw := rand.Text()
	key := ledger.ScopedKey("acme", raw)
	if claimed { // the middleware claimed it (Technique 52)
		_, err := st.Begin(ctx, "acme", raw, sha256.Sum256(nil))
		if err != nil {
			t.Fatal(err)
		}
	}
	tr := func() ledger.Transfer {
		return ledger.Transfer{ID: "t_" + rand.Text(), From: ids[0],
			To: ids[1], Amount: 30, Key: key}
	}
	holds, release := make(chan struct{}), make(chan struct{})
	aDone := make(chan []byte)
	go func() {
		var body []byte
		err := st.InTx(ctx, func(tx pgx.Tx) error {
			var err error
			body, err = transferTx(ctx, tx, tr())
			close(holds)
			<-release // hold the lock until B is waiting on it
			return err
		})
		if err != nil {
			t.Error(err)
		}
		aDone <- body
	}()
	<-holds
	bErr := make(chan error)
	go func() {
		bErr <- st.InTx(ctx, func(tx pgx.Tx) error {
			_, err := transferTx(ctx, tx, tr())
			return err
		})
	}()
	deadline := time.Now().Add(10 * time.Second)
	for count(t, st, `SELECT count(*) FROM pg_stat_activity
        WHERE wait_event_type = 'Lock'
        AND query LIKE '%INSERT INTO idem%'`) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("B never waited on A's lock")
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(release)
	a := <-aDone
	var pe *pgconn.PgError
	if err := <-bErr; !errors.As(err, &pe) || pe.Code != "40001" {
		t.Fatalf("B's first attempt: %v, want SQLSTATE 40001", err)
	}
	var b []byte
	err := st.InTx(ctx, func(tx pgx.Tx) error {
		var err error
		b, err = transferTx(ctx, tx, tr())
		return err
	})
	if err != nil || string(b) != string(a) || balance(t, st, ids[1]) != 30 {
		t.Fatalf("retry: %s %v; want %s, balance 30", b, err, a)
	}
}

// Two different keys at once both commit.
func TestDifferentKeysRace(t *testing.T) {
	st := open(t)
	ids := accounts(t, st, 100, 0)
	var errs [2]error
	race(func(i int) {
		_, errs[i] = st.Transfer(context.Background(), ledger.Transfer{
			ID: "t_" + rand.Text(), From: ids[0], To: ids[1], Amount: 30,
			Key: rand.Text()})
	})
	if errs[0] != nil || errs[1] != nil || balance(t, st, ids[1]) != 60 {
		t.Fatalf("errs %v %v, balance %d", errs[0], errs[1], balance(t, st, ids[1]))
	}
}

// The brief's canonical case: a stale holder and a lease
// takeover both reach commit. The row lock serialises them and
// the second replays the first one's response.
func TestTakeoverRace(t *testing.T) {
	st := open(t)
	ctx := t.Context()
	for range 10 {
		ids := accounts(t, st, 100, 0)
		key, body := rand.Text(), []byte(`{"amount":30}`)
		hash := sha256.Sum256(body)
		if s, err := st.Begin(ctx, "acme", key, hash); err != nil || s != nil {
			t.Fatalf("claim: %v %v", s, err)
		}
		// The first holder stalls past its lease...
		_, err := st.pool.Exec(ctx, `UPDATE idem SET locked_until = now() - interval '1 second'
            WHERE key = $1`, ledger.ScopedKey("acme", key))
		if err != nil {
			t.Fatal(err)
		}
		// ...and a retry takes the key over.
		if s, err := st.Begin(ctx, "acme", key, hash); err != nil || s != nil {
			t.Fatalf("takeover: %v %v", s, err)
		}
		scoped := ledger.ScopedKey("acme", key)
		var resp [2][]byte
		var errs [2]error
		race(func(i int) { // each handler made its own transfer ID
			resp[i], errs[i] = st.Transfer(context.Background(), ledger.Transfer{
				ID: "t_" + rand.Text(), From: ids[0], To: ids[1], Amount: 30, Key: scoped})
		})
		if errs[0] != nil || errs[1] != nil || string(resp[0]) != string(resp[1]) {
			t.Fatalf("errs %v %v; bodies %s / %s", errs[0], errs[1], resp[0], resp[1])
		}
		if balance(t, st, ids[0]) != 70 || balance(t, st, ids[1]) != 30 {
			t.Fatal("money moved twice")
		}
		moved(t, st, scoped)
		// A third attempt replays through the middleware path.
		s, err := st.Begin(ctx, "acme", key, hash)
		if err != nil || s == nil || s.Status != 201 || string(s.Body) != string(resp[0]) {
			t.Fatalf("later replay: %+v %v", s, err)
		}
	}
}

// If the stale holder finished with an error (Finish saved a
// 409), the takeover's Transfer must not replay that body as a
// 201 success, and must not move money.
func TestDoneWithErrorIsNotReplayedAsSuccess(t *testing.T) {
	st := open(t)
	ctx := t.Context()
	ids := accounts(t, st, 100, 0)
	key, hash := rand.Text(), sha256.Sum256([]byte("b"))
	if _, err := st.Begin(ctx, "acme", key, hash); err != nil {
		t.Fatal(err)
	}
	if err := st.Finish(ctx, "acme", key, ledger.Saved{Status: 409, Body: []byte(`{"code":"insufficient_funds"}`)}); err != nil {
		t.Fatal(err)
	}
	_, err := st.Transfer(ctx, ledger.Transfer{ID: "t_" + rand.Text(),
		From: ids[0], To: ids[1], Amount: 5, Key: ledger.ScopedKey("acme", key)})
	if !errors.Is(err, ledger.ErrBusy) || balance(t, st, ids[1]) != 0 {
		t.Fatalf("err %v, balance %d", err, balance(t, st, ids[1]))
	}
}

// Technique 56: the generated :execrows method reports the
// affected count, so a missing row is visible to the caller.
func TestAddToBalanceCountsRows(t *testing.T) {
	st := open(t)
	id := accounts(t, st, 10)[0]
	q := sqlcdb.New(st.pool)
	n, err := q.AddToBalance(t.Context(), sqlcdb.AddToBalanceParams{Delta: 5, ID: id})
	if err != nil || n != 1 {
		t.Fatalf("existing: %d %v", n, err)
	}
	n, err = q.AddToBalance(t.Context(), sqlcdb.AddToBalanceParams{Delta: 5, ID: "nope"})
	if err != nil || n != 0 {
		t.Fatalf("missing: %d %v", n, err)
	}
}

func addEvents(t *testing.T, st *Store, n int) {
	t.Helper()
	for i := range n {
		_, err := st.pool.Exec(context.Background(), `INSERT INTO outbox (topic, payload)
            VALUES ('transfer.created', $1)`, fmt.Sprintf(`{"id":"t_%d"}`, i))
		if err != nil {
			t.Fatal(err)
		}
	}
}

func unsent(t *testing.T, st *Store) int {
	return count(t, st, `SELECT count(*) FROM outbox WHERE sent_at IS NULL`)
}

// Technique 58: a publish failure on row 3 of 5 commits marks
// for rows 1-2 only; rows 3-5 stay unsent and go out next round.
func TestRelayMarksOnlySentRows(t *testing.T) {
	st := freshStore(t, true)
	addEvents(t, st, 5)
	var got []string
	fail := errors.New("broker down")
	n, err := st.relayOnce(t.Context(), func(_ context.Context, _ string, p []byte) error {
		if strings.Contains(string(p), `"t_2"`) {
			return fail
		}
		got = append(got, string(p))
		return nil
	})
	if n != 2 || !errors.Is(err, fail) || len(got) != 2 || unsent(t, st) != 3 {
		t.Fatalf("sent %d, err %v, published %d, unsent %d", n, err, len(got), unsent(t, st))
	}
	marked := count(t, st, `SELECT count(*) FROM outbox
        WHERE sent_at IS NOT NULL AND payload->>'id' IN ('t_0', 't_1')`)
	if marked != 2 {
		t.Fatalf("marked %d of the two sent rows", marked)
	}
	n, err = st.relayOnce(t.Context(), func(context.Context, string, []byte) error { return nil })
	if n != 3 || err != nil || unsent(t, st) != 0 {
		t.Fatalf("next round: sent %d, err %v, unsent %d", n, err, unsent(t, st))
	}
}

// The relay dies after publishing but before committing sent_at
// (here: its context ends between publish and mark). Nothing is
// marked, the restart sends everything again, and a consumer
// that drops repeats by the transfer ID in the payload sees each
// event once.
func TestRelayCrashBeforeCommit(t *testing.T) {
	st := freshStore(t, true)
	addEvents(t, st, 3)
	var deliveries []string
	ctx, cancel := context.WithCancel(t.Context())
	_, err := st.relayOnce(ctx, func(_ context.Context, _ string, p []byte) error {
		deliveries = append(deliveries, string(p))
		if len(deliveries) == 2 {
			cancel() // "kill -9" before the mark commits
		}
		return nil
	})
	if err == nil || unsent(t, st) != 3 {
		t.Fatalf("err %v, unsent %d (want all 3)", err, unsent(t, st))
	}
	n, err := st.relayOnce(t.Context(), func(_ context.Context, _ string, p []byte) error {
		deliveries = append(deliveries, string(p))
		return nil
	})
	if n != 3 || err != nil || unsent(t, st) != 0 {
		t.Fatalf("restart: sent %d, err %v", n, err)
	}
	seen := map[string]bool{}
	for _, d := range deliveries {
		seen[d] = true
	}
	if len(deliveries) != 5 || len(seen) != 3 {
		t.Fatalf("%d deliveries, %d distinct", len(deliveries), len(seen))
	}
}

// Two relays at once never claim the same row: with no
// failures, each event is published exactly once.
func TestTwoRelaysSkipLocked(t *testing.T) {
	st := freshStore(t, true)
	addEvents(t, st, 300)
	var mu sync.Mutex
	seen := map[string]int{}
	pub := func(_ context.Context, _ string, p []byte) error {
		mu.Lock()
		seen[string(p)]++
		mu.Unlock()
		return nil
	}
	for unsent(t, st) > 0 {
		race(func(int) {
			if _, err := st.relayOnce(context.Background(), pub); err != nil {
				t.Error(err)
			}
		})
	}
	for p, n := range seen {
		if n != 1 {
			t.Fatalf("%s published %d times", p, n)
		}
	}
	if len(seen) != 300 {
		t.Fatalf("published %d of 300", len(seen))
	}
}

// Technique 61: two migration Jobs that overlap take turns on
// the advisory lock; each version is applied once.
func TestConcurrentMigrate(t *testing.T) {
	st := freshStore(t, false)
	if err := st.CheckSchema(t.Context()); err == nil {
		t.Fatal("CheckSchema passed on an empty database")
	}
	if err := st.Ping(t.Context()); err == nil {
		t.Fatal("readiness ping passed on an empty database")
	}
	var errs [2]error
	race(func(i int) {
		errs[i] = st.Migrate(context.Background(), slog.New(slog.DiscardHandler))
	})
	if errs[0] != nil || errs[1] != nil {
		t.Fatalf("migrate: %v / %v", errs[0], errs[1])
	}
	if n := count(t, st, `SELECT count(*) FROM goose_db_version
        WHERE version_id > 0 GROUP BY version_id ORDER BY 1 DESC LIMIT 1`); n != 1 {
		t.Fatalf("a version was applied %d times", n)
	}
	if n := count(t, st, `SELECT count(*) FROM accounts WHERE id = 'house'`); n != 1 {
		t.Fatalf("%d house accounts", n)
	}
	if err := st.CheckSchema(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := st.Ping(t.Context()); err != nil {
		t.Fatalf("ready after migrate: %v", err)
	}
}

// Part VI synthesis: batches 1-2 commit, batch 3 hits a
// serialization failure on every attempt and rolls back whole.
// The job restarts from zero; committed rows replay through
// their keys, so money moves once and each transfer has one
// event.
func TestImportResumesWithoutDuplicates(t *testing.T) {
	st := open(t)
	ctx := t.Context()
	ids := accounts(t, st, 10_000, 0)
	file := "f_" + rand.Text()[:8]
	var batches [3][]ledger.Transfer
	for b := range batches {
		for r := range 4 {
			row := b*4 + r
			batches[b] = append(batches[b], ledger.Transfer{
				ID: fmt.Sprintf("%s_%d", file, row), From: ids[0], To: ids[1],
				Amount: 10, Key: ledger.ScopedKey("partner", fmt.Sprintf("%s#%d", file, row))})
		}
	}
	for _, b := range batches[:2] {
		if err := st.ImportBatch(ctx, b); err != nil {
			t.Fatal(err)
		}
	}
	var attempts atomic.Int32
	err := st.RunTx(ctx, func(tx pgx.Tx) error {
		attempts.Add(1)
		if err := importTx(ctx, tx, batches[2]); err != nil {
			return err
		}
		return &pgconn.PgError{Code: "40001"} // fails at commit time
	})
	if !retryable(err) || attempts.Load() != 3 || balance(t, st, ids[1]) != 80 {
		t.Fatalf("batch 3: %v after %d attempts, balance %d", err, attempts.Load(), balance(t, st, ids[1]))
	}
	for _, b := range batches { // restart from zero
		if err := st.ImportBatch(ctx, b); err != nil {
			t.Fatal(err)
		}
	}
	if balance(t, st, ids[1]) != 120 {
		t.Fatalf("destination %d, want 120", balance(t, st, ids[1]))
	}
	if n := count(t, st, `SELECT count(*) FROM transfers WHERE id LIKE $1`, file+"_%"); n != 12 {
		t.Fatalf("%d transfer rows, want 12", n)
	}
	if n := count(t, st, `SELECT count(*) FROM outbox WHERE payload->>'id' LIKE $1`, file+"_%"); n != 12 {
		t.Fatalf("%d events, want 12", n)
	}
}
