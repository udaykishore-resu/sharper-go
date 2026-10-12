//go:build go1.27

package jsonv2

import (
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"testing"
)

// The v2 defaults the technique lists, checked on Go 1.27.2.
func TestV2Defaults(t *testing.T) {
	type M struct {
		Meta map[string]int `json:"meta"`
	}
	if b, _ := json.Marshal(M{}); string(b) != `{"meta":{}}` {
		t.Errorf("nil map: %s", b)
	}
	if b, _ := jsonv1.Marshal(M{}); string(b) != `{"meta":null}` {
		t.Errorf("v1 nil map: %s", b)
	}
	bad := []byte("{\"amount\":1,\"note\":\"\xff\"}")
	var v struct {
		Amount int64
		Note   string `json:"note"`
	}
	if err := json.Unmarshal(bad, &v); err == nil {
		t.Error("v2 accepted invalid UTF-8")
	}
	if err := jsonv1.Unmarshal(bad, &v); err != nil {
		t.Errorf("v1 rejected invalid UTF-8: %v", err)
	}
}

// omitempty in v2 means "the JSON value is empty", so a zero
// number is kept; omitzero drops it.
func TestOmitEmptyVsOmitZero(t *testing.T) {
	type R struct {
		A int64  `json:"a,omitempty"`
		B int64  `json:"b,omitzero"`
		S string `json:"s,omitempty"`
	}
	b, err := json.Marshal(R{})
	if err != nil || string(b) != `{"a":0}` {
		t.Fatalf("got %s %v", b, err)
	}
}

// Code from the 1.25 experiment: an `inline` tag is ignored now,
// so the field stops flattening; `embed` is the new spelling.
func TestInlineIsNowEmbed(t *testing.T) {
	type Inner struct {
		X int `json:"x"`
	}
	type Old struct {
		In Inner `json:",inline"`
	}
	type New struct {
		In Inner `json:",embed"`
	}
	o, _ := json.Marshal(Old{Inner{1}})
	n, _ := json.Marshal(New{Inner{1}})
	if string(o) != `{"In":{"x":1}}` || string(n) != `{"x":1}` {
		t.Fatalf("inline %s, embed %s", o, n)
	}
}

// jsontext's Token accessors return an error in Go 1.27.
var (
	_ func(jsontext.Token) (int64, error)   = jsontext.Token.Int
	_ func(jsontext.Token) (uint64, error)  = jsontext.Token.Uint
	_ func(jsontext.Token) (float64, error) = jsontext.Token.Float
)
