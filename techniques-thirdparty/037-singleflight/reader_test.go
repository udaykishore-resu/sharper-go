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
