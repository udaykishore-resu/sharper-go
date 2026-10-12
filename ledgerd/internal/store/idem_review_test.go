//go:build integration

package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"sync"
	"testing"

	"example.com/ledgerd/internal/ledger"
)

// The transaction boundary of Technique 52, end to end against
// Postgres: each test drives the store's three verbs and
// Transfer the way the middleware and handler do.

func claim(t *testing.T, st *Store, client, key string,
	body []byte) {
	t.Helper()
	saved, err := st.Begin(t.Context(), client, key, sha256.Sum256(body))
	if err != nil || saved != nil {
		t.Fatalf("claim %s: saved=%v err=%v", key, saved, err)
	}
}

// expire ends a claim's lease now, as if Lease had passed.
func expire(t *testing.T, st *Store, client, key string) {
	t.Helper()
	_, err := st.pool.Exec(t.Context(), `UPDATE idem
        SET locked_until = now() - interval '1 second'
        WHERE key = $1`, ledger.ScopedKey(client, key))
	if err != nil {
		t.Fatal(err)
	}
}

func transfersWithKey(t *testing.T, st *Store, scoped string) int {
	t.Helper()
	var n int
	err := st.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM transfers WHERE idem_key = $1`,
		scoped).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// The transfer commits, the response is lost on the way back
// (no Finish ever runs), and the client retries with the same
// key and body: the retry replays the committed response.
func TestIdemReviewLostResponseReplays(t *testing.T) {
	st := open(t)
	ids := accounts(t, st, 1000, 0)
	key, body := rand.Text(), []byte(`{"amount":500}`)
	claim(t, st, "acme", key, body)
	resp, err := st.Transfer(t.Context(), ledger.Transfer{ID: "t_" + rand.Text(),
		From: ids[0], To: ids[1], Amount: 500, Key: ledger.ScopedKey("acme", key)})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := st.Begin(t.Context(), "acme", key, sha256.Sum256(body))
	if err != nil || saved == nil || saved.Status != 201 ||
		string(saved.Body) != string(resp) {
		t.Fatalf("retry: saved=%+v err=%v", saved, err)
	}
	if balance(t, st, ids[0]) != 500 || balance(t, st, ids[1]) != 500 {
		t.Fatal("money moved twice")
	}
	// A late Release from the first request can't free the key.
	if err := st.Release(t.Context(), "acme", key); err != nil {
		t.Fatal(err)
	}
	if s, _ := st.Begin(t.Context(), "acme", key, sha256.Sum256(body)); s == nil {
		t.Fatal("a committed key was released")
	}
}

// A different payload under the same key is refused in the store
// both while the first request runs and after it committed.
func TestIdemReviewOtherPayloadIs422(t *testing.T) {
	st := open(t)
	ids := accounts(t, st, 1000, 0)
	key := rand.Text()
	claim(t, st, "acme", key, []byte(`{"amount":5}`))
	other := sha256.Sum256([]byte(`{"amount":6}`))
	if _, err := st.Begin(t.Context(), "acme", key, other); !errors.Is(err, ledger.ErrMismatch) {
		t.Fatalf("in flight: %v", err)
	}
	_, err := st.Transfer(t.Context(), ledger.Transfer{ID: "t_" + rand.Text(),
		From: ids[0], To: ids[1], Amount: 5, Key: ledger.ScopedKey("acme", key)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Begin(t.Context(), "acme", key, other); !errors.Is(err, ledger.ErrMismatch) {
		t.Fatalf("done: %v", err)
	}
}

// The process dies between Begin and commit. Retries get
// ErrInFlight (409) until the lease ends, then one of them takes
// the claim over and the transfer runs once.
func TestIdemReviewCrashThenTakeover(t *testing.T) {
	st := open(t)
	ids := accounts(t, st, 1000, 0)
	key, body := rand.Text(), []byte(`{"amount":7}`)
	claim(t, st, "acme", key, body) // ...and the pod dies
	h := sha256.Sum256(body)
	if _, err := st.Begin(t.Context(), "acme", key, h); !errors.Is(err, ledger.ErrInFlight) {
		t.Fatalf("before the lease ends: %v", err)
	}
	expire(t, st, "acme", key)
	claim(t, st, "acme", key, body) // takeover
	if _, err := st.Begin(t.Context(), "acme", key, h); !errors.Is(err, ledger.ErrInFlight) {
		t.Fatalf("the takeover's claim has a fresh lease: %v", err)
	}
	scoped := ledger.ScopedKey("acme", key)
	if _, err := st.Transfer(t.Context(), ledger.Transfer{ID: "t_" + rand.Text(),
		From: ids[0], To: ids[1], Amount: 7, Key: scoped}); err != nil {
		t.Fatal(err)
	}
	if n := transfersWithKey(t, st, scoped); n != 1 || balance(t, st, ids[1]) != 7 {
		t.Fatalf("%d transfers, balance %d", n, balance(t, st, ids[1]))
	}
}

// A stale holder (its lease expired while it was slow) and the
// takeover both reach commit. The row lock serialises them: one
// moves the money, the other replays the first one's response.
func TestIdemReviewStaleHolderAndTakeoverCommitOnce(t *testing.T) {
	st := open(t)
	for range 10 {
		ids := accounts(t, st, 1000, 0)
		key, body := rand.Text(), []byte(`{"amount":300}`)
		claim(t, st, "acme", key, body) // holder A
		expire(t, st, "acme", key)
		claim(t, st, "acme", key, body) // takeover B
		scoped := ledger.ScopedKey("acme", key)
		var (
			wg    sync.WaitGroup
			resps [2][]byte
			errs  [2]error
		)
		for i := range 2 {
			wg.Go(func() {
				resps[i], errs[i] = st.Transfer(context.Background(),
					ledger.Transfer{ID: "t_" + rand.Text(), From: ids[0],
						To: ids[1], Amount: 300, Key: scoped})
			})
		}
		wg.Wait()
		if errs[0] != nil || errs[1] != nil ||
			string(resps[0]) != string(resps[1]) {
			t.Fatalf("A: %s %v; B: %s %v", resps[0], errs[0], resps[1], errs[1])
		}
		if n := transfersWithKey(t, st, scoped); n != 1 ||
			balance(t, st, ids[0]) != 700 || balance(t, st, ids[1]) != 300 {
			t.Fatalf("%d transfers, balances %d/%d", n,
				balance(t, st, ids[0]), balance(t, st, ids[1]))
		}
	}
}
