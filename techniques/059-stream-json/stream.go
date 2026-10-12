// Package stream is Technique 59: decode a huge JSON array one
// element at a time.
package stream

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type Transfer struct {
	ID     string `json:"id"`
	From   string `json:"from"`
	To     string `json:"to"`
	Amount int64  `json:"amount"`
}

// EachTransfer is the book's listing. It returns how many items
// fn accepted; on error, those n items were handed to fn and the
// rest were not.
func EachTransfer(r io.Reader,
	fn func(Transfer) error) (int, error) {
	dec := json.NewDecoder(r)
	if tok, err := dec.Token(); tok != json.Delim('[') {
		return 0, errors.Join(errors.New("want array"), err)
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
	if _, err := dec.Token(); err != nil { // ]
		return n, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return n, errors.New("data after the array")
	}
	return n, nil
}

// Import batches the stream and commits each batch with commit
// (in ledgerd, store.ImportBatch through RunTx). It returns how
// many items are committed. A decode error mid-stream returns
// that count and the error: the batch in progress is dropped,
// earlier batches stay committed, and the import is reported as
// failed at that offset, never as complete.
func Import(r io.Reader, size int,
	commit func([]Transfer) error) (int, error) {
	var batch []Transfer
	done := 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := commit(batch); err != nil {
			return err
		}
		done += len(batch)
		batch = batch[:0]
		return nil
	}
	_, err := EachTransfer(r, func(t Transfer) error {
		batch = append(batch, t)
		if len(batch) == size {
			return flush()
		}
		return nil
	})
	if err != nil {
		return done, fmt.Errorf("import failed after %d committed: %w",
			done, err)
	}
	return done, flush()
}
