// Package stream is Technique 59: decode a huge JSON array one
// element at a time.
package stream

import (
	"encoding/json"
	"fmt"
	"io"
)

type Transfer struct {
	ID     string `json:"id"`
	From   string `json:"from"`
	To     string `json:"to"`
	Amount int64  `json:"amount"`
}

func EachTransfer(r io.Reader,
	fn func(Transfer) error) (int, error) {
	dec := json.NewDecoder(r)
	if _, err := dec.Token(); err != nil { // [
		return 0, err
	}
	n := 0
	for dec.More() {
		var t Transfer
		if err := dec.Decode(&t); err != nil {
			return n, fmt.Errorf("item %d: %w", n, err)
		}
		if err := fn(t); err != nil {
			return n, err
		}
		n++
	}
	_, err := dec.Token() // ]
	return n, err
}
