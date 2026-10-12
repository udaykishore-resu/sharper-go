package cause

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

func TestTimeoutCause(t *testing.T) {
	qctx, stop := balanceCtx(t.Context(), time.Millisecond)
	defer stop()
	<-qctx.Done()
	if !errors.Is(qctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("Err = %v; old code keeps working", qctx.Err())
	}
	if !errors.Is(context.Cause(qctx), errBudget) {
		t.Fatalf("Cause = %v", context.Cause(qctx))
	}
}

// Calling the WithTimeoutCause cancel func sets no cause.
func TestEarlyCancelHasNoCause(t *testing.T) {
	qctx, stop := balanceCtx(t.Context(), time.Hour)
	stop()
	if context.Cause(qctx) != context.Canceled {
		t.Fatalf("Cause = %v", context.Cause(qctx))
	}
}

func TestSignalRoot(t *testing.T) {
	sigs := make(chan os.Signal, 1)
	root := rootFrom(sigs)
	sigs <- syscall.SIGTERM
	<-root.Done()
	var se sigErr
	if !errors.As(context.Cause(root), &se) || se.Error() != "received terminated" {
		t.Fatalf("Cause = %v", context.Cause(root))
	}
}

// Since Go 1.26, signal.NotifyContext sets the cause itself.
func TestNotifyContextCause(t *testing.T) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGUSR1)
	defer stop()
	if err := syscall.Kill(os.Getpid(), syscall.SIGUSR1); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("signal not delivered")
	}
	if got := context.Cause(ctx).Error(); got != "user defined signal 1 signal received" {
		t.Fatalf("Cause = %q", got)
	}
}

// The first cancel wins: a child reports its parent's cause, and
// later cancels change nothing.
func TestFirstCauseWins(t *testing.T) {
	root, cancel := context.WithCancelCause(context.Background())
	child, stop := balanceCtx(root, time.Hour)
	defer stop()
	first := sigErr{syscall.SIGTERM}
	cancel(first)
	cancel(errors.New("later"))
	stop()
	if got := context.Cause(child); got != first {
		t.Fatalf("child cause = %v, want %v", got, first)
	}
	if got := context.Cause(root); got != first {
		t.Fatalf("root cause = %v", got)
	}
}
