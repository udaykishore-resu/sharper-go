//go:build integration

package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"log"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"example.com/ledgerd/internal/ledger"
)

// Pin the major version production runs (Technique 68).
const pgImage = "postgres:17-alpine"

var dsn string

// TestMain starts one Postgres for the package and migrates it.
// Set LEDGERD_TEST_DATABASE_URL to use a running server instead.
// The fixture is shared, so it gets its own context, never one
// test's t.Context() (Technique 63).
func TestMain(m *testing.M) {
	ctx := context.Background()
	dsn = os.Getenv("LEDGERD_TEST_DATABASE_URL")
	var ctr *postgres.PostgresContainer
	if dsn == "" {
		var err error
		ctr, err = postgres.Run(ctx, pgImage,
			postgres.WithDatabase("ledgerd"),
			postgres.BasicWaitStrategies(),
		)
		if err != nil {
			log.Fatal(err)
		}
		dsn = ctr.MustConnectionString(ctx, "sslmode=disable")
	}
	st, err := Open(ctx, dsn, WithMaxConns(4))
	if err != nil {
		log.Fatal(err)
	}
	if err := st.Migrate(ctx, slog.New(slog.DiscardHandler)); err != nil {
		log.Fatal(err)
	}
	st.Close()
	code := m.Run()
	if ctr != nil {
		_ = tc.TerminateContainer(ctr)
	}
	os.Exit(code)
}

func open(t *testing.T) *Store {
	t.Helper()
	st, err := Open(t.Context(), dsn, WithMaxConns(8))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	return st
}

// accounts creates fresh accounts with the given balances and
// returns their IDs, so tests never share rows.
func accounts(t *testing.T, st *Store, balances ...int64) []string {
	t.Helper()
	ids := make([]string, len(balances))
	for i, b := range balances {
		ids[i] = "acc_" + rand.Text()[:10]
		_, err := st.pool.Exec(context.Background(),
			`INSERT INTO accounts (id, balance) VALUES ($1, $2)`, ids[i], b)
		if err != nil {
			t.Fatal(err)
		}
	}
	return ids
}

