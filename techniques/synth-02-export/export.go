// Package export is the fixed month-end export from the Part II
// synthesis: it ranges with two variables, always encodes
// balances, keeps retries as a pointer, and ends with a trailer
// that the upload step checks before it sends anything.
package export

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"

	accounts "github.com/udaykishore-resu/sharper-go/techniques/013-iterators"
)

// Line has no omitzero: a zero balance is data.
type Line struct {
	ID      string `json:"id"`
	Balance int64  `json:"balance"`
}

// Trailer closes the file with the number of lines before it.
type Trailer struct {
	Count int `json:"count"`
}

// Write encodes every account, then the trailer. A page error
// stops it before the trailer is written.
func Write(w io.Writer,
	accts iter.Seq2[accounts.Account, error]) error {
	enc := json.NewEncoder(w)
	n := 0
	for a, err := range accts {
		if err != nil {
			return fmt.Errorf("after %d accounts: %w", n, err)
		}
		if err := enc.Encode(Line{a.ID, a.Balance}); err != nil {
			return err
		}
		n++
	}
	return enc.Encode(Trailer{Count: n})
}

// ErrNoTrailer marks a file that ended early.
var ErrNoTrailer = errors.New("export: missing or wrong trailer")

// Verify reads a file back and refuses it unless the last line is
// a trailer whose count matches the lines before it.
func Verify(r io.Reader) error {
	sc := bufio.NewScanner(r)
	var lines []string
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if len(lines) == 0 {
		return ErrNoTrailer
	}
	var tr struct{ Count *int }
	last := lines[len(lines)-1]
	if json.Unmarshal([]byte(last), &tr) != nil || tr.Count == nil ||
		*tr.Count != len(lines)-1 {
		return ErrNoTrailer
	}
	return nil
}

// UploadConfig keeps Retries as a pointer: nil means "use the
// default", and 0 means "no retries".
type UploadConfig struct {
	Retries *int `json:"upload_retries"`
}

// Attempts is how many times the upload may run.
func (c UploadConfig) Attempts() int {
	if c.Retries == nil {
		return 1 + 3
	}
	return 1 + max(0, *c.Retries)
}

// Upload verifies the file first, then tries send up to Attempts
// times. A short file is never sent.
func Upload(c UploadConfig, file []byte,
	send func([]byte) error) error {
	if err := Verify(bytes.NewReader(file)); err != nil {
		return err
	}
	var err error
	for range c.Attempts() {
		if err = send(file); err == nil {
			return nil
		}
	}
	return err
}
