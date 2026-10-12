package decode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type transfer struct {
	From, To string
	Amount   int64
}

func post(h http.Handler, body string) int {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/", strings.NewReader(body)))
	return rec.Code
}

func handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var t transfer
		if decodeTransfer(w, r, &t) {
			w.WriteHeader(201)
		}
	})
}

func TestDecode(t *testing.T) {
	for body, want := range map[string]int{
		`{"From":"a","To":"b","Amount":1}`:              201,
		`{"From":"a","To":"b","Amount":1}}`:             400, // stray }
		`{"From":"a"}{"To":"b"}`:                        400, // second object
		`{"From":"a","To":"b","Ammount":1}`:             400, // typo
		`{"From":"` + strings.Repeat("x", 1<<17) + `"}`: 413,
	} {
		if got := post(handler(), body); got != want {
			t.Errorf("%.40q: got %d, want %d", body, got, want)
		}
	}
}

// When a body is wrapped twice, the smaller limit wins.
func TestSmallerLimitWins(t *testing.T) {
	h := LimitBody(1<<20, handler())
	if got := post(h, `{"From":"`+strings.Repeat("x", 100<<10)+`"}`); got != 413 {
		t.Fatalf("got %d", got)
	}
}

// dec.More can't do expectEOF's job: it says false at a stray }.
func TestMoreMissesStrayBrace(t *testing.T) {
	dec := json.NewDecoder(strings.NewReader(`{"a":1}}`))
	var v map[string]int
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	if dec.More() {
		t.Fatal("More reported another value")
	}
	if expectEOF(dec) == nil {
		t.Fatal("expectEOF let the stray } through")
	}
}
