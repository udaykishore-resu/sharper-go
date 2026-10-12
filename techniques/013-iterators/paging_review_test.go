package accounts

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
)

// The one-call claim has a condition: the first page must succeed
// and hold at least ten accounts. With fewer, the loop ends on the
// short page; if the first fetch fails, the caller gets the error.
func TestBreakConditions(t *testing.T) {
	p := &fakePager{n: 4}
	seen := 0
	for _, err := range Accounts(t.Context(), p) {
		if err != nil {
			t.Fatal(err)
		}
		seen++
	}
	if seen != 4 || p.calls != 1 {
		t.Fatalf("short first page: seen %d, calls %d", seen, p.calls)
	}
	p = &fakePager{n: 2000, failAt: 1}
	for _, err := range Accounts(t.Context(), p) {
		if err == nil {
			t.Fatal("want the first-page error")
		}
		break
	}
}

// A caller that breaks early never fetches the later pages, so it
// cannot see their errors: nothing was asked for.
func TestEarlyBreakSkipsLaterErrors(t *testing.T) {
	p := &fakePager{n: 2000, failAt: 2}
	for _, err := range Accounts(t.Context(), p) {
		if err != nil {
			t.Fatal(err)
		}
		break
	}
	if p.calls != 1 {
		t.Fatalf("calls = %d", p.calls)
	}
}

// livePager is a keyset pager over a set of IDs that a test can
// change between pages, like a table under concurrent writes.
type livePager struct {
	ids     []string // sorted
	onFetch func(call int, l *livePager)
	calls   int
}

func (l *livePager) Page(_ context.Context, after string,
	n int) ([]Account, error) {
	l.calls++
	if l.onFetch != nil {
		l.onFetch(l.calls, l)
	}
	var out []Account
	for _, id := range l.ids {
		if id > after && len(out) < n {
			out = append(out, Account{ID: id, Balance: 1})
		}
	}
	return out, nil
}

func (l *livePager) insert(id string) {
	l.ids = append(l.ids, id)
	slices.Sort(l.ids)
}

// Keyset paging is not a snapshot. Between page fetches, an
// account inserted behind the cursor is missed, one inserted
// ahead is seen, and a deleted one that was not read yet vanishes.
// No account is read twice.
func TestKeysetIsNotASnapshot(t *testing.T) {
	l := &livePager{}
	for i := range 1000 {
		l.insert(fmt.Sprintf("a%04d", i*2)) // even IDs
	}
	l.onFetch = func(call int, l *livePager) {
		if call == 2 { // after page 1 (a0000..a0998) was read
			l.insert("a0001") // behind the cursor
			l.insert("a1999") // ahead of it
			l.ids = slices.DeleteFunc(l.ids, func(id string) bool {
				return id == "a1998"
			})
		}
	}
	got := map[string]int{}
	for a, err := range Accounts(t.Context(), l) {
		if err != nil {
			t.Fatal(err)
		}
		got[a.ID]++
	}
	for id, n := range got {
		if n != 1 {
			t.Fatalf("%s read %d times", id, n)
		}
	}
	_, behind := got["a0001"]
	_, ahead := got["a1999"]
	_, deleted := got["a1998"]
	if behind || !ahead || deleted || len(got) != 1000 {
		t.Fatalf("behind=%v ahead=%v deleted=%v n=%d",
			behind, ahead, deleted, len(got))
	}
}

// Shutdown cancels ctx mid-loop; the caller sees context.Canceled
// and can treat it as a normal stop.
func TestCancelMidLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	p := ctxPager{&fakePager{n: 2000}}
	var got error
	n := 0
	for _, err := range Accounts(ctx, p) {
		if err != nil {
			got = err
			break
		}
		if n++; n == 500 {
			cancel()
		}
	}
	if !errors.Is(got, context.Canceled) || n != 500 {
		t.Fatalf("err = %v after %d accounts", got, n)
	}
}

type ctxPager struct{ *fakePager }

func (c ctxPager) Page(ctx context.Context, after string,
	n int) ([]Account, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.fakePager.Page(ctx, after, n)
}
