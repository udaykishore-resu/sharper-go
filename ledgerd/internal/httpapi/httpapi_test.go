package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"example.com/ledgerd/internal/ledger"
	"example.com/ledgerd/internal/obs"
)

// fakeStore returns the same sentinels as the real store, so the
// status mapping is tested for free (Technique 26).
type fakeStore struct {
	mu        sync.Mutex
	accounts  map[string]int64
	idem      map[string]*idemRow
	transfers int
	failNext  bool
}

type idemRow struct {
	hash  [32]byte
	done  bool
	saved Saved
}

func newFake() *fakeStore {
	return &fakeStore{
		accounts: map[string]int64{"house": 0, "a1": 1000, "a2": 0},
		idem:     map[string]*idemRow{},
	}
}

func (f *fakeStore) Account(_ context.Context, id string) (ledger.Account, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.accounts[id]
	if !ok {
		return ledger.Account{}, fmt.Errorf("account %s: %w", id, ledger.ErrNotFound)
	}
	return ledger.Account{ID: id, Balance: b}, nil
}

func (f *fakeStore) Entries(context.Context, string, int) ([]ledger.Transfer, error) {
	return nil, nil
}

func (f *fakeStore) Transfer(_ context.Context, tr ledger.Transfer) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext {
		f.failNext = false
		return nil, errors.New("connection reset")
	}
	if _, ok := f.accounts[tr.To]; !ok {
		return nil, fmt.Errorf("account %s: %w", tr.To, ledger.ErrNotFound)
	}
	if f.accounts[tr.From] < tr.Amount && tr.From != ledger.HouseAccount {
		return nil, fmt.Errorf("account %s: %w", tr.From, ledger.ErrFunds)
	}
	f.accounts[tr.From] -= tr.Amount
	f.accounts[tr.To] += tr.Amount
	f.transfers++
	body, _ := json.Marshal(map[string]any{"id": tr.ID, "amount": tr.Amount})
	if row := f.idem[tr.Key]; row != nil {
		row.done, row.saved = true, Saved{Status: 201, Body: body}
	}
	return body, nil
}

func (f *fakeStore) Begin(_ context.Context, client, key string, hash [32]byte) (*Saved, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := ledger.ScopedKey(client, key)
	row, ok := f.idem[k]
	switch {
	case !ok:
		f.idem[k] = &idemRow{hash: hash}
		return nil, nil
	case row.hash != hash:
		return nil, ErrMismatch
	case !row.done:
		return nil, ErrInFlight
	}
	s := row.saved
	return &s, nil
}

func (f *fakeStore) Finish(_ context.Context, client, key string, s Saved) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if row := f.idem[ledger.ScopedKey(client, key)]; row != nil && !row.done {
		row.done, row.saved = true, s
	}
	return nil
}

func (f *fakeStore) Release(_ context.Context, client, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := ledger.ScopedKey(client, key)
	if row := f.idem[k]; row != nil && !row.done {
		delete(f.idem, k)
	}
	return nil
}

const token = "0123456789abcdef"

// handler builds the production chain minus otelhttp and metrics.
func handler(st Store, opts ...Option) http.Handler {
	log := slog.New(slog.DiscardHandler)
	api := Chain(New(st, log, opts...).Routes(),
		RequestID,
		Auth(map[string]string{token: "acme"}),
		Deadline(time.Second),
		AccessLog(log),
		Recover(log),
		LimitBody(1<<20),
	)
	h := NewHealth(func(context.Context) error { return nil })
	h.SetReady()
	return h.Mux(api)
}

