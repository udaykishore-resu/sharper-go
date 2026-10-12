//go:build integration

package store

import (
	"context"
	"crypto/rand"
	"log/slog"
	"strings"
	"testing"
	"time"

	"example.com/ledgerd/internal/ledger"
)

// sentAt reads the outbox row for one transfer, through its own
// pool so nothing is cached in the store under test.
func sentAt(t *testing.T, transferID string) (exists, sent bool) {
	t.Helper()
	st, err := Open(context.Background(), dsn, WithMaxConns(1))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var at *time.Time
	err = st.pool.QueryRow(context.Background(), `SELECT sent_at
        FROM outbox WHERE payload->>'id' = $1`, transferID).Scan(&at)
	if err != nil {
		return false, false
	}
	return true, at != nil
}

// Part IV synthesis: background work that must survive a restart
// lives in a table, not a goroutine. The event row commits with
// the transfer before Transfer returns; a relay whose publish
// fails leaves it unsent; when that "pod" is gone, a relay on a
// new pool delivers it. Delivery is at least once, so receivers
// dedupe on the transfer ID.
func TestOutboxSurvivesRestart(t *testing.T) {
	podA := open(t)
	ids := accounts(t, podA, 100, 0)
	id := "t_" + rand.Text()
	_, err := podA.Transfer(t.Context(), ledger.Transfer{ID: id,
		From: ids[0], To: ids[1], Amount: 7,
		Key: ledger.ScopedKey("acme", rand.Text())})
	if err != nil {
		t.Fatal(err)
	}
	if ok, sent := sentAt(t, id); !ok || sent {
		t.Fatalf("after commit: row exists=%v sent=%v", ok, sent)
	}

	// Pod A: the partner is down for this event.
	ctxA, stopA := context.WithCancel(t.Context())
	tried := make(chan struct{}, 1)
	relayA := podA.StartRelay(ctxA, func(_ context.Context, _ string, p []byte) error {
		if strings.Contains(string(p), id) {
			select {
			case tried <- struct{}{}:
			default:
			}
			return context.DeadlineExceeded
		}
		return nil // older rows from other tests drain normally
	}, slog.New(slog.DiscardHandler), nil)
	select {
	case <-tried:
	case <-time.After(10 * time.Second):
		t.Fatal("relay A never tried the event")
	}
	stopA() // the pod is killed
	<-relayA.Done()
	podA.Close()
	if _, sent := sentAt(t, id); sent {
		t.Fatal("a failed publish marked the row sent")
	}

	// Pod B: a fresh pool and relay deliver the same row.
	podB := open(t)
	got := make(chan struct{}, 1)
	startRelay(t, podB, func(_ context.Context, _ string, p []byte) error {
		if strings.Contains(string(p), id) {
			select {
			case got <- struct{}{}:
			default:
			}
		}
		return nil
	})
	select {
	case <-got:
	case <-time.After(10 * time.Second):
		t.Fatal("event was lost across the restart")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, sent := sentAt(t, id); sent {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("delivered row never marked sent")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
