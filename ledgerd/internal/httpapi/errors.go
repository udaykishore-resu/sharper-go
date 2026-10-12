package httpapi

import (
	"context"
	"errors"
	"net/http"

	"example.com/ledgerd/internal/ledger"
	"example.com/ledgerd/internal/obs"
)

type apiErr struct {
	status int
	code   string
}

// table decides every status in one place (Technique 24). It is
// a slice because order matters: a joined error can match two
// rows, and the first wins, so specific rows go on top.
var table = []struct {
	target error
	apiErr
}{
	{ledger.ErrNotFound, apiErr{404, "not_found"}},
	{ledger.ErrFunds, apiErr{409, "insufficient_funds"}},
	{ledger.ErrLimit, apiErr{409, "limit_exceeded"}},
	{ledger.ErrDuplicate, apiErr{409, "duplicate_transfer"}},
	{ledger.ErrFrozen, apiErr{423, "account_frozen"}},
	{ledger.ErrInvalid, apiErr{422, "invalid_transfer"}},
	{ledger.ErrBusy, apiErr{503, "busy"}},
	{context.DeadlineExceeded,
		apiErr{503, "timeout"}},
	{context.Canceled, apiErr{499, "client_closed"}},
}

func classify(err error) apiErr {
	for _, row := range table {
		if errors.Is(err, row.target) {
			return row.apiErr
		}
	}
	return apiErr{500, "internal"}
}

// problem is an RFC 9457 problem details body. Clients switch on
// Code, never on the English.
type problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Code      string `json:"code"`
	Detail    string `json:"detail,omitzero"`
	Limit     int64  `json:"limit,omitzero"`
	Requested int64  `json:"requested,omitzero"`
}

// writeErr writes every error response. A 4xx message is about
// the client's input, so it is safe to return; a 5xx logs the
// full chain and sends fixed text.
func (a *API) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	e := classify(err)
	p := problem{Type: "about:blank", Status: e.status, Code: e.code,
		Title: http.StatusText(e.status)}
	switch {
	case e.status == 499:
		p.Title = "client closed request"
		a.log.InfoContext(r.Context(), "client left",
			"cause", context.Cause(r.Context()))
	case e.status >= 500:
		p.Title = "internal error"
		a.log.ErrorContext(r.Context(), "request failed",
			"route", r.Pattern, "status", e.status, "err", err,
			"cause", context.Cause(r.Context()))
	default:
		p.Detail = err.Error()
		obs.Rejects.Add(e.code, 1)
	}
	if le, ok := errors.AsType[*ledger.LimitError](err); ok {
		p.Limit, p.Requested = le.Limit, le.Requested
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(e.status)
	encodeJSON(w, p)
}
