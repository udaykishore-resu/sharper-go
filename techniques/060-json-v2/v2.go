//go:build go1.27

// Package jsonv2 is Technique 60: what encoding/json/v2 changes.
// ledgerd's go line is 1.26.0, so this file opts in with a
// go1.27 build constraint; importing the v2 packages for real
// means raising the go line on purpose.
package jsonv2

import (
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	"encoding/json/v2"
)

type Req struct {
	Amount int64 `json:"amount"`
}

// decodeStrict keeps v1 semantics except one: duplicate keys
// now fail.
func decodeStrict(body []byte) (Req, error) {
	var req Req
	err := json.Unmarshal(body, &req,
		jsonv1.DefaultOptionsV1(),
		jsontext.AllowDuplicateNames(false))
	return req, err
}

type Resp struct {
	Tags []string `json:"tags"`
}
