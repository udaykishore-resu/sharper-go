// Package intcover is Technique 72's subject: a small service whose
// real binary is driven by a smoke script, with coverage from that
// run merged with unit-test coverage. See cover.sh.
package intcover

import (
	"encoding/json"
	"net/http"
)

var balances = map[string]int64{"acc_1": 1250}

// Handler serves the two routes the smoke script calls.
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /v1/accounts/{id}", getAccount)
	return mux
}

func getAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	b, ok := balances[id]
	if !ok {
		http.Error(w, `{"code":"not_found"}`, http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"id": id, "balance": b})
}

// Validate is reached only by unit tests: the smoke script never
// sends a bad ID, which is the point of merging both runs.
func Validate(id string) error {
	if id == "" {
		return errEmpty
	}
	return nil
}

type validationError string

func (e validationError) Error() string { return string(e) }

const errEmpty = validationError("empty id")
