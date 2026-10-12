// Package httpapi is Technique 43: method and wildcard patterns
// in net/http's ServeMux.
package httpapi

import (
	"fmt"
	"net/http"
)

type API struct{}

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

// idem stands in for Technique 52's middleware.
func (a *API) idem(next http.Handler) http.Handler { return next }

func (a *API) createTransfer(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusCreated)
}

func (a *API) getAccount(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "account %s", r.PathValue("id"))
}

func (a *API) listEntries(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "entries %s", r.PathValue("id"))
}

func (a *API) houseAccount(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "house")
}
