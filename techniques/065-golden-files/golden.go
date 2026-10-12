// Package golden is Technique 65's subject: a handler whose JSON
// response is pinned by a golden file.
package golden

import (
	"encoding/json"
	"net/http"
)

type account struct {
	ID       string `json:"id"`
	Balance  int64  `json:"balance"`
	Currency string `json:"currency"`
}

var accounts = map[string]account{
	"acc_1": {"acc_1", 1250, "USD"},
}

func GetAccount(w http.ResponseWriter, r *http.Request) {
	a, ok := accounts[r.PathValue("id")]
	if !ok {
		http.Error(w, `{"code":"not_found"}`, http.StatusNotFound)
		return
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ") // one changed field, one changed line
	enc.Encode(a)
}
