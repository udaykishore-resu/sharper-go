package reader

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCollapses(t *testing.T) {
	var queries, shared atomic.Int32
	gate := make(chan struct{})
	r := &Reader{
		db: func(ctx context.Context, id string) (Account, error) {
			queries.Add(1)
			<-gate // hold the query until every caller has joined
			return Account{ID: id, Balance: 500}, ctx.Err()
		},
		Shared: func() { shared.Add(1) },
	}
	var wg sync.WaitGroup
	for range 200 {
		wg.Go(func() {
			a, err := r.Account(context.Background(), "acc_1")
			if err != nil || a.Balance != 500 {
				t.Errorf("got %+v, %v", a, err)
			}
		})
	}
	time.Sleep(50 * time.Millisecond)
	close(gate)
	wg.Wait()
	if queries.Load() != 1 || shared.Load() != 200 {
		t.Fatalf("queries=%d shared=%d", queries.Load(), shared.Load())
	}
}

// The first caller hanging up must not kill the shared query.
func TestFirstCallerLeaves(t *testing.T) {
	gate := make(chan struct{})
	r := &Reader{db: func(ctx context.Context, id string) (Account, error) {
		<-gate
		return Account{ID: id}, ctx.Err()
	}}
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() { _, err := r.Account(ctx, "acc_1"); first <- err }()
	time.Sleep(10 * time.Millisecond)
	second := make(chan error, 1)
	go func() { _, err := r.Account(context.Background(), "acc_1"); second <- err }()
	time.Sleep(10 * time.Millisecond)
	cancel()
	if err := <-first; err == nil {
		t.Fatal("first caller should give up on its own deadline")
	}
	close(gate)
	if err := <-second; err != nil {
		t.Fatalf("second caller got %v: the shared query died", err)
	}
}

// After a transfer commits, Forget on the account ID: the next
// reader starts a fresh query instead of joining one that began
// before the money moved.
func TestForgetStartsFresh(t *testing.T) {
	var queries atomic.Int32
	gate := make(chan struct{})
	r := &Reader{db: func(ctx context.Context, id string) (Account, error) {
		n := queries.Add(1)
		if n == 1 {
			<-gate // the stale query, started before the commit
			return Account{ID: id, Balance: 100}, nil
		}
		return Account{ID: id, Balance: 70}, nil
	}}
	stale := make(chan Account, 1)
	go func() { a, _ := r.Account(context.Background(), "acc_1"); stale <- a }()
	for queries.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	r.sf.Forget("acc_1") // the transfer committed
	a, err := r.Account(context.Background(), "acc_1")
	if err != nil || a.Balance != 70 || queries.Load() != 2 {
		t.Fatalf("got %+v, %v after %d queries", a, err, queries.Load())
	}
	close(gate)
	if a := <-stale; a.Balance != 100 {
		t.Fatalf("stale caller got %+v", a)
	}
}
