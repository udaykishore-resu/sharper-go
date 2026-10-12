//go:build go1.27

package jsonv2

import (
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"strings"
	"testing"
)

func TestDuplicateKeysFail(t *testing.T) {
	body := []byte(`{"amount":1,"amount":9}`)
	var v1 Req
	if err := jsonv1.Unmarshal(body, &v1); err != nil || v1.Amount != 9 {
		t.Fatalf("v1 takes the last value: %+v %v", v1, err)
	}
	if _, err := decodeStrict(body); err == nil {
		t.Fatal("a duplicate key reached the ledger")
	}
	if r, err := decodeStrict([]byte(`{"amount":5}`)); err != nil || r.Amount != 5 {
		t.Fatalf("normal body: %+v %v", r, err)
	}
}

// The golden diff from the story: "tags":null became "tags":[].
func TestNilSlice(t *testing.T) {
	old, _ := jsonv1.Marshal(Resp{})
	now, _ := json.Marshal(Resp{})
	kept, _ := json.Marshal(Resp{}, json.FormatNilSliceAsNull(true))
	if string(old) != `{"tags":null}` || string(now) != `{"tags":[]}` ||
		string(kept) != `{"tags":null}` {
		t.Fatalf("v1 %s, v2 %s, v2+option %s", old, now, kept)
	}
}

func TestCaseSensitive(t *testing.T) {
	var v1, v2 Req
	jsonv1.Unmarshal([]byte(`{"AMOUNT":7}`), &v1)
	json.Unmarshal([]byte(`{"AMOUNT":7}`), &v2)
	if v1.Amount != 7 || v2.Amount != 0 {
		t.Fatalf("v1 %d, v2 %d", v1.Amount, v2.Amount)
	}
}

func TestUnmarshalReadRejectsTrailingGarbage(t *testing.T) {
	var r Req
	if err := json.UnmarshalRead(strings.NewReader(`{"amount":1} x`), &r); err == nil {
		t.Fatal("trailing garbage accepted")
	}
}

// jsontext.Decoder.SkipValue skips a huge value without buffering it.
func TestSkipValue(t *testing.T) {
	dec := jsontext.NewDecoder(strings.NewReader(`{"blob":[1,2,3],"amount":4}`))
	dec.ReadToken() // {
	dec.ReadToken() // "blob"
	if err := dec.SkipValue(); err != nil {
		t.Fatal(err)
	}
	if tok, _ := dec.ReadToken(); tok.String() != "amount" {
		t.Fatalf("next token %v", tok)
	}
}