func do(t *testing.T, h http.Handler, method, path, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestTransferReplaysOnRetry(t *testing.T) {
	st := newFake()
	h := handler(st)
	body := `{"from":"a1","to":"a2","amount":500}`
	first := do(t, h, "POST", "/v1/transfers", "k-1", body)
	if first.Code != 201 {
		t.Fatalf("first: %d %s", first.Code, first.Body)
	}
	second := do(t, h, "POST", "/v1/transfers", "k-1", body)
	if second.Code != 201 || second.Header().Get("Idempotent-Replayed") != "true" ||
		second.Body.String() != first.Body.String() {
		t.Fatalf("retry: %d %v %s", second.Code, second.Header(), second.Body)
	}
	if st.transfers != 1 || st.accounts["a2"] != 500 {
		t.Fatalf("money moved %d times, a2=%d", st.transfers, st.accounts["a2"])
	}
	reused := do(t, h, "POST", "/v1/transfers", "k-1", `{"from":"a1","to":"a2","amount":9}`)
	if reused.Code != 422 {
		t.Fatalf("reused key with other body: %d", reused.Code)
	}
}

func TestTransfer5xxReleasesKey(t *testing.T) {
	st := newFake()
	st.failNext = true
	h := handler(st)
	body := `{"from":"a1","to":"a2","amount":5}`
	if rec := do(t, h, "POST", "/v1/transfers", "k-2", body); rec.Code != 500 {
		t.Fatalf("want 500, got %d", rec.Code)
	}
	if rec := do(t, h, "POST", "/v1/transfers", "k-2", body); rec.Code != 201 {
		t.Fatalf("retry after 5xx: %d %s", rec.Code, rec.Body)
	}
}

func TestTransfer4xxIsSavedForReplay(t *testing.T) {
	h := handler(newFake())
	body := `{"from":"a2","to":"a1","amount":5}` // a2 is empty
	first := do(t, h, "POST", "/v1/transfers", "k-3", body)
	second := do(t, h, "POST", "/v1/transfers", "k-3", body)
	if first.Code != 409 || second.Code != 409 ||
		second.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatalf("got %d then %d %v", first.Code, second.Code, second.Header())
	}
}

func TestTransferNeedsKey(t *testing.T) {
	rec := do(t, handler(newFake()), "POST", "/v1/transfers", "", `{}`)
	if rec.Code != 400 {
		t.Fatalf("no key: %d", rec.Code)
	}
}

func TestTransferValidationAndLimit(t *testing.T) {
	h := handler(newFake(), WithLimit(func() int64 { return 100 }))
	rec := do(t, h, "POST", "/v1/transfers", "k-4", `{"from":"a1","to":"a1","amount":-1}`)
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "invalid_transfer") {
		t.Fatalf("invalid: %d %s", rec.Code, rec.Body)
	}
	rec = do(t, h, "POST", "/v1/transfers", "k-5", `{"from":"a1","to":"a2","amount":500}`)
	var p problem
	json.Unmarshal(rec.Body.Bytes(), &p)
	if rec.Code != 409 || p.Code != "limit_exceeded" || p.Limit != 100 || p.Requested != 500 {
		t.Fatalf("limit: %d %+v", rec.Code, p)
	}
}

// Technique 48: one JSON value, at most 64 KiB, no unknown fields.
func TestDecodeTransfer(t *testing.T) {
	h := handler(newFake())
	for i, c := range []struct {
		body string
		want int
	}{
		{`{"from":"a1","to":"a2","amount":1}}`, 400},
		{`{"from":"a1","to":"a2","amount":1}{"x":1}`, 400},
		{`{"from":"a1","to":"a2","ammount":1}`, 400},
		{`{"from":"` + strings.Repeat("x", 70<<10) + `"}`, 413},
	} {
		rec := do(t, h, "POST", "/v1/transfers", fmt.Sprint("d-", i), c.body)
		if rec.Code != c.want {
			t.Errorf("body %.40q: got %d, want %d", c.body, rec.Code, c.want)
		}
	}
}