func balance(t *testing.T, st *Store, id string) int64 {
	t.Helper()
	a, err := st.Account(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return a.Balance
}

func TestMigrateTwiceIsNoop(t *testing.T) {
	st := open(t)
	if err := st.Migrate(t.Context(), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
}

// Technique 68's Try it: the same transfer twice with one key
// replays the first result and never returns a raw Postgres error.
func TestTransferSameKeyReplays(t *testing.T) {
	st := open(t)
	ids := accounts(t, st, 1000, 0)
	tr := ledger.Transfer{ID: "t_" + rand.Text(), From: ids[0], To: ids[1],
		Amount: 300, Key: ledger.ScopedKey("acme", rand.Text())}
	first, err := st.Transfer(t.Context(), tr)
	if err != nil {
		t.Fatal(err)
	}
	tr.ID = "t_" + rand.Text() // a retry may even carry a new ID
	second, err := st.Transfer(t.Context(), tr)
	if err != nil || string(second) != string(first) {
		t.Fatalf("replay = %s, %v; want %s", second, err, first)
	}
	if balance(t, st, ids[0]) != 700 || balance(t, st, ids[1]) != 300 {
		t.Fatal("money moved twice")
	}
}

// Technique 52 with Technique 57: the middleware's claim flips to
// done inside the transfer's transaction, so Release can't free it.
func TestMiddlewareFlow(t *testing.T) {
	st := open(t)
	ctx := t.Context()
	ids := accounts(t, st, 1000, 0)
	key, body := rand.Text(), []byte(`{"amount":5}`)
	hash := sha256.Sum256(body)
	saved, err := st.Begin(ctx, "acme", key, hash)
	if err != nil || saved != nil {
		t.Fatalf("claim: %v %v", saved, err)
	}
	if _, err := st.Begin(ctx, "acme", key, hash); !errors.Is(err, ledger.ErrInFlight) {
		t.Fatalf("second claim: %v", err)
	}
	if _, err := st.Begin(ctx, "acme", key, sha256.Sum256([]byte("x"))); !errors.Is(err, ledger.ErrMismatch) {
		t.Fatalf("other body: %v", err)
	}
	if _, err := st.Begin(ctx, "globex", key, hash); err != nil {
		t.Fatalf("keys must be scoped per client: %v", err)
	}
	resp, err := st.Transfer(ctx, ledger.Transfer{ID: "t_" + rand.Text(),
		From: ids[0], To: ids[1], Amount: 5, Key: ledger.ScopedKey("acme", key)})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Release(ctx, "acme", key); err != nil {
		t.Fatal(err)
	}
	saved, err = st.Begin(ctx, "acme", key, hash)
	if err != nil || saved == nil || saved.Status != 201 || string(saved.Body) != string(resp) {
		t.Fatalf("after commit: %+v %v", saved, err)
	}
}

func TestTransferErrors(t *testing.T) {
	st := open(t)
	ctx := t.Context()
	ids := accounts(t, st, 100, 0)
	_, err := st.pool.Exec(ctx, `UPDATE accounts SET frozen = true WHERE id = $1`, ids[1])
	if err != nil {
		t.Fatal(err)
	}
	free := accounts(t, st, 0)[0]
	for _, c := range []struct {
		name string
		tr   ledger.Transfer
		want error
	}{
		{"funds", ledger.Transfer{From: ids[0], To: free, Amount: 101}, ledger.ErrFunds},
		{"missing", ledger.Transfer{From: ids[0], To: "nope", Amount: 1}, ledger.ErrNotFound},
		{"frozen", ledger.Transfer{From: ids[0], To: ids[1], Amount: 1}, ledger.ErrFrozen},
	} {
		t.Run(c.name, func(t *testing.T) {
			c.tr.ID, c.tr.Key = "t_"+rand.Text(), rand.Text()
			_, err := st.Transfer(ctx, c.tr)
			if !errors.Is(err, c.want) {
				t.Fatalf("got %v, want %v", err, c.want)
			}
		})
	}
	id := "t_" + rand.Text()
	ok := ledger.Transfer{ID: id, From: ids[0], To: free, Amount: 1, Key: rand.Text()}
	if _, err := st.Transfer(ctx, ok); err != nil {
		t.Fatal(err)
	}
	ok.Key = rand.Text()
	if _, err := st.Transfer(ctx, ok); !errors.Is(err, ledger.ErrDuplicate) {
		t.Fatalf("reused transfer id: %v", err)
	}
}

// Rina's question: what happens under load? Serializable plus
// RunTx's retries means exactly the affordable transfers commit.
func TestConcurrentTransfersNeverOverdraw(t *testing.T) {
	st := open(t)
	ids := accounts(t, st, 500, 0)
	var ok, funds, busy atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			_, err := st.Transfer(context.Background(), ledger.Transfer{
				ID: "t_" + rand.Text(), From: ids[0], To: ids[1],
				Amount: 100, Key: rand.Text()})
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, ledger.ErrFunds):
				funds.Add(1)
			case errors.Is(err, ledger.ErrBusy): // lost every retry: 503
				busy.Add(1)
			default:
				t.Errorf("unexpected: %v", err)
			}
		})
	}
	wg.Wait()
	if ok.Load() > 5 || balance(t, st, ids[0]) < 0 ||
		balance(t, st, ids[0])+balance(t, st, ids[1]) != 500 {
		t.Fatalf("ok=%d funds=%d busy=%d balances %d/%d", ok.Load(), funds.Load(), busy.Load(),
			balance(t, st, ids[0]), balance(t, st, ids[1]))
	}
}

// startRelay is Technique 63's helper: the relay runs on
// t.Context() and the cleanup checks it stopped.
func startRelay(t *testing.T, st *Store, pub Publisher) *Relay {
	t.Helper()
	r := st.StartRelay(t.Context(), pub, slog.New(slog.DiscardHandler), nil)
	t.Cleanup(func() {
		select {
		case <-r.Done():
		case <-time.After(2 * time.Second):
			t.Error("relay did not stop")
		}
	})
	return r
}

// Technique 58: the event commits with the transfer and the relay
// publishes it.
func TestOutboxRelay(t *testing.T) {
	st := open(t)
	ids := accounts(t, st, 100, 0)
	id := "t_" + rand.Text()
	_, err := st.Transfer(t.Context(), ledger.Transfer{ID: id,
		From: ids[0], To: ids[1], Amount: 7, Key: rand.Text()})
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan string, 1000)
	startRelay(t, st, func(_ context.Context, topic string, p []byte) error {
		got <- topic + " " + string(p)
		return nil
	})
	deadline := time.After(10 * time.Second)
	for {
		select {
		case ev := <-got:
			if strings.Contains(ev, id) {
				return
			}
		case <-deadline:
			t.Fatalf("event for %s never published", id)
		}
	}
}
