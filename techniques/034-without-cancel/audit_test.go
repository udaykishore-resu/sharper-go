package audit

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

type ctxKey struct{}

type recorder struct {
	err   error
	value any
}

func (r *recorder) Record(ctx context.Context, _ Transfer) error {
	time.Sleep(20 * time.Millisecond) // the handler returned meanwhile
	r.err, r.value = ctx.Err(), ctx.Value(ctxKey{})
	return r.err
}

func TestDetachedWorkSurvivesCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.WithValue(t.Context(), ctxKey{}, "trace-1"))
	var wg sync.WaitGroup
	r := &recorder{}
	afterCommit(ctx, &wg, r, Transfer{"t1"})
	cancel() // the handler wrote 201 and returned
	wg.Wait()
	if r.err != nil || r.value != "trace-1" {
		t.Fatalf("err=%v value=%v", r.err, r.value)
	}
}

func TestAfterFuncWakesRead(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	errc := make(chan error, 1)
	go func() { errc <- readUntilCancel(ctx, client) }()
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case err := <-errc:
		if err == nil {
			t.Fatal("read returned no error")
		}
	case <-time.After(time.Second):
		t.Fatal("read never woke")
	}
}
