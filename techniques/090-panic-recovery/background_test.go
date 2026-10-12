package httpapi

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

type ctxKey struct{}

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func TestGoRecoversAndKeepsValues(t *testing.T) {
	var buf lockedBuf
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	parent, cancel := context.WithCancel(
		context.WithValue(context.Background(), ctxKey{}, "trace-1"))
	cancel() // the response has gone out
	before := panics.Value()
	done := make(chan struct{})
	var got any
	var errAfterCancel error
	Go(parent, log, func(ctx context.Context) {
		defer close(done)
		got = ctx.Value(ctxKey{})
		errAfterCancel = ctx.Err()
		nilMap()["x"] = 1
	})
	<-done
	// The recover runs just after done closes; wait for it.
	for end := time.Now().Add(5 * time.Second); panics.Value() != before+1; {
		if time.Now().After(end) {
			t.Fatal("panic not counted")
		}
		time.Sleep(time.Millisecond)
	}
	if got != "trace-1" || errAfterCancel != nil {
		t.Fatalf("value %v, err %v", got, errAfterCancel)
	}
	buf.mu.Lock()
	defer buf.mu.Unlock()
	if !strings.Contains(buf.b.String(), "panic in background") {
		t.Fatalf("log: %s", buf.b.String())
	}
}
