package statement

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

type fakeStore struct {
	now, peak, canceled atomic.Int32
	failID              string
}

func (f *fakeStore) Account(ctx context.Context, id string) (Account, error) {
	n := f.now.Add(1)
	defer f.now.Add(-1)
	for p := f.peak.Load(); n > p && !f.peak.CompareAndSwap(p, n); p = f.peak.Load() {
	}
	if id == f.failID {
		return Account{}, errors.New("connection reset")
	}
	select {
	case <-time.After(5 * time.Millisecond):
		return Account{ID: id, Balance: 1}, nil
	case <-ctx.Done():
		f.canceled.Add(1)
		return Account{}, ctx.Err()
	}
}

func ids(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprint("acc_", i)
	}
	return out
}

func TestLimitAndOrder(t *testing.T) {
	st := &fakeStore{}
	got, err := balances(t.Context(), st, ids(400))
	if err != nil || len(got) != 400 || got[399].ID != "acc_399" {
		t.Fatalf("got %d accounts, err %v", len(got), err)
	}
	if st.peak.Load() > 8 {
		t.Fatalf("peak %d > SetLimit(8)", st.peak.Load())
	}
}

func TestFirstErrorCancelsTheRest(t *testing.T) {
	st := &fakeStore{failID: "acc_3"}
	_, err := balances(t.Context(), st, ids(400))
	if err == nil || err.Error() != "account acc_3: connection reset" {
		t.Fatalf("err = %v", err)
	}
	if st.canceled.Load() == 0 {
		t.Fatal("no sibling saw the cancel")
	}
}