// Technique 43: methods, wildcards and the most specific match.
func TestRoutes(t *testing.T) {
	h := handler(newFake())
	rec := do(t, h, "GET", "/v1/accounts/house", "", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"balance":0`) {
		t.Fatalf("house: %d %s", rec.Code, rec.Body)
	}
	rec = do(t, h, "DELETE", "/v1/accounts/a1", "", "")
	if rec.Code != 405 || !strings.Contains(rec.Header().Get("Allow"), "GET") {
		t.Fatalf("DELETE: %d Allow=%q", rec.Code, rec.Header().Get("Allow"))
	}
	if rec := do(t, h, "GET", "/v1/accounts/nope", "", ""); rec.Code != 404 {
		t.Fatalf("missing account: %d", rec.Code)
	}
	if rec := do(t, h, "GET", "/v1/accounts/a1/entries?limit=99999", "", ""); rec.Code != 200 {
		t.Fatalf("entries: %d", rec.Code)
	}
}

func TestPageSize(t *testing.T) {
	for in, want := range map[int]int{0: 1, 50: 50, 9999: 500, -3: 1, 500: 500, 501: 500} {
		if got := pageSize(in); got != want {
			t.Errorf("pageSize(%d) = %d, want %d", in, got, want)
		}
	}
}

// Technique 24: no exported sentinel falls through to 500.
func TestClassifyCoversSentinels(t *testing.T) {
	for _, err := range []error{
		ledger.ErrNotFound, ledger.ErrFunds, ledger.ErrFrozen,
		ledger.ErrDuplicate, ledger.ErrInvalid, ledger.ErrBadAmount, ledger.ErrBusy,
		ledger.ErrSameAccount, ledger.ErrLimit, &ledger.LimitError{},
		context.DeadlineExceeded, context.Canceled,
	} {
		if got := classify(fmt.Errorf("wrapped: %w", err)); got.status == 500 {
			t.Errorf("classify(%v) = 500", err)
		}
	}
}

// Technique 49: probes skip auth, and draining fails readiness.
func TestProbes(t *testing.T) {
	h := NewHealth(func(context.Context) error { return nil })
	mux := h.Mux(Auth(nil)(http.NotFoundHandler()))
	get := func(path string) int {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec.Code
	}
	if get("/readyz") != 503 || get("/healthz") != 200 {
		t.Fatal("not ready before SetReady, or liveness failed")
	}
	h.SetReady()
	if get("/readyz") != 200 || get("/version") != 200 {
		t.Fatal("ready or version failed")
	}
	h.Drain()
	if get("/readyz") != 503 || get("/healthz") != 200 {
		t.Fatal("drain must fail readiness only")
	}
	if get("/v1/accounts/a1") != 401 {
		t.Fatal("API routes must still need auth")
	}
}

// Technique 90: a panic becomes one 500 and one counted metric.
func TestRecover(t *testing.T) {
	before := testutil.ToFloat64(obs.Panics)
	h := Recover(slog.New(slog.DiscardHandler))(http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) {
			nilMap()["x"] = 1 // a write to a nil map panics
		}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 500 || testutil.ToFloat64(obs.Panics) != before+1 {
		t.Fatalf("got %d, panics %v -> %v", rec.Code, before, testutil.ToFloat64(obs.Panics))
	}
}

// Technique 47: readers inside the copying layers see r.Pattern.
func TestAccessLogSeesPattern(t *testing.T) {
	var buf strings.Builder
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/accounts/{id}", func(http.ResponseWriter, *http.Request) {})
	h := Chain(mux, RequestID, Deadline(time.Second), AccessLog(log))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/v1/accounts/a9", nil))
	if !strings.Contains(buf.String(), `"route":"GET /v1/accounts/{id}"`) {
		t.Fatalf("access log: %s", buf.String())
	}
}

// Technique 47: ResponseController reaches the real writer through Unwrap.
func TestStatusWriterUnwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: rec}
	if err := http.NewResponseController(sw).Flush(); err != nil {
		t.Fatal(err)
	}
	if !rec.Flushed {
		t.Fatal("flush did not reach the recorder")
	}
}

func nilMap() map[string]int { return nil }
