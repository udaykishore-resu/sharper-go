// Package httpapi holds ledgerd's routes, handlers and
// middleware. It never imports the store: it declares the small
// interface it needs (Technique 26).
package httpapi

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"example.com/ledgerd/internal/ledger"
)

// Store is what the handlers need from persistence.
type Store interface {
	Account(ctx context.Context, id string) (ledger.Account, error)
	Entries(ctx context.Context, id string, limit int) ([]ledger.Transfer, error)
	Transfer(ctx context.Context, tr ledger.Transfer) ([]byte, error)
	IdemStore
}

// API holds the handlers' dependencies.
type API struct {
	st          Store
	log         *slog.Logger
	maxTransfer func() int64
}

// Option configures an API (Technique 25).
type Option func(*API)

// WithLimit reads the live transfer limit on every request;
// pass the config reloader's MaxTransfer (Technique 91).
func WithLimit(max func() int64) Option {
	return func(a *API) { a.maxTransfer = max }
}

// New builds the API. With no options there is no limit.
func New(st Store, log *slog.Logger, opts ...Option) *API {
	a := &API{st: st, log: log,
		maxTransfer: func() int64 { return 0 }}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Routes is the whole public API on one screen (Technique 43).
// Probes live on their own mux, outside auth (Technique 49), and
// idempotency wraps only the one route that moves money
// (Technique 52).
func (a *API) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("POST /v1/transfers",
		a.idem(http.HandlerFunc(a.createTransfer)))
	mux.HandleFunc("GET /v1/accounts/{id}", a.getAccount)
	mux.HandleFunc("GET /v1/accounts/{id}/entries",
		a.listEntries)
	mux.HandleFunc("GET /v1/accounts/house", a.houseAccount)
	return mux
}

type transferReq struct {
	ID     string `json:"id,omitzero"`
	From   string `json:"from"`
	To     string `json:"to"`
	Amount int64  `json:"amount"`
}

func (a *API) createTransfer(w http.ResponseWriter, r *http.Request) {
	var req transferReq
	if !decodeTransfer(w, r, &req) {
		return
	}
	tr := ledger.Transfer{
		ID:     cmp.Or(req.ID, "t_"+rand.Text()),
		From:   req.From,
		To:     req.To,
		Amount: req.Amount,
		Key: ledger.ScopedKey(Principal(r.Context()),
			r.Header.Get("Idempotency-Key")),
	}
	if err := errors.Join(tr.Validate(),
		tr.CheckLimit(a.maxTransfer())); err != nil {
		a.writeErr(w, r, err)
		return
	}
	resp, err := a.st.Transfer(r.Context(), tr)
	if err != nil {
		a.writeErr(w, r, err)
		return
	}
	a.log.InfoContext(r.Context(), "transfer posted", "transfer", tr)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(resp)
}

type accountOut struct {
	ID      string `json:"id"`
	Balance int64  `json:"balance"` // zero is a real answer
}

func (a *API) getAccount(w http.ResponseWriter, r *http.Request) {
	a.account(w, r, r.PathValue("id"))
}

func (a *API) houseAccount(w http.ResponseWriter, r *http.Request) {
	a.account(w, r, ledger.HouseAccount)
}

func (a *API) account(w http.ResponseWriter, r *http.Request, id string) {
	acct, err := a.st.Account(r.Context(), id)
	if err != nil {
		a.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, accountOut{acct.ID, acct.Balance})
}

const minPage, maxPage = 1, 500

// pageSize clamps a client's limit (Technique 15).
func pageSize(asked int) int {
	return max(minPage, min(asked, maxPage))
}

type entryOut struct {
	ID     string `json:"id"`
	From   string `json:"from"`
	To     string `json:"to"`
	Amount int64  `json:"amount"`
}

func (a *API) listEntries(w http.ResponseWriter, r *http.Request) {
	asked, err := strconv.Atoi(cmp.Or(r.URL.Query().Get("limit"), "100"))
	if err != nil {
		http.Error(w, "limit must be an integer", http.StatusBadRequest)
		return
	}
	ts, err := a.st.Entries(r.Context(), r.PathValue("id"), pageSize(asked))
	if err != nil {
		a.writeErr(w, r, err)
		return
	}
	out := make([]entryOut, 0, len(ts))
	for _, t := range ts {
		out = append(out, entryOut{t.ID, t.From, t.To, t.Amount})
	}
	writeJSON(w, http.StatusOK, out)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // headers are already sent
}
